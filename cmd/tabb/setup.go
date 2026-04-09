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
		// Fall back to the current executable
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

	manifestPath := filepath.Join(manifestDir, "com.tabb.json")

	// Check if manifest already exists with an extension ID
	existingID := ""
	if data, err := os.ReadFile(manifestPath); err == nil {
		var existing nativeManifest
		if json.Unmarshal(data, &existing) == nil && len(existing.AllowedOrigins) > 0 {
			origin := existing.AllowedOrigins[0]
			origin = strings.TrimPrefix(origin, "chrome-extension://")
			origin = strings.TrimSuffix(origin, "/")
			if origin != "" && origin != "EXTENSION_ID_HERE" {
				existingID = origin
			}
		}
	}

	// Write initial manifest with placeholder
	extensionID := "EXTENSION_ID_HERE"
	if existingID != "" {
		extensionID = existingID
	}

	if err := writeManifest(manifestPath, binaryPath, extensionID); err != nil {
		return err
	}

	fmt.Printf("Native Messaging manifest written to:\n  %s\n\n", manifestPath)
	fmt.Printf("Binary path: %s\n\n", binaryPath)

	if existingID != "" {
		fmt.Printf("Extension ID: %s (from existing manifest)\n\n", existingID)
		fmt.Println("Setup complete. Reload the extension in Chrome to connect.")
		return nil
	}

	// Guide user through extension setup
	fmt.Println("Next, load the extension in Chrome:")
	fmt.Println("  1. Open chrome://extensions")
	fmt.Println("  2. Enable 'Developer mode' (toggle in top right)")
	fmt.Println("  3. Click 'Load unpacked' and select the extension/ directory")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Paste the extension ID from Chrome and press Enter: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	extensionID = strings.TrimSpace(input)
	if extensionID == "" {
		fmt.Println("\nNo extension ID entered. You can re-run 'tabb setup' later to set it.")
		return nil
	}

	// Update manifest with the real extension ID
	if err := writeManifest(manifestPath, binaryPath, extensionID); err != nil {
		return err
	}

	fmt.Printf("\nManifest updated with extension ID: %s\n", extensionID)
	fmt.Println("Reload the extension in Chrome to establish the Native Messaging connection.")

	return nil
}

func writeManifest(path, binaryPath, extensionID string) error {
	manifest := nativeManifest{
		Name:        "com.tabb",
		Description: "tabb — manage Chrome tabs from the terminal",
		Path:        binaryPath,
		Type:        "stdio",
		AllowedOrigins: []string{
			fmt.Sprintf("chrome-extension://%s/", extensionID),
		},
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
