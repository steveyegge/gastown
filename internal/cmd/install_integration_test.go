//go:build e2e

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

	// Verify beads database was initialized (both metadata.json and dolt/ exist with dolt backend)
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	assertFileExists(t, metadataPath, ".beads/metadata.json")
	doltDir := filepath.Join(beadsDir, "dolt")
	assertDirExists(t, doltDir, ".beads/dolt/")

	// Verify prefix by running bd config get issue_prefix
	bdCmd := exec.Command("bd", "config", "get", "issue_prefix")
	bdCmd.Dir = hqPath
	prefixOutput, err := bdCmd.Output() // Use Output() to get only stdout
	if err != nil {
		// If Output() fails, try CombinedOutput for better error info
		combinedOut, _ := exec.Command("bd", "config", "get", "issue_prefix").CombinedOutput()
		t.Fatalf("bd config get issue_prefix failed: %v\nOutput: %s", err, combinedOut)
	}

	prefix := strings.TrimSpace(string(prefixOutput))
	if prefix != "hq" {
		t.Errorf("beads issue_prefix = %q, want %q", prefix, "hq")
	}
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
	cmd := exec.Command("bd", "--json", "slot", "show", issueID)
	cmd.Dir = townRoot
	output, err := cmd.Output()
	if err != nil {
		debugCmd := exec.Command("bd", "--json", "slot", "show", issueID)
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

// TestInstallDoctorClean validates that gt install creates a functional system.
// This test verifies:
// 1. gt install succeeds with proper structure
// 2. gt rig add succeeds
// 3. gt crew add succeeds
// 4. Basic commands work
//
// NOTE: Full doctor --fix verification is currently limited by known issues:
// - Doctor fix has bugs with bead creation (UNIQUE constraint errors)
// - Container environment lacks tmux for session checks
// - Test repos don't satisfy priming expectations (AGENTS.md length)
//
// TODO: Enable full doctor verification once these issues are resolved.
func TestInstallDoctorClean(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Clean environment for predictable behavior
	env := cleanE2EEnv()
	env = append(env, "HOME="+tmpDir)

	// Kill any stale dolt from previous test BEFORE install to avoid port 3307 conflict.
	_ = exec.Command("pkill", "-f", "dolt sql-server").Run()

	// Set up git identity in the test's temp HOME so EnsureDoltIdentity can copy it.
	configureGitIdentity(t, env)

	// 1. Install town with git (now includes dolt identity, HQ init, server start)
	t.Run("install", func(t *testing.T) {
		runGTCmd(t, gtBinary, tmpDir, env, "install", hqPath, "--name", "test-town", "--git")
	})
	t.Cleanup(func() {
		cmd := exec.Command(gtBinary, "dolt", "stop")
		cmd.Dir = hqPath
		cmd.Env = env
		_ = cmd.Run()
	})

	// 2. Verify core structure exists
	t.Run("verify-structure", func(t *testing.T) {
		assertDirExists(t, filepath.Join(hqPath, "mayor"), "mayor/")
		assertDirExists(t, filepath.Join(hqPath, "deacon"), "deacon/")
		assertDirExists(t, filepath.Join(hqPath, ".beads"), ".beads/")
		assertFileExists(t, filepath.Join(hqPath, "mayor", "town.json"), "mayor/town.json")
		assertFileExists(t, filepath.Join(hqPath, "mayor", "rigs.json"), "mayor/rigs.json")
	})

	// 3. Verify install bootstrapped dolt (identity, HQ database, server)
	t.Run("verify-dolt-bootstrap", func(t *testing.T) {
		// HQ database should exist
		hqDoltDir := filepath.Join(hqPath, ".dolt-data", "hq", ".dolt")
		assertDirExists(t, hqDoltDir, ".dolt-data/hq/.dolt")

		// Dolt identity should have been copied from git config
		nameCmd := exec.Command("dolt", "config", "--global", "--get", "user.name")
		nameCmd.Env = env
		nameOut, err := nameCmd.Output()
		if err != nil {
			t.Fatalf("dolt config --get user.name failed: %v", err)
		}
		if got := strings.TrimSpace(string(nameOut)); got != "Test User" {
			t.Errorf("dolt user.name = %q, want %q", got, "Test User")
		}

		// Dolt server should be reachable
		cmd := exec.Command(gtBinary, "dolt", "status")
		cmd.Dir = hqPath
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("dolt status failed (server may not be running): %v\n%s", err, out)
		}
		if strings.Contains(string(out), "not running") {
			t.Errorf("expected dolt server to be running, got: %s", out)
		}

		// init-rig hq again should be idempotent (no error, "already exists" message)
		initCmd := exec.Command(gtBinary, "dolt", "init-rig", "hq")
		initCmd.Dir = hqPath
		initCmd.Env = env
		initOut, err := initCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("re-running init-rig hq should be idempotent, got error: %v\n%s", err, initOut)
		}
		if !strings.Contains(string(initOut), "already exists") {
			t.Errorf("expected 'already exists' message, got: %s", initOut)
		}
	})

	// 4. Add a small public repo as a rig (CLI rejects local paths)
	t.Run("rig-add", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "rig", "add", "testrig",
			"https://github.com/octocat/Hello-World.git", "--prefix", "tr")
	})

	// 5. Verify rig structure exists
	t.Run("verify-rig-structure", func(t *testing.T) {
		rigPath := filepath.Join(hqPath, "testrig")
		assertDirExists(t, rigPath, "testrig/")
		assertDirExists(t, filepath.Join(rigPath, "witness"), "testrig/witness/")
		assertDirExists(t, filepath.Join(rigPath, "refinery"), "testrig/refinery/")
		assertDirExists(t, filepath.Join(rigPath, ".repo.git"), "testrig/.repo.git/")
	})

	// 5b. Verify rig beads: no orphaned Dolt database, metadata shares HQ
	t.Run("verify-rig-beads", func(t *testing.T) {
		assertRigBeadsNoOrphan(t, hqPath, "testrig", "tr")
	})

	// 6. Add a crew member
	t.Run("crew-add", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "crew", "add", "jayne", "--rig", "testrig")
	})

	// 7. Verify crew structure exists
	t.Run("verify-crew-structure", func(t *testing.T) {
		crewPath := filepath.Join(hqPath, "testrig", "crew", "jayne")
		assertDirExists(t, crewPath, "testrig/crew/jayne/")
	})

	// 8. Basic commands should work
	// Note: mail inbox and hook are omitted — fresh Dolt databases lack the
	// issues table, so these commands error with "table not found" until
	// the first bead is created.
	t.Run("commands", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "rig", "list")
		runGTCmd(t, gtBinary, hqPath, env, "crew", "list", "--rig", "testrig")
	})

	// 9. Doctor runs without crashing (may have warnings/errors but should not panic)
	t.Run("doctor-runs", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "doctor", "-v")
		cmd.Dir = hqPath
		cmd.Env = env
		out, _ := cmd.CombinedOutput()
		outStr := string(out)
		t.Logf("Doctor output:\n%s", outStr)
		// Fail on crashes/panics even though we tolerate doctor errors
		for _, signal := range []string{"panic:", "SIGSEGV", "runtime error"} {
			if strings.Contains(outStr, signal) {
				t.Fatalf("doctor crashed with %s", signal)
			}
		}
		// Regression: initBeads orphan bug should not create beads_<prefix> in .dolt-data/
		if strings.Contains(outStr, "⚠  dolt-orphaned-databases") {
			t.Errorf("doctor reports orphaned databases after rig-add (initBeads regression):\n%s", outStr)
		}
	})
}

