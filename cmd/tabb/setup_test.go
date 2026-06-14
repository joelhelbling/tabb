package main

import (
	"path/filepath"
	"testing"
)

func TestParseExtensionID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"plain id", "abcdefghijklmnopabcdefghijklmnop", "abcdefghijklmnopabcdefghijklmnop", false},
		{"trims whitespace", "  abcdefghijklmnopabcdefghijklmnop \n", "abcdefghijklmnopabcdefghijklmnop", false},
		{"strips origin scheme and slash", "chrome-extension://abcdefghijklmnopabcdefghijklmnop/", "abcdefghijklmnopabcdefghijklmnop", false},
		{"strips origin scheme no slash", "chrome-extension://abcdefghijklmnopabcdefghijklmnop", "abcdefghijklmnopabcdefghijklmnop", false},
		{"empty", "", "", true},
		{"wrong length", "abcdef", "", true},
		{"invalid chars", "abcdefghijklmnopabcdefghijklmnoz", "", true},
		{"uppercase rejected", "ABCDEFGHIJKLMNOPABCDEFGHIJKLMNOP", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseExtensionID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseExtensionID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNativeMessagingDirsDarwinIncludesChromiumBrowsers(t *testing.T) {
	home := "/Users/test"
	dirs := nativeMessagingDirs(home, "darwin")

	mustContain := []string{
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"),
		filepath.Join(home, "Library", "Application Support", "Vivaldi", "NativeMessagingHosts"),
		filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
		filepath.Join(home, "Library", "Application Support", "com.operasoftware.Opera", "NativeMessagingHosts"),
	}
	for _, want := range mustContain {
		if !contains(dirs, want) {
			t.Errorf("nativeMessagingDirs(darwin) missing %q\ngot: %v", want, dirs)
		}
	}
}

func TestNativeMessagingDirsLinuxIncludesChromiumBrowsers(t *testing.T) {
	home := "/home/test"
	dirs := nativeMessagingDirs(home, "linux")

	mustContain := []string{
		filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "vivaldi", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
	}
	for _, want := range mustContain {
		if !contains(dirs, want) {
			t.Errorf("nativeMessagingDirs(linux) missing %q\ngot: %v", want, dirs)
		}
	}
}

func TestNativeMessagingDirsUnsupportedOS(t *testing.T) {
	if dirs := nativeMessagingDirs("/home/test", "plan9"); dirs != nil {
		t.Errorf("expected nil for unsupported OS, got %v", dirs)
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
