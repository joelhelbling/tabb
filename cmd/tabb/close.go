package main

import (
	"fmt"
	"strconv"

	"github.com/joelhelbling/tabb/internal/protocol"
)

func runClose(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tabb close <tab-id>")
	}

	tabID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tab ID %q: must be a number", args[0])
	}

	params := map[string]any{
		"tabId": tabID,
	}

	_, err = sendRequest(protocol.ActionCloseTab, params)
	if err != nil {
		return err
	}

	fmt.Printf("Closed tab %d\n", tabID)
	return nil
}
