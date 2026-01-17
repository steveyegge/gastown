//go:build integration

package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// TestInstallCreatesCorrectStructure validates that a fresh gt install
// creates the expected directory structure and configuration files.
func TestInstallCreatesCorrectStructure(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	// Build gt binary for testing
	gtBinary := buildGT(t)

	// Run gt install
	cmd := exec.Command(gtBinary, "install", hqPath, "--name", "test-town")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify directory structure
	assertDirExists(t, hqPath, "HQ root")
	assertDirExists(t, filepath.Join(hqPath, "mayor"), "mayor/")

	// Verify mayor/town.json
	townPath := filepath.Join(hqPath, "mayor", "town.json")
	assertFileExists(t, townPath, "mayor/town.json")

	townConfig, err := config.LoadTownConfig(townPath)
	if err != nil {
		t.Fatalf("failed to load town.json: %v", err)
	}
	if townConfig.Type != "town" {
		t.Errorf("town.json type = %q, want %q", townConfig.Type, "town")
	}
	if townConfig.Name != "test-town" {
		t.Errorf("town.json name = %q, want %q", townConfig.Name, "test-town")
	}

	// Verify mayor/rigs.json
	rigsPath := filepath.Join(hqPath, "mayor", "rigs.json")
	assertFileExists(t, rigsPath, "mayor/rigs.json")

	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("failed to load rigs.json: %v", err)
	}
	if len(rigsConfig.Rigs) != 0 {
		t.Errorf("rigs.json should be empty, got %d rigs", len(rigsConfig.Rigs))
	}

	// Verify CLAUDE.md exists in mayor/ (not town root, to avoid inheritance pollution)
	claudePath := filepath.Join(hqPath, "mayor", "CLAUDE.md")
	assertFileExists(t, claudePath, "mayor/CLAUDE.md")

	// Verify Claude settings exist in mayor/.claude/ (not town root/.claude/)
	// Mayor settings go here to avoid polluting child workspaces via directory traversal
	mayorSettingsPath := filepath.Join(hqPath, "mayor", ".claude", "settings.json")
	assertFileExists(t, mayorSettingsPath, "mayor/.claude/settings.json")

	// Verify deacon settings exist in deacon/.claude/
	deaconSettingsPath := filepath.Join(hqPath, "deacon", ".claude", "settings.json")
	assertFileExists(t, deaconSettingsPath, "deacon/.claude/settings.json")
}

// TestInstallBeadsHasCorrectPrefix validates that beads is initialized
// with the correct "hq-" prefix for town-level beads.
func TestInstallBeadsHasCorrectPrefix(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping beads prefix test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	// Build gt binary for testing
	gtBinary := buildGT(t)

	// Run gt install (includes beads init by default)
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify .beads/ directory exists
	beadsDir := filepath.Join(hqPath, ".beads")
	assertDirExists(t, beadsDir, ".beads/")

	// Verify beads database was created
	dbPath := filepath.Join(beadsDir, "beads.db")
	assertFileExists(t, dbPath, ".beads/beads.db")

	// Verify prefix by running bd config get issue_prefix
	// Use --no-daemon to avoid daemon startup issues in test environment
	bdCmd := exec.Command("bd", "--no-daemon", "config", "get", "issue_prefix")
	bdCmd.Dir = hqPath
	prefixOutput, err := bdCmd.Output() // Use Output() to get only stdout
	if err != nil {
		// If Output() fails, try CombinedOutput for better error info
		combinedOut, _ := exec.Command("bd", "--no-daemon", "config", "get", "issue_prefix").CombinedOutput()
		t.Fatalf("bd config get issue_prefix failed: %v\nOutput: %s", err, combinedOut)
	}

	prefix := strings.TrimSpace(string(prefixOutput))
	if prefix != "hq" {
		t.Errorf("beads issue_prefix = %q, want %q", prefix, "hq")
	}
}

