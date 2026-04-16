package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/joelhelbling/tabb/internal/profile"
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
		mcp.NewTool("list_profiles",
			mcp.WithDescription("List all tabb browser profiles the user has registered. Call this when the user refers to a specific profile (e.g. 'my Brave tabs', 'my work browser') or when another tool returns a 'multiple profiles' error. Returns each profile's name, browser, profileId, and whether it currently has a live socket (active=true means Chrome is running with the tabb extension loaded for that profile)."),
		),
		handleListProfiles,
	)

	s.AddTool(
		mcp.NewTool("list_tabs",
			mcp.WithDescription("List all open Chrome browser tabs. Returns metadata for each tab including ID, title, URL, and status. Use the filter parameter to search by title or URL."),
			mcp.WithString("filter",
				mcp.Description("Optional text to filter tabs by title or URL"),
			),
			mcp.WithString("profile",
				mcp.Description("Optional profile name (case-insensitive) to target a specific browser profile. If omitted, tabb auto-detects when exactly one profile is active. Call list_profiles first if you need to know what's available."),
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
			mcp.WithString("profile",
				mcp.Description("Optional profile name (case-insensitive) to target a specific browser profile. If omitted, tabb auto-detects when exactly one profile is active. Call list_profiles first if you need to know what's available."),
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
			mcp.WithString("profile",
				mcp.Description("Optional profile name (case-insensitive) to target a specific browser profile. If omitted, tabb auto-detects when exactly one profile is active. Call list_profiles first if you need to know what's available."),
			),
		),
		handleCloseTab,
	)

	s.AddTool(
		mcp.NewTool("focus_tab",
			mcp.WithDescription("Bring a Chrome tab to the foreground (activate it and focus its window). Optionally reload the tab. Useful when the user wants to find a tab or retry a failed show_tab."),
			mcp.WithNumber("tab_id",
				mcp.Required(),
				mcp.Description("The tab ID (from list_tabs results)"),
			),
			mcp.WithBoolean("reload",
				mcp.Description("If true, also reload the tab after focusing it"),
			),
			mcp.WithString("profile",
				mcp.Description("Optional profile name (case-insensitive) to target a specific browser profile. If omitted, tabb auto-detects when exactly one profile is active. Call list_profiles first if you need to know what's available."),
			),
		),
		handleFocusTab,
	)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "tabb mcp server error: %v\n", err)
		return err
	}
	return nil
}

func mcpRequest(action string, params map[string]any, profile string) (*protocol.Response, error) {
	conn, err := resolveAndDial(profile)
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

func handleListProfiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tabbDir, err := socket.Dir()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to locate tabb directory: %v", err)), nil
	}

	profiles, err := profile.Load(profile.ProfilesPath(tabbDir))
	if err != nil {
		if errors.Is(err, profile.ErrLegacySchema) {
			return mcp.NewToolResultError("profiles.json is in legacy format; please run 'tabb setup' to migrate."), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Failed to load profiles: %v", err)), nil
	}

	activeIDs, err := profile.ActiveSockets(tabbDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read active sockets: %v", err)), nil
	}
	activeSet := make(map[string]bool, len(activeIDs))
	for _, id := range activeIDs {
		activeSet[id] = true
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	type profileOut struct {
		Name      string `json:"name"`
		Browser   string `json:"browser"`
		ProfileID string `json:"profileId"`
		Active    bool   `json:"active"`
	}
	out := make([]profileOut, 0, len(names))
	for _, name := range names {
		e := profiles[name]
		out = append(out, profileOut{
			Name:      name,
			Browser:   e.Browser,
			ProfileID: e.ProfileID,
			Active:    activeSet[e.ProfileID],
		})
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func handleListTabs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params := map[string]any{}
	if filter := request.GetString("filter", ""); filter != "" {
		params["filter"] = filter
	}
	profileName := request.GetString("profile", "")

	resp, err := mcpRequest(protocol.ActionListTabs, params, profileName)
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
	profileName := request.GetString("profile", "")

	params := map[string]any{
		"tabId": int(tabID),
		"raw":   raw,
	}

	resp, err := mcpRequest(protocol.ActionShowTab, params, profileName)
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

func handleFocusTab(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tabID, err := request.RequireFloat("tab_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	prof := request.GetString("profile", "")
	reload := request.GetBool("reload", false)

	params := map[string]any{
		"tabId":  int(tabID),
		"reload": reload,
	}

	_, err = mcpRequest(protocol.ActionFocusTab, params, prof)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to focus tab: %v", err)), nil
	}

	result := fmt.Sprintf("Focused tab %d", int(tabID))
	if reload {
		result += " (reloaded)"
	}
	return mcp.NewToolResultText(result), nil
}

func handleCloseTab(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tabID, err := request.RequireFloat("tab_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	profileName := request.GetString("profile", "")
	params := map[string]any{
		"tabId": int(tabID),
	}

	_, err = mcpRequest(protocol.ActionCloseTab, params, profileName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to close tab: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Closed tab %d", int(tabID))), nil
}
