package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joelhelbling/tabb/internal/native"
	"github.com/joelhelbling/tabb/internal/protocol"
	"github.com/joelhelbling/tabb/internal/socket"
)

// HandshakeInfo is the data captured from the extension's handshake message.
type HandshakeInfo struct {
	ProfileID   string
	ExtensionID string
	Browser     string
}

// errHandshakeTimeout is returned by waitForHandshake when no message arrives
// in time. Wrapped so tests can match it with errors.Is.
var errHandshakeTimeout = errors.New("handshake timeout")

// waitForHandshake reads a single Native Messaging message from r and requires
// it to be a handshake containing a profileId. Any other first message is a
// hard error — older extension builds that don't send a profileId will be
// refused and the user will see a clear message to reinstall the extension.
func waitForHandshake(r io.Reader, timeout time.Duration) (HandshakeInfo, error) {
	type result struct {
		msg []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := native.ReadMessage(r)
		ch <- result{msg, err}
	}()

	var msg []byte
	select {
	case res := <-ch:
		if res.err != nil {
			return HandshakeInfo{}, fmt.Errorf("reading handshake: %w", res.err)
		}
		msg = res.msg
	case <-time.After(timeout):
		return HandshakeInfo{}, errHandshakeTimeout
	}

	var raw map[string]any
	if err := json.Unmarshal(msg, &raw); err != nil {
		return HandshakeInfo{}, fmt.Errorf("parsing handshake: %w", err)
	}
	action, _ := raw["action"].(string)
	if action != protocol.ActionHandshake {
		return HandshakeInfo{}, fmt.Errorf("expected handshake as first message, got %q (extension may be out of date — reinstall from extension/)", action)
	}
	params, _ := raw["params"].(map[string]any)
	if params == nil {
		return HandshakeInfo{}, fmt.Errorf("handshake missing params")
	}
	info := HandshakeInfo{}
	info.ProfileID, _ = params["profileId"].(string)
	info.ExtensionID, _ = params["extensionId"].(string)
	info.Browser, _ = params["browser"].(string)
	if info.ProfileID == "" {
		return HandshakeInfo{}, fmt.Errorf("handshake missing profileId (extension may be out of date — reinstall from extension/)")
	}
	return info, nil
}

// runHost is the Native Messaging host entry point. Chrome launches this binary
// and communicates over stdin/stdout. The handshake is read first so we can
// bind the Unix socket at the per-profile path (~/.tabb/<profileId>.sock).
// The extensionID argument (from argv) is accepted but no longer used for
// socket naming — it is kept only for log context.
func runHost(extensionIDArg string) error {
	log.SetOutput(os.Stderr)
	log.SetPrefix("tabb-host: ")

	// Phase 1: wait for handshake. 5s is generous — the extension sends it
	// immediately on connect.
	info, err := waitForHandshake(os.Stdin, 5*time.Second)
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}
	log.Printf("handshake: profileId=%s extensionId=%s browser=%s",
		info.ProfileID, info.ExtensionID, info.Browser)

	// Persist browser info keyed by profileId so `tabb setup` can discover it.
	saveBrowserInfo(info)

	// Phase 2: bind the socket at the per-profile path.
	ln, err := socket.Listen(info.ProfileID)
	if err != nil {
		return fmt.Errorf("starting socket server: %w", err)
	}
	defer func() {
		ln.Close()
		socket.Cleanup(info.ProfileID)
	}()

	pending := &pendingRequests{m: make(map[string]chan protocol.Response)}

	// Start reading subsequent messages (responses to CLI requests) from the extension.
	go readFromExtension(pending, info.ProfileID)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		ln.Close()
		socket.Cleanup(info.ProfileID)
		os.Exit(0)
	}()

	log.Println("listening on socket")

	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil // listener closed
		}
		go handleSocketClient(conn, pending)
	}
}

// readFromExtension reads Native Messaging responses from the Chrome extension
// on stdin and dispatches them to waiting socket clients. By the time this
// runs, the handshake has already been consumed in runHost.
func readFromExtension(pending *pendingRequests, profileID string) {
	for {
		msg, err := native.ReadMessage(os.Stdin)
		if err != nil {
			log.Printf("extension disconnected: %v", err)
			socket.Cleanup(profileID)
			os.Exit(0)
		}

		var resp protocol.Response
		if err := json.Unmarshal(msg, &resp); err != nil {
			log.Printf("invalid response from extension: %v", err)
			continue
		}

		if ch := pending.get(resp.ID); ch != nil {
			ch <- resp
		}
	}
}

// saveBrowserInfo writes ~/.tabb/<profileID>.browser containing the browser
// name so that `tabb setup` can surface a sensible default profile name.
func saveBrowserInfo(info HandshakeInfo) {
	dir, err := socket.Dir()
	if err != nil {
		log.Printf("cannot save browser info: %v", err)
		return
	}
	// File contents: "<browser>\n<extensionId>\n"
	// setup.go parses both lines.
	content := fmt.Sprintf("%s\n%s\n", info.Browser, info.ExtensionID)
	path := filepath.Join(dir, info.ProfileID+".browser")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		log.Printf("cannot write %s: %v", path, err)
	}
}

// handleSocketClient handles a single CLI or MCP client connection on the Unix socket.
func handleSocketClient(conn net.Conn, pending *pendingRequests) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req protocol.Request
		if err := decoder.Decode(&req); err != nil {
			return // client disconnected
		}

		// Create a channel to receive the response
		ch := make(chan protocol.Response, 1)
		pending.set(req.ID, ch)

		// Forward request to the Chrome extension via Native Messaging
		if err := native.WriteMessage(os.Stdout, req); err != nil {
			log.Printf("error writing to extension: %v", err)
			encoder.Encode(protocol.Response{
				ID:    req.ID,
				Error: "failed to communicate with Chrome extension",
			})
			pending.delete(req.ID)
			continue
		}

		// Wait for the response from the extension
		resp := <-ch
		pending.delete(req.ID)

		if err := encoder.Encode(resp); err != nil {
			return // client disconnected
		}
	}
}

// pendingRequests tracks socket clients waiting for extension responses.
type pendingRequests struct {
	mu sync.Mutex
	m  map[string]chan protocol.Response
}

func (p *pendingRequests) set(id string, ch chan protocol.Response) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[id] = ch
}

func (p *pendingRequests) get(id string) chan protocol.Response {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.m[id]
}

func (p *pendingRequests) delete(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, id)
}
