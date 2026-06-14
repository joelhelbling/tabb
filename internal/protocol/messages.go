// Package protocol defines the message types shared between the Chrome extension
// and the Go binary, as well as between socket clients and the binary.
package protocol

// Request is a message sent from a socket client (CLI/MCP) to the native host,
// which forwards it to the Chrome extension.
type Request struct {
	ID     string         `json:"id"`
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// Response is a message sent from the Chrome extension back through the native
// host to the socket client.
type Response struct {
	ID    string `json:"id"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// Tab represents metadata for a single browser tab.
type Tab struct {
	ID         int    `json:"id"`
	WindowID   int    `json:"windowId"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Status     string `json:"status"`
	Active     bool   `json:"active"`
	Pinned     bool   `json:"pinned"`
	Audible    bool   `json:"audible"`
	Discarded  bool   `json:"discarded"`
	FavIconURL string `json:"favIconUrl,omitempty"`
	Index      int    `json:"index"`
}

// TabContent represents the full content of a tab, returned by show_tab.
type TabContent struct {
	Tab
	Content string `json:"content"`
}

// Action constants matching the Chrome extension's message handler.
const (
	ActionListTabs  = "list_tabs"
	ActionShowTab   = "show_tab"
	ActionCloseTab  = "close_tab"
	ActionFocusTab  = "focus_tab"
	ActionHandshake = "handshake"
)
