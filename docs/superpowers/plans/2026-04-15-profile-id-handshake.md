# Profile-ID Handshake Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the bug where Vivaldi (and other Chromium browsers that reuse extension IDs across profiles) causes a second tabb profile to clobber the first's socket. Replace extension-ID keyed identity with an extension-generated per-profile UUID.

**Architecture:** The Chrome extension generates a stable UUID on first run and stores it in `chrome.storage.local` (which is naturally per-profile). On native-host startup, the host reads messages from stdin until it receives a `handshake` containing that `profileId`, then binds `~/.tabb/<profileId>.sock`. `profiles.json` is upgraded from `name → extensionID` (flat string) to `name → {profileId, extensionId, browser}`. Old-schema files are detected and rejected with a re-setup prompt (no automatic migration). The setup flow changes from "paste the extension ID" to "reload the extension, we'll detect it."

**Tech Stack:** Go (stdlib only, plus `testing`), vanilla JavaScript (Chrome Extension Manifest V3), Chrome Native Messaging protocol (length-prefixed JSON over stdin/stdout), Unix domain sockets.

---

## File Structure

**Modified files:**
- `internal/profile/profile.go` — new `Entry` struct, new `Map = map[string]Entry`, `ErrLegacySchema`, updated helpers and `Resolve`.
- `internal/profile/profile_test.go` — updated existing tests, new tests for schema migration detection and new `Entry` shape.
- `internal/native/native.go` — add small helper for handshake-first read with timeout (if needed).
- `cmd/tabb/host.go` — `runHost` restructured: wait-for-handshake phase, then bind socket, then run request loop. Remove `browserName` global and `saveBrowserName`'s use of extensionID as the key (now keyed by profileId).
- `cmd/tabb/host_test.go` — **new**, unit tests for `waitForHandshake` reader function.
- `cmd/tabb/setup.go` — rewrite to a detect-by-reload flow; rebuild `allowed_origins` from the new schema.
- `cmd/tabb/profiles.go` — update to read/print new schema.
- `cmd/tabb/main.go` — no change to routing; comment update only if needed.
- `extension/background.js` — generate/persist `profileId` in `chrome.storage.local`; include in handshake.

**New concepts:**
- `profile.Entry{ProfileID, ExtensionID, Browser string}` — canonical profile record.
- `profile.ErrLegacySchema` — sentinel error returned by `Load` when the file is in old format.
- `waitForHandshake(r io.Reader, timeout time.Duration) (HandshakeInfo, error)` — extracted from `runHost` so it can be unit tested.

---

## Task 1: Profile package — new `Entry` type + schema migration detection (TDD)

**Files:**
- Modify: `internal/profile/profile.go` (whole file)
- Modify: `internal/profile/profile_test.go` (update existing tests, add new ones)

### Step 1.1: Write failing tests for the new schema

- [ ] Replace the contents of `internal/profile/profile_test.go` with the following. This updates existing tests to the new schema and adds migration-detection tests.

```go
package profile_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joelhelbling/tabb/internal/profile"
)

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	m, err := profile.Load(path)
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	original := profile.Map{
		"Work":     {ProfileID: "uuid-work", ExtensionID: "ext-abc", Browser: "Vivaldi"},
		"Personal": {ProfileID: "uuid-personal", ExtensionID: "ext-abc", Browser: "Vivaldi"},
	}

	if err := profile.Save(path, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := profile.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("expected %d entries, got %d", len(original), len(loaded))
	}
	for k, v := range original {
		if loaded[k] != v {
			t.Errorf("key %q: expected %+v, got %+v", k, v, loaded[k])
		}
	}
}

func TestLoadLegacySchemaReturnsSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	// Old-style profiles.json: string values, not objects.
	legacy := `{"Work":"abc123","Personal":"def456"}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("failed to write legacy profiles.json: %v", err)
	}

	_, err := profile.Load(path)
	if err == nil {
		t.Fatal("expected ErrLegacySchema, got nil")
	}
	if !errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("expected ErrLegacySchema, got: %v", err)
	}
}

func TestLoadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := profile.Load(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
	if errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("corrupt JSON should not be reported as legacy schema, got: %v", err)
	}
}

