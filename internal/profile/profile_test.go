package profile_test

import (
	"errors"
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
		"Work":     {ProfileID: "uuid-work", ExtensionID: "ext-abc", Browser: "Vivaldi"},
		"Personal": {ProfileID: "uuid-personal", ExtensionID: "ext-abc", Browser: "Vivaldi"},
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
			t.Errorf("key %q: expected %+v, got %+v", k, v, loaded[k])
		}
	}
}

func TestLoadLegacySchemaReturnsSentinel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	legacy := `{"Work":"abc123","Personal":"def456"}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("failed to write legacy profiles.json: %v", err)
	}

	_, err := profile.Load(path)
	if err == nil {
		t.Fatal("expected ErrLegacySchema, got nil")
	}
	if !errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("expected ErrLegacySchema, got: %v", err)
	}
}

func TestLoadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")

	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := profile.Load(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
	if errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("corrupt JSON should not be reported as legacy schema, got: %v", err)
	}
}

func TestFindByNameCaseInsensitive(t *testing.T) {
	m := profile.Map{
		"Brave-Work": {ProfileID: "uuid-brave", ExtensionID: "ext-brave", Browser: "Brave"},
		"Personal":   {ProfileID: "uuid-personal", ExtensionID: "ext-personal", Browser: "Chrome"},
	}

	name, entry, found := profile.FindByName(m, "brave-work")
	if !found {
		t.Fatal("expected to find 'brave-work' case-insensitively")
	}
	if name != "Brave-Work" {
		t.Errorf("expected original casing 'Brave-Work', got %q", name)
	}
	if entry.ProfileID != "uuid-brave" {
		t.Errorf("expected profile ID 'uuid-brave', got %q", entry.ProfileID)
	}

	_, _, found2 := profile.FindByName(m, "nonexistent")
	if found2 {
		t.Error("expected not found for 'nonexistent'")
	}
}

func TestFindByProfileID(t *testing.T) {
	m := profile.Map{
		"Work": {ProfileID: "uuid-work", ExtensionID: "abc123", Browser: "Chrome"},
		"Home": {ProfileID: "uuid-home", ExtensionID: "xyz789", Browser: "Chrome"},
	}

	name, found := profile.FindByProfileID(m, "uuid-work")
	if !found {
		t.Fatal("expected to find profile ID 'uuid-work'")
	}
	if name != "Work" {
		t.Errorf("expected profile name 'Work', got %q", name)
	}

	_, found2 := profile.FindByProfileID(m, "notfound")
	if found2 {
		t.Error("expected not found for unknown profile ID")
	}
}

func TestNameAvailable(t *testing.T) {
	m := profile.Map{
		"Work": {ProfileID: "uuid-work", ExtensionID: "abc123", Browser: "Chrome"},
	}

	if !profile.NameAvailable(m, "Personal") {
		t.Error("'Personal' should be available")
	}
	if !profile.NameAvailable(m, "work-extra") {
		t.Error("'work-extra' should be available")
	}
	if profile.NameAvailable(m, "work") {
		t.Error("'work' should NOT be available (case-insensitive match with 'Work')")
	}
	if profile.NameAvailable(m, "WORK") {
		t.Error("'WORK' should NOT be available (case-insensitive match with 'Work')")
	}
}

func TestExtensionIDs(t *testing.T) {
	m := profile.Map{
		"Work":     {ProfileID: "uuid-work", ExtensionID: "shared-ext", Browser: "Vivaldi"},
		"Personal": {ProfileID: "uuid-personal", ExtensionID: "shared-ext", Browser: "Vivaldi"},
		"Other":    {ProfileID: "uuid-other", ExtensionID: "different-ext", Browser: "Chrome"},
	}

	ids := profile.ExtensionIDs(m)
	if len(ids) != 2 {
		t.Errorf("expected 2 unique extension IDs, got %d: %v", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen["shared-ext"] || !seen["different-ext"] {
		t.Errorf("missing expected extension IDs: %v", ids)
	}
}

func TestResolveOnlyOneSocket(t *testing.T) {
	dir := t.TempDir()

	sockFile := filepath.Join(dir, "uuid-abc.sock")
	if err := os.WriteFile(sockFile, []byte{}, 0600); err != nil {
		t.Fatalf("failed to create sock file: %v", err)
	}

	profilesPath := filepath.Join(dir, "profiles.json")

	profileID, err := profile.Resolve(dir, profilesPath, "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-abc" {
		t.Errorf("expected 'uuid-abc', got %q", profileID)
	}
}

func TestResolveFlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	m := profile.Map{
		"Flag-Profile": {ProfileID: "uuid-flag", ExtensionID: "ext", Browser: "Chrome"},
		"Env-Profile":  {ProfileID: "uuid-env", ExtensionID: "ext", Browser: "Chrome"},
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	profileID, err := profile.Resolve(dir, profilesPath, "Flag-Profile", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-flag" {
		t.Errorf("expected 'uuid-flag' (flag takes priority), got %q", profileID)
	}
}

func TestResolveEnvVar(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	m := profile.Map{
		"Env-Profile": {ProfileID: "uuid-env", ExtensionID: "ext", Browser: "Chrome"},
	}
	if err := profile.Save(profilesPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	profileID, err := profile.Resolve(dir, profilesPath, "", "Env-Profile")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if profileID != "uuid-env" {
		t.Errorf("expected 'uuid-env', got %q", profileID)
	}
}

func TestResolveMultipleSocketsNoProfile(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"uuid-aaa.sock", "uuid-bbb.sock"} {
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
	os.WriteFile(filepath.Join(dir, "tabb.sock"), []byte{}, 0600)

	_, err := profile.Resolve(dir, filepath.Join(dir, "profiles.json"), "", "")
	if err == nil {
		t.Fatal("expected error for legacy socket")
	}
	if !strings.Contains(err.Error(), "tabb setup") {
		t.Errorf("expected migration hint in error, got: %v", err)
	}
}

func TestResolveLegacyProfilesSchema(t *testing.T) {
	dir := t.TempDir()
	profilesPath := filepath.Join(dir, "profiles.json")

	if err := os.WriteFile(filepath.Join(dir, "uuid-x.sock"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	legacy := `{"Work":"abc123"}`
	if err := os.WriteFile(profilesPath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := profile.Resolve(dir, profilesPath, "Work", "")
	if err == nil {
		t.Fatal("expected legacy-schema error")
	}
	if !errors.Is(err, profile.ErrLegacySchema) {
		t.Errorf("expected ErrLegacySchema, got: %v", err)
	}
}
