// Package native implements Chrome's Native Messaging protocol.
// Messages are length-prefixed JSON: 4-byte little-endian uint32 length, then JSON payload.
package native

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// ReadMessage reads a single Native Messaging message from r.
func ReadMessage(r io.Reader) (json.RawMessage, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("reading message length: %w", err)
	}
	if length > 1024*1024 {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}
	msg := make([]byte, length)
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, fmt.Errorf("reading message body: %w", err)
	}
	return json.RawMessage(msg), nil
}

// WriteMessage writes a single Native Messaging message to w.
func WriteMessage(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}
	if len(data) > 1024*1024 {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}
	length := uint32(len(data))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return fmt.Errorf("writing message length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing message body: %w", err)
	}
	return nil
}
