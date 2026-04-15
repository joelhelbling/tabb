package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joelhelbling/tabb/internal/profile"
)

type nativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// discoveredProfile is a profileId seen in ~/.tabb/*.browser but not yet
// present in profiles.json.
type discoveredProfile struct {
	ProfileID   string
	ExtensionID string
	Browser     string
}

func runSetup() error {
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

	profilesPath := profile.ProfilesPath(tabbDir)
	profiles, err := profile.Load(profilesPath)
	if err != nil {
		if errors.Is(err, profile.ErrLegacySchema) {
			return migrateLegacyProfiles(tabbDir, profilesPath)
		}
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	// On first run we still need to place the Native Messaging manifest so the
	// browser can even launch the host. Write it now using whatever extension IDs
	// we have (possibly none); we'll rewrite it at the end too.
	manifestPath := filepath.Join(manifestDir, "com.tabb.json")
	if err := writeManifest(manifestPath, binaryPath, profile.ExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Println("Load the extension in your browser:")
	fmt.Println("  1. Open chrome://extensions (or equivalent)")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println("  4. If it's already loaded, click its reload icon")
	fmt.Println()
	if len(profiles) > 0 {
		fmt.Println("Registered profiles:")
		for name, e := range profiles {
			fmt.Printf("  %s  (%s)\n", name, e.Browser)
		}
		fmt.Println()
	}
	fmt.Print("Press Enter when the extension is loaded/reloaded: ")
	if _, err := reader.ReadString('\n'); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	discovered, err := discoverNewProfiles(tabbDir, profiles)
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		fmt.Println("\nNo new profiles detected.")
		fmt.Println("Make sure the extension is loaded and the browser is running,")
		fmt.Println("then re-run 'tabb setup'.")
		return nil
	}

	for _, d := range discovered {
		fmt.Printf("\nNew profile detected: browser=%s profileId=%s\n", d.Browser, d.ProfileID)
		defaultName := suggestProfileName(profiles, d.Browser)
		var prompt string
		if defaultName != "" {
			prompt = fmt.Sprintf("Profile name [%s]: ", defaultName)
		} else {
			prompt = "Profile name: "
		}
		fmt.Print(prompt)
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
		if !profile.NameAvailable(profiles, profileName) {
			return fmt.Errorf("profile name %q is already in use (case-insensitive)", profileName)
		}
		profiles[profileName] = profile.Entry{
			ProfileID:   d.ProfileID,
			ExtensionID: d.ExtensionID,
			Browser:     d.Browser,
		}
		fmt.Printf("Registered %q\n", profileName)
	}

	if err := profile.Save(profilesPath, profiles); err != nil {
		return err
	}

	// Rewrite the manifest with the full set of extension IDs.
	if err := writeManifest(manifestPath, binaryPath, profile.ExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Printf("\nSaved %s\n", profilesPath)
	fmt.Printf("Manifest updated: %s\n", manifestPath)
	fmt.Println("You can now run 'tabb list' (or use the MCP tools).")
	return nil
}

// discoverNewProfiles scans tabbDir for *.browser files whose profileId is not
// already registered in profiles. Each .browser file is written by the native
// host on handshake and contains "<browser>\n<extensionId>\n".
func discoverNewProfiles(tabbDir string, profiles profile.Map) ([]discoveredProfile, error) {
	entries, err := os.ReadDir(tabbDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tabb directory: %w", err)
	}

	registered := map[string]bool{}
	for _, e := range profiles {
		registered[e.ProfileID] = true
	}

	var out []discoveredProfile
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".browser") {
			continue
		}
		profileID := strings.TrimSuffix(name, ".browser")
		if registered[profileID] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tabbDir, name))
		if err != nil {
			continue
		}
		lines := strings.SplitN(strings.TrimRight(string(data), "\n"), "\n", 2)
		browser := ""
		extensionID := ""
		if len(lines) > 0 {
			browser = lines[0]
		}
		if len(lines) > 1 {
			extensionID = lines[1]
		}
		out = append(out, discoveredProfile{
			ProfileID:   profileID,
			ExtensionID: extensionID,
			Browser:     browser,
		})
	}
	return out, nil
}

// migrateLegacyProfiles handles the case where profiles.json is in the old
// string-valued schema. We cannot automatically map old extensionIDs to new
// profileIDs, so we move the old file aside and ask the user to re-run setup.
func migrateLegacyProfiles(tabbDir, profilesPath string) error {
	backup := profilesPath + ".legacy"
	if err := os.Rename(profilesPath, backup); err != nil {
		return fmt.Errorf("moving legacy profiles.json aside: %w", err)
	}
	entries, _ := os.ReadDir(tabbDir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".sock") || strings.HasSuffix(name, ".browser") {
			os.Remove(filepath.Join(tabbDir, name))
		}
	}
	fmt.Printf("Legacy profiles.json moved to %s\n", backup)
	fmt.Println("Re-run 'tabb setup' after reloading the extension in each browser profile.")
	return nil
}

func writeManifest(path, binaryPath string, extensionIDs []string) error {
	origins := make([]string, 0, len(extensionIDs))
	for _, id := range extensionIDs {
		origins = append(origins, fmt.Sprintf("chrome-extension://%s/", id))
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

// suggestProfileName picks a default based on the browser name, falling back
// to "Default" for the first profile or a numbered suffix thereafter.
func suggestProfileName(profiles profile.Map, browser string) string {
	if len(profiles) == 0 {
		if browser != "" {
			return browser
		}
		return "Default"
	}
	if browser != "" && profile.NameAvailable(profiles, browser) {
		return browser
	}
	if browser != "" {
		for i := 2; i <= 99; i++ {
			candidate := fmt.Sprintf("%s-%d", browser, i)
			if profile.NameAvailable(profiles, candidate) {
				return candidate
			}
		}
	}
	return ""
}