func TestFindByNameCaseInsensitive(t *testing.T) {
	m := profile.Map{
		"Brave-Work": {ProfileID: "uuid-brave", ExtensionID: "ext-brave", Browser: "Brave"},
		"Personal":   {ProfileID: "uuid-personal", ExtensionID: "ext-personal", Browser: "Chrome"},
	}

	name, entry, found := profile.FindByName(m, "brave-work")
	if !found {
		t.Fatal("expected to find 'brave-work' case-insensitively")
	}
	if name != "Brave-Work" {
		t.Errorf("expected original casing 'Brave-Work', got %q", name)
	}
	if entry.ProfileID != "uuid-brave" {
		t.Errorf("expected profile ID 'uuid-brave', got %q", entry.ProfileID)
	}

	_, _, found2 := profile.FindByName(m, "nonexistent")
	if found2 {
		t.Error("expected not found for 'nonexistent'")
	}
}

func TestFindByProfileID(t *testing.T) {
	m := profile.Map{
		"Work": {ProfileID: "uuid-work", ExtensionID: "abc123", Browser: "Chrome"},
		"Home": {ProfileID: "uuid-home", ExtensionID: "xyz789", Browser: "Chrome"},
	}

	name, found := profile.FindByProfileID(m, "uuid-work")
	if !found {
		t.Fatal("expected to find profile ID 'uuid-work'")
	}
	if name != "Work" {
		t.Errorf("expected profile name 'Work', got %q", name)
	}

	_, found2 := profile.FindByProfileID(m, "notfound")
	if found2 {
		t.Error("expected not found for unknown profile ID")
	}
}

func TestNameAvailable(t *testing.T) {
	m := profile.Map{
		"Work": {ProfileID: "uuid-work", ExtensionID: "abc123", Browser: "Chrome"},
	}

	if !profile.NameAvailable(m, "Personal") {
		t.Error("'Personal' should be available")
	}
	if !profile.NameAvailable(m, "work-extra") {
		t.Error("'work-extra' should be available")
	}
	if profile.NameAvailable(m, "work") {
		t.Error("'work' should NOT be available (case-insensitive match with 'Work')")
	}
	if profile.NameAvailable(m, "WORK") {
		t.Error("'WORK' should NOT be available (case-insensitive match with 'Work')")
	}
}

