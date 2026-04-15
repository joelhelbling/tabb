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
  profile/           # Profile management (profiles.json, resolution)
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
- Unix socket path: `~/.tabb/<extensionId>.sock` (one per browser/profile)
- Native Messaging host name: `com.tabb`
- CLI output: human-readable by default, `--json` flag for machine-readable
- `show` outputs markdown with YAML frontmatter

## Commands

- `tabb [--profile <name>] list` — list tab metadata
- `tabb [--profile <name>] show <tab-id>` — page content as markdown (Readability mode), `--raw` for full DOM
- `tabb [--profile <name>] close <tab-id>` — close a tab
- `tabb profiles` — list configured profiles and their status
- `tabb mcp` — run as MCP stdio server
- `tabb setup` — install Native Messaging host manifest and register a profile

## Profiles

tabb supports multiple browser profiles and Chrome-based browsers. Each extension installation
gets its own named profile. Profile data is stored in `~/.tabb/profiles.json`.

- Socket files: `~/.tabb/<extensionId>.sock`
- Browser info: `~/.tabb/<extensionId>.browser` (written by native host on handshake)
- Profile resolution: `--profile` flag > `TABB_PROFILE` env var > auto-detect (single socket)

## Claude Code Plugin

This repo is installable as a Claude Code plugin. The plugin provides:
- MCP tools (`list_tabs`, `show_tab`, `close_tab`) via `.mcp.json`
- Skills (slash commands) via `skills/` directory
- Plugin manifest in `.claude-plugin/plugin.json`

## Development

- Load extension unpacked from `extension/` directory in Chrome
- Run `go build ./cmd/tabb` to build the binary
- Socket file is created when the extension connects and launches the native host
