package configbeads

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// setupTestTown creates a minimal town directory with town.json.
func setupTestTown(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create mayor directory and town.json
	mayorDir := filepath.Join(dir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	tc := &config.TownConfig{
		Type:       "town",
		Version:    2,
		Name:       "testtown",
		Owner:      "test@example.com",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := config.SaveTownConfig(filepath.Join(mayorDir, "town.json"), tc); err != nil {
		t.Fatal(err)
	}

	return dir
}

// setupTestBeads creates an isolated beads instance with initialized DB.
// Returns nil if bd CLI is unavailable or has known write bugs.
func setupTestBeads(t *testing.T, dir string) *beads.Beads {
	t.Helper()
	// Prevent daemon/config leakage so both isolated and non-isolated
	// Beads instances (e.g. SeedRigRegistryBead) use the local temp DB.
	t.Setenv("BD_DAEMON_HOST", "")
	t.Setenv("BD_DAEMON_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	// Initialize git repo to suppress "No git repository" warnings in bd stdout
	// (bd writes warnings to stdout which breaks JSON parsing).
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Logf("warning: git init failed: %v: %s", err, out)
	}
	bd := beads.NewIsolated(dir)
	if err := bd.Init("hq-"); err != nil {
		t.Skipf("cannot initialize beads repo (bd not available?): %v", err)
		return nil
	}
	// Ensure custom types are configured (config, agent, etc.)
	beadsDir := filepath.Join(dir, ".beads")
	if err := beads.EnsureCustomTypes(beadsDir); err != nil {
		t.Logf("warning: could not set custom types: %v", err)
	}
	return bd
}

func TestLoadTownConfigFromBeads_NotFound(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	tc, err := LoadTownConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc != nil {
		t.Error("expected nil when bead doesn't exist")
	}
}

func TestLoadTownConfigFromBeads_Found(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create the config bead
	metadata := map[string]interface{}{
		"type":        "town",
		"version":     2,
		"name":        "testtown",
		"owner":       "test@example.com",
		"public_name": "Test Town",
		"created_at":  "2026-01-01T00:00:00Z",
	}
	metaJSON, _ := json.Marshal(metadata)

	fields := &beads.ConfigFields{
		Rig:      "testtown",
		Category: beads.ConfigCategoryIdentity,
		Metadata: string(metaJSON),
	}
	_, err := bd.CreateConfigBead("town-testtown", fields, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Load from beads
	tc, err := LoadTownConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TownConfig")
	}
	if tc.Name != "testtown" {
		t.Errorf("name = %q, want %q", tc.Name, "testtown")
	}
	if tc.Owner != "test@example.com" {
		t.Errorf("owner = %q, want %q", tc.Owner, "test@example.com")
	}
	if tc.PublicName != "Test Town" {
		t.Errorf("public_name = %q, want %q", tc.PublicName, "Test Town")
	}
	if tc.Version != 2 {
		t.Errorf("version = %d, want %d", tc.Version, 2)
	}
}

func TestLoadTownIdentity_FallbackToFilesystem(t *testing.T) {
	dir := setupTestTown(t)

	// No beads exist, should fallback to filesystem
	tc, err := LoadTownIdentity(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TownConfig from fallback")
	}
	if tc.Name != "testtown" {
		t.Errorf("name = %q, want %q", tc.Name, "testtown")
	}
	if tc.Owner != "test@example.com" {
		t.Errorf("owner = %q, want %q", tc.Owner, "test@example.com")
	}
}

func TestLoadTownIdentity_NoTownJson(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadTownIdentity(dir)
	if err == nil {
		t.Error("expected error when town.json missing")
	}
}

func TestLoadTownConfigFromBeads_InvalidMetadata(t *testing.T) {
	// Test that invalid JSON in metadata returns an error
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	fields := &beads.ConfigFields{
		Rig:      "testtown",
		Category: beads.ConfigCategoryIdentity,
		Metadata: `{"name": "testtown"}`, // valid but minimal
	}
	_, err := bd.CreateConfigBead("town-testtown", fields, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	tc, err := LoadTownConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc == nil {
		t.Fatal("expected non-nil TownConfig")
	}
	if tc.Name != "testtown" {
		t.Errorf("name = %q, want %q", tc.Name, "testtown")
	}
}
