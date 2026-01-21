//go:build integration

// Package cmd contains integration tests for the rig command.
//
// Run with: go test -tags=integration ./internal/cmd -run TestRigAdd -v
package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

// createTestGitRepo creates a minimal git repository for testing.
// Returns the path to the bare repo URL (suitable for cloning).
func createTestGitRepo(t *testing.T, name string) string {
	t.Helper()

	// Create a regular repo with initial commit
	repoDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	// Initialize git repo with explicit main branch
	// (system default may vary, causing checkout failures)
	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create initial file and commit
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Return the path as a file:// URL
	return repoDir
}

// setupTestTown creates a minimal Gas Town workspace for testing.
// Returns townRoot and a cleanup function.
func setupTestTown(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()

	// Create mayor directory (required for rigs.json)
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	// Create empty rigs.json
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs:    make(map[string]config.RigEntry),
	}
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	// Create .beads directory for routes
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	return townRoot
}

// mockBdCommand creates a fake bd binary that simulates bd behavior.
// This avoids needing bd installed for tests.
func mockBdCommand(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	logPath := filepath.Join(binDir, "bd.log")

	// Create a script that simulates bd init and other commands
	// Also logs all create commands for verification.
	// Note: beads.run() prepends --no-daemon --allow-stale to all commands,
	// so we need to find the actual command in the argument list.
	script := `#!/bin/sh
# Mock bd for testing
LOG_FILE="` + logPath + `"

# Find the actual command (skip global flags like --no-daemon, --allow-stale)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;; # skip flags
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  init)
    # Create .beads directory and config.yaml
    mkdir -p .beads
    prefix="gt"
    # Handle both --prefix=value and --prefix value forms
    next_is_prefix=false
    for arg in "$@"; do
      if [ "$next_is_prefix" = true ]; then
        prefix="$arg"
        next_is_prefix=false
      else
        case "$arg" in
          --prefix=*) prefix="${arg#--prefix=}" ;;
          --prefix) next_is_prefix=true ;;
        esac
      fi
    done
    echo "prefix: $prefix" > .beads/config.yaml
    exit 0
    ;;
  migrate)
    exit 0
    ;;
  show)
    echo '{"error":"not found"}' >&2
    exit 1
    ;;
  create)
    # Log all create commands for verification
    echo "$@" >> "$LOG_FILE"
    # Extract the ID from --id=xxx argument
    bead_id=""
    for arg in "$@"; do
      case "$arg" in
        --id=*) bead_id="${arg#--id=}" ;;
      esac
    done
    # Return valid JSON for bead creation
    echo "{\"id\":\"$bead_id\",\"status\":\"open\",\"created_at\":\"2025-01-01T00:00:00Z\"}"
    exit 0
    ;;
  mol|list)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	// Prepend to PATH
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return logPath
}

// TestRigAddCreatesCorrectStructure verifies that gt rig add creates
// the expected directory structure.
func TestRigAddCreatesCorrectStructure(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "testproject")

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	// Create rig manager and add rig
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	_, err = mgr.AddRig(rig.AddRigOptions{
		Name:        "testrig",
		GitURL:      gitURL,
		BeadsPrefix: "tr",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	rigPath := filepath.Join(townRoot, "testrig")

	// Verify directory structure
	expectedDirs := []string{
		"",             // rig root
		"mayor",        // mayor container
		"mayor/rig",    // mayor clone
		"refinery",     // refinery container
		"refinery/rig", // refinery worktree
		"witness",      // witness dir
		"polecats",     // polecats dir
		"crew",         // crew dir
		".beads",       // beads dir
		"plugins",      // plugins dir
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(rigPath, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", dir)
		}
	}

	// Verify config.json exists
	configPath := filepath.Join(rigPath, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config.json not found: %v", err)
	}

	// Verify .repo.git (bare repo) exists
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if _, err := os.Stat(bareRepoPath); err != nil {
		t.Errorf(".repo.git not found: %v", err)
	}

	// Verify mayor/rig is a git repo
	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
	gitDirPath := filepath.Join(mayorRigPath, ".git")
	if _, err := os.Stat(gitDirPath); err != nil {
		t.Errorf("mayor/rig/.git not found: %v", err)
	}

	// Verify refinery/rig is a git worktree (has .git file pointing to bare repo)
	refineryRigPath := filepath.Join(rigPath, "refinery", "rig")
	refineryGitPath := filepath.Join(refineryRigPath, ".git")
	info, err := os.Stat(refineryGitPath)
	if err != nil {
		t.Errorf("refinery/rig/.git not found: %v", err)
	} else if info.IsDir() {
		t.Errorf("refinery/rig/.git should be a file (worktree), not a directory")
	}

	// Verify Claude settings are created in correct locations (outside git repos).
	// Settings in parent directories are inherited by agents via directory traversal,
	// without polluting the source repos.
	expectedSettings := []struct {
		path string
		desc string
	}{
		{filepath.Join(rigPath, "witness", ".claude", "settings.json"), "witness/.claude/settings.json"},
		{filepath.Join(rigPath, "refinery", ".claude", "settings.json"), "refinery/.claude/settings.json"},
		{filepath.Join(rigPath, "crew", ".claude", "settings.json"), "crew/.claude/settings.json"},
		{filepath.Join(rigPath, "polecats", ".claude", "settings.json"), "polecats/.claude/settings.json"},
	}

	for _, s := range expectedSettings {
		if _, err := os.Stat(s.path); err != nil {
			t.Errorf("%s not found: %v", s.desc, err)
		}
	}

	// Verify settings are NOT created inside source repos (these would be wrong)
	wrongLocations := []struct {
		path string
		desc string
	}{
		{filepath.Join(rigPath, "witness", "rig", ".claude", "settings.json"), "witness/rig/.claude (inside source repo)"},
		{filepath.Join(rigPath, "refinery", "rig", ".claude", "settings.json"), "refinery/rig/.claude (inside source repo)"},
	}

	for _, w := range wrongLocations {
		if _, err := os.Stat(w.path); err == nil {
			t.Errorf("%s should NOT exist (settings would pollute source repo)", w.desc)
		}
	}
}

// TestRigAddInitializesBeads verifies that beads is initialized with
// the correct prefix.
func TestRigAddInitializesBeads(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "beadstest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	newRig, err := mgr.AddRig(rig.AddRigOptions{
		Name:        "beadstest",
		GitURL:      gitURL,
		BeadsPrefix: "bt",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// Verify rig config has correct prefix
	if newRig.Config == nil {
		t.Fatal("rig.Config is nil")
	}
	if newRig.Config.Prefix != "bt" {
		t.Errorf("rig.Config.Prefix = %q, want %q", newRig.Config.Prefix, "bt")
	}

	// Verify .beads directory was created
	beadsDir := filepath.Join(townRoot, "beadstest", ".beads")
	if _, err := os.Stat(beadsDir); err != nil {
		t.Errorf(".beads directory not found: %v", err)
	}

	// Verify config.yaml exists with correct prefix
	configPath := filepath.Join(beadsDir, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf(".beads/config.yaml not found: %v", err)
	} else {
		content, err := os.ReadFile(configPath)
		if err != nil {
			t.Errorf("reading config.yaml: %v", err)
		} else if !strings.Contains(string(content), "prefix: bt") && !strings.Contains(string(content), "prefix:bt") {
			t.Errorf("config.yaml doesn't contain expected prefix, got: %s", string(content))
		}
	}

	// =========================================================================
	// IMPORTANT: Verify routes.jsonl does NOT exist in the rig's .beads directory
	// =========================================================================
	//
	// WHY WE DON'T CREATE routes.jsonl IN RIG DIRECTORIES:
	//
	// 1. BD'S WALK-UP ROUTING MECHANISM:
	//    When bd needs to find routing configuration, it walks up the directory
	//    tree looking for a .beads directory with routes.jsonl. It stops at the
	//    first routes.jsonl it finds. If a rig has its own routes.jsonl, bd will
	//    use that and NEVER reach the town-level routes.jsonl, breaking cross-rig
	//    routing entirely.
	//
	// 2. TOWN-LEVEL ROUTING IS THE SOURCE OF TRUTH:
	//    All routing configuration belongs in the town's .beads/routes.jsonl.
	//    This single file contains prefix->path mappings for ALL rigs, enabling
	//    bd to route issue IDs like "tr-123" to the correct rig directory.
	//
	// 3. HISTORICAL BUG - BD AUTO-EXPORT CORRUPTION:
	//    There was a bug where bd's auto-export feature would write issue data
	//    to routes.jsonl if issues.jsonl didn't exist. This corrupted routing
	//    config with issue JSON objects. We now create empty issues.jsonl files
	//    proactively to prevent this, but we also verify routes.jsonl doesn't
	//    exist as a defense-in-depth measure.
	//
	// 4. DOCTOR CHECK EXISTS:
	//    The "rig-routes-jsonl" doctor check detects and can fix (delete) any
	//    routes.jsonl files that appear in rig .beads directories.
	//
	// If you're modifying rig creation and thinking about adding routes.jsonl
	// to the rig's .beads directory - DON'T. It will break cross-rig routing.
	// =========================================================================
	rigRoutesPath := filepath.Join(beadsDir, "routes.jsonl")
	if _, err := os.Stat(rigRoutesPath); err == nil {
		t.Errorf("routes.jsonl should NOT exist in rig .beads directory (breaks bd walk-up routing)")
	}

	// Verify issues.jsonl DOES exist (prevents bd auto-export corruption)
	rigIssuesPath := filepath.Join(beadsDir, "issues.jsonl")
	if _, err := os.Stat(rigIssuesPath); err != nil {
		t.Errorf("issues.jsonl should exist in rig .beads directory (prevents auto-export corruption): %v", err)
	}
}

// TestRigAddUpdatesRoutes verifies that routes.jsonl is updated
// with the new rig's route.
func TestRigAddUpdatesRoutes(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "routetest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	newRig, err := mgr.AddRig(rig.AddRigOptions{
		Name:        "routetest",
		GitURL:      gitURL,
		BeadsPrefix: "rt",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// Append route to routes.jsonl (this is done by the CLI command, not AddRig)
	// The CLI command in runRigAdd calls beads.AppendRoute after AddRig succeeds
	if newRig.Config != nil && newRig.Config.Prefix != "" {
		route := beads.Route{
			Prefix: newRig.Config.Prefix + "-",
			Path:   "routetest",
		}
		if err := beads.AppendRoute(townRoot, route); err != nil {
			t.Fatalf("AppendRoute: %v", err)
		}
	}

	// Save rigs config (normally done by the command)
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	// Load routes and verify the new route exists
	townBeadsDir := filepath.Join(townRoot, ".beads")
	routes, err := beads.LoadRoutes(townBeadsDir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	// Find route for our rig
	var foundRoute *beads.Route
	for _, r := range routes {
		if r.Prefix == "rt-" {
			foundRoute = &r
			break
		}
	}

	if foundRoute == nil {
		t.Error("route with prefix 'rt-' not found in routes.jsonl")
		t.Logf("routes: %+v", routes)
	} else {
		// The path should point to the rig (or mayor/rig if .beads is tracked in source)
		if !strings.HasPrefix(foundRoute.Path, "routetest") {
			t.Errorf("route path = %q, want prefix 'routetest'", foundRoute.Path)
		}
	}
}

// TestRigAddUpdatesRigsJson verifies that rigs.json is updated
// with the new rig entry.
func TestRigAddUpdatesRigsJson(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "jsontest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	_, err = mgr.AddRig(rig.AddRigOptions{
		Name:        "jsontest",
		GitURL:      gitURL,
		BeadsPrefix: "jt",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// Save rigs config (normally done by the command)
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	// Reload and verify
	rigsConfig2, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("reload rigs.json: %v", err)
	}

	entry, ok := rigsConfig2.Rigs["jsontest"]
	if !ok {
		t.Error("rig 'jsontest' not found in rigs.json")
		t.Logf("rigs: %+v", rigsConfig2.Rigs)
	} else {
		if entry.GitURL != gitURL {
			t.Errorf("GitURL = %q, want %q", entry.GitURL, gitURL)
		}
		if entry.BeadsConfig == nil {
			t.Error("BeadsConfig is nil")
		} else if entry.BeadsConfig.Prefix != "jt" {
			t.Errorf("BeadsConfig.Prefix = %q, want %q", entry.BeadsConfig.Prefix, "jt")
		}
	}
}

// TestRigAddDerivesPrefix verifies that when no prefix is specified,
// one is derived from the rig name.
func TestRigAddDerivesPrefix(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "myproject")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	newRig, err := mgr.AddRig(rig.AddRigOptions{
		Name:   "myproject",
		GitURL: gitURL,
		// No BeadsPrefix - should be derived
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// For a single-word name like "myproject", the prefix should be first 2 chars
	if newRig.Config.Prefix != "my" {
		t.Errorf("derived prefix = %q, want %q", newRig.Config.Prefix, "my")
	}
}

// TestRigAddCreatesRigConfig verifies that config.json contains
// the correct rig configuration.
func TestRigAddCreatesRigConfig(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "configtest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	_, err = mgr.AddRig(rig.AddRigOptions{
		Name:        "configtest",
		GitURL:      gitURL,
		BeadsPrefix: "ct",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// Read and verify config.json
	configPath := filepath.Join(townRoot, "configtest", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}

	var rigCfg rig.RigConfig
	if err := json.Unmarshal(data, &rigCfg); err != nil {
		t.Fatalf("parsing config.json: %v", err)
	}

	if rigCfg.Type != "rig" {
		t.Errorf("Type = %q, want 'rig'", rigCfg.Type)
	}
	if rigCfg.Name != "configtest" {
		t.Errorf("Name = %q, want 'configtest'", rigCfg.Name)
	}
	if rigCfg.GitURL != gitURL {
		t.Errorf("GitURL = %q, want %q", rigCfg.GitURL, gitURL)
	}
	if rigCfg.Beads == nil {
		t.Error("Beads config is nil")
	} else if rigCfg.Beads.Prefix != "ct" {
		t.Errorf("Beads.Prefix = %q, want 'ct'", rigCfg.Beads.Prefix)
	}
	if rigCfg.DefaultBranch == "" {
		t.Error("DefaultBranch is empty")
	}
}

// TestRigAddCreatesAgentDirs verifies that agent state files are created.
func TestRigAddCreatesAgentDirs(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "agenttest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	_, err = mgr.AddRig(rig.AddRigOptions{
		Name:        "agenttest",
		GitURL:      gitURL,
		BeadsPrefix: "at",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	rigPath := filepath.Join(townRoot, "agenttest")

	// Verify agent directories exist (state.json files are no longer created)
	expectedDirs := []string{
		"witness",
		"refinery",
		"mayor",
	}

	for _, dir := range expectedDirs {
		path := filepath.Join(rigPath, dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", dir)
		}
	}
}

// TestRigAddRejectsInvalidNames verifies that rig names with invalid
// characters are rejected.
func TestRigAddRejectsInvalidNames(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "validname")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	// Characters that break agent ID parsing (hyphens, dots, spaces)
	// Note: underscores are allowed
	invalidNames := []string{
		"my-rig",       // hyphens break agent ID parsing
		"my.rig",       // dots break parsing
		"my rig",       // spaces are invalid
		"my-multi-rig", // multiple hyphens
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := mgr.AddRig(rig.AddRigOptions{
				Name:   name,
				GitURL: gitURL,
			})
			if err == nil {
				t.Errorf("AddRig(%q) should have failed", name)
			} else if !strings.Contains(err.Error(), "invalid characters") {
				t.Errorf("AddRig(%q) error = %v, want 'invalid characters'", name, err)
			}
		})
	}
}

// TestRigAddCreatesAgentBeads verifies that gt rig add creates
// witness and refinery agent beads via the manager's initAgentBeads.
func TestRigAddCreatesAgentBeads(t *testing.T) {
	bdLogPath := mockBdCommand(t)
	townRoot := setupTestTown(t)
	gitURL := createTestGitRepo(t, "agentbeadtest")

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}

	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	// AddRig internally calls initAgentBeads which creates witness and refinery beads
	newRig, err := mgr.AddRig(rig.AddRigOptions{
		Name:        "agentbeadtest",
		GitURL:      gitURL,
		BeadsPrefix: "ab",
	})
	if err != nil {
		t.Fatalf("AddRig: %v", err)
	}

	// Verify the mock bd was called with correct create commands
	logContent, err := os.ReadFile(bdLogPath)
	if err != nil {
		t.Fatalf("reading bd log: %v", err)
	}
	logStr := string(logContent)

	// Expected bead IDs that initAgentBeads should create
	witnessID := beads.WitnessBeadIDWithPrefix(newRig.Config.Prefix, "agentbeadtest")
	refineryID := beads.RefineryBeadIDWithPrefix(newRig.Config.Prefix, "agentbeadtest")

	expectedIDs := []struct {
		id   string
		desc string
	}{
		{witnessID, "witness agent bead"},
		{refineryID, "refinery agent bead"},
	}

	for _, expected := range expectedIDs {
		if !strings.Contains(logStr, expected.id) {
			t.Errorf("bd create log should contain %s (%s), got:\n%s", expected.id, expected.desc, logStr)
		}
	}

	// Verify correct prefix is used (ab-)
	if !strings.Contains(logStr, "ab-") {
		t.Errorf("bd create log should contain prefix 'ab-', got:\n%s", logStr)
	}
}

// TestAgentBeadIDs verifies the agent bead ID generation functions.
func TestAgentBeadIDs(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		expected string
	}{
		{
			"WitnessBeadIDWithPrefix",
			func() string { return beads.WitnessBeadIDWithPrefix("ab", "myrig") },
			"ab-myrig-witness",
		},
		{
			"RefineryBeadIDWithPrefix",
			func() string { return beads.RefineryBeadIDWithPrefix("ab", "myrig") },
			"ab-myrig-refinery",
		},
		{
			"RigBeadIDWithPrefix",
			func() string { return beads.RigBeadIDWithPrefix("ab", "myrig") },
			"ab-rig-myrig",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.fn()
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}
