package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/joelhelbling/tabb/internal/protocol"
	"github.com/joelhelbling/tabb/internal/socket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func runMCP() error {
	s := server.NewMCPServer(
		"tabb",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool("list_tabs",
			mcp.WithDescription("List all open Chrome browser tabs. Returns metadata for each tab including ID, title, URL, and status. Use the filter parameter to search by title or URL."),
			mcp.WithString("filter",
				mcp.Description("Optional text to filter tabs by title or URL"),
			),
		),
		handleListTabs,
	)

	s.AddTool(
		mcp.NewTool("show_tab",
			mcp.WithDescription("Get the content of a Chrome tab as markdown with YAML frontmatter metadata. By default uses Readability mode (extracts article content). Set raw=true for the full page DOM as markdown."),
			mcp.WithNumber("tab_id",
				mcp.Required(),
				mcp.Description("The tab ID (from list_tabs results)"),
			),
			mcp.WithBoolean("raw",
				mcp.Description("If true, return full DOM as markdown instead of Readability-extracted content"),
			),
		),
		handleShowTab,
	)

	s.AddTool(
		mcp.NewTool("close_tab",
			mcp.WithDescription("Close a Chrome browser tab by its ID."),
			mcp.WithNumber("tab_id",
				mcp.Required(),
				mcp.Description("The tab ID to close (from list_tabs results)"),
			),
		),
		handleCloseTab,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "tabb mcp server error: %v\n", err)
		return err
	}
	return nil
}

func mcpRequest(action string, params map[string]any) (*protocol.Response, error) {
	conn, err := socket.Dial()
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

func handleListTabs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := map[string]any{}
	if filter := request.GetString("filter", ""); filter != "" {
		params["filter"] = filter
	}

	resp, err := mcpRequest(protocol.ActionListTabs, params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list tabs: %v", err)), nil
	}

	data, _ := json.MarshalIndent(resp.Data, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleShowTab(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tabID, err := request.RequireFloat("tab_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	raw := request.GetBool("raw", false)

	params := map[string]any{
		"tabId": int(tabID),
		"raw":   raw,
	}

	resp, err := mcpRequest(protocol.ActionShowTab, params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to show tab: %v", err)), nil
	}

	// Parse and format as markdown with YAML frontmatter
	data, _ := json.Marshal(resp.Data)
	var content protocol.TabContent
	if err := json.Unmarshal(data, &content); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse tab content: %v", err)), nil
	}

	result := fmt.Sprintf("---\ntitle: %q\nurl: %s\ntab_id: %d\nstatus: %s\nactive: %t\npinned: %t\n---\n\n%s",
		content.Title, content.URL, content.ID, content.Status, content.Active, content.Pinned, content.Content)

	return mcp.NewToolResultText(result), nil
}

func handleCloseTab(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tabID, err := request.RequireFloat("tab_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	params := map[string]any{
		"tabId": int(tabID),
	}

	_, err = mcpRequest(protocol.ActionCloseTab, params)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to close tab: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Closed tab %d", int(tabID))), nil
}