func TestExtensionIDs(t *testing.T) {
	m := profile.Map{
		"Work":     {ProfileID: "uuid-work", ExtensionID: "shared-ext", Browser: "Vivaldi"},
		"Personal": {ProfileID: "uuid-personal", ExtensionID: "shared-ext", Browser: "Vivaldi"},
		"Other":    {ProfileID: "uuid-other", ExtensionID: "different-ext", Browser: "Chrome"},
	}

	ids := profile.ExtensionIDs(m)
	// Expect deduplicated set
	if len(ids) != 2 {
		t.Errorf("expected 2 unique extension IDs, got %d: %v", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen["shared-ext"] || !seen["different-ext"] {
		t.Errorf("missing expected extension IDs: %v", ids)
	}
}

func TestResolveOnlyOneSocket(t *testing.T) {
	dir := t.TempDir()

	sockFile := filepath.Join(dir, "uuid-abc.sock")
	if err := os.WriteFile(sockFile, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create sock file: %v", err)
	}

	profilesPath := filepath.Join(dir, "profiles.json")

	profileID, err := profile.Resolve(dir, profilesPath, "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-abc" {
		t.Errorf("expected 'uuid-abc', got %q", profileID)
	}
}

func TestResolveFlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	m := profile.Map{
		"Flag-Profile": {ProfileID: "uuid-flag", ExtensionID: "ext", Browser: "Chrome"},
		"Env-Profile":  {ProfileID: "uuid-env", ExtensionID: "ext", Browser: "Chrome"},
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	profileID, err := profile.Resolve(dir, profilesPath, "Flag-Profile", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-flag" {
		t.Errorf("expected 'uuid-flag' (flag takes priority), got %q", profileID)
	}
}

func TestResolveEnvVar(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	m := profile.Map{
		"Env-Profile": {ProfileID: "uuid-env", ExtensionID: "ext", Browser: "Chrome"},
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	profileID, err := profile.Resolve(dir, profilesPath, "", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-env" {
		t.Errorf("expected 'uuid-env', got %q", profileID)
	}
}

func TestResolveMultipleSocketsNoProfile(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"uuid-aaa.sock", "uuid-bbb.sock"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte{}, 0600); err != nil {
			t.Fatalf("failed to create sock file %s: %v", name, err)
		}
	}

	profilesPath := filepath.Join(dir, "profiles.json")

	_, err := profile.Resolve(dir, profilesPath, "", "")
	if err == nil {
		t.Fatal("expected an error when multiple sockets exist")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("expected error to contain 'multiple', got: %v", err)
	}
}

func TestResolveNoSockets(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	_, err := profile.Resolve(dir, profilesPath, "", "")
	if err == nil {
		t.Fatal("expected an error when no sockets exist")
	}
	if !strings.Contains(err.Error(), "no active tabb sockets found") {
		t.Errorf("expected 'no active tabb sockets found' error, got: %v", err)
	}
}

func TestResolveLegacySocket(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tabb.sock"), []byte{}, 0600)

	_, err := profile.Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err == nil {
		t.Fatal("expected error for legacy socket")
	}
	if !strings.Contains(err.Error(), "tabb setup") {
		t.Errorf("expected migration hint in error, got: %v", err)
	}
}

func TestResolveLegacyProfilesSchema(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	// Create one sock file so we don't exit early with "no sockets"
	if err := os.WriteFile(filepath.Join(dir, "uuid-x.sock"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	// Write a legacy profiles.json
	legacy := `{"Work":"abc123"}`
	if err := os.WriteFile(profilesPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	// Using --profile flag forces a profiles.json read, which must surface the legacy error
	_, err := profile.Resolve(dir, profilesPath, "Work", "")
	if err == nil {
		t.Fatal("expected legacy-schema error")
	}
	if !errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("expected ErrLegacySchema, got: %v", err)
	}
}
```

- [ ] **Step 1.2: Run tests — they must fail to compile**

Run: `go test ./internal/profile/...`

Expected: build errors like `undefined: profile.Entry`, `undefined: profile.ErrLegacySchema`, `undefined: profile.FindByProfileID`, `undefined: profile.ExtensionIDs`. That's the failing state we want before writing the implementation.

### Step 1.3: Rewrite `internal/profile/profile.go` to the new schema

- [ ] Replace the contents of `internal/profile/profile.go` with:

```go
// Package profile manages ~/.tabb/profiles.json and provides profile resolution logic.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is a single profile record.
type Entry struct {
	ProfileID   string `json:"profileId"`
	ExtensionID string `json:"extensionId"`
	Browser     string `json:"browser,omitempty"`
}

// Map maps profile names to entries.
type Map map[string]Entry

// ErrLegacySchema is returned by Load when profiles.json is in the old
// string-valued format and must be migrated by re-running `tabb setup`.
var ErrLegacySchema = errors.New("profiles.json is in legacy format; re-run 'tabb setup' to migrate")

// Load reads profiles.json from path. Returns an empty map if the file does not exist.
// If the file is in the legacy (string-valued) format, returns ErrLegacySchema.
func Load(path string) (Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Map{}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}

	// Detect legacy format: values are raw JSON strings rather than objects.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	for _, v := range raw {
		trimmed := strings.TrimSpace(string(v))
		if len(trimmed) > 0 && trimmed[0] == '"' {
			return nil, ErrLegacySchema
		}
	}

	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	if m == nil {
		m = Map{}
	}
	return m, nil
}

// Save writes profiles as indented JSON to path.
func Save(path string, profiles Map) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding profiles: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing profiles: %w", err)
	}
	return nil
}

// FindByName performs a case-insensitive lookup by profile name.
// Returns the original-cased name, the entry, and whether it was found.
func FindByName(profiles Map, name string) (string, Entry, bool) {
	lower := strings.ToLower(name)
	for k, v := range profiles {
		if strings.ToLower(k) == lower {
			return k, v, true
		}
	}
	return "", Entry{}, false
}

// FindByProfileID performs a reverse lookup by profile ID.
func FindByProfileID(profiles Map, profileID string) (string, bool) {
	for name, e := range profiles {
		if e.ProfileID == profileID {
			return name, true
		}
	}
	return "", false
}

// NameAvailable reports whether the given name is not already in use (case-insensitive).
func NameAvailable(profiles Map, name string) bool {
	_, _, found := FindByName(profiles, name)
	return !found
}

// ExtensionIDs returns the deduplicated set of extension IDs across all entries.
// Used to populate allowed_origins in the Native Messaging host manifest.
func ExtensionIDs(profiles Map) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range profiles {
		if e.ExtensionID == "" || seen[e.ExtensionID] {
			continue
		}
		seen[e.ExtensionID] = true
		out = append(out, e.ExtensionID)
	}
	return out
}

// ProfilesPath returns the path to profiles.json within tabbDir.
func ProfilesPath(tabbDir string) string {
	return filepath.Join(tabbDir, "profiles.json")
}

