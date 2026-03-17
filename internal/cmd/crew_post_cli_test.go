package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

// ---------------------------------------------------------------------------
// Section 7: gt crew post — CLI-level tests
// ---------------------------------------------------------------------------

// setupTestTownForCrewPost creates a test town with a rig and crew member,
// chdir's into the rig directory, and returns a cleanup func.
// The caller must defer the cleanup.
func setupTestTownForCrewPost(t *testing.T, rigName, crewName string) (townRoot string, cleanup func()) {
	t.Helper()

	townRoot = setupTestTownForCrewList(t, map[string][]string{
		rigName: {crewName},
	})

	// Create settings dir so SaveRigSettings has a target
	settingsDir := filepath.Join(townRoot, rigName, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write initial empty settings
	settings := config.NewRigSettings()
	if err := config.SaveRigSettings(filepath.Join(settingsDir, "config.json"), settings); err != nil {
		t.Fatal(err)
	}

	originalWd, _ := os.Getwd()
	if err := os.Chdir(filepath.Join(townRoot, rigName)); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	return townRoot, func() {
		os.Chdir(originalWd)
		crewRig = ""
		crewPostClear = false
	}
}

// 7.1: gt crew post <name> <posting> prints success message
func TestCrewPostCLI_SuccessMessage(t *testing.T) {
	townRoot, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()
	_ = townRoot

	crewRig = "testrig"
	output := captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "dispatcher"}); err != nil {
			t.Fatalf("runCrewPost: %v", err)
		}
	})

	if !strings.Contains(output, "Set posting") {
		t.Errorf("expected success message containing 'Set posting', got: %q", output)
	}
	if !strings.Contains(output, "dave") {
		t.Errorf("expected output to mention worker name 'dave', got: %q", output)
	}
	if !strings.Contains(output, "dispatcher") {
		t.Errorf("expected output to mention posting 'dispatcher', got: %q", output)
	}
}

// 7.2: after setting posting, config.json has worker_postings entry
func TestCrewPostCLI_ConfigHasWorkerPostings(t *testing.T) {
	townRoot, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"
	captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "dispatcher"}); err != nil {
			t.Fatalf("runCrewPost: %v", err)
		}
	})

	settingsPath := filepath.Join(townRoot, "testrig", "settings", "config.json")
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.WorkerPostings["dave"]; got != "dispatcher" {
		t.Errorf("WorkerPostings[dave] = %q, want %q", got, "dispatcher")
	}
}

// 7.3: --clear removes posting and shows removed name
func TestCrewPostCLI_ClearRemovesPosting(t *testing.T) {
	townRoot, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	// Set a posting first
	captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "dispatcher"}); err != nil {
			t.Fatalf("set: %v", err)
		}
	})

	// Now clear it
	crewPostClear = true
	output := captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave"}); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})

	if !strings.Contains(output, "Cleared") {
		t.Errorf("expected 'Cleared' in output, got: %q", output)
	}
	if !strings.Contains(output, "dispatcher") {
		t.Errorf("expected removed posting name 'dispatcher' in output, got: %q", output)
	}

	// Verify config
	settingsPath := filepath.Join(townRoot, "testrig", "settings", "config.json")
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.WorkerPostings["dave"]; ok {
		t.Error("WorkerPostings[dave] should not exist after clear")
	}
}

// 7.4: worker_postings key absent after clear (not empty {})
func TestCrewPostCLI_ClearRemovesKeyEntirely(t *testing.T) {
	townRoot, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	// Set and clear
	captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "dispatcher"}); err != nil {
			t.Fatalf("set: %v", err)
		}
	})
	crewPostClear = true
	captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave"}); err != nil {
			t.Fatalf("clear: %v", err)
		}
	})

	// Read raw JSON and verify worker_postings key is absent
	settingsPath := filepath.Join(townRoot, "testrig", "settings", "config.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["worker_postings"]; ok {
		t.Errorf("worker_postings key should be absent from JSON, got: %s", string(raw["worker_postings"]))
	}
}

// 7.5: update message old -> new
func TestCrewPostCLI_UpdateMessage(t *testing.T) {
	_, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	// Set initial
	captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "dispatcher"}); err != nil {
			t.Fatalf("set initial: %v", err)
		}
	})

	// Update to inspector
	output := captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave", "inspector"}); err != nil {
			t.Fatalf("update: %v", err)
		}
	})

	if !strings.Contains(output, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %q", output)
	}
	if !strings.Contains(output, "dispatcher") {
		t.Errorf("expected old posting 'dispatcher' in output, got: %q", output)
	}
	if !strings.Contains(output, "inspector") {
		t.Errorf("expected new posting 'inspector' in output, got: %q", output)
	}
}

