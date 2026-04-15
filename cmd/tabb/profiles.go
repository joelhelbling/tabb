package main

import (
	"errors"
	"fmt"
	"sort"

	"github.com/joelhelbling/tabb/internal/profile"
)

func runProfiles() error {
	tabbDir, err := tabbHomeDir()
	if err != nil {
		return err
	}
	profilesPath := profile.ProfilesPath(tabbDir)

	profiles, err := profile.Load(profilesPath)
	if err != nil {
		if errors.Is(err, profile.ErrLegacySchema) {
			fmt.Println("profiles.json is in the legacy format — run 'tabb setup' to migrate.")
			return nil
		}
		return err
	}

	active, err := profile.ActiveSockets(tabbDir)
	if err != nil {
		return err
	}
	activeSet := map[string]bool{}
	for _, id := range active {
		activeSet[id] = true
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles configured. Run 'tabb setup' to register one.")
		return nil
	}

	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		e := profiles[name]
		status := "inactive"
		if activeSet[e.ProfileID] {
			status = "active"
		}
		fmt.Printf("%-20s  %-10s  %-36s  %s\n", name, e.Browser, e.ProfileID, status)
	}
	return nil
}
