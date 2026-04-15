package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joelhelbling/tabb/internal/profile"
)

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	m, err := profile.Load(path)
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("expected empty map, got %v", m)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	original := profile.Map{
		"Work":    "abc123",
		"Personal": "def456",
	}

	if err := profile.Save(path, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := profile.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != len(original) {
		t.Fatalf("expected %d entries, got %d", len(original), len(loaded))
	}
	for k, v := range original {
		if loaded[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, loaded[k])
		}
	}
}

func TestFindByNameCaseInsensitive(t *testing.T) {
	m := profile.Map{
		"Brave-Work": "ext-brave",
		"Personal":   "ext-personal",
	}

	name, id, found := profile.FindByName(m, "brave-work")
	if !found {
		t.Fatal("expected to find 'brave-work' case-insensitively")
	}
	if name != "Brave-Work" {
		t.Errorf("expected original casing 'Brave-Work', got %q", name)
	}
	if id != "ext-brave" {
		t.Errorf("expected extension ID 'ext-brave', got %q", id)
	}

	// Also test exact match
	name2, id2, found2 := profile.FindByName(m, "Personal")
	if !found2 {
		t.Fatal("expected to find 'Personal'")
	}
	if name2 != "Personal" {
		t.Errorf("expected 'Personal', got %q", name2)
	}
	if id2 != "ext-personal" {
		t.Errorf("expected 'ext-personal', got %q", id2)
	}

	// Test not found
	_, _, found3 := profile.FindByName(m, "nonexistent")
	if found3 {
		t.Error("expected not found for 'nonexistent'")
	}
}

func TestFindByExtensionID(t *testing.T) {
	m := profile.Map{
		"Work": "abc123",
		"Home": "xyz789",
	}

	name, found := profile.FindByExtensionID(m, "abc123")
	if !found {
		t.Fatal("expected to find extension ID 'abc123'")
	}
	if name != "Work" {
		t.Errorf("expected profile name 'Work', got %q", name)
	}

	_, found2 := profile.FindByExtensionID(m, "notfound")
	if found2 {
		t.Error("expected not found for unknown extension ID")
	}
}

func TestNameAvailable(t *testing.T) {
	m := profile.Map{
		"Work": "abc123",
	}

	if !profile.NameAvailable(m, "Personal") {
		t.Error("'Personal' should be available")
	}
	if !profile.NameAvailable(m, "work-extra") {
		t.Error("'work-extra' should be available (different from 'Work')")
	}
	if profile.NameAvailable(m, "work") {
		t.Error("'work' should NOT be available (case-insensitive match with 'Work')")
	}
	if profile.NameAvailable(m, "WORK") {
		t.Error("'WORK' should NOT be available (case-insensitive match with 'Work')")
	}
}

func TestResolveOnlyOneSocket(t *testing.T) {
	dir := t.TempDir()

	// Create one sock file
	sockFile := filepath.Join(dir, "abc123.sock")
	if err := os.WriteFile(sockFile, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create sock file: %v", err)
	}

	profilesPath := filepath.Join(dir, "profiles.json")

	extID, err := profile.Resolve(dir, profilesPath, "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if extID != "abc123" {
		t.Errorf("expected 'abc123', got %q", extID)
	}
}

func TestResolveFlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	// Set up profiles with two entries
	m := profile.Map{
		"Flag-Profile": "flag-ext-id",
		"Env-Profile":  "env-ext-id",
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	extID, err := profile.Resolve(dir, profilesPath, "Flag-Profile", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if extID != "flag-ext-id" {
		t.Errorf("expected 'flag-ext-id' (flag takes priority), got %q", extID)
	}
}

func TestResolveEnvVar(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	m := profile.Map{
		"Env-Profile": "env-ext-id",
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	extID, err := profile.Resolve(dir, profilesPath, "", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if extID != "env-ext-id" {
		t.Errorf("expected 'env-ext-id', got %q", extID)
	}
}

func TestResolveMultipleSocketsNoProfile(t *testing.T) {
	dir := t.TempDir()

	// Create multiple sock files
	for _, name := range []string{"aaa111.sock", "bbb222.sock"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte{}, 0600); err != nil {
			t.Fatalf("failed to create sock file %s: %v", name, err)
		}
	}

	profilesPath := filepath.Join(dir, "profiles.json")

	_, err := profile.Resolve(dir, profilesPath, "", "")
	if err == nil {
		t.Fatal("expected an error when multiple sockets exist")
	}
	if !strings.Contains(err.Error(), "multiple") {
		t.Errorf("expected error to contain 'multiple', got: %v", err)
	}
}

func TestResolveNoSockets(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	// Empty dir — no sock files
	_, err := profile.Resolve(dir, profilesPath, "", "")
	if err == nil {
		t.Fatal("expected an error when no sockets exist")
	}
	if !strings.Contains(err.Error(), "no active tabb sockets found") {
		t.Errorf("expected 'no active tabb sockets found' error, got: %v", err)
	}
}

func TestResolveLegacySocket(t *testing.T) {
	dir := t.TempDir()
	// Create the old-style tabb.sock
	os.WriteFile(filepath.Join(dir, "tabb.sock"), []byte{}, 0600)

	_, err := profile.Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err == nil {
		t.Fatal("expected error for legacy socket")
	}
	if !strings.Contains(err.Error(), "tabb setup") {
		t.Errorf("expected migration hint in error, got: %v", err)
	}
}
