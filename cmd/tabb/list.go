package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/joelhelbling/tabb/internal/protocol"
)

func runList(args []string, profileFlag string) error {
	asJSON := false
	var filter string

	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			filter = arg
		}
	}

	params := map[string]any{}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := sendRequest(protocol.ActionListTabs, params, profileFlag)
	if err != nil {
		return err
	}

	// Parse the tab list from the response data
	data, err := json.Marshal(resp.Data)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	var tabs []protocol.Tab
	if err := json.Unmarshal(data, &tabs); err != nil {
		return fmt.Errorf("parsing tab list: %w", err)
	}

	if asJSON {
		out, _ := json.MarshalIndent(tabs, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// Human-readable table output
	if len(tabs) == 0 {
		fmt.Println("No tabs found.")
		return nil
	}

	for _, t := range tabs {
		status := ""
		if t.Active {
			status = " *"
		} else if t.Discarded {
			status = " (discarded)"
		} else if t.Pinned {
			status = " (pinned)"
		}

		title := t.Title
		if len(title) > 60 {
			title = title[:57] + "..."
		}

		fmt.Printf("%d\t%s%s\n\t%s\n", t.ID, title, status, truncateURL(t.URL))
	}

	return nil
}

func truncateURL(u string) string {
	// Strip common prefixes for readability
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if len(u) > 80 {
		return u[:77] + "..."
	}
	return u
}
