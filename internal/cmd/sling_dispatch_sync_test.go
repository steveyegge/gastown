package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/formula"
)

// Tests for syncBeadToRig — the town→rig sync step of per-rig beads architecture.
// Symmetric counterpart to syncBeadsToTown in internal/deacon/reap_completed.go.

func TestSyncBeadToRig_NoRigBeads(t *testing.T) {
	// If the rig has no .beads/ directory, syncBeadToRig should no-op (return nil).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigPath := filepath.Join(tmp, rigName)
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	// Rig dir exists but no .beads/ inside it.
	if err := syncBeadToRig(tmp, rigName); err != nil {
		t.Errorf("expected nil when rig has no .beads/, got: %v", err)
	}
}

func TestSyncBeadToRig_NoTownBeads(t *testing.T) {
	// If the town-root has no .beads/ directory, syncBeadToRig should no-op (return nil).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Rig .beads/ exists, but town-root has no .beads/.
	if err := syncBeadToRig(tmp, rigName); err != nil {
		t.Errorf("expected nil when town-root has no .beads/, got: %v", err)
	}
}

func TestSyncBeadToRig_EmptyRigName(t *testing.T) {
	// Empty rigName should be a no-op (return nil) — no rig to sync to.
	tmp := t.TempDir()
	if err := syncBeadToRig(tmp, ""); err != nil {
		t.Errorf("expected nil for empty rigName, got: %v", err)
	}
}

