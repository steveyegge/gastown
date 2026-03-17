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
// Section 12: Handoff and lifecycle tests (gt-3d0)
//
// clearPostingOnCrewHandoff() only deletes .runtime/posting for crew roles.
// Polecats exit via gt done, not gt handoff. Their posting persists because
// nothing deletes the file; the next session inherits it via gt prime.
// ---------------------------------------------------------------------------

// 12.1: crew handoff clears .runtime/posting
func TestLifecycle_CrewHandoffClearsPosting(t *testing.T) {
	t.Setenv("GT_ROLE", "gastown/crew/max")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if err := posting.Write(tmpDir, "dispatcher"); err != nil {
		t.Fatalf("posting.Write: %v", err)
	}
	if got := posting.Read(tmpDir); got != "dispatcher" {
		t.Fatalf("posting before handoff = %q, want %q", got, "dispatcher")
	}

	clearPostingOnCrewHandoff()

	if got := posting.Read(tmpDir); got != "" {
		t.Errorf("posting after crew handoff = %q, want empty", got)
	}
}

// 12.2: no posting file present, handoff produces no error
func TestLifecycle_CrewHandoffNoFileNoError(t *testing.T) {
	t.Setenv("GT_ROLE", "gastown/crew/bear")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// No posting file exists — should not error
	clearPostingOnCrewHandoff()

	if got := posting.Read(tmpDir); got != "" {
		t.Errorf("posting.Read = %q, want empty", got)
	}
}

// 12.3: persistent posting unaffected by crew handoff
func TestLifecycle_PersistentPostingUnaffectedByHandoff(t *testing.T) {
	t.Setenv("GT_ROLE", "gastown/crew/diesel")

	townRoot := t.TempDir()
	rigName := "gastown"
	workerName := "diesel"

	// Set up persistent posting in rig settings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "inspector"}
	data, _ := json.Marshal(settings)
	settingsPath := filepath.Join(settingsDir, "config.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Also write a session posting
	workDir := filepath.Join(rigPath, "crew", workerName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Simulate handoff clearing — crew handoff clears session posting
	t.Chdir(workDir)
	clearPostingOnCrewHandoff()

	// Session posting should be cleared
	if got := posting.Read(workDir); got != "" {
		t.Errorf("session posting after handoff = %q, want empty", got)
	}

	// Persistent posting in config should be untouched
	loaded, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadRigSettings: %v", err)
	}
	if got := loaded.WorkerPostings[workerName]; got != "inspector" {
		t.Errorf("persistent posting after handoff = %q, want %q", got, "inspector")
	}
}

// 12.4: polecat new session picks up existing .runtime/posting
func TestLifecycle_PolecatPicksUpExistingPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate a pre-existing posting file from a previous session
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	// New session: resolvePostingName should pick up the existing file
	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "scout" {
		t.Errorf("resolvePostingName() = %q, want %q", name, "scout")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}
}

// 12.5: polecat handoff preserves .runtime/posting
func TestLifecycle_PolecatHandoffPreservesPosting(t *testing.T) {
	t.Setenv("GT_ROLE", "gastown/polecats/Toast")

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if err := posting.Write(tmpDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	// clearPostingOnCrewHandoff only clears for crew — polecat is unaffected
	clearPostingOnCrewHandoff()

	if got := posting.Read(tmpDir); got != "inspector" {
		t.Errorf("posting after polecat handoff = %q, want %q", got, "inspector")
	}
}

// 12.6: polecat done clears posting (via posting.Clear)
func TestLifecycle_PolecatDoneClearsPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "dispatcher" {
		t.Fatalf("posting before done = %q, want %q", got, "dispatcher")
	}

	// gt done calls posting.Clear on the polecat's workdir as part of cleanup
	if err := posting.Clear(workDir); err != nil {
		t.Fatalf("posting.Clear: %v", err)
	}

	if got := posting.Read(workDir); got != "" {
		t.Errorf("posting after done = %q, want empty", got)
	}

	// .runtime/posting file should not exist
	postingPath := filepath.Join(workDir, ".runtime", "posting")
	if _, err := os.Stat(postingPath); !os.IsNotExist(err) {
		t.Errorf("expected .runtime/posting to not exist after done, err = %v", err)
	}
}