// 7.6: non-existent template succeeds with warning showing define-at paths and removal hint
func TestCrewPostCLI_NonExistentTemplateWarns(t *testing.T) {
	_, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	var runErr error
	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			runErr = runCrewPost(&cobra.Command{}, []string{"dave", "tactician"})
		})
	})

	if runErr != nil {
		t.Fatalf("expected success for non-existent template, got error: %v", runErr)
	}
	if !strings.Contains(stderr, "Warning") {
		t.Errorf("expected warning on stderr for missing template, got: %q", stderr)
	}
	if !strings.Contains(stderr, "tactician") {
		t.Errorf("expected posting name 'tactician' in warning, got: %q", stderr)
	}

	// Verify define-at path hints
	if !strings.Contains(stderr, "Define it at:") {
		t.Errorf("expected 'Define it at:' in warning, got: %q", stderr)
	}
	if !strings.Contains(stderr, "Rig:") {
		t.Errorf("expected rig path hint in warning, got: %q", stderr)
	}
	if !strings.Contains(stderr, "postings/tactician.md.tmpl") {
		t.Errorf("expected template path 'postings/tactician.md.tmpl' in warning, got: %q", stderr)
	}

	// Verify removal hint
	if !strings.Contains(stderr, "Or remove: gt crew post dave --clear") {
		t.Errorf("expected removal hint 'Or remove: gt crew post dave --clear' in warning, got: %q", stderr)
	}

	// Verify built-in postings list is included (spec 7.6)
	if !strings.Contains(stderr, "Available built-in postings:") {
		t.Errorf("expected 'Available built-in postings:' in warning, got: %q", stderr)
	}
	for _, name := range []string{"dispatcher", "inspector", "scout"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("expected built-in posting %q in warning, got: %q", name, stderr)
		}
	}
}

// 7.7: NEG no args
func TestCrewPostCLI_NEG_NoArgs(t *testing.T) {
	cmd := crewPostCmd
	crewPostClear = false
	defer func() { crewPostClear = false }()

	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "requires 2 arguments") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// 7.8: NEG --clear with no posting set
func TestCrewPostCLI_NEG_ClearNoPosting(t *testing.T) {
	_, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"
	crewPostClear = true

	output := captureStdout(t, func() {
		if err := runCrewPost(&cobra.Command{}, []string{"dave"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No posting set") {
		t.Errorf("expected 'No posting set' message, got: %q", output)
	}
}

// 7.9: NEG nonexistent worker
func TestCrewPostCLI_NEG_NonexistentWorker(t *testing.T) {
	_, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	err := runCrewPost(&cobra.Command{}, []string{"nonexistent-worker", "dispatcher"})
	if err == nil {
		t.Fatal("expected error for nonexistent worker")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// 7.10: NEG empty name
func TestCrewPostCLI_NEG_EmptyName(t *testing.T) {
	_, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	err := runCrewPost(&cobra.Command{}, []string{"dave", ""})
	if err == nil {
		t.Fatal("expected error for empty posting name")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got: %v", err)
	}
}

// 7.11: NEG --clear with extra arg
func TestCrewPostCLI_NEG_ClearWithExtraArg(t *testing.T) {
	cmd := crewPostCmd
	crewPostClear = true
	defer func() { crewPostClear = false }()

	err := cmd.Args(cmd, []string{"dave", "dispatcher"})
	if err == nil {
		t.Fatal("expected error for --clear with extra arg")
	}
	if !strings.Contains(err.Error(), "--clear requires exactly 1 argument") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// 7.12: NEG path traversal stored as-is with warning
func TestCrewPostCLI_NEG_PathTraversalStoredWithWarning(t *testing.T) {
	townRoot, cleanup := setupTestTownForCrewPost(t, "testrig", "dave")
	defer cleanup()

	crewRig = "testrig"

	var runErr error
	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			runErr = runCrewPost(&cobra.Command{}, []string{"dave", "../../etc/passwd"})
		})
	})

	// crew post stores the name as-is (no validation on the name itself)
	// but warns that no template exists
	if runErr != nil {
		t.Fatalf("expected success (stored as-is), got error: %v", runErr)
	}
	if !strings.Contains(stderr, "Warning") {
		t.Errorf("expected warning on stderr for path traversal name, got: %q", stderr)
	}

	// Verify it was stored
	settingsPath := filepath.Join(townRoot, "testrig", "settings", "config.json")
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.WorkerPostings["dave"]; got != "../../etc/passwd" {
		t.Errorf("WorkerPostings[dave] = %q, want %q", got, "../../etc/passwd")
	}
}
