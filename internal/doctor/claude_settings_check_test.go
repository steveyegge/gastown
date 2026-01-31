package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClaudeSettingsCheck(t *testing.T) {
	check := NewClaudeSettingsCheck()

	if check.Name() != "claude-settings" {
		t.Errorf("expected name 'claude-settings', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestClaudeSettingsCheck_NoSettingsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no settings files, got %v", result.Status)
	}
}

// createSettingsFile creates a settings file at the given path.
func createSettingsFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	content := `{"enabledPlugins": ["plugin1"]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// Tests for valid settings.local.json in correct working directories

func TestClaudeSettingsCheck_ValidMayorSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid mayor settings at correct location with correct filename
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createSettingsFile(t, mayorSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidDeaconSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid deacon settings at correct location with correct filename
	deaconSettings := filepath.Join(tmpDir, "deacon", ".claude", "settings.local.json")
	createSettingsFile(t, deaconSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid deacon settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidWitnessSettingsInWitnessDir(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Working dir is witness/ when witness/rig/ doesn't exist
	// settings.local.json in witness/ is correct
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", ".claude", "settings.local.json")
	createSettingsFile(t, witnessSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid witness settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidWitnessSettingsInWitnessRigDir(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// When witness/rig/ exists, working dir is witness/rig/
	// settings.local.json in witness/rig/ is correct
	witnessRigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(witnessRigDir, 0755); err != nil {
		t.Fatal(err)
	}
	witnessSettings := filepath.Join(witnessRigDir, ".claude", "settings.local.json")
	createSettingsFile(t, witnessSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid witness settings in rig dir, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidRefinerySettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Working dir is refinery/rig/ (always)
	// settings.local.json in refinery/rig/ is correct
	refinerySettings := filepath.Join(tmpDir, rigName, "refinery", "rig", ".claude", "settings.local.json")
	createSettingsFile(t, refinerySettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid refinery settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidCrewSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Working dir is crew/<name>/ (always)
	// settings.local.json in crew/<name>/ is correct
	crewSettings := filepath.Join(tmpDir, rigName, "crew", "agent1", ".claude", "settings.local.json")
	createSettingsFile(t, crewSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid crew settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidPolecatSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Working dir is polecats/<name>/<rig>/ (always)
	// settings.local.json in polecats/<name>/<rig>/ is correct
	pcSettings := filepath.Join(tmpDir, rigName, "polecats", "pc1", rigName, ".claude", "settings.local.json")
	createSettingsFile(t, pcSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid polecat settings, got %v: %s", result.Status, result.Message)
	}
}

// Tests for stale settings.json (old filename)

func TestClaudeSettingsCheck_OldFilenameInMayor(t *testing.T) {
	tmpDir := t.TempDir()

	// settings.json is the OLD filename - should be marked stale
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createSettingsFile(t, mayorSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for old filename, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "old filename") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention old filename, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_OldFilenameInWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// settings.json in working dir (crew/<name>/) is stale - should use settings.local.json
	crewSettings := filepath.Join(tmpDir, rigName, "crew", "agent1", ".claude", "settings.json")
	createSettingsFile(t, crewSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for old filename, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "old filename") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention old filename, got %v", result.Details)
	}
}

// Tests for wrong location (parent directory instead of working directory)

func TestClaudeSettingsCheck_WrongLocationRefinery(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Settings in refinery/ parent dir is WRONG - should be in refinery/rig/
	wrongSettings := filepath.Join(tmpDir, rigName, "refinery", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationWitness(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// When witness/rig/ exists, settings in witness/ are WRONG
	witnessRigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(witnessRigDir, 0755); err != nil {
		t.Fatal(err)
	}
	wrongSettings := filepath.Join(tmpDir, rigName, "witness", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationCrew(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Settings in crew/ parent dir is WRONG - should be in crew/<name>/
	wrongSettings := filepath.Join(tmpDir, rigName, "crew", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationPolecat(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Settings in polecats/ parent dir is WRONG - should be in polecats/<name>/<rig>/
	wrongSettings := filepath.Join(tmpDir, rigName, "polecats", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationPolecatIntermediate(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Settings in polecats/<name>/ is also WRONG - should be in polecats/<name>/<rig>/
	wrongSettings := filepath.Join(tmpDir, rigName, "polecats", "pc1", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

// Tests for town root settings (always wrong)

func TestClaudeSettingsCheck_TownRootSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Settings at town root is ALWAYS wrong (pollutes all agents)
	wrongSettings := filepath.Join(tmpDir, ".claude", "settings.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for town root settings, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") && strings.Contains(d, "mayor") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location for mayor, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_TownRootCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()

	// CLAUDE.md at town root is WRONG (pollutes all agents)
	wrongCLAUDEmd := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(wrongCLAUDEmd, []byte("# Mayor Context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for town root CLAUDE.md, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "CLAUDE.md") && strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention CLAUDE.md wrong location, got %v", result.Details)
	}
}

// Tests for multiple stale files

func TestClaudeSettingsCheck_MultipleStaleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create multiple stale files
	// 1. Old filename in mayor
	createSettingsFile(t, filepath.Join(tmpDir, "mayor", ".claude", "settings.json"))
	// 2. Wrong location for refinery (parent dir instead of refinery/rig/)
	createSettingsFile(t, filepath.Join(tmpDir, rigName, "refinery", ".claude", "settings.local.json"))
	// 3. Old filename in crew working dir
	createSettingsFile(t, filepath.Join(tmpDir, rigName, "crew", "agent1", ".claude", "settings.json"))

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for multiple stale files, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "3 stale") {
		t.Errorf("expected message about 3 stale files, got %q", result.Message)
	}
}

// Tests for Fix behavior

func TestClaudeSettingsCheck_FixDeletesStaleFile(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create stale settings in wrong location (parent dir)
	wrongSettings := filepath.Join(tmpDir, rigName, "refinery", ".claude", "settings.local.json")
	createSettingsFile(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected stale settings to be deleted")
	}

	// Verify check passes after fix
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_FixDeletesTownRootFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale files at town root
	staleTownRootSettings := filepath.Join(tmpDir, ".claude", "settings.json")
	createSettingsFile(t, staleTownRootSettings)

	staleCLAUDEmd := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(staleCLAUDEmd, []byte("# Mayor Context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify settings.json was deleted
	if _, err := os.Stat(staleTownRootSettings); !os.IsNotExist(err) {
		t.Error("expected town root settings.json to be deleted")
	}

	// Verify CLAUDE.md was deleted
	if _, err := os.Stat(staleCLAUDEmd); !os.IsNotExist(err) {
		t.Error("expected town root CLAUDE.md to be deleted")
	}
}

func TestClaudeSettingsCheck_SkipsNonRigDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be skipped
	for _, skipDir := range []string{"mayor", "deacon", "daemon", ".git", "docs", ".hidden"} {
		dir := filepath.Join(tmpDir, skipDir, "witness", "rig", ".claude")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// These should NOT be detected as rig witness settings
		settingsPath := filepath.Join(dir, "settings.json")
		createSettingsFile(t, settingsPath)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	_ = check.Run(ctx)

	// Should only find mayor and deacon settings (if any) - not nested witness dirs
	// In this case, we're putting settings in mayor/witness/rig/.claude/ which is invalid
	// The skip logic should prevent these from being found as "rig" witness settings
	if len(check.staleSettings) != 0 {
		t.Errorf("expected 0 stale files from skipped dirs, got %d", len(check.staleSettings))
	}
}

// Git status tests

// initTestGitRepo initializes a git repo in the given directory for settings tests.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

// gitAddAndCommit adds and commits a file.
func gitAddAndCommit(t *testing.T, repoDir, filePath string) {
	t.Helper()
	// Get relative path from repo root
	relPath, err := filepath.Rel(repoDir, filePath)
	if err != nil {
		t.Fatal(err)
	}

	cmds := [][]string{
		{"git", "add", relPath},
		{"git", "commit", "-m", "Add file"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestClaudeSettingsCheck_GitStatusUntracked(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create an untracked stale settings file (old filename)
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for stale file, got %v", result.Status)
	}
	// Should mention "untracked"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "untracked") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention untracked, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_GitStatusTrackedClean(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it (tracked, clean)
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)
	gitAddAndCommit(t, rigDir, staleSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for stale file, got %v", result.Status)
	}
	// Should mention "tracked but unmodified"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "tracked but unmodified") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention tracked but unmodified, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_GitStatusTrackedModified(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)
	gitAddAndCommit(t, rigDir, staleSettings)

	// Modify the file after commit
	if err := os.WriteFile(staleSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for stale file, got %v", result.Status)
	}
	// Should mention "local modifications"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "local modifications") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention local modifications, got %v", result.Details)
	}
	// Should also mention manual review
	if !strings.Contains(result.FixHint, "manual review") {
		t.Errorf("expected fix hint to mention manual review, got %q", result.FixHint)
	}
}

func TestClaudeSettingsCheck_FixSkipsModifiedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)
	gitAddAndCommit(t, rigDir, staleSettings)

	// Modify the file after commit
	if err := os.WriteFile(staleSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should NOT delete the modified file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file still exists (was skipped)
	if _, err := os.Stat(staleSettings); os.IsNotExist(err) {
		t.Error("expected modified file to be preserved, but it was deleted")
	}
}

func TestClaudeSettingsCheck_FixDeletesUntrackedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create an untracked stale settings file (not git added)
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should delete the untracked file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(staleSettings); !os.IsNotExist(err) {
		t.Error("expected untracked file to be deleted")
	}
}

func TestClaudeSettingsCheck_FixDeletesTrackedCleanFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it (tracked, clean)
	staleSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createSettingsFile(t, staleSettings)
	gitAddAndCommit(t, rigDir, staleSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should delete the tracked clean file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(staleSettings); !os.IsNotExist(err) {
		t.Error("expected tracked clean file to be deleted")
	}
}
