# Multi-Profile Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support multiple Chrome browser profiles and Chrome-based browsers (Brave, Edge, etc.) by giving each extension installation its own named profile, socket, and manifest entry.

**Architecture:** Each browser/profile gets a unique socket file (`~/.tabb/<extensionId>.sock`) created by the native host process. A `~/.tabb/profiles.json` file maps human-friendly names to extension IDs. The CLI resolves profiles via `--profile` flag, `TABB_PROFILE` env var, or auto-detection (single socket). The extension sends a handshake on connect, including browser name, to enable smart default profile names during setup.

**Tech Stack:** Go standard library, Chrome Extension APIs (`navigator.userAgentData`)

---

## File Structure

```
internal/
  profile/
    profile.go          # NEW — profiles.json read/write, profile resolution, socket discovery
    profile_test.go     # NEW — tests for profile logic
internal/
  socket/
    socket.go           # MODIFY — parameterize Path/Listen/Dial/Cleanup to accept extension ID
cmd/tabb/
    main.go             # MODIFY — parse extension ID from arg, pass to host; add "profiles" command
    host.go             # MODIFY — pass extension ID to socket.Listen; handle handshake messages
    setup.go            # MODIFY — new interactive flow: prompt for name, accumulate allowed_origins
    client.go           # MODIFY — resolve profile to socket path before dialing
    profiles.go         # NEW — tabb profiles command implementation
    mcp.go              # MODIFY — use profile-aware dial
extension/
    background.js       # MODIFY — send handshake with browser name on connect
```

---

### Task 1: Parameterize Socket Path by Extension ID

Currently `internal/socket/socket.go` uses a hardcoded `tabb.sock` filename. All socket functions need to accept an extension ID and use `<extensionId>.sock` instead.

**Files:**
- Modify: `internal/socket/socket.go`

- [ ] **Step 1: Update `Path()` to accept an extension ID**

Replace the current `Path()` function and constants:

```go
const dirName = ".tabb"

// Path returns the full path to the Unix domain socket for a given extension ID.
func Path(extensionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, dirName, extensionID+".sock"), nil
}
```

Remove the `sockName` constant (line 13).

- [ ] **Step 2: Update `Listen()` to accept an extension ID**

Change the signature and first line:

```go
func Listen(extensionID string) (net.Listener, error) {
	sockPath, err := Path(extensionID)
```

The rest of the function body stays the same.

- [ ] **Step 3: Update `Dial()` to accept an extension ID**

```go
func Dial(extensionID string) (net.Conn, error) {
	sockPath, err := Path(extensionID)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("connecting to tabb socket for extension %s (is Chrome running with the tabb extension?): %w", extensionID, err)
	}
	return conn, nil
}
```

- [ ] **Step 4: Update `Cleanup()` to accept an extension ID**

```go
func Cleanup(extensionID string) error {
	sockPath, err := Path(extensionID)
	if err != nil {
		return err
	}
	return os.Remove(sockPath)
}
```

- [ ] **Step 5: Add `Dir()` helper for discovering sockets**

Add this function for the profile discovery logic to use later:

```go
// Dir returns the path to the tabb directory (~/.tabb).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, dirName), nil
}
```

- [ ] **Step 6: Verify build compiles (expect compile errors in callers)**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./internal/socket/`
Expected: PASS (the package itself compiles; callers will break in later tasks)

- [ ] **Step 7: Commit**

```bash
git add internal/socket/socket.go
git commit -m "refactor: parameterize socket functions by extension ID"
```

---

### Task 2: Create Profile Module

This module manages `~/.tabb/profiles.json` and provides profile resolution logic for the CLI.

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

- [ ] **Step 1: Write tests for profile loading and saving**

Create `internal/profile/profile_test.go`:

```go
package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	profiles, err := Load(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected empty map, got %v", profiles)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	profiles := Map{"Work": "abc123", "Personal": "def456"}
	if err := Save(path, profiles); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded["Work"] != "abc123" || loaded["Personal"] != "def456" {
		t.Fatalf("unexpected profiles: %v", loaded)
	}
}

