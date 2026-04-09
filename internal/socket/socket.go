// Package socket manages the Unix domain socket server that CLI and MCP clients connect to.
package socket

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const (
	dirName  = ".tabb"
	sockName = "tabb.sock"
)

// Path returns the full path to the Unix domain socket.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, dirName, sockName), nil
}

// Listen creates the socket directory and starts listening on the Unix domain socket.
// The socket file is created with mode 0600 (owner-only access).
func Listen() (net.Listener, error) {
	sockPath, err := Path()
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
	dir := filepath.Dir(sockPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating socket directory: %w", err)
	}

	// Remove stale socket file if it exists
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing stale socket: %w", err)
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on socket: %w", err)
	}

	// Set socket permissions to owner-only
	if err := os.Chmod(sockPath, 0600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("setting socket permissions: %w", err)
	}

	return ln, nil
}

// Dial connects to the Unix domain socket as a client.
func Dial() (net.Conn, error) {
	sockPath, err := Path()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to tabb socket (is Chrome running with the tabb extension?): %w", err)
	}
	return conn, nil
}

// Cleanup removes the socket file.
func Cleanup() error {
	sockPath, err := Path()
	if err != nil {
		return err
	}
	return os.Remove(sockPath)
}