func TestSyncBeadToRig_BdNotInPath(t *testing.T) {
	// When both .beads/ dirs exist but bd is not in PATH, expect a non-nil error.
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	townBeadsDir := filepath.Join(tmp, ".beads")
	for _, d := range []string{rigBeadsDir, townBeadsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Temporarily replace PATH with an empty value so bd cannot be found.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer func() { os.Setenv("PATH", origPath) }()

	err := syncBeadToRig(tmp, rigName)
	if err == nil {
		t.Error("expected non-nil error when bd is not in PATH, got nil")
	}
}

func TestSyncBeadToRig_RunsInRigContext(t *testing.T) {
	// When both .beads/ dirs exist and PATH is restricted, the call fails (bd absent)
	// but it should reach the exec.Command stage, not short-circuit earlier.
	// This test mirrors TestSyncBeadToRig_BdNotInPath and validates that the function
	// reaches the command execution stage (i.e., the path validation guards passed).
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	townBeadsDir := filepath.Join(tmp, ".beads")
	for _, d := range []string{rigBeadsDir, townBeadsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", "")
	err := syncBeadToRig(tmp, rigName)
	// Should error (bd not found), not nil — proves exec was attempted.
	if err == nil {
		t.Error("expected exec error when bd absent, got nil")
	}
}

func TestSyncBeadToRig_NoOpWhenRigNameEmpty(t *testing.T) {
	// Duplicate guard: rigName="" must always short-circuit, even if .beads/ dirs exist.
	tmp := t.TempDir()
	// Create both .beads/ dirs to confirm the short-circuit is rigName-driven, not dir-driven.
	for _, d := range []string{filepath.Join(tmp, ".beads"), filepath.Join(tmp, "rigs", ".beads")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := syncBeadToRig(tmp, ""); err != nil {
		t.Errorf("expected nil for empty rigName even with .beads/ dirs present, got: %v", err)
	}
}

// --- syncFormulasToRig tests ---

func TestSyncFormulasToRig_EmptyRigName(t *testing.T) {
	// Empty rigName should be a no-op.
	tmp := t.TempDir()
	if err := syncFormulasToRig(tmp, ""); err != nil {
		t.Errorf("expected nil for empty rigName, got: %v", err)
	}
}

func TestSyncFormulasToRig_NoRigBeads(t *testing.T) {
	// If the rig has no .beads/ directory, syncFormulasToRig should no-op.
	tmp := t.TempDir()
	rigName := "my-rig"
	rigPath := filepath.Join(tmp, rigName)
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := syncFormulasToRig(tmp, rigName); err != nil {
		t.Errorf("expected nil when rig has no .beads/, got: %v", err)
	}
}

func TestSyncFormulasToRig_ProvisionsFreshRig(t *testing.T) {
	// When the rig has a .beads/ directory but no formulas, syncFormulasToRig
	// should provision embedded formulas into .beads/formulas/.
	tmp := t.TempDir()
	rigName := "my-rig"
	rigBeadsDir := filepath.Join(tmp, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := syncFormulasToRig(tmp, rigName); err != nil {
		t.Fatalf("syncFormulasToRig error: %v", err)
	}

	// Verify formulas were provisioned
	formulasDir := filepath.Join(rigBeadsDir, "formulas")
	entries, err := os.ReadDir(formulasDir)
	if err != nil {
		t.Fatalf("reading formulas dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected formulas to be provisioned, got empty directory")
	}

	// Verify mol-polecat-work.formula.toml exists specifically
	molPath := filepath.Join(formulasDir, "mol-polecat-work.formula.toml")
	if _, err := os.Stat(molPath); os.IsNotExist(err) {
		t.Fatal("mol-polecat-work.formula.toml not provisioned")
	}
}

func TestSyncFormulasToRig_UpdatesOutdatedFormula(t *testing.T) {
	// When a rig has an old formula (different from embedded), syncFormulasToRig
	// should update it to the latest embedded version.
	tmp := t.TempDir()
	rigName := "my-rig"
	rigPath := filepath.Join(tmp, rigName)

	// First provision to get the initial state
	if _, err := formula.ProvisionFormulas(rigPath); err != nil {
		t.Fatalf("initial provision: %v", err)
	}

	// Verify mol-polecat-work exists
	formulasDir := filepath.Join(rigPath, ".beads", "formulas")
	molPath := filepath.Join(formulasDir, "mol-polecat-work.formula.toml")
	originalContent, err := os.ReadFile(molPath)
	if err != nil {
		t.Fatalf("reading original mol-polecat-work: %v", err)
	}

	// Simulate an "old" version by writing stale content.
	// The installed record still has the original hash, so UpdateFormulas sees
	// this as "user hasn't modified" (current == installed but != embedded).
	// We need to write content that differs from embedded AND update the
	// installed record to match, simulating the upgrade scenario.
	staleContent := []byte("# stale formula\n[steps]\n")
	if err := os.WriteFile(molPath, staleContent, 0644); err != nil {
		t.Fatalf("writing stale formula: %v", err)
	}

	// Update .installed.json so the stale content hash matches the "installed" hash,
	// simulating a formula that was installed by an older gt version.
	// formula.UpdateFormulas will see: current==installed but !=embedded → outdated → safe to update.
	installedJSON := `{"formulas":{"mol-polecat-work.formula.toml":"` + hashBytes(staleContent) + `"}}`
	if err := os.WriteFile(filepath.Join(formulasDir, ".installed.json"), []byte(installedJSON), 0644); err != nil {
		t.Fatalf("writing installed record: %v", err)
	}

	// Now sync — should update the outdated formula
	if err := syncFormulasToRig(tmp, rigName); err != nil {
		t.Fatalf("syncFormulasToRig error: %v", err)
	}

	// Verify the formula was updated to the embedded version
	updatedContent, err := os.ReadFile(molPath)
	if err != nil {
		t.Fatalf("reading updated mol-polecat-work: %v", err)
	}
	if string(updatedContent) == string(staleContent) {
		t.Fatal("formula was not updated — still has stale content")
	}
	if string(updatedContent) != string(originalContent) {
		t.Fatal("formula was updated but doesn't match embedded content")
	}
}

func TestSyncFormulasToRig_NoRigDir(t *testing.T) {
	// If the rig directory itself doesn't exist, syncFormulasToRig should no-op.
	tmp := t.TempDir()
	if err := syncFormulasToRig(tmp, "nonexistent-rig"); err != nil {
		t.Errorf("expected nil for nonexistent rig, got: %v", err)
	}
}

// hashBytes computes SHA256 hex of data (test helper).
func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
