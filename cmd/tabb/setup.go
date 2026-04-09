package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	manifest := nativeManifest{
		Name:        "com.tabb",
		Description: "tabb — manage Chrome tabs from the terminal",
		Path:        binaryPath,
		Type:        "stdio",
		AllowedOrigins: []string{
			// This will be updated with the actual extension ID after sideloading
			"chrome-extension://EXTENSION_ID_HERE/",
		},
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDir, err := nativeMessagingDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	manifestPath := filepath.Join(manifestDir, "com.tabb.json")
	if err := os.WriteFile(manifestPath, manifestJSON, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	fmt.Printf("Native Messaging manifest written to:\n  %s\n\n", manifestPath)
	fmt.Printf("Binary path: %s\n\n", binaryPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Load the extension in Chrome:")
	fmt.Println("     - Open chrome://extensions")
	fmt.Println("     - Enable 'Developer mode'")
	fmt.Println("     - Click 'Load unpacked' and select the extension/ directory")
	fmt.Println("  2. Copy the extension ID from the extensions page")
	fmt.Println("  3. Update the 'allowed_origins' in the manifest file above")
	fmt.Printf("     with: chrome-extension://YOUR_EXTENSION_ID/\n")
	fmt.Println("  4. Reload the extension to establish the Native Messaging connection")

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
