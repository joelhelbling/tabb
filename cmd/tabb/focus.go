package main

import (
	"fmt"
	"strconv"

	"github.com/joelhelbling/tabb/internal/protocol"
)

func runFocus(args []string, profileFlag string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tabb focus <tab-id> [--reload]")
	}

	tabID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tab ID: %s", args[0])
	}

	reload := false
	for _, arg := range args[1:] {
		if arg == "--reload" {
			reload = true
		}
	}

	params := map[string]any{
		"tabId":  tabID,
		"reload": reload,
	}

	resp, err := sendRequest(protocol.ActionFocusTab, params, profileFlag)
	if err != nil {
		return err
	}

	if reload {
		fmt.Printf("Focused and reloaded tab %d\n", tabID)
	} else {
		fmt.Printf("Focused tab %d\n", tabID)
	}
	_ = resp
	return nil
}
