# tabb

Manage Chrome browser tabs from the terminal or an AI assistant.

`tabb` exposes your browser tabs as a unix-style CLI and MCP server. List tabs, read their content as markdown, and close them — from your terminal or from Claude Code.

## Architecture

```
┌──────────────┐  Native Messaging  ┌───────────────┐  Unix socket  ┌───────────┐
│   Chrome     │──(stdin/stdout)───→│    tabb       │←──────────────│ CLI / MCP │
│  Extension   │                    │  (Go binary)  │               │  clients  │
└──────────────┘                    └───────────────┘               └───────────┘
```

A thin Chrome extension talks to a Go binary via Chrome's Native Messaging protocol. The binary also listens on a Unix domain socket so CLI commands and MCP clients can reach your tabs. No daemon, no open ports — the binary runs only while Chrome is open.

## Install

### 1. Build the binary

```bash
go install github.com/joelhelbling/tabb/cmd/tabb@latest
```

Or build from source:

```bash
git clone https://github.com/joelhelbling/tabb.git
cd tabb
go build -o tabb ./cmd/tabb
# Move to somewhere on your PATH:
mv tabb /usr/local/bin/
```

### 2. Load the extension

1. Open `chrome://extensions` in Chrome
2. Enable **Developer mode** (toggle in top right)
3. Click **Load unpacked** and select the `extension/` directory from this repo
4. Note the **extension ID** shown on the extensions page

### 3. Set up Native Messaging

```bash
tabb setup
```

This writes a Native Messaging manifest to your Chrome config directory. It will prompt you to paste the extension ID from step 2 and ask you to name this profile (defaults to the browser name or "Default"). The extension ID and profile name are saved to `~/.tabb/profiles.json`.

Once setup is complete, reload the extension in Chrome. It will connect to the native host and a Unix socket will be created at `~/.tabb/<extensionId>.sock`.

#### Multiple browsers / profiles

Run `tabb setup` once per browser or Chrome profile. Each gets its own named profile and socket. All extension IDs are accumulated in the Native Messaging manifest's `allowed_origins`.

```bash
tabb setup   # first profile → "Default" (or browser name)
tabb setup   # second profile → prompted for a name, e.g. "Brave"
```

## Usage

### CLI

```bash
# List all tabs
tabb list

# List tabs matching a filter
tabb list react

# JSON output
tabb list --json

# Show tab content as markdown (with YAML frontmatter)
tabb show 12345

# Show raw DOM as markdown (no Readability extraction)
tabb show 12345 --raw

# Close a tab
tabb close 12345

# Focus a tab (bring to foreground)
tabb focus 12345

# Focus and reload a tab
tabb focus 12345 --reload

# List configured profiles
tabb profiles
```

#### Targeting a specific profile

If you have multiple profiles, tabb auto-detects when only one is active. With multiple active profiles, specify which one:

```bash
# Via flag
tabb --profile Brave list

# Via environment variable
TABB_PROFILE=Brave tabb list
```

The `--profile` flag takes precedence over `TABB_PROFILE`.

### MCP Server (for Claude Code)

Add to your Claude Code MCP config (`~/.claude/settings.json` or project `.mcp.json`):

```json
{
  "mcpServers": {
    "tabb": {
      "command": "tabb",
      "args": ["mcp"]
    }
  }
}
```

This exposes tools to Claude:
- **list_tabs** — list open tabs with optional filter
- **show_tab** — get tab content as markdown
- **focus_tab** — bring a tab to the foreground, optionally reload it
- **close_tab** — close a tab
- **list_profiles** — list registered browser profiles and their status

### Claude Code Plugin

This repo is a Claude Code plugin marketplace. Install the plugin and the MCP server is wired up automatically:

```
/plugin marketplace add joelhelbling/tabb
/plugin install tabb
```

**Prerequisite**: the `tabb` binary must already be on your `$PATH` (install it via `go install`, `make install`, or the steps above). The plugin configures Claude Code to run `tabb mcp`; it does not install the binary itself. You also still need the Chrome extension loaded and `tabb setup` completed, same as for CLI use.

The plugin also ships a skill that teaches Claude Code to reach for tabb whenever you mention the browser tabs you currently have open.

## Tabignore

Tabignore lets you hide sensitive tabs from tabb. Ignored tabs are filtered **in the extension** before any data reaches the Unix socket or CLI.

### Adding patterns

- **Right-click any page** → "Add to tabignore" (adds the domain)
- **Extension popup** → manage patterns with full control over type

### Pattern types

| Type | Example | Matches |
|------|---------|---------|
| Domain | `bank.com` | All URLs on bank.com and subdomains |
| Domain + path | `google.com/mail` | Gmail specifically |
| Full URL | `https://example.com/secret?id=123` | That exact URL |
| Regex | `bank\|finance\|trading` | URLs matching the regex |

## Security

- **Unix sockets** (`~/.tabb/<profileId>.sock`) have mode `0600` — only your user can connect. Same trust model as `~/.ssh`.
- **Tabignore filtering** happens in the extension before data leaves Chrome. Ignored tab URLs and content never reach the socket.
- **No network exposure** — the binary uses Native Messaging (stdin/stdout) and a Unix domain socket. No TCP ports are opened.
- **Extension permissions**: `tabs` (list tabs), `scripting` (read page content), `nativeMessaging`, `contextMenus`, `storage`. The `scripting` permission is required for content extraction and is only used by your local extension code.

## Development

```bash
# Build
go build ./cmd/tabb

# Run the native host directly (for debugging)
tabb host

# The extension auto-reconnects every 5 seconds if the host isn't running
```

## Changelog

### 2026-04-15

- **Profile-ID handshake** — Fixed a bug where Chromium browsers that reuse the same extension ID across profiles (notably Vivaldi) would collide on the same Unix socket. The extension now generates a per-profile UUID stored in `chrome.storage.local` and sends it in the handshake. Sockets are keyed on `profileId` instead of `extensionId`. `profiles.json` upgraded to a structured schema; legacy format is detected and migrated on next `tabb setup`.
- **Plugin marketplace layout** — Moved Claude Code plugin assets into `plugins/tabb/` so `/plugin install tabb` no longer copies Go source, Makefile, or extension code into users' plugin caches. Added `.claude-plugin/marketplace.json` so the repo can be added as a marketplace.
- **tabb skill** — Added a discoverability skill (`plugins/tabb/skills/tabb/SKILL.md`) that teaches Claude Code to reach for the tabb MCP server when the user mentions their open browser tabs.
- **MCP profile support** — Added a `list_profiles` tool and an optional `profile` parameter on `list_tabs`, `show_tab`, and `close_tab` so the AI can target a specific browser profile.
- **Setup flow** — `tabb setup` now detects new profiles by reload-and-scan instead of requiring the user to paste an extension ID.

## License

MIT