// TestInstallTownRoleSlots validates that town-level agent beads
// have their role slot set after install.
func TestInstallTownRoleSlots(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping role slot test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install (includes beads init by default)
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Log install output for CI debugging
	t.Logf("gt install output:\n%s", output)

	// Verify beads directory was created
	beadsDir := filepath.Join(hqPath, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Fatalf("beads directory not created at %s", beadsDir)
	}

	// List beads for debugging
	listCmd := exec.Command("bd", "--no-daemon", "list", "--type=agent")
	listCmd.Dir = hqPath
	listOutput, _ := listCmd.CombinedOutput()
	t.Logf("bd list --type=agent output:\n%s", listOutput)

	assertSlotValue(t, hqPath, "hq-mayor", "role", "hq-mayor-role")
	assertSlotValue(t, hqPath, "hq-deacon", "role", "hq-deacon-role")
}

// TestInstallIdempotent validates that running gt install twice
// on the same directory fails without --force flag.
func TestInstallIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// First install should succeed
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("first install failed: %v\nOutput: %s", err, output)
	}

	// Second install without --force should fail
	cmd = exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("second install should have failed without --force")
	}
	if !strings.Contains(string(output), "already a Gas Town HQ") {
		t.Errorf("expected 'already a Gas Town HQ' error, got: %s", output)
	}

	// Third install with --force should succeed
	cmd = exec.Command(gtBinary, "install", hqPath, "--no-beads", "--force")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install with --force failed: %v\nOutput: %s", err, output)
	}
}

// TestInstallFormulasProvisioned validates that embedded formulas are copied
// to .beads/formulas/ during installation.
func TestInstallFormulasProvisioned(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping formulas test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install (includes beads and formula provisioning)
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify .beads/formulas/ directory exists
	formulasDir := filepath.Join(hqPath, ".beads", "formulas")
	assertDirExists(t, formulasDir, ".beads/formulas/")

	// Verify at least some expected formulas exist
	expectedFormulas := []string{
		"mol-deacon-patrol.formula.toml",
		"mol-refinery-patrol.formula.toml",
		"code-review.formula.toml",
	}
	for _, f := range expectedFormulas {
		formulaPath := filepath.Join(formulasDir, f)
		assertFileExists(t, formulaPath, f)
	}

	// Verify the count matches embedded formulas
	entries, err := os.ReadDir(formulasDir)
	if err != nil {
		t.Fatalf("failed to read formulas dir: %v", err)
	}
	// Count only formula files (not directories)
	var fileCount int
	for _, e := range entries {
		if !e.IsDir() {
			fileCount++
		}
	}
	// Should have at least 20 formulas (allows for some variation)
	if fileCount < 20 {
		t.Errorf("expected at least 20 formulas, got %d", fileCount)
	}
}

// TestInstallWrappersInExistingTown validates that --wrappers works in an
// existing town without requiring --force or recreating HQ structure.
func TestInstallWrappersInExistingTown(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	binDir := filepath.Join(tmpDir, "bin")

	// Create bin directory for wrappers
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	gtBinary := buildGT(t)

	// First: create HQ without wrappers
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("first install failed: %v\nOutput: %s", err, output)
	}

	// Verify town.json exists (proves HQ was created)
	townPath := filepath.Join(hqPath, "mayor", "town.json")
	assertFileExists(t, townPath, "mayor/town.json")

	// Get modification time of town.json before wrapper install
	townInfo, err := os.Stat(townPath)
	if err != nil {
		t.Fatalf("failed to stat town.json: %v", err)
	}
	townModBefore := townInfo.ModTime()

	// Second: install --wrappers in same directory (should not recreate HQ)
	cmd = exec.Command(gtBinary, "install", hqPath, "--wrappers")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install --wrappers in existing town failed: %v\nOutput: %s", err, output)
	}

	// Verify town.json was NOT modified (HQ was not recreated)
	townInfo, err = os.Stat(townPath)
	if err != nil {
		t.Fatalf("failed to stat town.json after wrapper install: %v", err)
	}
	if townInfo.ModTime() != townModBefore {
		t.Errorf("town.json was modified during --wrappers install, HQ should not be recreated")
	}

	// Verify output mentions wrapper installation
	if !strings.Contains(string(output), "gt-codex") && !strings.Contains(string(output), "gt-opencode") {
		t.Errorf("expected output to mention wrappers, got: %s", output)
	}
}

