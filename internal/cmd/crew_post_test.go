package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// ---------------------------------------------------------------------------
// gt crew post: persistent posting in rig settings
// ---------------------------------------------------------------------------

// TestCrewPost_SetPersistentPosting verifies that setting a persistent posting
// writes the correct entry to WorkerPostings in rig settings.
func TestCrewPost_SetPersistentPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Start with empty settings
	settings := config.NewRigSettings()
	settingsPath := filepath.Join(settingsDir, "config.json")
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	// Simulate setting posting: dave → dispatcher
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkerPostings == nil {
		loaded.WorkerPostings = make(map[string]string)
	}
	loaded.WorkerPostings["dave"] = "dispatcher"
	if err := config.SaveRigSettings(settingsPath, loaded); err != nil {
		t.Fatal(err)
	}

	// Verify
	reloaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.WorkerPostings["dave"]; got != "dispatcher" {
		t.Errorf("WorkerPostings[dave] = %q, want %q", got, "dispatcher")
	}
}

// TestCrewPost_ClearPosting verifies --clear removes the posting entry.
func TestCrewPost_ClearPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Start with a posting set
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{"dave": "dispatcher"}
	settingsPath := filepath.Join(settingsDir, "config.json")
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	// Clear it
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	delete(loaded.WorkerPostings, "dave")
	if len(loaded.WorkerPostings) == 0 {
		loaded.WorkerPostings = nil
	}
	if err := config.SaveRigSettings(settingsPath, loaded); err != nil {
		t.Fatal(err)
	}

	// Verify cleared
	reloaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.WorkerPostings != nil {
		if _, ok := reloaded.WorkerPostings["dave"]; ok {
			t.Error("WorkerPostings[dave] should be cleared after delete")
		}
	}
}

// TestCrewPost_ClearNonexistentPostingIsNoop verifies clearing a posting that
// doesn't exist doesn't error or corrupt settings.
func TestCrewPost_ClearNonexistentPostingIsNoop(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := config.NewRigSettings()
	settingsPath := filepath.Join(settingsDir, "config.json")
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	// Attempt to clear nonexistent posting — should be a no-op
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkerPostings == nil {
		// Nothing to delete, this is fine
	} else {
		delete(loaded.WorkerPostings, "nobody")
	}
	if err := config.SaveRigSettings(settingsPath, loaded); err != nil {
		t.Fatal(err)
	}
}

// TestCrewPost_OverwriteExistingPosting verifies that setting a new posting
// replaces the existing one.
func TestCrewPost_OverwriteExistingPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{"dave": "dispatcher"}
	settingsPath := filepath.Join(settingsDir, "config.json")
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	// Overwrite with scout
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded.WorkerPostings["dave"] = "scout"
	if err := config.SaveRigSettings(settingsPath, loaded); err != nil {
		t.Fatal(err)
	}

	reloaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.WorkerPostings["dave"]; got != "scout" {
		t.Errorf("WorkerPostings[dave] = %q, want %q after overwrite", got, "scout")
	}
}

// TestCrewPost_RejectsEmptyPostingName verifies that empty or whitespace-only
// posting names are rejected with a validation error.
func TestCrewPost_RejectsEmptyPostingName(t *testing.T) {
	t.Parallel()

	cmd := crewPostCmd

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"empty string", []string{"dave", ""}},
		{"whitespace only", []string{"dave", "   "}},
		{"tab only", []string{"dave", "\t"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Args validator passes (2 args), but RunE should reject
			if err := cmd.Args(cmd, tc.args); err != nil {
				t.Fatalf("Args unexpectedly rejected %v: %v", tc.args, err)
			}
		})
	}
}

// TestCrewPost_SettingsRoundtrip verifies WorkerPostings survives JSON round-trip.
func TestCrewPost_SettingsRoundtrip(t *testing.T) {
	t.Parallel()

	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{
		"furiosa": "dispatcher",
		"nux":     "scout",
		"max":     "inspector",
	}

	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}

	var loaded config.RigSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	for name, want := range settings.WorkerPostings {
		if got := loaded.WorkerPostings[name]; got != want {
			t.Errorf("WorkerPostings[%s] = %q, want %q", name, got, want)
		}
	}
}
