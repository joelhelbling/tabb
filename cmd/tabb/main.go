package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Chrome launches the native host with the extension origin as the first arg
	// (e.g., "chrome-extension://abcdef123456/"). Detect this and run as host.
	if strings.HasPrefix(os.Args[1], "chrome-extension://") {
		extID := strings.TrimPrefix(os.Args[1], "chrome-extension://")
		extID = strings.TrimSuffix(extID, "/")
		if err := runHost(extID); err != nil {
			fmt.Fprintf(os.Stderr, "tabb: %v\n", err)
			os.Exit(1)
		}
		return
	}

	var err error
	switch os.Args[1] {
	case "host":
		err = runHost("")
	case "list", "ls":
		err = runList(os.Args[2:])
	case "show":
		err = runShow(os.Args[2:])
	case "close":
		err = runClose(os.Args[2:])
	case "mcp":
		err = runMCP()
	case "setup":
		err = runSetup()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "tabb: unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "tabb: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`tabb — manage Chrome tabs from the terminal

Usage:
  tabb list [--json] [filter]   List open tabs
  tabb show <tab-id> [--raw]    Show tab content as markdown
  tabb close <tab-id>           Close a tab
  tabb mcp                      Run as MCP stdio server
  tabb setup                    Install Native Messaging host manifest
  tabb host                     Run as Native Messaging host (called by Chrome)
  tabb help                     Show this help
`)
}