// TestInstallNoBeadsFlag validates that --no-beads skips beads initialization.
func TestInstallNoBeadsFlag(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install with --no-beads
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install --no-beads failed: %v\nOutput: %s", err, output)
	}

	// Verify .beads/ directory does NOT exist
	beadsDir := filepath.Join(hqPath, ".beads")
	if _, err := os.Stat(beadsDir); !os.IsNotExist(err) {
		t.Errorf(".beads/ should not exist with --no-beads flag")
	}
}

// buildGT builds the gt binary and returns its path.
// It caches the build across tests in the same run.
var cachedGTBinary string

func buildGT(t *testing.T) string {
	t.Helper()

	if cachedGTBinary != "" {
		// Verify cached binary still exists
		if _, err := os.Stat(cachedGTBinary); err == nil {
			return cachedGTBinary
		}
		// Binary was cleaned up, rebuild
		cachedGTBinary = ""
	}

	// Find project root (where go.mod is)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up to find go.mod
	projectRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatal("could not find project root (go.mod)")
		}
		projectRoot = parent
	}

	// Build gt binary to a persistent temp location (not per-test)
	tmpDir := os.TempDir()
	tmpBinary := filepath.Join(tmpDir, "gt-integration-test")
	cmd := exec.Command("go", "build", "-o", tmpBinary, "./cmd/gt")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build gt: %v\nOutput: %s", err, output)
	}

	cachedGTBinary = tmpBinary
	return tmpBinary
}

// assertDirExists checks that the given path exists and is a directory.
func assertDirExists(t *testing.T, path, name string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("%s does not exist: %v", name, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", name)
	}
}

// assertFileExists checks that the given path exists and is a file.
func assertFileExists(t *testing.T, path, name string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Errorf("%s does not exist: %v", name, err)
		return
	}
	if info.IsDir() {
		t.Errorf("%s is a directory, expected file", name)
	}
}

