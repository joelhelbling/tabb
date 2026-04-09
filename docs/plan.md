# tabb: Implementation Plan

## Context

Joel has many browser tabs open and wants an agentic assistant (Claude Code) to help manage them — listing tabs, reading their content, and closing them — without losing important threads. The tool should work as both a CLI and an MCP server, following unix-tool composability principles. Privacy is a priority: a "tabignore" system ensures sensitive tabs never leave the browser.

## Architecture

```
┌──────────────┐  Native Messaging  ┌──────────────┐  Unix socket    ┌───────────┐
│   Chrome     │──(stdin/stdout)───→│    tabb       │←───────────────│ CLI / MCP │
│  Extension   │                    │  (Go binary)  │                │  clients  │
└──────────────┘                    └──────────────┘                └───────────┘
```

- **Extension** handles Chrome APIs (tabs, scripting, bookmarks) and tabignore filtering
- **Go binary** is the Native Messaging host + Unix socket server
- **CLI commands** (`list`, `show`, `close`) connect to the Unix socket
- **MCP mode** (`tabb mcp`) is a stdio MCP server that bridges to the socket

## Phase 1: Project Scaffolding

1. Initialize Go module (`github.com/joelhelbling/tabb`)
2. Set up directory structure:
   ```
   /
   ├── .claude-plugin/
   │   └── plugin.json      # Claude Code plugin manifest
   ├── .mcp.json             # MCP server auto-config for plugin users
   ├── cmd/tabb/             # Go CLI entrypoint
   ├── internal/
   │   ├── native/           # Native Messaging protocol (stdin/stdout, length-prefixed JSON)
   │   ├── socket/           # Unix domain socket server
   │   ├── mcp/              # MCP protocol bridge (stdio ↔ socket)
   │   └── protocol/         # Shared message types between extension and binary
   ├── extension/
   │   ├── manifest.json     # Manifest V3
   │   ├── background.js     # Service worker: native messaging, tab queries, tabignore
   │   ├── content.js        # Content script for DOM extraction
   │   ├── popup.html/js     # Settings UI for tabignore
   │   └── readability.js    # Mozilla Readability (vendored or bundled)
   ├── skills/
   │   └── tabb/
   │       └── SKILL.md      # Claude Code skill (e.g. /tabb:triage)
   ├── go.mod
   └── README.md
   ```
3. Create CLAUDE.md with project conventions
4. Create `.claude-plugin/plugin.json` and `.mcp.json` so the repo is installable as a Claude Code plugin from day one

## Phase 2: Chrome Extension (Minimal)

1. **manifest.json** — Manifest V3 with permissions: `tabs`, `scripting`, `nativeMessaging`, `contextMenus`, `storage`
2. **background.js (service worker)**:
   - On extension startup, connect to native host via `chrome.runtime.connectNative('com.tabb')`
   - Listen for messages from native host, dispatch to Chrome APIs
   - Message types: `list_tabs`, `show_tab`, `close_tab`
   - `list_tabs`: call `chrome.tabs.query({})`, filter through tabignore, return metadata
   - `show_tab`: call `chrome.scripting.executeScript()` to extract DOM, run through Readability, return markdown
   - `close_tab`: call `chrome.tabs.remove(tabId)`
   - Tabignore: load patterns from `chrome.storage.local`, filter tabs before responding
3. **Tabignore UI**:
   - Context menu: right-click tab → "Add to tabignore" → popup with pattern type selection (domain / domain+path / full URL / regex)
   - Extension popup: settings page to view/add/edit/remove tabignore patterns
4. **Content extraction**:
   - Bundle Mozilla Readability for article extraction
   - Use Turndown (or similar) for HTML→Markdown conversion
   - `--raw` mode: skip Readability, convert full DOM

## Phase 3: Go Binary — Native Messaging Host

1. **Native Messaging protocol**: Read/write length-prefixed JSON on stdin/stdout per Chrome's spec
2. **Unix socket server**: Create `~/.tabb/tabb.sock` (mode 0600) on startup, remove on shutdown
3. **Message routing**: CLI request arrives on socket → binary forwards to extension via stdin/stdout → extension responds → binary forwards back to socket client
4. **Lifecycle**: Binary starts when extension connects, exits when extension disconnects (Chrome closes). Clean up socket file on exit.

## Phase 4: CLI Commands

1. **`tabb list`** — Connect to socket, send `list_tabs` request, print tab metadata as table or JSON
   - Flags: `--json` for machine-readable output, filter by title/URL pattern
2. **`tabb show <tab-id>`** — Send `show_tab` request, print markdown with YAML frontmatter (title, URL, tab status)
   - Flags: `--raw` for full DOM instead of Readability
3. **`tabb close <tab-id>`** — Send `close_tab` request, confirm
4. **`tabb setup`** — Write Native Messaging host manifest to the correct OS path, print instructions for loading the extension

## Phase 5: MCP Server

1. **`tabb mcp`** — Stdio MCP server (for Claude Code's `mcpServers` config)
2. Connects to Unix socket internally
3. Exposes three tools: `list_tabs`, `show_tab`, `close_tab`
4. Tool schemas with descriptions optimized for LLM understanding

## Phase 6: Claude Code Plugin & Skills

1. **`.claude-plugin/plugin.json`** — Plugin manifest with name, version, description, author
2. **`.mcp.json`** — Pre-configured MCP server entry so plugin users get tools automatically:
   ```json
   { "tabb": { "command": "tabb", "args": ["mcp"] } }
   ```
3. **`skills/tabb/SKILL.md`** — Initial skill (deferred until common workflows emerge from using MCP tools)

## Phase 7: Install & Documentation

1. **`tabb setup`** command:
   - Writes `com.tabb.json` manifest to `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`
   - Prints sideloading instructions for the extension
2. **README.md**: Install, setup, sideloading instructions, CLI usage, MCP config, plugin install, security notes

## Verification

1. `go build ./cmd/tabb` compiles successfully
2. Load extension unpacked in Chrome → extension connects to native host → socket created
3. `tabb list` returns tab metadata
4. `tabb show <id>` returns markdown content with YAML frontmatter
5. `tabb close <id>` closes the tab
6. Right-click a tab → "Add to tabignore" → tab no longer appears in `list`
7. Add `tabb mcp` to Claude Code's MCP config → Claude can list and read tabs
8. Install as Claude Code plugin (`/plugin install tabb`) → MCP tools auto-configured