// 12.7: crash/respawn: file survives, picked up by gt prime
func TestLifecycle_CrashRespawnFilesSurvive(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "alpha"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write posting (simulating state before crash)
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify the file is on disk (survives process death)
	postingPath := filepath.Join(workDir, ".runtime", "posting")
	data, err := os.ReadFile(postingPath)
	if err != nil {
		t.Fatalf("posting file should survive on disk: %v", err)
	}
	if got := string(data); got != "dispatcher\n" {
		t.Errorf("raw posting file = %q, want %q", got, "dispatcher\n")
	}

	// Simulate respawn: new session reads from disk via resolvePostingName
	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Errorf("after respawn, resolvePostingName() = %q, want %q", name, "dispatcher")
	}
	if level != "session" {
		t.Errorf("after respawn, level = %q, want %q", level, "session")
	}
}

// 12.8: witness role with posting file: handoff preserves .runtime/posting
func TestLifecycle_WitnessHandoffPreservesPosting(t *testing.T) {
	t.Setenv("GT_ROLE", "gastown/witness")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if err := posting.Write(tmpDir, "inspector"); err != nil {
		t.Fatalf("posting.Write: %v", err)
	}

	clearPostingOnCrewHandoff()

	if got := posting.Read(tmpDir); got != "inspector" {
		t.Errorf("posting after witness handoff = %q, want %q", got, "inspector")
	}
}

// ---------------------------------------------------------------------------
// Section 13: Session launch clears stale posting (gt-jdv)
//
// When a polecat is spawned or reused via gt sling, stale .runtime/posting
// files from prior sessions must be cleared before gt prime runs.
// If --posting was explicitly passed, the new posting replaces the stale one.
// ---------------------------------------------------------------------------

// 13.1: session launch without --posting clears stale .runtime/posting
func TestLifecycle_SessionLaunchClearsStalePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulate stale posting from a prior session
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "scout" {
		t.Fatalf("stale posting = %q, want %q", got, "scout")
	}

	// Simulate what SessionManager.Start() does: clear stale posting
	if err := posting.Clear(workDir); err != nil {
		t.Fatalf("posting.Clear: %v", err)
	}
	// No --posting flag, so no write

	if got := posting.Read(workDir); got != "" {
		t.Errorf("posting after session launch (no --posting) = %q, want empty", got)
	}

	// .runtime/posting file should not exist
	postingPath := filepath.Join(workDir, ".runtime", "posting")
	if _, err := os.Stat(postingPath); !os.IsNotExist(err) {
		t.Errorf("expected .runtime/posting to not exist, err = %v", err)
	}
}

// 13.2: session launch with --posting overwrites stale .runtime/posting
func TestLifecycle_SessionLaunchWithPostingOverwritesStale(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulate stale posting from a prior session
	if err := posting.Write(workDir, "old-stale-posting"); err != nil {
		t.Fatal(err)
	}

	// Simulate what SessionManager.Start() does: clear then write new posting
	if err := posting.Clear(workDir); err != nil {
		t.Fatalf("posting.Clear: %v", err)
	}
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatalf("posting.Write: %v", err)
	}

	if got := posting.Read(workDir); got != "dispatcher" {
		t.Errorf("posting after session launch (--posting dispatcher) = %q, want %q", got, "dispatcher")
	}
}

// 13.3: session launch with no pre-existing posting and no --posting is clean
func TestLifecycle_SessionLaunchCleanNoPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// No stale posting exists. Clear is a no-op.
	if err := posting.Clear(workDir); err != nil {
		t.Fatalf("posting.Clear on empty dir: %v", err)
	}

	if got := posting.Read(workDir); got != "" {
		t.Errorf("posting = %q, want empty", got)
	}
}

// 13.4: reuse path clears stale posting (existing polecat, no fresh spawn)
func TestLifecycle_ReuseClearsStalePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulate stale posting from a prior manual test
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	// Simulate the sling reuse path: clear stale posting, no --posting flag
	_ = posting.Clear(workDir)

	if got := posting.Read(workDir); got != "" {
		t.Errorf("posting after reuse (no --posting) = %q, want empty", got)
	}
}

// 12.9: no GT_ROLE set with posting file: handoff preserves .runtime/posting
func TestLifecycle_NoRoleHandoffPreservesPosting(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if err := posting.Write(tmpDir, "dispatcher"); err != nil {
		t.Fatalf("posting.Write: %v", err)
	}

	clearPostingOnCrewHandoff()

	if got := posting.Read(tmpDir); got != "dispatcher" {
		t.Errorf("posting with no GT_ROLE = %q, want %q", got, "dispatcher")
	}
}
