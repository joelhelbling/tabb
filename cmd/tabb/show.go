package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/joelhelbling/tabb/internal/protocol"
)

func runShow(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tabb show <tab-id> [--raw]")
	}

	raw := false
	var tabIDStr string

	for _, arg := range args {
		switch arg {
		case "--raw":
			raw = true
		default:
			tabIDStr = arg
		}
	}

	tabID, err := strconv.Atoi(tabIDStr)
	if err != nil {
		return fmt.Errorf("invalid tab ID %q: must be a number", tabIDStr)
	}

	params := map[string]any{
		"tabId": tabID,
		"raw":   raw,
	}

	resp, err := sendRequest(protocol.ActionShowTab, params)
	if err != nil {
		return err
	}

	// Parse the tab content from the response
	data, err := json.Marshal(resp.Data)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	var content protocol.TabContent
	if err := json.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("parsing tab content: %w", err)
	}

	// Output as markdown with YAML frontmatter
	fmt.Printf("---\n")
	fmt.Printf("title: %q\n", content.Title)
	fmt.Printf("url: %s\n", content.URL)
	fmt.Printf("tab_id: %d\n", content.ID)
	fmt.Printf("status: %s\n", content.Status)
	fmt.Printf("active: %t\n", content.Active)
	fmt.Printf("pinned: %t\n", content.Pinned)
	fmt.Printf("---\n\n")
	fmt.Print(content.Content)

	return nil
}
