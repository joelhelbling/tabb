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

### 2. Set up Native Messaging

```bash
tabb setup
```

This writes a Native Messaging manifest to your Chrome config directory and prints next steps.

### 3. Load the extension

1. Open `chrome://extensions` in Chrome
2. Enable **Developer mode** (toggle in top right)
3. Click **Load unpacked** and select the `extension/` directory from this repo
4. Copy the **extension ID** shown on the extensions page
5. Edit the Native Messaging manifest (path shown by `tabb setup`) and replace `EXTENSION_ID_HERE` with your actual extension ID
6. Click the reload button on the extension card

The extension will connect to the native host, and the Unix socket will be created at `~/.tabb/tabb.sock`.

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
```

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

This exposes three tools to Claude:
- **list_tabs** — list open tabs with optional filter
- **show_tab** — get tab content as markdown
- **close_tab** — close a tab

### Claude Code Plugin

If you install tabb as a Claude Code plugin, the MCP server is configured automatically:

```
/plugin marketplace add joelhelbling/tabb
/plugin install tabb
```

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

- **Unix socket** (`~/.tabb/tabb.sock`) has mode `0600` — only your user can connect. Same trust model as `~/.ssh`.
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

## License

MIT