// TestInstallWithDaemon validates that gt install creates a functional system
// with the daemon running. This extends TestInstallDoctorClean by:
// 1. Starting the daemon after install
// 2. Verifying the daemon is healthy
// 3. Running basic operations with daemon support
func TestInstallWithDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Clean environment for predictable behavior
	env := cleanE2EEnv()
	env = append(env, "HOME="+tmpDir)

	// Kill any stale dolt from previous test BEFORE install to avoid port 3307 conflict.
	_ = exec.Command("pkill", "-f", "dolt sql-server").Run()

	// Set up git identity in the test's temp HOME so EnsureDoltIdentity can copy it.
	configureGitIdentity(t, env)

	// 1. Install town with git (now includes dolt identity, HQ init, server start)
	t.Run("install", func(t *testing.T) {
		runGTCmd(t, gtBinary, tmpDir, env, "install", hqPath, "--name", "test-town", "--git")
	})
	t.Cleanup(func() {
		cmd := exec.Command(gtBinary, "dolt", "stop")
		cmd.Dir = hqPath
		cmd.Env = env
		_ = cmd.Run()
	})

	// 2. Start daemon
	t.Run("daemon-start", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "daemon", "start")
	})
	t.Cleanup(func() {
		cmd := exec.Command(gtBinary, "daemon", "stop")
		cmd.Dir = hqPath
		cmd.Env = env
		_ = cmd.Run()
	})

	// 4. Verify daemon is running
	t.Run("daemon-status", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "daemon", "status")
		cmd.Dir = hqPath
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("daemon status failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "running") {
			t.Errorf("expected daemon to be running, got: %s", out)
		}
	})

	// 5. Add a small public repo as a rig (CLI rejects local paths)
	t.Run("rig-add", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "rig", "add", "testrig",
			"https://github.com/octocat/Hello-World.git", "--prefix", "tr")
	})

	// 5b. Verify rig beads: no orphaned Dolt database, metadata shares HQ
	t.Run("verify-rig-beads", func(t *testing.T) {
		assertRigBeadsNoOrphan(t, hqPath, "testrig", "tr")
	})

	// 6. Add crew member
	t.Run("crew-add", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "crew", "add", "jayne", "--rig", "testrig")
	})

	// 7. Verify commands work with daemon running
	// Note: mail inbox and hook are omitted — fresh Dolt databases lack the
	// issues table, so these commands error with "table not found" until
	// the first bead is created.
	t.Run("commands", func(t *testing.T) {
		runGTCmd(t, gtBinary, hqPath, env, "rig", "list")
		runGTCmd(t, gtBinary, hqPath, env, "crew", "list", "--rig", "testrig")
	})

	// 8. Verify daemon shows in doctor output
	t.Run("doctor-daemon-check", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "doctor", "-v")
		cmd.Dir = hqPath
		cmd.Env = env
		out, _ := cmd.CombinedOutput()
		outStr := string(out)
		t.Logf("Doctor output:\n%s", outStr)
		// Fail on crashes/panics even though we tolerate doctor errors
		for _, signal := range []string{"panic:", "SIGSEGV", "runtime error"} {
			if strings.Contains(outStr, signal) {
				t.Fatalf("doctor crashed with %s", signal)
			}
		}
		// Regression: initBeads orphan bug should not create beads_<prefix> in .dolt-data/
		if strings.Contains(outStr, "⚠  dolt-orphaned-databases") {
			t.Errorf("doctor reports orphaned databases after rig-add (initBeads regression):\n%s", outStr)
		}
	})
}

