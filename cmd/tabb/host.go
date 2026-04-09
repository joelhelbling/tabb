package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/joelhelbling/tabb/internal/native"
	"github.com/joelhelbling/tabb/internal/protocol"
	"github.com/joelhelbling/tabb/internal/socket"
)

// runHost is the Native Messaging host entry point. Chrome launches this binary
// and communicates over stdin/stdout. It also creates a Unix socket so CLI/MCP
// clients can send requests that get forwarded to the extension.
func runHost() error {
	log.SetOutput(os.Stderr)
	log.SetPrefix("tabb-host: ")

	// Track pending requests from socket clients awaiting extension responses
	pending := &pendingRequests{
		m: make(map[string]chan protocol.Response),
	}

	// Start reading from stdin (messages from the Chrome extension)
	go readFromExtension(pending)

	// Start the Unix socket server
	ln, err := socket.Listen()
	if err != nil {
		return fmt.Errorf("starting socket server: %w", err)
	}
	defer func() {
		ln.Close()
		socket.Cleanup()
	}()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		ln.Close()
		socket.Cleanup()
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
// on stdin and dispatches them to waiting socket clients.
func readFromExtension(pending *pendingRequests) {
	for {
		msg, err := native.ReadMessage(os.Stdin)
		if err != nil {
			log.Printf("extension disconnected: %v", err)
			socket.Cleanup()
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