func TestFindByNameCaseInsensitive(t *testing.T) {
	profiles := Map{"Brave-Work": "abc123", "Default": "def456"}

	name, id, ok := FindByName(profiles, "brave-work")
	if !ok {
		t.Fatal("expected to find profile")
	}
	if name != "Brave-Work" {
		t.Errorf("expected original casing 'Brave-Work', got %q", name)
	}
	if id != "abc123" {
		t.Errorf("expected id 'abc123', got %q", id)
	}

	_, _, ok = FindByName(profiles, "nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestFindByExtensionID(t *testing.T) {
	profiles := Map{"Work": "abc123", "Personal": "def456"}

	name, ok := FindByExtensionID(profiles, "def456")
	if !ok || name != "Personal" {
		t.Fatalf("expected 'Personal', got %q (ok=%v)", name, ok)
	}

	_, ok = FindByExtensionID(profiles, "unknown")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestNameAvailable(t *testing.T) {
	profiles := Map{"Default": "abc123"}

	if NameAvailable(profiles, "Default") {
		t.Error("expected Default to be unavailable")
	}
	if NameAvailable(profiles, "default") {
		t.Error("expected case-insensitive match to be unavailable")
	}
	if !NameAvailable(profiles, "Work") {
		t.Error("expected Work to be available")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/`
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: Implement the profile module**

Create `internal/profile/profile.go`:

```go
// Package profile manages the ~/.tabb/profiles.json file that maps
// human-friendly profile names to Chrome extension IDs.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Map is a mapping of profile name → extension ID.
type Map map[string]string

// Load reads profiles.json from the given path.
// Returns an empty map if the file doesn't exist.
func Load(path string) (Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Map{}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	return m, nil
}

// Save writes profiles to the given path as formatted JSON.
func Save(path string, profiles Map) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profiles: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing profiles: %w", err)
	}
	return nil
}

// FindByName looks up a profile by name using case-insensitive matching.
// Returns the original-cased name, the extension ID, and whether it was found.
func FindByName(profiles Map, name string) (string, string, bool) {
	lower := strings.ToLower(name)
	for k, v := range profiles {
		if strings.ToLower(k) == lower {
			return k, v, true
		}
	}
	return "", "", false
}

// FindByExtensionID looks up a profile name by its extension ID.
// Returns the profile name and whether it was found.
func FindByExtensionID(profiles Map, extensionID string) (string, bool) {
	for k, v := range profiles {
		if v == extensionID {
			return k, true
		}
	}
	return "", false
}

// NameAvailable returns true if no existing profile uses this name
// (case-insensitive comparison).
func NameAvailable(profiles Map, name string) bool {
	_, _, found := FindByName(profiles, name)
	return !found
}

// ProfilesPath returns the path to profiles.json inside the given tabb directory.
func ProfilesPath(tabbDir string) string {
	return tabbDir + "/profiles.json"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/ -v`
Expected: PASS — all 4 tests pass

- [ ] **Step 5: Write tests for profile resolution (CLI logic)**

Add to `internal/profile/profile_test.go`:

```go
func TestResolveOnlyOneSocket(t *testing.T) {
	dir := t.TempDir()
	// Create one socket file
	os.WriteFile(filepath.Join(dir, "abc123.sock"), []byte{}, 0600)

	id, err := Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc123" {
		t.Errorf("expected 'abc123', got %q", id)
	}
}

func TestResolveFlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	Save(path, Map{"Work": "abc123", "Personal": "def456"})

	id, err := Resolve(dir, path, "Work", "Personal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc123" {
		t.Errorf("expected flag profile 'abc123', got %q", id)
	}
}

func TestResolveEnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	Save(path, Map{"Brave": "xyz789"})

	id, err := Resolve(dir, path, "", "brave")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "xyz789" {
		t.Errorf("expected 'xyz789', got %q", id)
	}
}

func TestResolveMultipleSocketsNoProfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "abc123.sock"), []byte{}, 0600)
	os.WriteFile(filepath.Join(dir, "def456.sock"), []byte{}, 0600)

	path := filepath.Join(dir, "profiles.json")
	Save(path, Map{"Work": "abc123", "Personal": "def456"})

	_, err := Resolve(dir, path, "", "")
	if err == nil {
		t.Fatal("expected error for ambiguous profiles")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("expected 'multiple' in error, got: %v", err)
	}
}

