package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/joelhelbling/tabb/internal/profile"
	"github.com/joelhelbling/tabb/internal/protocol"
	"github.com/joelhelbling/tabb/internal/socket"
)

// resolveAndDial determines the correct profile socket and connects to it.
func resolveAndDial(flagProfile string) (net.Conn, error) {
	tabbDir, err := socket.Dir()
	if err != nil {
		return nil, err
	}
	profilesPath := profile.ProfilesPath(tabbDir)
	envProfile := os.Getenv("TABB_PROFILE")

	extID, err := profile.Resolve(tabbDir, profilesPath, flagProfile, envProfile)
	if err != nil {
		return nil, err
	}

	return socket.Dial(extID)
}

// sendRequest connects to the Unix socket, sends a request, and returns the response.
func sendRequest(action string, params map[string]any, flagProfile string) (*protocol.Response, error) {
	conn, err := resolveAndDial(flagProfile)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := protocol.Request{
		ID:     generateID(),
		Action: action,
		Params: params,
	}

	return doRequest(conn, req)
}

func doRequest(conn net.Conn, req protocol.Request) (*protocol.Response, error) {
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	var resp protocol.Response
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	return &resp, nil
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
