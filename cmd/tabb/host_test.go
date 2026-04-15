package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// writeNativeMessage writes a length-prefixed JSON message in the Chrome Native
// Messaging format into buf, so tests can build synthetic stdin streams.
func writeNativeMessage(t *testing.T, buf *bytes.Buffer, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(data))); err != nil {
		t.Fatalf("length prefix: %v", err)
	}
	buf.Write(data)
}

func TestWaitForHandshakeSuccess(t *testing.T) {
	var buf bytes.Buffer
	writeNativeMessage(t, &buf, map[string]any{
		"action": "handshake",
		"params": map[string]any{
			"profileId":   "uuid-123",
			"extensionId": "ext-abc",
			"browser":     "Vivaldi",
		},
	})

	info, err := waitForHandshake(&buf, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.ProfileID != "uuid-123" {
		t.Errorf("ProfileID = %q, want uuid-123", info.ProfileID)
	}
	if info.ExtensionID != "ext-abc" {
		t.Errorf("ExtensionID = %q, want ext-abc", info.ExtensionID)
	}
	if info.Browser != "Vivaldi" {
		t.Errorf("Browser = %q, want Vivaldi", info.Browser)
	}
}

func TestWaitForHandshakeMissingProfileID(t *testing.T) {
	var buf bytes.Buffer
	writeNativeMessage(t, &buf, map[string]any{
		"action": "handshake",
		"params": map[string]any{
			"extensionId": "ext-abc",
			"browser":     "Vivaldi",
		},
	})

	_, err := waitForHandshake(&buf, time.Second)
	if err == nil {
		t.Fatal("expected error for handshake without profileId")
	}
	if !strings.Contains(err.Error(), "profileId") {
		t.Errorf("expected error to mention profileId, got: %v", err)
	}
}

func TestWaitForHandshakeWrongFirstMessage(t *testing.T) {
	var buf bytes.Buffer
	writeNativeMessage(t, &buf, map[string]any{
		"id":     "req-1",
		"action": "list_tabs",
	})

	_, err := waitForHandshake(&buf, time.Second)
	if err == nil {
		t.Fatal("expected error when first message is not a handshake")
	}
	if !strings.Contains(err.Error(), "handshake") {
		t.Errorf("expected error to mention handshake, got: %v", err)
	}
}

func TestWaitForHandshakeEOF(t *testing.T) {
	var buf bytes.Buffer
	_, err := waitForHandshake(&buf, time.Second)
	if err == nil {
		t.Fatal("expected error on empty stream")
	}
}

func TestWaitForHandshakeTimeout(t *testing.T) {
	r := &blockingReader{}
	start := time.Now()
	_, err := waitForHandshake(r, 50*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, errHandshakeTimeout) {
		t.Errorf("expected errHandshakeTimeout, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForHandshake took too long: %v", elapsed)
	}
}

type blockingReader struct{}

func (b *blockingReader) Read(p []byte) (int, error) {
	time.Sleep(10 * time.Second)
	return 0, nil
}