func TestResolveNoSockets(t *testing.T) {
	dir := t.TempDir()
	_, err := Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err == nil {
		t.Fatal("expected error for no sockets")
	}
}
```

- [ ] **Step 6: Run tests to verify new tests fail**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/`
Expected: FAIL — `Resolve` not defined

- [ ] **Step 7: Implement `Resolve()`**

Add to `internal/profile/profile.go`:

```go
// Resolve determines which extension ID to connect to.
// Priority: flagProfile > envProfile > auto-detect (single socket).
// Returns the extension ID to use.
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
		_, extID, found := FindByName(profiles, profileName)
		if !found {
			return "", fmt.Errorf("profile %q not found in %s", profileName, profilesPath)
		}
		return extID, nil
	}

	// Auto-detect: find active sockets
	sockets, err := ActiveSockets(tabbDir)
	if err != nil {
		return "", err
	}

	switch len(sockets) {
	case 0:
		return "", fmt.Errorf("no active tabb sockets found (is Chrome running with the tabb extension?)")
	case 1:
		return sockets[0], nil
	default:
		profiles, _ := Load(profilesPath)
		return "", multipleProfilesError(sockets, profiles)
	}
}

// ActiveSockets returns extension IDs for all .sock files in tabbDir.
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
		if strings.HasSuffix(name, ".sock") {
			ids = append(ids, strings.TrimSuffix(name, ".sock"))
		}
	}
	return ids, nil
}

func multipleProfilesError(socketIDs []string, profiles Map) error {
	var lines []string
	lines = append(lines, "multiple tabb profiles are active — specify one with --profile or TABB_PROFILE:")
	for _, id := range socketIDs {
		name, found := FindByExtensionID(profiles, id)
		if found {
			lines = append(lines, fmt.Sprintf("  %s (%s)", name, id))
		} else {
			lines = append(lines, fmt.Sprintf("  (unnamed) (%s)", id))
		}
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}
```

Add `"os"` and `"errors"` to the import block if not already present (they are — `errors` and `os` are already imported).

- [ ] **Step 8: Run all profile tests**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/ -v`
Expected: PASS — all 9 tests pass

- [ ] **Step 9: Commit**

```bash
git add internal/profile/
git commit -m "feat: add profile module for multi-profile name/ID management"
```

---

### Task 3: Update Native Host to Use Extension-ID-Based Sockets

The native host receives the extension ID from Chrome as `os.Args[1]`. Parse it out and pass it to the socket functions.

**Files:**
- Modify: `cmd/tabb/main.go`
- Modify: `cmd/tabb/host.go`

- [ ] **Step 1: Parse extension ID in `main.go` and pass to `runHost()`**

Change the `runHost()` call in main.go (lines 17-23) and the signature:

In `main.go`, replace lines 17-22:

```go
	if strings.HasPrefix(os.Args[1], "chrome-extension://") {
		extID := strings.TrimPrefix(os.Args[1], "chrome-extension://")
		extID = strings.TrimSuffix(extID, "/")
		if err := runHost(extID); err != nil {
			fmt.Fprintf(os.Stderr, "tabb: %v\n", err)
			os.Exit(1)
		}
		return
	}
```

Also update the `"host"` case (line 28) — this manual invocation won't have a profile, but keep it working for debugging. Replace line 28:

```go
	case "host":
		err = runHost("")