func assertSlotValue(t *testing.T, townRoot, issueID, slot, want string) {
	t.Helper()
	cmd := exec.Command("bd", "--no-daemon", "--json", "slot", "show", issueID)
	cmd.Dir = townRoot
	output, err := cmd.Output()
	if err != nil {
		debugCmd := exec.Command("bd", "--no-daemon", "--json", "slot", "show", issueID)
		debugCmd.Dir = townRoot
		combined, _ := debugCmd.CombinedOutput()
		t.Fatalf("bd slot show %s failed: %v\nOutput: %s", issueID, err, combined)
	}

	var parsed struct {
		Slots map[string]*string `json:"slots"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("parsing slot show output failed: %v\nOutput: %s", err, output)
	}

	var got string
	if value, ok := parsed.Slots[slot]; ok && value != nil {
		got = *value
	}
	if got != want {
		t.Fatalf("slot %s for %s = %q, want %q", slot, issueID, got, want)
	}
}

// TestInstallProtectedBranchesDefault validates that branch protection is
// enabled by default with main,master.
func TestInstallProtectedBranchesDefault(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping protected branches test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install (default: --protected-branches=main,master)
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify output mentions protected branches
	if !strings.Contains(string(output), "Protected branches") {
		t.Errorf("expected output to mention protected branches, got: %s", output)
	}

	// Verify TownSettings has protected branches
	settingsPath := filepath.Join(hqPath, "settings", "config.json")
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load town settings: %v", err)
	}

	if len(settings.ProtectedBranches) != 2 {
		t.Errorf("expected 2 protected branches, got %d: %v", len(settings.ProtectedBranches), settings.ProtectedBranches)
	}

	// Verify pre-push hook exists and blocks main
	hookPath := filepath.Join(hqPath, ".githooks", "pre-push")
	assertFileExists(t, hookPath, ".githooks/pre-push")

	hookContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed to read pre-push hook: %v", err)
	}

	if !strings.Contains(string(hookContent), "main|master") {
		t.Errorf("pre-push hook should contain main|master pattern")
	}

	// Verify hook is executable
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("failed to stat hook: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("pre-push hook should be executable")
	}
}

// TestInstallProtectedBranchesCustom validates custom protected branches.
func TestInstallProtectedBranchesCustom(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping protected branches test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install with custom protected branches
	cmd := exec.Command(gtBinary, "install", hqPath, "--protected-branches=production,staging")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify TownSettings has custom protected branches
	settingsPath := filepath.Join(hqPath, "settings", "config.json")
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load town settings: %v", err)
	}

	if len(settings.ProtectedBranches) != 2 {
		t.Errorf("expected 2 protected branches, got %d", len(settings.ProtectedBranches))
	}
	if settings.ProtectedBranches[0] != "production" {
		t.Errorf("expected first branch to be 'production', got %q", settings.ProtectedBranches[0])
	}
	if settings.ProtectedBranches[1] != "staging" {
		t.Errorf("expected second branch to be 'staging', got %q", settings.ProtectedBranches[1])
	}

	// Verify pre-push hook has custom pattern
	hookPath := filepath.Join(hqPath, ".githooks", "pre-push")
	hookContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed to read pre-push hook: %v", err)
	}

	if !strings.Contains(string(hookContent), "production|staging") {
		t.Errorf("pre-push hook should contain production|staging pattern")
	}
}

// TestInstallProtectedBranchesDisabled validates disabling protection.
func TestInstallProtectedBranchesDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install with empty protected branches (disabled)
	cmd := exec.Command(gtBinary, "install", hqPath, "--protected-branches=", "--no-beads")
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Verify output does NOT mention protected branches
	if strings.Contains(string(output), "Protected branches:") {
		t.Errorf("expected output to NOT mention protected branches when disabled")
	}

	// Verify pre-push hook does NOT exist
	hookPath := filepath.Join(hqPath, ".githooks", "pre-push")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("pre-push hook should not exist when protection is disabled")
	}
}

// TestInstallProtectedBranchesRoleBeads validates role beads get merge policy.
func TestInstallProtectedBranchesRoleBeads(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping role beads test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// Run gt install with default protection
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Check that polecat role bead has merge policy
	bdCmd := exec.Command("bd", "--no-daemon", "show", "hq-polecat-role")
	bdCmd.Dir = hqPath
	roleOutput, err := bdCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd show hq-polecat-role failed: %v\nOutput: %s", err, roleOutput)
	}

	roleStr := string(roleOutput)
	if !strings.Contains(roleStr, "merge_policy_require_approval: true") {
		t.Errorf("polecat role should have merge_policy_require_approval: true\nGot: %s", roleStr)
	}
	if !strings.Contains(roleStr, "merge_policy_blocked_targets: main,master") {
		t.Errorf("polecat role should have merge_policy_blocked_targets: main,master\nGot: %s", roleStr)
	}
}

// TestConfigProtectedBranchesCommand validates the config command.
func TestConfigProtectedBranchesCommand(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping config command test")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")

	gtBinary := buildGT(t)

	// First install with default protection
	cmd := exec.Command(gtBinary, "install", hqPath)
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test: show current protected branches
	cmd = exec.Command(gtBinary, "config", "protected-branches")
	cmd.Dir = hqPath
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt config protected-branches failed: %v\nOutput: %s", err, output)
	}
	if !strings.Contains(string(output), "main") {
		t.Errorf("expected output to show 'main', got: %s", output)
	}

	// Test: change protected branches
	cmd = exec.Command(gtBinary, "config", "protected-branches", "production")
	cmd.Dir = hqPath
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt config protected-branches production failed: %v\nOutput: %s", err, output)
	}

	// Verify settings were updated
	settingsPath := filepath.Join(hqPath, "settings", "config.json")
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load town settings: %v", err)
	}
	if len(settings.ProtectedBranches) != 1 || settings.ProtectedBranches[0] != "production" {
		t.Errorf("expected [production], got %v", settings.ProtectedBranches)
	}

	// Verify hook was regenerated
	hookPath := filepath.Join(hqPath, ".githooks", "pre-push")
	hookContent, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("failed to read hook: %v", err)
	}
	if !strings.Contains(string(hookContent), "production)") {
		t.Errorf("hook should contain production pattern")
	}

	// Test: clear protection
	cmd = exec.Command(gtBinary, "config", "protected-branches", "--clear")
	cmd.Dir = hqPath
	cmd.Env = append(os.Environ(), "HOME="+tmpDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt config protected-branches --clear failed: %v\nOutput: %s", err, output)
	}

	// Verify settings were cleared
	settings, err = config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("failed to load town settings: %v", err)
	}
	if len(settings.ProtectedBranches) != 0 {
		t.Errorf("expected empty protected branches after --clear, got %v", settings.ProtectedBranches)
	}
}
