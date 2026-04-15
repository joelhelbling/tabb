// Package profile manages ~/.tabb/profiles.json and provides profile resolution logic.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Map maps profile names to extension IDs.
type Map map[string]string

// Load reads profiles.json from path. Returns an empty map if the file does not exist.
func Load(path string) (Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Map{}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	return m, nil
}

// Save writes profiles as indented JSON to path.
func Save(path string, profiles Map) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding profiles: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing profiles: %w", err)
	}
	return nil
}

// FindByName performs a case-insensitive lookup by profile name.
// Returns the original-cased name, the extension ID, and whether it was found.
func FindByName(profiles Map, name string) (string, string, bool) {
	lower := strings.ToLower(name)
	for k, v := range profiles {
		if strings.ToLower(k) == lower {
			return k, v, true
		}
	}
	return "", "", false
}

// FindByExtensionID performs a reverse lookup by extension ID.
// Returns the profile name and whether it was found.
func FindByExtensionID(profiles Map, extensionID string) (string, bool) {
	for name, id := range profiles {
		if id == extensionID {
			return name, true
		}
	}
	return "", false
}

// NameAvailable reports whether the given name is not already in use (case-insensitive).
func NameAvailable(profiles Map, name string) bool {
	_, _, found := FindByName(profiles, name)
	return !found
}

// ProfilesPath returns the path to profiles.json within tabbDir.
func ProfilesPath(tabbDir string) string {
	return filepath.Join(tabbDir, "profiles.json")
}

// ActiveSockets reads tabbDir and returns extension IDs from *.sock files.
// Returns nil if the directory does not exist.
// Skips the legacy tabb.sock file.
func ActiveSockets(tabbDir string) ([]string, error) {
	entries, err := os.ReadDir(tabbDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tabb directory: %w", err)
	}

	var ids []string
	for _, e := range entries {
		name := e.Name()
		if name == "tabb.sock" {
			continue // legacy socket, skip
		}
		if strings.HasSuffix(name, ".sock") {
			ids = append(ids, strings.TrimSuffix(name, ".sock"))
		}
	}
	return ids, nil
}

// HasLegacySocket returns true if the old-style tabb.sock exists.
func HasLegacySocket(tabbDir string) bool {
	_, err := os.Stat(filepath.Join(tabbDir, "tabb.sock"))
	return err == nil
}

// Resolve determines which extension ID to connect to.
// Priority: flagProfile > envProfile > auto-detect from active sockets.
func Resolve(tabbDir, profilesPath, flagProfile, envProfile string) (string, error) {
	profileName := flagProfile
	if profileName == "" {
		profileName = envProfile
	}

	if profileName != "" {
		profiles, err := Load(profilesPath)
		if err != nil {
			return "", err
		}
		_, id, found := FindByName(profiles, profileName)
		if !found {
			return "", fmt.Errorf("profile %q not found in %s", profileName, profilesPath)
		}
		return id, nil
	}

	// Auto-detect from active sockets
	sockets, err := ActiveSockets(tabbDir)
	if err != nil {
		return "", err
	}

	switch len(sockets) {
	case 0:
		if HasLegacySocket(tabbDir) {
			return "", fmt.Errorf("found legacy tabb.sock — run 'tabb setup' to migrate to the new multi-profile format")
		}
		return "", fmt.Errorf("no active tabb sockets found (is Chrome running with the tabb extension?)")
	case 1:
		return sockets[0], nil
	default:
		profiles, err := Load(profilesPath)
		if err != nil {
			return "", err
		}
		return "", multipleProfilesError(sockets, profiles)
	}
}

// multipleProfilesError formats an error listing all active profiles.
func multipleProfilesError(socketIDs []string, profiles Map) error {
	var sb strings.Builder
	sb.WriteString("multiple active tabb profiles found; specify one with --profile or TABB_PROFILE:\n")
	for _, id := range socketIDs {
		if name, found := FindByExtensionID(profiles, id); found {
			fmt.Fprintf(&sb, "  %s (%s)\n", name, id)
		} else {
			fmt.Fprintf(&sb, "  (unnamed) (%s)\n", id)
		}
	}
	return errors.New(strings.TrimRight(sb.String(), "\n"))
}