```

- [ ] **Step 2: Update `runHost()` to accept and use the extension ID**

In `host.go`, change the signature and socket calls:

Replace the function signature (line 21):
```go
func runHost(extensionID string) error {
```

Replace the `socket.Listen()` call (line 34):
```go
	ln, err := socket.Listen(extensionID)
```

Replace the `socket.Cleanup()` calls. In the defer block (lines 38-41):
```go
	defer func() {
		ln.Close()
		socket.Cleanup(extensionID)
	}()
```

In the signal handler goroutine (lines 46-51):
```go
	go func() {
		<-sigCh
		ln.Close()
		socket.Cleanup(extensionID)
		os.Exit(0)
	}()
```

In `readFromExtension` (lines 72-73):
```go
			socket.Cleanup(extensionID)
```

Wait — `readFromExtension` doesn't have access to `extensionID`. Pass it as a parameter. Change the call on line 31:
```go
	go readFromExtension(pending, extensionID)
```

Change the function signature (line 66):
```go
func readFromExtension(pending *pendingRequests, extensionID string) {
```

And the cleanup call inside it (line 73):
```go
			socket.Cleanup(extensionID)
```

- [ ] **Step 3: Verify the project compiles (expect errors from client.go/mcp.go)**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./cmd/tabb/ 2>&1 | head -20`
Expected: Compile errors in `client.go` and `mcp.go` because `socket.Dial()` now requires an argument. This is expected — we'll fix those in Task 5.

- [ ] **Step 4: Commit**

```bash
git add cmd/tabb/main.go cmd/tabb/host.go
git commit -m "feat: native host creates per-extension-ID socket files"
```

---

### Task 4: Extension Handshake with Browser Name

The extension sends a handshake message on connect so the native host can store the browser name for use during `tabb setup` and `tabb profiles`.

**Files:**
- Modify: `extension/background.js`
- Modify: `cmd/tabb/host.go`
- Modify: `internal/protocol/messages.go`

- [ ] **Step 1: Add handshake action constant to protocol**

In `internal/protocol/messages.go`, add to the action constants block (after line 47):

```go
	ActionHandshake = "handshake"
```

- [ ] **Step 2: Add handshake message to extension**

In `extension/background.js`, update the `connect()` function. After the line `console.log("tabb: connected to native host");` (line 25), add:

```javascript
  // Send handshake with browser info
  const brands = navigator.userAgentData?.brands || [];
  const browser = brands.find(b =>
    ["Google Chrome", "Brave", "Microsoft Edge", "Opera", "Vivaldi"].includes(b.brand)
  );
  port.postMessage({
    action: "handshake",
    params: {
      browser: browser?.brand || "Chrome",
      extensionId: chrome.runtime.id
    }
  });
```

- [ ] **Step 3: Handle handshake in dispatch (extension side)**

In `extension/background.js`, add a case to the `dispatch()` function (after line 35, the `case "list_tabs":` line):

```javascript
    case "handshake":
      return { id: msg.id, data: { ok: true } };
```

- [ ] **Step 4: Handle handshake in native host**

In `cmd/tabb/host.go`, the handshake arrives as a Native Messaging message on stdin — but it's a `Request` from the extension (with `action: "handshake"`), not a `Response`. Currently `readFromExtension` only expects `Response` objects.

Actually — looking at the flow more carefully: the extension sends the handshake via `port.postMessage()`, and `dispatch()` handles incoming messages from the host. The handshake is sent *to* the host, not dispatched from it. So it arrives on `os.Stdin` in the host.

But `readFromExtension` currently unmarshals everything as `protocol.Response`. The handshake is an unsolicited message (no matching request ID). We need to handle it specially.

Add a `HandshakeInfo` struct and a package-level variable to `host.go`:

```go
type handshakeInfo struct {
	Browser     string `json:"browser"`
	ExtensionID string `json:"extensionId"`
}

var browserName string
```

Modify `readFromExtension` to detect handshake messages. The handshake arrives as a JSON object with `action: "handshake"`. Replace the inner loop body of `readFromExtension` (lines 67-84):

```go
func readFromExtension(pending *pendingRequests, extensionID string) {
	for {
		msg, err := native.ReadMessage(os.Stdin)
		if err != nil {
			log.Printf("extension disconnected: %v", err)
			socket.Cleanup(extensionID)
			os.Exit(0)
		}

		// Check if this is a handshake message
		var raw map[string]any
		if err := json.Unmarshal(msg, &raw); err != nil {
			log.Printf("invalid message from extension: %v", err)
			continue
		}

		if raw["action"] == "handshake" {
			if params, ok := raw["params"].(map[string]any); ok {
				if b, ok := params["browser"].(string); ok {
					browserName = b
					log.Printf("browser: %s", browserName)
				}
			}
			saveBrowserName(extensionID, browserName)
			continue
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
```

Add `saveBrowserName` to persist it so `tabb setup` can read it later:

```go
// saveBrowserName writes the browser name to ~/.tabb/<extensionID>.browser
// so that tabb setup and tabb profiles can read it.
func saveBrowserName(extensionID, browser string) {
	dir, err := socket.Dir()
	if err != nil {
		log.Printf("cannot save browser name: %v", err)
		return
	}
	path := filepath.Join(dir, extensionID+".browser")
	os.WriteFile(path, []byte(browser), 0644)
}
```

Add `"path/filepath"` to the imports in `host.go` if not already present.

- [ ] **Step 5: Build the extension and host to check for errors**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./internal/protocol/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add extension/background.js internal/protocol/messages.go cmd/tabb/host.go
git commit -m "feat: extension sends browser handshake on connect"
```

---

### Task 5: Profile-Aware CLI Client

Update the CLI client to resolve which profile/socket to connect to before dialing.

**Files:**
- Modify: `cmd/tabb/client.go`
- Modify: `cmd/tabb/list.go`
- Modify: `cmd/tabb/show.go`
- Modify: `cmd/tabb/close.go`
- Modify: `cmd/tabb/mcp.go`
- Modify: `cmd/tabb/main.go`

- [ ] **Step 1: Update `client.go` to use profile resolution**

Replace the contents of `cmd/tabb/client.go`:

```go
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
```

- [ ] **Step 2: Add `--profile` flag parsing to `main.go`**

Add a helper function to `main.go` that extracts `--profile` from args and returns the remaining args. Add this before `printUsage()`:

```go
// extractProfileFlag scans args for --profile=<name> or --profile <name>,
// removes it, and returns (profileName, remainingArgs).
func extractProfileFlag(args []string) (string, []string) {
	var profileName string
	var remaining []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--profile" && i+1 < len(args) {
			profileName = args[i+1]
			i++ // skip next
		} else if strings.HasPrefix(arg, "--profile=") {
			profileName = strings.TrimPrefix(arg, "--profile=")
		} else {
			remaining = append(remaining, arg)
		}
	}
	return profileName, remaining
}
```

Update the command routing in `main()`. Replace lines 25-48 (after the `return` from the chrome-extension block):

```go
	profileFlag, cmdArgs := extractProfileFlag(os.Args[1:])
	if len(cmdArgs) == 0 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch cmdArgs[0] {
	case "host":
		err = runHost("")
	case "list", "ls":
		err = runList(cmdArgs[1:], profileFlag)
	case "show":
		err = runShow(cmdArgs[1:], profileFlag)
	case "close":
		err = runClose(cmdArgs[1:], profileFlag)
	case "mcp":
		err = runMCP()
	case "profiles":
		err = runProfiles()
	case "setup":
		err = runSetup()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "tabb: unknown command %q\n\n", cmdArgs[0])
		printUsage()
		os.Exit(1)
	}
