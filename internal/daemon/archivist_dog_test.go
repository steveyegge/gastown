package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchivistDogInterval_Default(t *testing.T) {
	got := archivistDogInterval(nil)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_NilArchivistDog(t *testing.T) {
	config := &DaemonPatrolConfig{Patrols: &PatrolsConfig{}}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:     true,
				IntervalStr: "3m",
			},
		},
	}
	got := archivistDogInterval(config)
	if got != 3*time.Minute {
		t.Errorf("expected 3m, got %v", got)
	}
}

func TestArchivistDogInterval_InvalidFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v for invalid config, got %v", defaultArchivistDogInterval, got)
	}
}

func TestScanRigNotes_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results for empty dir, got %d", len(results))
	}
}

func TestScanRigNotes_NoNotes(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a rig directory with domain/ but no notes/
	rigDir := filepath.Join(tmpDir, "rigs", "my-rig", "domain")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results for rig without notes dir, got %d", len(results))
	}
}

func TestScanRigNotes_WithMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	notesDir := filepath.Join(tmpDir, "rigs", "backend", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create some markdown files
	for _, name := range []string{"auth-patterns.md", "api-design.md"} {
		if err := os.WriteFile(filepath.Join(notesDir, name), []byte("# test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create a non-md file that should be ignored
	if err := os.WriteFile(filepath.Join(notesDir, "scratch.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 1 {
		t.Fatalf("expected 1 rig with notes, got %d", len(results))
	}
	if results[0].Rig != "backend" {
		t.Errorf("expected rig 'backend', got %q", results[0].Rig)
	}
	if len(results[0].Files) != 2 {
		t.Errorf("expected 2 md files, got %d", len(results[0].Files))
	}
}

func TestScanRigNotes_MultipleRigs(t *testing.T) {
	tmpDir := t.TempDir()
	for _, rig := range []string{"backend", "frontend", "infra"} {
		notesDir := filepath.Join(tmpDir, "rigs", rig, "domain", "notes")
		if err := os.MkdirAll(notesDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(notesDir, "note.md"), []byte("# test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 3 {
		t.Errorf("expected 3 rigs with notes, got %d", len(results))
	}
}

func TestScanRigNotes_TopLevelLayout(t *testing.T) {
	tmpDir := t.TempDir()
	// Top-level rig layout (symlink pattern): <townRoot>/<rig>/domain/notes/
	notesDir := filepath.Join(tmpDir, "my-rig", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "note.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 1 {
		t.Fatalf("expected 1 rig with notes in top-level layout, got %d", len(results))
	}
	if results[0].Rig != "my-rig" {
		t.Errorf("expected rig 'my-rig', got %q", results[0].Rig)
	}
}

func TestScanRigNotes_HiddenFilesIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	notesDir := filepath.Join(tmpDir, "rigs", "backend", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Hidden file should be ignored
	if err := os.WriteFile(filepath.Join(notesDir, ".hidden.md"), []byte("# hidden"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results (hidden files only), got %d", len(results))
	}
}

func TestCooldown_Fresh(t *testing.T) {
	c := newArchivistDogCooldowns()
	if !c.canDispatch("backend") {
		t.Error("expected canDispatch=true for fresh rig")
	}
}

func TestCooldown_AfterDispatch(t *testing.T) {
	c := newArchivistDogCooldowns()
	c.markDispatched("backend")
	if c.canDispatch("backend") {
		t.Error("expected canDispatch=false immediately after dispatch")
	}
}

func TestCooldown_DifferentRigs(t *testing.T) {
	c := newArchivistDogCooldowns()
	c.markDispatched("backend")
	if !c.canDispatch("frontend") {
		t.Error("expected canDispatch=true for different rig")
	}
}

func TestCooldown_Expired(t *testing.T) {
	c := newArchivistDogCooldowns()
	// Manually set a time in the past
	c.mu.Lock()
	c.lastDispatched["backend"] = time.Now().Add(-archivistDogCooldownPerRig - time.Second)
	c.mu.Unlock()

	if !c.canDispatch("backend") {
		t.Error("expected canDispatch=true after cooldown expired")
	}
}

func TestArchivistAgentCommand_Default(t *testing.T) {
	got := archivistAgentCommand(nil)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistAgentCommand_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	got := archivistAgentCommand(config)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistAgentCommand_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:      true,
				AgentCommand: "/usr/local/bin/my-agent",
			},
		},
	}
	got := archivistAgentCommand(config)
	if got != "/usr/local/bin/my-agent" {
		t.Errorf("expected configured command, got %q", got)
	}
}

func TestArchivistAgentCommand_EmptyFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:      true,
				AgentCommand: "",
			},
		},
	}
	got := archivistAgentCommand(config)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q for empty config, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistPromptTemplate_Formatting(t *testing.T) {
	prompt := fmt.Sprintf(archivistPromptTemplate,
		"backend",
		"rigs/backend/domain/notes",
		"rigs/backend/domain",
		"auth-patterns.md, api-design.md",
	)
	if !strings.Contains(prompt, `"backend"`) {
		t.Error("prompt should contain rig name")
	}
	if !strings.Contains(prompt, "rigs/backend/domain/notes") {
		t.Error("prompt should contain notes dir")
	}
	if !strings.Contains(prompt, "auth-patterns.md, api-design.md") {
		t.Error("prompt should contain file list")
	}
}

func TestIsPatrolEnabled_ArchivistDog(t *testing.T) {
	// Nil config -> disabled (opt-in)
	if IsPatrolEnabled(nil, "archivist_dog") {
		t.Error("expected disabled for nil config")
	}

	// Explicitly enabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{Enabled: true},
		},
	}
	if !IsPatrolEnabled(config, "archivist_dog") {
		t.Error("expected enabled when config says enabled")
	}

	// Explicitly disabled
	config.Patrols.ArchivistDog.Enabled = false
	if IsPatrolEnabled(config, "archivist_dog") {
		t.Error("expected disabled when config says disabled")
	}
}
