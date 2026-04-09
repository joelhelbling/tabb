# tabb

A CLI and MCP server for managing Chrome browser tabs from the terminal or an AI assistant.

## Architecture

Chrome Extension → Native Messaging (stdin/stdout) → Go binary → Unix domain socket ← CLI / MCP clients

- **Extension** (`extension/`): Manifest V3 Chrome extension. Handles Chrome APIs, tabignore filtering. Thin — proxies requests to/from native host.
- **Go binary** (`cmd/tabb/`): Native Messaging host + Unix socket server. Routes messages between extension and CLI/MCP clients.
- **CLI commands**: `list`, `show`, `close` — connect to Unix socket.
- **MCP server**: `tabb mcp` — stdio MCP server bridging to Unix socket.

## Project Structure

```
.claude-plugin/
  plugin.json        # Claude Code plugin manifest
.mcp.json            # MCP server auto-configuration for plugin users
cmd/tabb/            # Go CLI entrypoint
internal/
  native/            # Native Messaging protocol (length-prefixed JSON)
  socket/            # Unix domain socket server
  mcp/               # MCP protocol bridge
  protocol/          # Shared message types
extension/
  manifest.json      # Manifest V3
  background.js      # Service worker
  lib/               # Vendored libraries (Readability, Turndown)
skills/
  tabb/
    SKILL.md         # Claude Code skill (slash command), e.g. /tabb:triage
```

## Conventions

- Go code uses standard library where possible
- Extension is vanilla JS (no build step)
- Unix socket path: `~/.tabb/tabb.sock`
- Native Messaging host name: `com.tabb`
- CLI output: human-readable by default, `--json` flag for machine-readable
- `show` outputs markdown with YAML frontmatter

## Commands

- `tabb list` — list tab metadata
- `tabb show <tab-id>` — page content as markdown (Readability mode), `--raw` for full DOM
- `tabb close <tab-id>` — close a tab
- `tabb mcp` — run as MCP stdio server
- `tabb setup` — install Native Messaging host manifest

## Claude Code Plugin

This repo is installable as a Claude Code plugin. The plugin provides:
- MCP tools (`list_tabs`, `show_tab`, `close_tab`) via `.mcp.json`
- Skills (slash commands) via `skills/` directory
- Plugin manifest in `.claude-plugin/plugin.json`

## Development

- Load extension unpacked from `extension/` directory in Chrome
- Run `go build ./cmd/tabb` to build the binary
- Socket file is created when the extension connects and launches the native host