```

Remove the old `if len(os.Args) < 2` check at the top since we now check `len(cmdArgs) == 0` after extracting the flag.

Update `printUsage()` to document the new flag and command:

```go
func printUsage() {
	fmt.Print(`tabb — manage Chrome tabs from the terminal

Usage:
  tabb [--profile <name>] list [--json] [filter]   List open tabs
  tabb [--profile <name>] show <tab-id> [--raw]    Show tab content as markdown
  tabb [--profile <name>] close <tab-id>           Close a tab
  tabb profiles                                     List configured profiles
  tabb mcp                                          Run as MCP stdio server
  tabb setup                                        Install Native Messaging host manifest
  tabb help                                         Show this help

Environment:
  TABB_PROFILE   Default profile name (overridden by --profile flag)
`)
}
```

- [ ] **Step 3: Update `list.go` to accept profile parameter**

Change the signature and the `sendRequest` call:

```go
func runList(args []string, profileFlag string) error {
```

Update the `sendRequest` call (line 29):

```go
	resp, err := sendRequest(protocol.ActionListTabs, params, profileFlag)
```

- [ ] **Step 4: Update `show.go` to accept profile parameter**

Change the signature:

```go
func runShow(args []string, profileFlag string) error {
```

Update the `sendRequest` call (line 38):

```go
	resp, err := sendRequest(protocol.ActionShowTab, params, profileFlag)
```

- [ ] **Step 5: Update `close.go` to accept profile parameter**

Change the signature:

```go
func runClose(args []string, profileFlag string) error {
```

Update the `sendRequest` call (line 24):

```go
	_, err = sendRequest(protocol.ActionCloseTab, params, profileFlag)
```

- [ ] **Step 6: Update `mcp.go` to use profile-aware dial**

Replace `mcpRequest` (lines 64-77):

```go
func mcpRequest(action string, params map[string]any) (*protocol.Response, error) {
	conn, err := resolveAndDial("")
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
```

Remove the `socket` import from `mcp.go` since `resolveAndDial` handles it now. The import block becomes:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/joelhelbling/tabb/internal/protocol"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)
```

- [ ] **Step 7: Build to verify compilation**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./cmd/tabb/`
Expected: Compile error — `runProfiles` not yet defined. That's fine, we'll create it in Task 6.

- [ ] **Step 8: Commit**

```bash
git add cmd/tabb/client.go cmd/tabb/main.go cmd/tabb/list.go cmd/tabb/show.go cmd/tabb/close.go cmd/tabb/mcp.go
git commit -m "feat: CLI resolves profile via --profile flag, TABB_PROFILE env, or auto-detect"
```

---

### Task 6: `tabb profiles` Command

Implements the `tabb profiles` command that lists all configured and active profiles.

**Files:**
- Create: `cmd/tabb/profiles.go`

- [ ] **Step 1: Implement `runProfiles()`**

Create `cmd/tabb/profiles.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/joelhelbling/tabb/internal/profile"
	"github.com/joelhelbling/tabb/internal/socket"
)

func runProfiles() error {
	tabbDir, err := socket.Dir()
	if err != nil {
		return err
	}
	profilesPath := profile.ProfilesPath(tabbDir)

	profiles, err := profile.Load(profilesPath)
	if err != nil {
		return err
	}

	activeSockets, err := profile.ActiveSockets(tabbDir)
	if err != nil {
		return err
	}

	activeSet := make(map[string]bool)
	for _, id := range activeSockets {
		activeSet[id] = true
	}

	if len(profiles) == 0 && len(activeSockets) == 0 {
		fmt.Println("No profiles configured. Run 'tabb setup' to add one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tBROWSER\tSTATUS\tEXTENSION ID")

	// Show named profiles
	for name, extID := range profiles {
		status := "inactive"
		if activeSet[extID] {
			status = "active"
			delete(activeSet, extID)
		}
		browser := readBrowserName(tabbDir, extID)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, browser, status, extID)
	}

	// Show unnamed active sockets
	for id := range activeSet {
		browser := readBrowserName(tabbDir, id)
		fmt.Fprintf(w, "(unnamed)\t%s\tactive\t%s\n", browser, id)
	}

	w.Flush()
	return nil
}

// readBrowserName reads the .browser file for a given extension ID.
func readBrowserName(tabbDir, extensionID string) string {
	data, err := os.ReadFile(filepath.Join(tabbDir, extensionID+".browser"))
	if err != nil {
		return "—"
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "—"
	}
	return name
}
```

- [ ] **Step 2: Build and verify compilation**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./cmd/tabb/`
Expected: PASS — full project should now compile

- [ ] **Step 3: Commit**

```bash
git add cmd/tabb/profiles.go
git commit -m "feat: add 'tabb profiles' command to list configured profiles"
```

---

### Task 7: Update `tabb setup` for Multi-Profile Flow

Rework setup to prompt for a profile name, accumulate extension IDs in `allowed_origins`, and write to `profiles.json`.

**Files:**
- Modify: `cmd/tabb/setup.go`

- [ ] **Step 1: Rewrite `runSetup()` for multi-profile support**

Replace the entire `runSetup()` function in `setup.go`:

```go
func runSetup() error {
	// Find the tabb binary path
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

	profilesPath := filepath.Join(tabbDir, "profiles.json")
	profiles, err := loadProfiles(profilesPath)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	// Guide user through extension setup
	fmt.Println("Load the extension in your browser:")
	fmt.Println("  1. Open chrome://extensions (or equivalent)")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println()

	fmt.Print("Paste the extension ID and press Enter: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	extensionID := strings.TrimSpace(input)
	if extensionID == "" {
		fmt.Println("\nNo extension ID entered. You can re-run 'tabb setup' later.")
		return nil
	}

	// Check if this extension ID is already registered
	if existingName, found := findProfileByExtID(profiles, extensionID); found {
		fmt.Printf("\nExtension ID already registered as profile %q.\n", existingName)
		fmt.Println("Reload the extension to connect.")
		return nil
	}

	// Suggest a profile name
	defaultName := suggestProfileName(tabbDir, extensionID, profiles)
	if defaultName != "" {
		fmt.Printf("Profile name [%s]: ", defaultName)
	} else {
		fmt.Print("Profile name: ")
	}
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

	// Check for name conflicts (case-insensitive)
	if !isNameAvailable(profiles, profileName) {
		return fmt.Errorf("profile name %q is already in use (case-insensitive)", profileName)
	}

	// Save profile
	profiles[profileName] = extensionID
	if err := saveProfiles(profilesPath, profiles); err != nil {
		return err
	}

	// Write manifest with ALL registered extension IDs
	manifestPath := filepath.Join(manifestDir, "com.tabb.json")
	if err := writeManifest(manifestPath, binaryPath, allExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Printf("\nProfile %q registered with extension ID: %s\n", profileName, extensionID)
	fmt.Printf("Manifest updated: %s\n", manifestPath)
	fmt.Println("Reload the extension in your browser to establish the connection.")

	return nil
}
```

- [ ] **Step 2: Update `writeManifest` to accept multiple extension IDs**

Replace the `writeManifest` function:

```go
func writeManifest(path, binaryPath string, extensionIDs []string) error {
	origins := make([]string, len(extensionIDs))
	for i, id := range extensionIDs {
		origins[i] = fmt.Sprintf("chrome-extension://%s/", id)
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
```

- [ ] **Step 3: Add helper functions for profile management in setup**

Add these helpers to `setup.go`:

```go
func tabbHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".tabb"), nil
}

func loadProfiles(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	return m, nil
}

func saveProfiles(path string, profiles map[string]string) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profiles: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func findProfileByExtID(profiles map[string]string, extID string) (string, bool) {
	for name, id := range profiles {
		if id == extID {
			return name, true
		}
	}
	return "", false
}

func isNameAvailable(profiles map[string]string, name string) bool {
	lower := strings.ToLower(name)
	for k := range profiles {
		if strings.ToLower(k) == lower {
			return false
		}
	}
	return true
}

func allExtensionIDs(profiles map[string]string) []string {
	ids := make([]string, 0, len(profiles))
	for _, id := range profiles {
		ids = append(ids, id)
	}
	return ids
}

// suggestProfileName reads the .browser file (written by the native host
// after a handshake) and suggests a default profile name.
func suggestProfileName(tabbDir, extensionID string, profiles map[string]string) string {
	// Try reading browser name from handshake file
	data, err := os.ReadFile(filepath.Join(tabbDir, extensionID+".browser"))
	browserName := ""
	if err == nil {
		browserName = strings.TrimSpace(string(data))
	}

	// If first profile ever, default to "Default"
	if len(profiles) == 0 {
		if browserName != "" {
			return browserName
		}
		return "Default"
	}

	// Try browser name if available and not taken
	if browserName != "" && isNameAvailable(profiles, browserName) {
		return browserName
	}

	// Try browser name with incrementing suffix
	if browserName != "" {
		for i := 2; i <= 99; i++ {
			candidate := fmt.Sprintf("%s-%d", browserName, i)
			if isNameAvailable(profiles, candidate) {
				return candidate
			}
		}
	}

	// No good default — user must choose
	return ""
}
```

- [ ] **Step 4: Build and verify compilation**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./cmd/tabb/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/tabb/setup.go
git commit -m "feat: tabb setup supports multi-profile with name prompting and manifest accumulation"
```

---

### Task 8: Migrate Existing Single-Socket Installations

Users who already have tabb set up with the old `tabb.sock` need a migration path. When the CLI encounters `tabb.sock` (the old name), it should suggest running `tabb setup` to migrate.

**Files:**
- Modify: `internal/profile/profile.go`

- [ ] **Step 1: Write test for legacy socket detection**

Add to `internal/profile/profile_test.go`:

```go
func TestResolveLegacySocket(t *testing.T) {
	dir := t.TempDir()
	// Create the old-style tabb.sock
	os.WriteFile(filepath.Join(dir, "tabb.sock"), []byte{}, 0600)

	_, err := Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err == nil {
		t.Fatal("expected error for legacy socket")
	}
	if !strings.Contains(err.Error(), "tabb setup") {
		t.Errorf("expected migration hint in error, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/ -run TestResolveLegacySocket`
Expected: FAIL — currently `tabb.sock` would be treated as extension ID "tabb", which is wrong but wouldn't trigger the expected error message.

- [ ] **Step 3: Add legacy socket detection to `ActiveSockets`**

In `internal/profile/profile.go`, update `ActiveSockets` to skip `tabb.sock`, and add a `HasLegacySocket` function:

```go
// ActiveSockets returns extension IDs for all .sock files in tabbDir,
// excluding the legacy tabb.sock.
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
```

Add `"path/filepath"` to the imports in `profile.go`.

Update `Resolve` to check for the legacy socket. In the `case 0:` branch, change:

```go
	case 0:
		if HasLegacySocket(tabbDir) {
			return "", fmt.Errorf("found legacy tabb.sock — run 'tabb setup' to migrate to the new multi-profile format")
		}
		return "", fmt.Errorf("no active tabb sockets found (is Chrome running with the tabb extension?)")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./internal/profile/ -v`
Expected: PASS — all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat: detect legacy tabb.sock and guide user to run tabb setup"
```

---

### Task 9: Update CLAUDE.md and Help Text

Update project documentation and help text to reflect multi-profile support.

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update Commands section in CLAUDE.md**

In `CLAUDE.md`, replace the Commands section:

```markdown
## Commands

- `tabb [--profile <name>] list` — list tab metadata
- `tabb [--profile <name>] show <tab-id>` — page content as markdown (Readability mode), `--raw` for full DOM
- `tabb [--profile <name>] close <tab-id>` — close a tab
- `tabb profiles` — list configured profiles and their status
- `tabb mcp` — run as MCP stdio server
- `tabb setup` — install Native Messaging host manifest and register a profile
```

- [ ] **Step 2: Add Profiles section to CLAUDE.md**

After the Commands section, add:

```markdown
## Profiles

tabb supports multiple browser profiles and Chrome-based browsers. Each extension installation
gets its own named profile. Profile data is stored in `~/.tabb/profiles.json`.

- Socket files: `~/.tabb/<extensionId>.sock`
- Browser info: `~/.tabb/<extensionId>.browser` (written by native host on handshake)
- Profile resolution: `--profile` flag > `TABB_PROFILE` env var > auto-detect (single socket)
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with multi-profile support details"
```

---

### Task 10: End-to-End Verification

Verify the full system works with a single profile (the common case), ensuring no regressions.

- [ ] **Step 1: Build the binary**

Run: `cd /Users/joelhelbling/code/ai/tabb && go build ./cmd/tabb/`
Expected: PASS — clean build, no errors

- [ ] **Step 2: Run all tests**

Run: `cd /Users/joelhelbling/code/ai/tabb && go test ./... -v`
Expected: PASS — all profile tests pass

- [ ] **Step 3: Verify `tabb help` shows updated usage**

Run: `./tabb help`
Expected: Shows `--profile` flag, `profiles` command, and `TABB_PROFILE` env var

- [ ] **Step 4: Verify `tabb profiles` works with no profiles**

Run: `./tabb profiles`
Expected: "No profiles configured. Run 'tabb setup' to add one."

- [ ] **Step 5: Clean up build artifact**

Run: `rm -f ./tabb`

- [ ] **Step 6: Commit any final fixes if needed**
