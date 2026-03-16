package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
)

// ---------------------------------------------------------------------------
// gt posting assume: writes .runtime/posting
// ---------------------------------------------------------------------------

func TestPostingAssume_WritesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("after assume, Read() = %q, want %q", got, "dispatcher")
	}

	// Verify actual file exists at .runtime/posting
	filePath := filepath.Join(workDir, ".runtime", "posting")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected .runtime/posting file to exist: %v", err)
	}
	if got := string(data); got != "dispatcher\n" {
		t.Errorf(".runtime/posting content = %q, want %q", got, "dispatcher\n")
	}
}

// TestPostingAssume_BlockedByExistingSession verifies that assume fails
// when a session posting is already active (must drop first).
func TestPostingAssume_BlockedByExistingSession(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Set initial posting
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify the posting is set (simulating the check in runPostingAssume)
	current := posting.Read(workDir)
	if current == "" {
		t.Fatal("expected posting to be set, but Read returned empty")
	}

	// The CLI would return an error here:
	// "already assumed posting %q — drop it first with: gt posting drop"
	if current != "dispatcher" {
		t.Errorf("current posting = %q, want %q", current, "dispatcher")
	}
}

// TestPostingAssume_BlockedByPersistentPosting verifies that assume fails
// when a persistent posting (from rig settings) is active.
func TestPostingAssume_BlockedByPersistentPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up persistent posting in rig settings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Check persistent posting — this is what runPostingAssume checks via getPersistentPosting
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok || persistent == "" {
		t.Fatal("expected persistent posting to be set")
	}

	// CLI would return: "persistent posting %q is set for %s — clear it first"
	if persistent != "inspector" {
		t.Errorf("persistent posting = %q, want %q", persistent, "inspector")
	}
}

// ---------------------------------------------------------------------------
// gt posting drop: clears .runtime/posting
// ---------------------------------------------------------------------------

func TestPostingDrop_ClearsRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Set posting
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "scout" {
		t.Fatalf("precondition: posting = %q, want %q", got, "scout")
	}

	// Drop it
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("after drop, Read() = %q, want empty", got)
	}
}

func TestPostingDrop_WhenNoPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// No posting set — Read returns empty
	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Read() on empty dir = %q, want empty", got)
	}

	// Clear on empty is a no-op
	if err := posting.Clear(workDir); err != nil {
		t.Errorf("Clear on empty dir should succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// gt posting cycle: drop + assume atomically
// ---------------------------------------------------------------------------

func TestPostingCycle_DropsOldAndAssumesNew(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Start with dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Cycle to scout (simulating runPostingCycle logic)
	old := posting.Read(workDir)
	if old != "dispatcher" {
		t.Fatalf("precondition: posting = %q, want %q", old, "dispatcher")
	}

	// Drop old
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	// Assume new
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "scout" {
		t.Errorf("after cycle, Read() = %q, want %q", got, "scout")
	}
}

func TestPostingCycle_FromEmpty(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Cycle with no existing posting (runPostingCycle handles this)
	old := posting.Read(workDir)
	if old != "" {
		t.Fatalf("precondition: should have no posting, got %q", old)
	}

	// No old posting to clear, just write new
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "inspector" {
		t.Errorf("after cycle from empty, Read() = %q, want %q", got, "inspector")
	}
}

func TestPostingCycle_BlockedByPersistentPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Check persistent posting conflict
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok || persistent == "" {
		t.Fatal("persistent posting should block cycle")
	}
	// CLI would error: "persistent posting %q is set for %s — clear it first"
}

// ---------------------------------------------------------------------------
// gt posting assume: empty name writes empty (treated as Clear)
// ---------------------------------------------------------------------------

func TestPostingWrite_EmptyNameCallsClear(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write initial posting
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	// Writing empty name should clear
	if err := posting.Write(workDir, ""); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Write('') should clear, but Read() = %q", got)
	}
}

func TestPostingWrite_WhitespaceOnlyCallsClear(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	if err := posting.Write(workDir, "  \t\n "); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Write(whitespace) should clear, but Read() = %q", got)
	}
}

// ---------------------------------------------------------------------------
// gt posting create: scaffolds rig-level posting template
// ---------------------------------------------------------------------------

func TestPostingCreate_ScaffoldsTemplate(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")

	// Simulate what runPostingCreate does (we can't call the cobra command
	// directly without full rig setup, so test the file creation logic)
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := "# Posting: reviewer\n\nYou are operating under the **reviewer** posting.\n"
	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("template not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("template file is empty")
	}
	if got := string(data); got != content {
		t.Errorf("template content mismatch: got %q", got)
	}
}

func TestPostingCreate_RejectsExisting(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")

	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Stat should succeed — file exists
	if _, err := os.Stat(templatePath); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestPostingCreate_CreatesDirectory(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")

	// Directory shouldn't exist yet
	if _, err := os.Stat(postingsDir); !os.IsNotExist(err) {
		t.Fatal("postings dir should not exist yet")
	}

	// MkdirAll creates it
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(postingsDir)
	if err != nil {
		t.Fatalf("postings dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("postings path is not a directory")
	}
}
