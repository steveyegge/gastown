package crew

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// newIsolatedBeads creates a beads instance in tmpDir and inits it.
// Skips the test if bd binary is not available.
func newIsolatedBeads(t *testing.T) *beads.Beads {
	t.Helper()
	tmpDir := t.TempDir()
	b := beads.NewIsolated(tmpDir)
	if err := b.Init("test"); err != nil {
		t.Skipf("bd init failed (no bd binary): %v", err)
	}
	return b
}

// writePersonaFile writes content to <dir>/.personas/<name>.md, creating dirs as needed.
func writePersonaFile(t *testing.T, dir, name, content string) {
	t.Helper()
	personaDir := filepath.Join(dir, ".personas")
	if err := os.MkdirAll(personaDir, 0o755); err != nil {
		t.Fatalf("mkdir .personas: %v", err)
	}
	path := filepath.Join(personaDir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write persona file %s: %v", path, err)
	}
}

func TestSyncPersonasFromFiles_CreatesBeads(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	writePersonaFile(t, rigDir, "rust-expert", "# Rust Expert\n\nYou are a Rust expert.\n")
	writePersonaFile(t, townDir, "go-dev", "# Go Dev\n\nYou write Go.\n")

	updated, err := SyncPersonasFromFiles(townDir, rigDir, "test", "myrig", b, false)
	if err != nil {
		t.Fatalf("SyncPersonasFromFiles: %v", err)
	}

	// Both personas should have been created
	if len(updated) != 2 {
		t.Errorf("expected 2 updated personas, got %d: %v", len(updated), updated)
	}

	// Rig-level bead should exist
	rigBeadID := beads.PersonaBeadID("test", "myrig", "rust-expert")
	if _, err := b.Show(rigBeadID); err != nil {
		t.Errorf("rig persona bead %q not found: %v", rigBeadID, err)
	}

	// Town-level bead should exist (rig="" for town-level)
	townBeadID := beads.PersonaBeadID("test", "", "go-dev")
	if _, err := b.Show(townBeadID); err != nil {
		t.Errorf("town persona bead %q not found: %v", townBeadID, err)
	}
}

func TestSyncPersonasFromFiles_SkipsOnSameHash(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	content := "# Alice\n\nYou are Alice.\n"
	writePersonaFile(t, rigDir, "alice", content)

	// First sync — should create
	first, err := SyncPersonasFromFiles(townDir, rigDir, "test", "rig", b, false)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(first) != 1 {
		t.Errorf("expected 1 updated on first sync, got %d", len(first))
	}

	// Second sync with same file — should be a no-op
	second, err := SyncPersonasFromFiles(townDir, rigDir, "test", "rig", b, false)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("expected 0 updated on second sync (same hash), got %d: %v", len(second), second)
	}
}

func TestSyncPersonasFromFiles_UpdatesOnHashChange(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	writePersonaFile(t, rigDir, "alice", "# Alice v1\n\nOld content.\n")

	if _, err := SyncPersonasFromFiles(townDir, rigDir, "test", "rig", b, false); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Update the file
	writePersonaFile(t, rigDir, "alice", "# Alice v2\n\nNew content.\n")

	updated, err := SyncPersonasFromFiles(townDir, rigDir, "test", "rig", b, false)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(updated) != 1 || updated[0] != "alice" {
		t.Errorf("expected [alice] updated, got %v", updated)
	}
}

func TestSyncPersonasFromFiles_RigOverridesTown(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	// Same name at both levels
	rigContent := "# Shared (rig)\n\nRig version.\n"
	townContent := "# Shared (town)\n\nTown version.\n"
	writePersonaFile(t, rigDir, "shared", rigContent)
	writePersonaFile(t, townDir, "shared", townContent)

	updated, err := SyncPersonasFromFiles(townDir, rigDir, "test", "myrig", b, false)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Only rig-level bead should be created (town skipped because rigSet["shared"]=true)
	if len(updated) != 1 {
		t.Errorf("expected 1 updated (rig wins), got %d: %v", len(updated), updated)
	}

	// Rig-level bead should have rig content (not town content)
	rigBeadID := beads.PersonaBeadID("test", "myrig", "shared")
	content, err := beads.GetPersonaContent(b, rigBeadID)
	if err != nil {
		t.Fatalf("GetPersonaContent: %v", err)
	}
	if content != rigContent {
		t.Errorf("rig bead content = %q, want %q", content, rigContent)
	}

	// A second sync should be a no-op (same hash, rig still wins)
	second, err := SyncPersonasFromFiles(townDir, rigDir, "test", "myrig", b, false)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("expected no updates on second sync, got %v", second)
	}
}

func TestSyncPersonasFromFiles_MissingDirSilent(t *testing.T) {
	b := newIsolatedBeads(t)
	// Directories that exist but have no .personas/ subdirectory
	rigDir := t.TempDir()
	townDir := t.TempDir()

	updated, err := SyncPersonasFromFiles(townDir, rigDir, "test", "rig", b, false)
	if err != nil {
		t.Errorf("expected no error for missing .personas dirs, got: %v", err)
	}
	if len(updated) != 0 {
		t.Errorf("expected empty result, got %v", updated)
	}
}

func TestEnsurePersonaBeadExists_FindsExistingBead(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	content := "# Rust Expert\n\nYou are Rust.\n"
	hash := beads.PersonaBeadID("test", "rig", "rust-expert") // just a unique string for hash
	id, _, err := beads.EnsurePersonaBead(b, "test", "rig", "rust-expert", content, hash)
	if err != nil {
		t.Fatalf("EnsurePersonaBead: %v", err)
	}

	got, err := EnsurePersonaBeadExists(townDir, rigDir, "test", "rig", "rust-expert", b)
	if err != nil {
		t.Fatalf("EnsurePersonaBeadExists: %v", err)
	}
	if got != id {
		t.Errorf("EnsurePersonaBeadExists = %q, want %q", got, id)
	}
}

func TestEnsurePersonaBeadExists_BootstrapsFromFile(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	writePersonaFile(t, rigDir, "go-dev", "# Go Dev\n\nYou write Go.\n")

	id, err := EnsurePersonaBeadExists(townDir, rigDir, "test", "rig", "go-dev", b)
	if err != nil {
		t.Fatalf("EnsurePersonaBeadExists: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty bead ID after bootstrap")
	}
	expectedID := beads.PersonaBeadID("test", "rig", "go-dev")
	if id != expectedID {
		t.Errorf("bead ID = %q, want %q", id, expectedID)
	}
}

func TestEnsurePersonaBeadExists_ErrorIfNotFound(t *testing.T) {
	b := newIsolatedBeads(t)
	rigDir := t.TempDir()
	townDir := t.TempDir()

	_, err := EnsurePersonaBeadExists(townDir, rigDir, "test", "rig", "nonexistent", b)
	if err == nil {
		t.Error("expected error when persona not found anywhere")
	}
}