// ActiveSockets reads tabbDir and returns profile IDs from *.sock files.
// Returns nil if the directory does not exist. Skips the legacy tabb.sock file.
func ActiveSockets(tabbDir string) ([]string, error) {
	entries, err := os.ReadDir(tabbDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tabb directory: %w", err)
	}

	var ids []string
	for _, e := range entries {
		name := e.Name()
		if name == "tabb.sock" {
			continue // legacy socket, skip
		}
		if strings.HasSuffix(name, ".sock") {
			ids = append(ids, strings.TrimSuffix(name, ".sock"))
		}
	}
	return ids, nil
}

// HasLegacySocket returns true if the old-style tabb.sock exists.
func HasLegacySocket(tabbDir string) bool {
	_, err := os.Stat(filepath.Join(tabbDir, "tabb.sock"))
	return err == nil
}

// Resolve determines which profile ID to connect to.
// Priority: flagProfile > envProfile > auto-detect from active sockets.
func Resolve(tabbDir, profilesPath, flagProfile, envProfile string) (string, error) {
	profileName := flagProfile
	if profileName == "" {
		profileName = envProfile
	}

	if profileName != "" {
		profiles, err := Load(profilesPath)
		if err != nil {
			return "", err
		}
		_, entry, found := FindByName(profiles, profileName)
		if !found {
			return "", fmt.Errorf("profile %q not found in %s", profileName, profilesPath)
		}
		return entry.ProfileID, nil
	}

	sockets, err := ActiveSockets(tabbDir)
	if err != nil {
		return "", err
	}

	switch len(sockets) {
	case 0:
		if HasLegacySocket(tabbDir) {
			return "", fmt.Errorf("found legacy tabb.sock — run 'tabb setup' to migrate to the new multi-profile format")
		}
		return "", fmt.Errorf("no active tabb sockets found (is Chrome running with the tabb extension?)")
	case 1:
		return sockets[0], nil
	default:
		profiles, err := Load(profilesPath)
		if err != nil {
			// If legacy, surface that — it's a better error than "multiple".
			return "", err
		}
		return "", multipleProfilesError(sockets, profiles)
	}
}

func multipleProfilesError(socketIDs []string, profiles Map) error {
	var sb strings.Builder
	sb.WriteString("multiple active tabb profiles found; specify one with --profile or TABB_PROFILE:\n")
	for _, id := range socketIDs {
		if name, found := FindByProfileID(profiles, id); found {
			fmt.Fprintf(&sb, "  %s (%s)\n", name, id)
		} else {
			fmt.Fprintf(&sb, "  (unnamed) (%s)\n", id)
		}
	}
	return errors.New(strings.TrimRight(sb.String(), "\n"))
}
```

- [ ] **Step 1.4: Run tests — must pass**

Run: `go test ./internal/profile/... -v`

Expected: all tests pass, including the new `TestLoadLegacySchemaReturnsSentinel`, `TestLoadCorruptJSON`, `TestFindByProfileID`, `TestExtensionIDs`, and `TestResolveLegacyProfilesSchema`.

- [ ] **Step 1.5: Commit**

```bash
git add internal/profile/profile.go internal/profile/profile_test.go
git commit -m "$(cat <<'EOF'
refactor(profile): new schema with profileId + legacy detection

profiles.json now maps name to {profileId, extensionId, browser}.
Load returns ErrLegacySchema when the file is in the old string-valued
format so callers can guide the user through re-running 'tabb setup'.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Extract handshake-first reader + unit test

This task is the reason Task 3 (updating `runHost`) is safe: we factor the handshake-reading logic out into a pure function that operates on any `io.Reader`, unit test it against synthetic input, then plug it into the real process in Task 3.

**Files:**
- Modify: `cmd/tabb/host.go` (add `HandshakeInfo` type + `waitForHandshake` function; do NOT wire it into `runHost` yet)
- Create: `cmd/tabb/host_test.go`

### Step 2.1: Write failing tests

- [ ] Create `cmd/tabb/host_test.go`:

```go
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
	// First message is not a handshake — older extensions would send a regular
	// request here. We hard-fail instead of silently accepting.
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
	// blockingReader never returns — waitForHandshake must give up after the timeout.
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
	// Block "forever" — tests use a short timeout so this is safe.
	time.Sleep(10 * time.Second)
	return 0, nil
}
```

- [ ] **Step 2.2: Run tests — must fail to compile**

Run: `go test ./cmd/tabb/... -run TestWaitForHandshake`

