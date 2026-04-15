package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/joelhelbling/tabb/internal/profile"
	"github.com/joelhelbling/tabb/internal/socket"
)

func runProfiles() error {
	tabbDir, err := socket.Dir()
	if err != nil {
		return err
	}
	profilesPath := profile.ProfilesPath(tabbDir)

	profiles, err := profile.Load(profilesPath)
	if err != nil {
		return err
	}

	activeSockets, err := profile.ActiveSockets(tabbDir)
	if err != nil {
		return err
	}

	activeSet := make(map[string]bool)
	for _, id := range activeSockets {
		activeSet[id] = true
	}

	if len(profiles) == 0 && len(activeSockets) == 0 {
		fmt.Println("No profiles configured. Run 'tabb setup' to add one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tBROWSER\tSTATUS\tEXTENSION ID")

	// Show named profiles
	for name, extID := range profiles {
		status := "inactive"
		if activeSet[extID] {
			status = "active"
			delete(activeSet, extID)
		}
		browser := readBrowserName(tabbDir, extID)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, browser, status, extID)
	}

	// Show unnamed active sockets
	for id := range activeSet {
		browser := readBrowserName(tabbDir, id)
		fmt.Fprintf(w, "(unnamed)\t%s\tactive\t%s\n", browser, id)
	}

	w.Flush()
	return nil
}

// readBrowserName reads the .browser file for a given extension ID.
func readBrowserName(tabbDir, extensionID string) string {
	data, err := os.ReadFile(filepath.Join(tabbDir, extensionID+".browser"))
	if err != nil {
		return "—"
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "—"
	}
	return name
}