// cleanE2EEnv returns os.Environ() with all GT_* variables removed.
// This ensures tests don't inherit stale role environment from CI or previous tests.
func cleanE2EEnv() []string {
	var clean []string
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GT_") {
			clean = append(clean, env)
		}
	}
	return clean
}

// configureGitIdentity sets git global config in the test's temp HOME directory.
// Tests override HOME to a temp dir for isolation, so git/dolt can't find the
// container's build-time global config. EnsureDoltIdentity copies from git config,
// so git identity must be available before gt install.
func configureGitIdentity(t *testing.T, env []string) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "--global", "user.name", "Test User"},
		{"config", "--global", "user.email", "test@test.com"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

// runGTCmd runs a gt command and fails the test if it fails.
func runGTCmd(t *testing.T, binary, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt %v failed: %v\n%s", args, err, out)
	}
}

// assertRigBeadsNoOrphan verifies that gt rig add for a non-tracked rig correctly
// sets up the beads directory without creating a standalone Dolt database.
//
// Non-tracked rigs (no .beads/ in the source repo) share the HQ Dolt database.
// Previously, initBeads ran bd init which created beads_<prefix> in .dolt-data/,
// then EnsureMetadataWithDB immediately overwrote metadata.json to point at HQ,
// leaving beads_<prefix> as an orphan. This helper asserts the regression is absent.
func assertRigBeadsNoOrphan(t *testing.T, hqPath, rigName, prefix string) {
	t.Helper()
	beadsDir := filepath.Join(hqPath, rigName, ".beads")

	// config.yaml must exist with the configured prefix
	configPath := filepath.Join(beadsDir, "config.yaml")
	assertFileExists(t, configPath, rigName+"/.beads/config.yaml")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s/.beads/config.yaml: %v", rigName, err)
	}
	if !strings.Contains(string(configContent), prefix) {
		t.Errorf("%s/.beads/config.yaml should contain prefix %q, got:\n%s", rigName, prefix, configContent)
	}

	// metadata.json must point at the HQ database, not a per-rig standalone database.
	// Non-tracked rigs share HQ so that beads dispatched from HQ are visible to bd ready/list.
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	assertFileExists(t, metadataPath, rigName+"/.beads/metadata.json")
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read %s/.beads/metadata.json: %v", rigName, err)
	}
	var metadata struct {
		DoltDatabase string `json:"dolt_database"`
	}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("parse %s/.beads/metadata.json: %v\nContent: %s", rigName, err, metadataBytes)
	}
	if metadata.DoltDatabase != "hq" {
		t.Errorf("%s/.beads/metadata.json dolt_database = %q, want %q (non-tracked rigs share HQ)", rigName, metadata.DoltDatabase, "hq")
	}

	// No standalone beads_<prefix> database should exist in .dolt-data/.
	// Only the HQ database ("hq") is expected there for a fresh install + rig-add.
	doltDataDir := filepath.Join(hqPath, ".dolt-data")
	entries, err := os.ReadDir(doltDataDir)
	if err != nil {
		t.Fatalf("read .dolt-data/: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "beads_") {
			t.Errorf(".dolt-data/%s is an orphaned database: initBeads should not create standalone Dolt databases for non-tracked rigs", e.Name())
		}
	}
}
