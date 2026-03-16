package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/posting"
)

// ---------------------------------------------------------------------------
// resolvePostingName: 2-layer resolution precedence (session > config)
// ---------------------------------------------------------------------------

func TestResolvePostingName_SessionTakesPrecedence(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up rig config with persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.RigSettings{
		Type:           "rig-settings",
		Version:        1,
		WorkerPostings: map[string]string{workerName: "inspector"},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Set up work dir with session posting
	workDir := filepath.Join(rigPath, "polecats", workerName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  workerName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Errorf("resolvePostingName() name = %q, want %q (session should take precedence)", name, "dispatcher")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}
}

func TestResolvePostingName_FallsBackToConfig(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up rig config with persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.RigSettings{
		Type:           "rig-settings",
		Version:        1,
		WorkerPostings: map[string]string{workerName: "scout"},
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Work dir with NO session posting
	workDir := filepath.Join(rigPath, "polecats", workerName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  workerName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "scout" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "scout")
	}
	if level != "config" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "config")
	}
}

func TestResolvePostingName_NoPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	workDir := filepath.Join(townRoot, "testrig", "polecats", "Toast")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      "testrig",
		Polecat:  "Toast",
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "" {
		t.Errorf("resolvePostingName() name = %q, want empty", name)
	}
	if level != "" {
		t.Errorf("resolvePostingName() level = %q, want empty", level)
	}
}

func TestResolvePostingName_SessionOnlyNoConfig(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	workDir := filepath.Join(townRoot, "testrig", "polecats", "Toast")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Session posting but no rig settings file
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      "testrig",
		Polecat:  "Toast",
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "inspector" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "inspector")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}
}

// ---------------------------------------------------------------------------
// Conflict rules: session state interactions
// ---------------------------------------------------------------------------

func TestConflictRule_SessionPostingBlocksReassume(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write initial session posting
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify it's set
	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Fatalf("expected initial posting to be set, got %q", got)
	}

	// Writing over it directly works (Write is low-level)
	// The conflict check is in the CLI command layer (runPostingAssume),
	// but we test the state: if .runtime/posting exists, Read returns the value.
	got2 := posting.Read(workDir)
	if got2 == "" {
		t.Error("session posting should still exist after Read")
	}
}

func TestConflictRule_DropClearsSession(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("after Clear, Read() = %q, want empty", got)
	}
}

func TestConflictRule_CycleDropAndAssume(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulate gt posting cycle: drop + assume atomically
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}
	// Cycle: clear then write new
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "inspector" {
		t.Errorf("after cycle, Read() = %q, want %q", got, "inspector")
	}
}

func TestConflictRule_SessionPostingDoesNotAffectRuntimeDir(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write creates .runtime/ directory
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	info, err := os.Stat(runtimeDir)
	if err != nil {
		t.Fatalf(".runtime dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error(".runtime should be a directory")
	}

	// Clear removes the file but .runtime/ dir persists
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	// .runtime/ dir still exists (only the file is removed)
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		t.Error(".runtime dir should still exist after Clear (only file removed)")
	}
}

// ---------------------------------------------------------------------------
// GT_POSTING env injection — additional edge cases
// ---------------------------------------------------------------------------

func TestAgentEnv_PostingAllRoles(t *testing.T) {
	t.Parallel()
	// Verify GT_POSTING works for all worker-capable roles
	roles := []struct {
		role      string
		rig       string
		agentName string
	}{
		{"polecat", "myrig", "Toast"},
		{"crew", "myrig", "alice"},
	}
	for _, r := range roles {
		t.Run(r.role, func(t *testing.T) {
			t.Parallel()
			env := config.AgentEnv(config.AgentEnvConfig{
				Role:         r.role,
				Rig:          r.rig,
				AgentName:    r.agentName,
				Posting:      "dispatcher",
				PostingLevel: "embedded",
			})
			if got := env["GT_POSTING"]; got != "dispatcher" {
				t.Errorf("GT_POSTING = %q, want %q", got, "dispatcher")
			}
			if got := env["GT_POSTING_LEVEL"]; got != "embedded" {
				t.Errorf("GT_POSTING_LEVEL = %q, want %q", got, "embedded")
			}
		})
	}
}

func TestAgentEnv_PostingNotSetForNonWorkerRoles(t *testing.T) {
	t.Parallel()
	// Non-worker roles shouldn't have posting set (caller wouldn't set it),
	// but verify the env vars aren't present when Posting is empty
	roles := []string{"mayor", "deacon", "witness", "refinery"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			env := config.AgentEnv(config.AgentEnvConfig{
				Role:     role,
				Rig:      "myrig",
				TownRoot: "/town",
			})
			if _, ok := env["GT_POSTING"]; ok {
				t.Errorf("GT_POSTING should not be set for role %s", role)
			}
			if _, ok := env["GT_POSTING_LEVEL"]; ok {
				t.Errorf("GT_POSTING_LEVEL should not be set for role %s", role)
			}
		})
	}
}
