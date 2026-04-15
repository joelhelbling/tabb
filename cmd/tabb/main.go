package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	// Chrome launches the native host with the extension origin as the first arg
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "chrome-extension://") {
		extID := strings.TrimPrefix(os.Args[1], "chrome-extension://")
		extID = strings.TrimSuffix(extID, "/")
		if err := runHost(extID); err != nil {
			fmt.Fprintf(os.Stderr, "tabb: %v\n", err)
			os.Exit(1)
		}
		return
	}

	profileFlag, cmdArgs := extractProfileFlag(os.Args[1:])
	if len(cmdArgs) == 0 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch cmdArgs[0] {
	case "host":
		err = runHost("")
	case "list", "ls":
		err = runList(cmdArgs[1:], profileFlag)
	case "show":
		err = runShow(cmdArgs[1:], profileFlag)
	case "close":
		err = runClose(cmdArgs[1:], profileFlag)
	case "mcp":
		err = runMCP()
	case "profiles":
		err = runProfiles()
	case "setup":
		err = runSetup()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "tabb: unknown command %q\n\n", cmdArgs[0])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "tabb: %v\n", err)
		os.Exit(1)
	}
}

// extractProfileFlag scans args for --profile=<name> or --profile <name>,
// removes it, and returns (profileName, remainingArgs).
func extractProfileFlag(args []string) (string, []string) {
	var profileName string
	var remaining []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--profile" && i+1 < len(args) {
			profileName = args[i+1]
			i++ // skip next
		} else if strings.HasPrefix(arg, "--profile=") {
			profileName = strings.TrimPrefix(arg, "--profile=")
		} else {
			remaining = append(remaining, arg)
		}
	}
	return profileName, remaining
}

func printUsage() {
	fmt.Print(`tabb — manage Chrome tabs from the terminal

Usage:
  tabb [--profile <name>] list [--json] [filter]   List open tabs
  tabb [--profile <name>] show <tab-id> [--raw]    Show tab content as markdown
  tabb [--profile <name>] close <tab-id>           Close a tab
  tabb profiles                                     List configured profiles
  tabb mcp                                          Run as MCP stdio server
  tabb setup                                        Install Native Messaging host manifest
  tabb help                                         Show this help

Environment:
  TABB_PROFILE   Default profile name (overridden by --profile flag)
`)
}