Expected: `undefined: waitForHandshake`, `undefined: errHandshakeTimeout`, `undefined: HandshakeInfo`.

### Step 2.3: Add the handshake reader to `cmd/tabb/host.go`

- [ ] Add the following near the top of `cmd/tabb/host.go`, right after the imports block. Do **not** wire it into `runHost` yet — Task 3 does that.

```go
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
```

- [ ] Add the new imports to the existing import block in `cmd/tabb/host.go`. The final import block should read:

```go
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
```

- [ ] **Step 2.4: Run tests — must pass**

Run: `go test ./cmd/tabb/... -run TestWaitForHandshake -v`

Expected: all five tests pass.

- [ ] **Step 2.5: Commit**

```bash
git add cmd/tabb/host.go cmd/tabb/host_test.go
git commit -m "$(cat <<'EOF'
refactor(host): extract waitForHandshake with unit tests

Pure reader function that consumes a single Native Messaging message
and requires it to be a handshake containing a profileId. Not wired
into runHost yet.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wire `runHost` to the handshake-first flow

**Files:**
- Modify: `cmd/tabb/host.go` — rewrite `runHost` and `readFromExtension`, remove `browserName` global.

### Step 3.1: Replace `runHost` and related functions

- [ ] In `cmd/tabb/host.go`, delete the `browserName` global variable and the `saveBrowserName` function. Replace the `runHost` and `readFromExtension` functions with:

```go
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
```

- [ ] **Step 3.2: Build**

Run: `go build ./cmd/tabb`

Expected: clean build. If `errors`, `io`, or `time` are reported as unused, remove them (they should all be used by `waitForHandshake` from Task 2).

- [ ] **Step 3.3: Run the full test suite**

Run: `go test ./...`

Expected: all profile and host tests pass. (The handshake tests we wrote in Task 2 still pass because `waitForHandshake` is unchanged.)

- [ ] **Step 3.4: Commit**

```bash
git add cmd/tabb/host.go
git commit -m "$(cat <<'EOF'
feat(host): bind socket at profileId after handshake

runHost now waits for the extension's handshake before binding the
Unix socket, so the socket path is keyed on the extension-generated
profileId instead of the browser-assigned extensionID. This fixes
the Vivaldi bug where two profiles collide on the same extensionID.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Extension — generate and send `profileId`

**Files:**
- Modify: `extension/background.js` (only the `connect()` function near the top)

### Step 4.1: Update `connect()` to include `profileId` in the handshake

- [ ] In `extension/background.js`, replace the current `connect()` function (lines 7–39) with:

```javascript
async function getOrCreateProfileId() {
  const { tabbProfileId } = await chrome.storage.local.get("tabbProfileId");
  if (tabbProfileId) return tabbProfileId;
  const newId = crypto.randomUUID();
  await chrome.storage.local.set({ tabbProfileId: newId });
  return newId;
}

async function connect() {
  const profileId = await getOrCreateProfileId();

  port = chrome.runtime.connectNative(NATIVE_HOST);

  port.onMessage.addListener((msg) => {
    dispatch(msg)
      .then((response) => port.postMessage(response))
      .catch((err) =>
        port.postMessage({ id: msg.id, error: err.message || String(err) })
      );
  });

  port.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError?.message || "unknown";
    console.log("tabb: native host disconnected:", err);
    port = null;
    setTimeout(connect, 5000);
  });

  console.log("tabb: connected to native host");

  // Send handshake with browser info and per-profile UUID
  const brands = navigator.userAgentData?.brands || [];
  const browser = brands.find((b) =>
    ["Google Chrome", "Brave", "Microsoft Edge", "Opera", "Vivaldi"].includes(b.brand)
  );
  port.postMessage({
    action: "handshake",
    params: {
      profileId: profileId,
      browser: browser?.brand || "Chrome",
      extensionId: chrome.runtime.id,
    },
  });
}

connect();
```

- [ ] **Step 4.2: Verify syntax**

There is no JS test runner in this repo. Syntax-check by running:

```bash
node --check extension/background.js
```

Expected: no output (success).

- [ ] **Step 4.3: Commit**

