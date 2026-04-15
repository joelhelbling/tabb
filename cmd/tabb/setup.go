package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type nativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func runSetup() error {
	// Find the tabb binary path
	binaryPath, err := exec.LookPath("tabb")
	if err != nil {
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine tabb binary path: %w", err)
		}
	}
	binaryPath, _ = filepath.Abs(binaryPath)

	manifestDir, err := nativeMessagingDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	tabbDir, err := tabbHomeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tabbDir, 0700); err != nil {
		return fmt.Errorf("creating tabb directory: %w", err)
	}

	profilesPath := filepath.Join(tabbDir, "profiles.json")
	profiles, err := loadProfiles(profilesPath)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	// Guide user through extension setup
	fmt.Println("Load the extension in your browser:")
	fmt.Println("  1. Open chrome://extensions (or equivalent)")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println()

	fmt.Print("Paste the extension ID and press Enter: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	extensionID := strings.TrimSpace(input)
	if extensionID == "" {
		fmt.Println("\nNo extension ID entered. You can re-run 'tabb setup' later.")
		return nil
	}

	// Check if this extension ID is already registered
	if existingName, found := findProfileByExtID(profiles, extensionID); found {
		fmt.Printf("\nExtension ID already registered as profile %q.\n", existingName)
		fmt.Println("Reload the extension to connect.")
		return nil
	}

	// Suggest a profile name
	defaultName := suggestProfileName(tabbDir, extensionID, profiles)
	if defaultName != "" {
		fmt.Printf("Profile name [%s]: ", defaultName)
	} else {
		fmt.Print("Profile name: ")
	}
	nameInput, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	profileName := strings.TrimSpace(nameInput)
	if profileName == "" {
		profileName = defaultName
	}
	if profileName == "" {
		return fmt.Errorf("profile name is required")
	}

	// Check for name conflicts (case-insensitive)
	if !isNameAvailable(profiles, profileName) {
		return fmt.Errorf("profile name %q is already in use (case-insensitive)", profileName)
	}

	// Save profile
	profiles[profileName] = extensionID
	if err := saveProfiles(profilesPath, profiles); err != nil {
		return err
	}

	// Write manifest with ALL registered extension IDs
	manifestPath := filepath.Join(manifestDir, "com.tabb.json")
	if err := writeManifest(manifestPath, binaryPath, allExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Printf("\nProfile %q registered with extension ID: %s\n", profileName, extensionID)
	fmt.Printf("Manifest updated: %s\n", manifestPath)
	fmt.Println("Reload the extension in your browser to establish the connection.")

	return nil
}

func writeManifest(path, binaryPath string, extensionIDs []string) error {
	origins := make([]string, len(extensionIDs))
	for i, id := range extensionIDs {
		origins[i] = fmt.Sprintf("chrome-extension://%s/", id)
	}

	manifest := nativeManifest{
		Name:           "com.tabb",
		Description:    "tabb — manage Chrome tabs from the terminal",
		Path:           binaryPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}
	return nil
}

func nativeMessagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s (only macOS and Linux are supported)", runtime.GOOS)
	}
}

func tabbHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".tabb"), nil
}

func loadProfiles(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	return m, nil
}

func saveProfiles(path string, profiles map[string]string) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling profiles: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func findProfileByExtID(profiles map[string]string, extID string) (string, bool) {
	for name, id := range profiles {
		if id == extID {
			return name, true
		}
	}
	return "", false
}

func isNameAvailable(profiles map[string]string, name string) bool {
	lower := strings.ToLower(name)
	for k := range profiles {
		if strings.ToLower(k) == lower {
			return false
		}
	}
	return true
}

func allExtensionIDs(profiles map[string]string) []string {
	ids := make([]string, 0, len(profiles))
	for _, id := range profiles {
		ids = append(ids, id)
	}
	return ids
}

// suggestProfileName reads the .browser file (written by the native host
// after a handshake) and suggests a default profile name.
func suggestProfileName(tabbDir, extensionID string, profiles map[string]string) string {
	// Try reading browser name from handshake file
	data, err := os.ReadFile(filepath.Join(tabbDir, extensionID+".browser"))
	browserName := ""
	if err == nil {
		browserName = strings.TrimSpace(string(data))
	}

	// If first profile ever, default to "Default"
	if len(profiles) == 0 {
		if browserName != "" {
			return browserName
		}
		return "Default"
	}

	// Try browser name if available and not taken
	if browserName != "" && isNameAvailable(profiles, browserName) {
		return browserName
	}

	// Try browser name with incrementing suffix
	if browserName != "" {
		for i := 2; i <= 99; i++ {
			candidate := fmt.Sprintf("%s-%d", browserName, i)
			if isNameAvailable(profiles, candidate) {
				return candidate
			}
		}
	}

	// No good default — user must choose
	return ""
}
