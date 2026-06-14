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

	manifestDirs, err := installedManifestDirs()
	if err != nil {
		return err
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

	fmt.Println("Load the extension in your browser:")
	fmt.Println("  1. Open chrome://extensions (or equivalent)")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println()
	if len(profiles) > 0 {
		fmt.Println("Registered profiles:")
		for name, e := range profiles {
			fmt.Printf("  %s  (%s)\n", name, e.Browser)
		}
		fmt.Println()
	}

	// Chrome will only launch the native host for extension origins explicitly
	// listed in the manifest's allowed_origins. The host writes the .browser
	// file we discover profiles from — so we must authorize the extension's ID
	// BEFORE asking the user to reload, or the host can never start (chicken
	// and egg). Collect the IDs the user pastes from the extensions page.
	fmt.Println("Copy each extension's ID from the extensions page")
	fmt.Println("(the 32-letter string shown under the extension's name).")
	extensionIDs := profile.ExtensionIDs(profiles)
	seen := map[string]bool{}
	for _, id := range extensionIDs {
		seen[id] = true
	}
	for {
		fmt.Print("Paste an extension ID (or press Enter when done): ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("reading input: %w", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		id, perr := parseExtensionID(line)
		if perr != nil {
			fmt.Printf("  %v\n", perr)
			continue
		}
		if !seen[id] {
			seen[id] = true
			extensionIDs = append(extensionIDs, id)
		}
		fmt.Printf("  Authorized %s\n", id)
	}

	// Write the manifest (to every installed browser's host directory) with the
	// authorized extension IDs so the browser can launch the host on reload.
	written, err := writeManifests(manifestDirs, binaryPath, extensionIDs)
	if err != nil {
		return err
	}
	fmt.Println("\nNative Messaging host manifest written to:")
	for _, p := range written {
		fmt.Printf("  %s\n", p)
	}

	if len(extensionIDs) == 0 {
		fmt.Println("\nNo extension IDs provided, so no extension is authorized to launch")
		fmt.Println("the host and no profiles can be detected. Re-run 'tabb setup' and")
		fmt.Println("paste the extension ID from the extensions page.")
		return nil
	}

	fmt.Println("\nNow reload the extension (click its reload icon) so it connects to the host.")
	fmt.Print("Press Enter once you've reloaded it: ")
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

	// Rewrite the manifests with the full set of extension IDs (a discovered
	// profile may carry an ID the user didn't paste).
	if _, err := writeManifests(manifestDirs, binaryPath, profile.ExtensionIDs(profiles)); err != nil {
		return err
	}

	fmt.Printf("\nSaved %s\n", profilesPath)
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

// nativeMessagingDirs returns the candidate NativeMessagingHosts directories for
// all Chromium-family browsers tabb knows about, for the given home dir and OS.
// It is pure (no filesystem access) so callers can filter by what's installed.
// Returns nil for unsupported operating systems.
func nativeMessagingDirs(home, goos string) []string {
	var base string
	var subs []string
	switch goos {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")
		subs = []string{
			"Google/Chrome", "Google/Chrome Beta", "Google/Chrome Dev", "Google/Chrome Canary",
			"Chromium",
			"BraveSoftware/Brave-Browser", "BraveSoftware/Brave-Browser-Beta", "BraveSoftware/Brave-Browser-Nightly",
			"Microsoft Edge", "Microsoft Edge Beta", "Microsoft Edge Dev", "Microsoft Edge Canary",
			"Vivaldi", "Vivaldi Snapshot",
			"com.operasoftware.Opera", "com.operasoftware.OperaNext", "com.operasoftware.OperaDeveloper",
			"Arc/User Data",
		}
	case "linux":
		base = filepath.Join(home, ".config")
		subs = []string{
			"google-chrome", "google-chrome-beta", "google-chrome-unstable",
			"chromium",
			"BraveSoftware/Brave-Browser",
			"microsoft-edge", "microsoft-edge-beta", "microsoft-edge-dev",
			"vivaldi", "vivaldi-snapshot",
			"opera",
		}
	default:
		return nil
	}
	dirs := make([]string, len(subs))
	for i, s := range subs {
		dirs[i] = filepath.Join(base, filepath.FromSlash(s), "NativeMessagingHosts")
	}
	return dirs
}

// installedManifestDirs returns the NativeMessagingHosts directories for the
// Chromium-family browsers that appear to be installed — detected by the
// existence of the browser's data directory (the parent of NativeMessagingHosts).
// If none are detected it falls back to the canonical Chrome directory so setup
// still places a manifest somewhere sensible.
func installedManifestDirs() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	candidates := nativeMessagingDirs(home, runtime.GOOS)
	if candidates == nil {
		return nil, fmt.Errorf("unsupported OS: %s (only macOS and Linux are supported)", runtime.GOOS)
	}
	var out []string
	for _, dir := range candidates {
		parent := filepath.Dir(dir) // the browser's data directory
		if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
			out = append(out, dir)
		}
	}
	if len(out) == 0 {
		out = append(out, candidates[0])
	}
	return out, nil
}

// parseExtensionID normalizes a pasted Chrome extension identifier. It accepts a
// bare ID, a full chrome-extension:// origin, or either with surrounding
// whitespace, and validates the canonical 32-character a–p form.
func parseExtensionID(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "chrome-extension://")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty extension ID")
	}
	if len(s) != 32 {
		return "", fmt.Errorf("extension ID should be 32 letters (a–p); got %d characters: %q", len(s), s)
	}
	for _, r := range s {
		if r < 'a' || r > 'p' {
			return "", fmt.Errorf("extension ID should contain only the letters a–p; got %q", s)
		}
	}
	return s, nil
}

// writeManifests writes the Native Messaging host manifest into every directory
// in dirs (creating each if needed) and returns the paths written.
func writeManifests(dirs []string, binaryPath string, extensionIDs []string) ([]string, error) {
	var written []string
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating manifest directory %s: %w", dir, err)
		}
		path := filepath.Join(dir, "com.tabb.json")
		if err := writeManifest(path, binaryPath, extensionIDs); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return written, nil
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