```bash
git add extension/background.js
git commit -m "$(cat <<'EOF'
feat(extension): generate and send profileId in handshake

chrome.storage.local is per-profile, so the generated UUID uniquely
identifies each browser profile even when the browser assigns the
same extension ID to multiple profiles (as Vivaldi does).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Rewrite `tabb setup` to use detection instead of paste

**Files:**
- Modify: `cmd/tabb/setup.go` (whole file)

### Step 5.1: Replace the setup flow

The new flow: user runs `tabb setup`, the command lists already-registered profiles (if any), tells the user to load/reload the extension, then waits for Enter. On Enter it scans `~/.tabb/*.browser` for entries whose profileId is not yet in `profiles.json` and prompts for a name for each new one.

- [ ] Replace the contents of `cmd/tabb/setup.go` with:

```go
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joelhelbling/tabb/internal/profile"
)

type nativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// discoveredProfile is a profileId seen in ~/.tabb/*.browser but not yet
// present in profiles.json.
type discoveredProfile struct {
	ProfileID   string
	ExtensionID string
	Browser     string
}

func runSetup() error {
	binaryPath, err := exec.LookPath("tabb")
	if err != nil {
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine tabb binary path: %w", err)
		}
	}
	binaryPath, _ = filepath.Abs(binaryPath)

	manifestDir, err := nativeMessagingDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	tabbDir, err := tabbHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tabbDir, 0700); err != nil {
		return fmt.Errorf("creating tabb directory: %w", err)
	}

	profilesPath := profile.ProfilesPath(tabbDir)
	profiles, err := profile.Load(profilesPath)
	if err != nil {
		if errors.Is(err, profile.ErrLegacySchema) {
			return migrateLegacyProfiles(tabbDir, profilesPath)
		}
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	// On first run we still need to place the Native Messaging manifest so the
	// browser can even launch the host. Write it now using whatever extension IDs
	// we have (possibly none); we'll rewrite it at the end too.
	manifestPath := filepath.Join(manifestDir, "com.tabb.json")
	if err := writeManifest(manifestPath, binaryPath, profile.ExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Println("Load the extension in your browser:")
	fmt.Println("  1. Open chrome://extensions (or equivalent)")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println("  4. If it's already loaded, click its reload icon")
	fmt.Println()
	if len(profiles) > 0 {
		fmt.Println("Registered profiles:")
		for name, e := range profiles {
			fmt.Printf("  %s  (%s)\n", name, e.Browser)
		}
		fmt.Println()
	}
	fmt.Print("Press Enter when the extension is loaded/reloaded: ")
	if _, err := reader.ReadString('\n'); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	discovered, err := discoverNewProfiles(tabbDir, profiles)
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		fmt.Println("\nNo new profiles detected.")
		fmt.Println("Make sure the extension is loaded and the browser is running,")
		fmt.Println("then re-run 'tabb setup'.")
		return nil
	}

	for _, d := range discovered {
		fmt.Printf("\nNew profile detected: browser=%s profileId=%s\n", d.Browser, d.ProfileID)
		defaultName := suggestProfileName(profiles, d.Browser)
		var prompt string
		if defaultName != "" {
			prompt = fmt.Sprintf("Profile name [%s]: ", defaultName)
		} else {
			prompt = "Profile name: "
		}
		fmt.Print(prompt)
		nameInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		profileName := strings.TrimSpace(nameInput)
		if profileName == "" {
			profileName = defaultName
		}
		if profileName == "" {
			return fmt.Errorf("profile name is required")
		}
		if !profile.NameAvailable(profiles, profileName) {
			return fmt.Errorf("profile name %q is already in use (case-insensitive)", profileName)
		}
		profiles[profileName] = profile.Entry{
			ProfileID:   d.ProfileID,
			ExtensionID: d.ExtensionID,
			Browser:     d.Browser,
		}
		fmt.Printf("Registered %q\n", profileName)
	}

	if err := profile.Save(profilesPath, profiles); err != nil {
		return err
	}

	// Rewrite the manifest with the full set of extension IDs.
	if err := writeManifest(manifestPath, binaryPath, profile.ExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Printf("\nSaved %s\n", profilesPath)
	fmt.Printf("Manifest updated: %s\n", manifestPath)
	fmt.Println("You can now run 'tabb list' (or use the MCP tools).")
	return nil
}

// discoverNewProfiles scans tabbDir for *.browser files whose profileId is not
// already registered in profiles. Each .browser file is written by the native
// host on handshake and contains "<browser>\n<extensionId>\n".
func discoverNewProfiles(tabbDir string, profiles profile.Map) ([]discoveredProfile, error) {
	entries, err := os.ReadDir(tabbDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tabb directory: %w", err)
	}

	registered := map[string]bool{}
	for _, e := range profiles {
		registered[e.ProfileID] = true
	}

	var out []discoveredProfile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".browser") {
			continue
		}
		profileID := strings.TrimSuffix(name, ".browser")
		if registered[profileID] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tabbDir, name))
		if err != nil {
			continue
		}
		lines := strings.SplitN(strings.TrimRight(string(data), "\n"), "\n", 2)
		browser := ""
		extensionID := ""
		if len(lines) > 0 {
			browser = lines[0]
		}
		if len(lines) > 1 {
			extensionID = lines[1]
		}
		out = append(out, discoveredProfile{
			ProfileID:   profileID,
			ExtensionID: extensionID,
			Browser:     browser,
		})
	}
	return out, nil
}

// migrateLegacyProfiles handles the case where profiles.json is in the old
// string-valued schema. We cannot automatically map old extensionIDs to new
// profileIDs, so we move the old file aside and ask the user to re-run setup.
func migrateLegacyProfiles(tabbDir, profilesPath string) error {
	backup := profilesPath + ".legacy"
	if err := os.Rename(profilesPath, backup); err != nil {
		return fmt.Errorf("moving legacy profiles.json aside: %w", err)
	}
	// Also delete any extension-ID-keyed .browser or .sock files so setup
	// starts fresh. Ignore errors — best effort.
	entries, _ := os.ReadDir(tabbDir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".sock") || strings.HasSuffix(name, ".browser") {
			os.Remove(filepath.Join(tabbDir, name))
		}
	}
	fmt.Printf("Legacy profiles.json moved to %s\n", backup)
	fmt.Println("Re-run 'tabb setup' after reloading the extension in each browser profile.")
	return nil
}

func writeManifest(path, binaryPath string, extensionIDs []string) error {
	origins := make([]string, 0, len(extensionIDs))
	for _, id := range extensionIDs {
		origins = append(origins, fmt.Sprintf("chrome-extension://%s/", id))
	}

	manifest := nativeManifest{
		Name:           "com.tabb",
		Description:    "tabb — manage Chrome tabs from the terminal",
		Path:           binaryPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	return nil
}

func nativeMessagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s (only macOS and Linux are supported)", runtime.GOOS)
	}
}

func tabbHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".tabb"), nil
}

// suggestProfileName picks a default based on the browser name, falling back
// to "Default" for the first profile or a numbered suffix thereafter.
func suggestProfileName(profiles profile.Map, browser string) string {
	if len(profiles) == 0 {
		if browser != "" {
			return browser
		}
		return "Default"
	}
	if browser != "" && profile.NameAvailable(profiles, browser) {
		return browser
	}
	if browser != "" {
		for i := 2; i <= 99; i++ {
			candidate := fmt.Sprintf("%s-%d", browser, i)
			if profile.NameAvailable(profiles, candidate) {
				return candidate
			}
		}
	}
	return ""
}
```

- [ ] **Step 5.2: Build**

Run: `go build ./cmd/tabb`

Expected: clean build. If the build complains about unused helpers that lived in the old setup.go (`loadProfiles`, `saveProfiles`, `findProfileByExtID`, `isNameAvailable`, `allExtensionIDs`) referenced from other files, update those callers to use the `profile` package directly.

- [ ] **Step 5.3: Run the test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 5.4: Commit**

```bash
git add cmd/tabb/setup.go
git commit -m "$(cat <<'EOF'
feat(setup): detect new profiles by reload instead of pasted extension ID

'tabb setup' now prompts the user to load/reload the extension, then
scans ~/.tabb/*.browser for profileIds not yet registered. Legacy
profiles.json files are moved aside with a clear re-run prompt.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Update `tabb profiles` to the new schema

**Files:**
- Modify: `cmd/tabb/profiles.go`

### Step 6.1: Inspect and update

- [ ] Read `cmd/tabb/profiles.go`. It currently calls `loadProfiles` (now deleted) and prints `name → extensionID`. Update it to:
  1. Use `profile.Load(profile.ProfilesPath(tabbDir))` and handle `profile.ErrLegacySchema` by printing a clear re-setup message.
  2. Print each entry as `name  browser  profileId` with active-status annotation derived from `profile.ActiveSockets`.

Concrete replacement (drop-in for the file body — adapt field names as the existing code uses them):

```go
package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/joelhelbling/tabb/internal/profile"
)

func runProfiles() error {
	tabbDir, err := tabbHomeDir()
	if err != nil {
		return err
	}
	profilesPath := profile.ProfilesPath(tabbDir)

	profiles, err := profile.Load(profilesPath)
	if err != nil {
		if errors.Is(err, profile.ErrLegacySchema) {
			fmt.Println("profiles.json is in the legacy format — run 'tabb setup' to migrate.")
			return nil
		}
		return err
	}

	active, err := profile.ActiveSockets(tabbDir)
	if err != nil {
		return err
	}
	activeSet := map[string]bool{}
	for _, id := range active {
		activeSet[id] = true
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles configured. Run 'tabb setup' to register one.")
		return nil
	}

	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		e := profiles[name]
		status := "inactive"
		if activeSet[e.ProfileID] {
			status = "active"
		}
		fmt.Printf("%-20s  %-10s  %-36s  %s\n", name, e.Browser, e.ProfileID, status)
	}
	return nil
}
```

- [ ] **Step 6.2: Build and run tests**

Run: `go build ./cmd/tabb && go test ./...`

Expected: clean build, tests pass.

- [ ] **Step 6.3: Commit**

```bash
git add cmd/tabb/profiles.go
git commit -m "$(cat <<'EOF'
refactor(profiles cmd): use new Entry schema and active-socket status

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: End-to-end manual verification

There is no automated integration test for the extension ↔ native host loop, so this task is a hand-rolled smoke test. Track completion in the checkboxes.

- [ ] **Step 7.1: Clean up any legacy state**

```bash
ls ~/.tabb/
# If there are old *.sock, *.browser, or tabb.sock files from pre-change runs,
# remove them after confirming no extension is actively using them.
```

- [ ] **Step 7.2: Build and install**

```bash
make install PREFIX=$HOME/.local
```

- [ ] **Step 7.3: Reload the extension in each browser profile**

In Vivaldi (or whichever browser reproduced the bug):
1. Profile A → `vivaldi://extensions` → reload tabb extension.
2. Profile B → `vivaldi://extensions` → reload tabb extension.

- [ ] **Step 7.4: Run setup once per profile**

```bash
tabb setup
# Press Enter at the prompt. Expect "New profile detected" for the most
# recently reloaded extension. Name it (e.g., "Vivaldi-Work"). Save.
```

Switch browser profiles, reload the extension again, and re-run `tabb setup` to register the second profile (e.g., "Vivaldi-Personal").

- [ ] **Step 7.5: Verify two distinct sockets exist**

```bash
ls ~/.tabb/*.sock
```

Expected: two `.sock` files with different UUID names. (Before the fix, the second profile would have clobbered the first — there would only be one.)

- [ ] **Step 7.6: Exercise both profiles**

```bash
tabb --profile Vivaldi-Work list
tabb --profile Vivaldi-Personal list
```

Expected: each command returns tabs from the corresponding browser profile. Confirm by opening a distinctive tab in one profile and checking it shows up only in that profile's list.

- [ ] **Step 7.7: Verify legacy detection**

```bash
cp ~/.tabb/profiles.json /tmp/profiles-backup.json
echo '{"Old":"abc123"}' > ~/.tabb/profiles.json
tabb profiles
# Expected: "profiles.json is in the legacy format — run 'tabb setup' to migrate."
tabb setup
# Expected: "Legacy profiles.json moved to …profiles.json.legacy" and a fresh setup run.
cp /tmp/profiles-backup.json ~/.tabb/profiles.json  # restore real profiles
```

- [ ] **Step 7.8: Run the full Go test suite one more time**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 7.9: Commit any final cleanup**

If Task 7 surfaced small fixes (typos, missing log lines, etc.), commit them:

```bash
git add -u
git commit -m "$(cat <<'EOF'
chore: smoke-test fixes after profile-id handshake migration

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

- **Spec coverage:** Handshake-first socket binding (Task 3), extension UUID generation (Task 4), schema change + legacy detection (Task 1), detection-based setup (Task 5), test for migration detection (Task 1, `TestLoadLegacySchemaReturnsSentinel` and `TestResolveLegacyProfilesSchema`), updated `tabb profiles` (Task 6), manual verification (Task 7). No gaps.
- **Hard-fail on missing handshake:** enforced in `waitForHandshake` (Task 2) — any non-handshake first message or missing `profileId` is a hard error; timeout is `errHandshakeTimeout`.
- **Type consistency:** `profile.Entry` fields (`ProfileID`, `ExtensionID`, `Browser`) used consistently across tests, profile package, host, setup, and profiles command. `HandshakeInfo` fields match.
- **Naming quirk to watch:** the native host writes `~/.tabb/<profileId>.browser` with two lines (`browser\nextensionId\n`); `discoverNewProfiles` parses both. Do not change one without the other.
