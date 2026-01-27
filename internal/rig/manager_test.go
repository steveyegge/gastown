package rig

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
)

func setupTestTown(t *testing.T) (string, *config.RigsConfig) {
	t.Helper()
	root := t.TempDir()

	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs:    make(map[string]config.RigEntry),
	}

	return root, rigsConfig
}

func writeFakeBD(t *testing.T, script string, windowsScript string) string {
	t.Helper()
	binDir := t.TempDir()

	if runtime.GOOS == "windows" {
		if windowsScript == "" {
			t.Fatal("windows script is required on Windows")
		}
		scriptPath := filepath.Join(binDir, "bd.cmd")
		if err := os.WriteFile(scriptPath, []byte(windowsScript), 0644); err != nil {
			t.Fatalf("write fake bd: %v", err)
		}
		return binDir
	}

	scriptPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	return binDir
}

func assertBeadsDirLog(t *testing.T, logPath, want string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading beads dir log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatalf("expected beads dir log entries, got none")
	}
	for _, line := range lines {
		trimmed := strings.TrimSuffix(line, "\r")
		if trimmed != want {
			t.Fatalf("BEADS_DIR = %q, want %q", trimmed, want)
		}
	}
}

func createTestRig(t *testing.T, root, name string) {
	t.Helper()

	rigPath := filepath.Join(root, name)
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	// Create agent dirs (witness, refinery, mayor)
	for _, dir := range AgentDirs {
		dirPath := filepath.Join(rigPath, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// Create some polecats
	polecatsDir := filepath.Join(rigPath, "polecats")
	for _, polecat := range []string{"Toast", "Cheedo"} {
		if err := os.MkdirAll(filepath.Join(polecatsDir, polecat), 0755); err != nil {
			t.Fatalf("mkdir polecat: %v", err)
		}
	}
	// Create a shared support dir that should not be treated as a polecat worktree.
	if err := os.MkdirAll(filepath.Join(polecatsDir, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir polecats/.claude: %v", err)
	}
}

func TestDiscoverRigs(t *testing.T) {
	root, rigsConfig := setupTestTown(t)

	// Create test rig
	createTestRig(t, root, "gastown")
	rigsConfig.Rigs["gastown"] = config.RigEntry{
		GitURL: "git@github.com:test/gastown.git",
	}

	manager := NewManager(root, rigsConfig, git.NewGit(root))

	rigs, err := manager.DiscoverRigs()
	if err != nil {
		t.Fatalf("DiscoverRigs: %v", err)
	}

	if len(rigs) != 1 {
		t.Errorf("rigs count = %d, want 1", len(rigs))
	}

	rig := rigs[0]
	if rig.Name != "gastown" {
		t.Errorf("Name = %q, want gastown", rig.Name)
	}
	if len(rig.Polecats) != 2 {
		t.Errorf("Polecats count = %d, want 2", len(rig.Polecats))
	}
	if slices.Contains(rig.Polecats, ".claude") {
		t.Errorf("expected polecats/.claude to be ignored, got %v", rig.Polecats)
	}
	if !rig.HasWitness {
		t.Error("expected HasWitness = true")
	}
	if !rig.HasRefinery {
		t.Error("expected HasRefinery = true")
	}
}

func TestGetRig(t *testing.T) {
	root, rigsConfig := setupTestTown(t)

	createTestRig(t, root, "test-rig")
	rigsConfig.Rigs["test-rig"] = config.RigEntry{
		GitURL: "git@github.com:test/test-rig.git",
	}

	manager := NewManager(root, rigsConfig, git.NewGit(root))

	rig, err := manager.GetRig("test-rig")
	if err != nil {
		t.Fatalf("GetRig: %v", err)
	}

	if rig.Name != "test-rig" {
		t.Errorf("Name = %q, want test-rig", rig.Name)
	}
}

func TestGetRigNotFound(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	_, err := manager.GetRig("nonexistent")
	if err != ErrRigNotFound {
		t.Errorf("GetRig = %v, want ErrRigNotFound", err)
	}
}

func TestRigExists(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	rigsConfig.Rigs["exists"] = config.RigEntry{}

	manager := NewManager(root, rigsConfig, git.NewGit(root))

	if !manager.RigExists("exists") {
		t.Error("expected RigExists = true for existing rig")
	}
	if manager.RigExists("nonexistent") {
		t.Error("expected RigExists = false for nonexistent rig")
	}
}

func TestRemoveRig(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	rigsConfig.Rigs["to-remove"] = config.RigEntry{}

	manager := NewManager(root, rigsConfig, git.NewGit(root))

	if err := manager.RemoveRig("to-remove"); err != nil {
		t.Fatalf("RemoveRig: %v", err)
	}

	if manager.RigExists("to-remove") {
		t.Error("rig should not exist after removal")
	}
}

func TestRemoveRigNotFound(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	err := manager.RemoveRig("nonexistent")
	if err != ErrRigNotFound {
		t.Errorf("RemoveRig = %v, want ErrRigNotFound", err)
	}
}

func TestAddRig_RejectsInvalidNames(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	tests := []struct {
		name      string
		wantError string
	}{
		{"op-baby", `rig name "op-baby" contains invalid characters`},
		{"my.rig", `rig name "my.rig" contains invalid characters`},
		{"my rig", `rig name "my rig" contains invalid characters`},
		{"op-baby-test", `rig name "op-baby-test" contains invalid characters`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.AddRig(AddRigOptions{
				Name:   tt.name,
				GitURL: "git@github.com:test/test.git",
			})
			if err == nil {
				t.Errorf("AddRig(%q) succeeded, want error containing %q", tt.name, tt.wantError)
				return
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("AddRig(%q) error = %q, want error containing %q", tt.name, err.Error(), tt.wantError)
			}
		})
	}
}

func TestListRigNames(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	rigsConfig.Rigs["rig1"] = config.RigEntry{}
	rigsConfig.Rigs["rig2"] = config.RigEntry{}

	manager := NewManager(root, rigsConfig, git.NewGit(root))

	names := manager.ListRigNames()
	if len(names) != 2 {
		t.Errorf("names count = %d, want 2", len(names))
	}
}

func TestRigSummary(t *testing.T) {
	rig := &Rig{
		Name:        "test",
		Polecats:    []string{"a", "b", "c"},
		HasWitness:  true,
		HasRefinery: false,
	}

	summary := rig.Summary()

	if summary.Name != "test" {
		t.Errorf("Name = %q, want test", summary.Name)
	}
	if summary.PolecatCount != 3 {
		t.Errorf("PolecatCount = %d, want 3", summary.PolecatCount)
	}
	if !summary.HasWitness {
		t.Error("expected HasWitness = true")
	}
	if summary.HasRefinery {
		t.Error("expected HasRefinery = false")
	}
}

func TestEnsureGitignoreEntry_AddsEntry(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	gitignorePath := filepath.Join(root, ".gitignore")

	if err := manager.ensureGitignoreEntry(gitignorePath, ".test-entry/"); err != nil {
		t.Fatalf("ensureGitignoreEntry: %v", err)
	}

	content, _ := os.ReadFile(gitignorePath)
	if string(content) != ".test-entry/\n" {
		t.Errorf("content = %q, want .test-entry/", string(content))
	}
}

func TestEnsureGitignoreEntry_DoesNotDuplicate(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	gitignorePath := filepath.Join(root, ".gitignore")

	// Pre-populate with the entry
	if err := os.WriteFile(gitignorePath, []byte(".test-entry/\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	if err := manager.ensureGitignoreEntry(gitignorePath, ".test-entry/"); err != nil {
		t.Fatalf("ensureGitignoreEntry: %v", err)
	}

	content, _ := os.ReadFile(gitignorePath)
	if string(content) != ".test-entry/\n" {
		t.Errorf("content = %q, want single .test-entry/", string(content))
	}
}

func TestEnsureGitignoreEntry_AppendsToExisting(t *testing.T) {
	root, rigsConfig := setupTestTown(t)
	manager := NewManager(root, rigsConfig, git.NewGit(root))

	gitignorePath := filepath.Join(root, ".gitignore")

	// Pre-populate with existing entries
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n*.log\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	if err := manager.ensureGitignoreEntry(gitignorePath, ".test-entry/"); err != nil {
		t.Fatalf("ensureGitignoreEntry: %v", err)
	}

	content, _ := os.ReadFile(gitignorePath)
	expected := "node_modules/\n*.log\n.test-entry/\n"
	if string(content) != expected {
		t.Errorf("content = %q, want %q", string(content), expected)
	}
}

func TestInitBeads_TrackedBeads_CreatesRedirect(t *testing.T) {
	t.Parallel()
	// When the cloned repo has tracked beads (mayor/rig/.beads exists),
	// initBeads should create a redirect file at <rig>/.beads/redirect
	// pointing to mayor/rig/.beads instead of creating a local database.
	rigPath := t.TempDir()

	// Simulate tracked beads in the cloned repo
	mayorBeadsDir := filepath.Join(rigPath, "mayor", "rig", ".beads")
	if err := os.MkdirAll(mayorBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mayor beads: %v", err)
	}
	// Create a config file to simulate a real beads directory
	if err := os.WriteFile(filepath.Join(mayorBeadsDir, "config.yaml"), []byte("prefix: gt\n"), 0644); err != nil {
		t.Fatalf("write mayor config: %v", err)
	}

	manager := &Manager{}
	if err := manager.initBeads(rigPath, "gt"); err != nil {
		t.Fatalf("initBeads: %v", err)
	}

	// Verify redirect file was created
	redirectPath := filepath.Join(rigPath, ".beads", "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("reading redirect file: %v", err)
	}

	expected := "mayor/rig/.beads\n"
	if string(content) != expected {
		t.Errorf("redirect content = %q, want %q", string(content), expected)
	}

	// Verify no local database was created (no config.yaml at rig level)
	rigConfigPath := filepath.Join(rigPath, ".beads", "config.yaml")
	if _, err := os.Stat(rigConfigPath); !os.IsNotExist(err) {
		t.Errorf("expected no config.yaml at rig level when using redirect, but it exists")
	}
}

func TestInitBeads_LocalBeads_CreatesDatabase(t *testing.T) {
	// Cannot use t.Parallel() due to t.Setenv
	// When the cloned repo does NOT have tracked beads (no mayor/rig/.beads),
	// initBeads should create a local database at <rig>/.beads/
	rigPath := t.TempDir()

	// Create mayor/rig directory but WITHOUT .beads (no tracked beads)
	mayorRigDir := filepath.Join(rigPath, "mayor", "rig")
	if err := os.MkdirAll(mayorRigDir, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Use fake bd that succeeds
	script := `#!/usr/bin/env bash
set -e
if [[ "$1" == "init" ]]; then
  # Simulate successful bd init
  exit 0
fi
exit 0
`
	windowsScript := "@echo off\r\nif \"%1\"==\"init\" exit /b 0\r\nexit /b 0\r\n"
	binDir := writeFakeBD(t, script, windowsScript)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manager := &Manager{}
	if err := manager.initBeads(rigPath, "gt"); err != nil {
		t.Fatalf("initBeads: %v", err)
	}

	// Verify NO redirect file was created
	redirectPath := filepath.Join(rigPath, ".beads", "redirect")
	if _, err := os.Stat(redirectPath); !os.IsNotExist(err) {
		t.Errorf("expected no redirect file for local beads, but it exists")
	}

	// Verify .beads directory was created
	beadsDir := filepath.Join(rigPath, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Errorf("expected .beads directory to be created")
	}
}

func TestInitBeadsWritesConfigOnFailure(t *testing.T) {
	rigPath := t.TempDir()
	beadsDir := filepath.Join(rigPath, ".beads")

	script := `#!/usr/bin/env bash
set -e
if [[ -n "$BEADS_DIR_LOG" ]]; then
  echo "${BEADS_DIR:-<unset>}" >> "$BEADS_DIR_LOG"
fi
cmd="$1"
shift
if [[ "$cmd" == "init" ]]; then
  echo "bd init failed" >&2
  exit 1
fi
echo "unexpected command: $cmd" >&2
exit 1
`
	windowsScript := "@echo off\r\nif defined BEADS_DIR_LOG (\r\n  if defined BEADS_DIR (\r\n    echo %BEADS_DIR%>>\"%BEADS_DIR_LOG%\"\r\n  ) else (\r\n    echo ^<unset^> >>\"%BEADS_DIR_LOG%\"\r\n  )\r\n)\r\nif \"%1\"==\"init\" (\r\n  exit /b 1\r\n)\r\nexit /b 1\r\n"

	binDir := writeFakeBD(t, script, windowsScript)
	beadsDirLog := filepath.Join(t.TempDir(), "beads-dir.log")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BEADS_DIR_LOG", beadsDirLog)

	manager := &Manager{}
	if err := manager.initBeads(rigPath, "gt"); err != nil {
		t.Fatalf("initBeads: %v", err)
	}

	configPath := filepath.Join(beadsDir, "config.yaml")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	if string(config) != "prefix: gt\n" {
		t.Fatalf("config.yaml = %q, want %q", string(config), "prefix: gt\n")
	}
	assertBeadsDirLog(t, beadsDirLog, beadsDir)
}

func TestInitAgentBeadsUsesRigBeadsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake bd stub is not compatible with multiline descriptions on Windows")
	}

	// Rig-level agent beads (witness, refinery) are stored in rig beads.
	// Town-level agents (mayor, deacon) are created by gt install in town beads.
	// This test verifies that rig agent beads are created in the rig directory,
	// using the resolved rig beads directory for BEADS_DIR.
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrip")
	rigBeadsDir := filepath.Join(rigPath, ".beads")

	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir rig beads dir: %v", err)
	}

	// Track which agent IDs were created
	var createdAgents []string

	script := `#!/usr/bin/env bash
set -e
if [[ -n "$BEADS_DIR_LOG" ]]; then
  echo "${BEADS_DIR:-<unset>}" >> "$BEADS_DIR_LOG"
fi
if [[ "$1" == "--no-daemon" ]]; then
  shift
fi
if [[ "$1" == "--allow-stale" ]]; then
  shift
fi
cmd="$1"
shift
case "$cmd" in
  show)
    # Return empty to indicate agent doesn't exist yet
    echo "[]"
    ;;
  create)
    id=""
    title=""
    for arg in "$@"; do
      case "$arg" in
        --id=*) id="${arg#--id=}" ;;
        --title=*) title="${arg#--title=}" ;;
      esac
    done
    # Log the created agent ID for verification
    echo "$id" >> "$AGENT_LOG"
    printf '{"id":"%s","title":"%s","description":"","issue_type":"agent"}' "$id" "$title"
    ;;
  slot)
    # Accept slot commands
    ;;
  config)
    # Accept config commands (e.g., "bd config set types.custom ...")
    ;;
  *)
    echo "unexpected command: $cmd" >&2
    exit 1
    ;;
esac
`
	windowsScript := "@echo off\r\nsetlocal enabledelayedexpansion\r\nif defined BEADS_DIR_LOG (\r\n  if defined BEADS_DIR (\r\n    echo %BEADS_DIR%>>\"%BEADS_DIR_LOG%\"\r\n  ) else (\r\n    echo ^<unset^> >>\"%BEADS_DIR_LOG%\"\r\n  )\r\n)\r\nset \"cmd=%1\"\r\nset \"arg2=%2\"\r\nset \"arg3=%3\"\r\nif \"%cmd%\"==\"--no-daemon\" (\r\n  set \"cmd=%2\"\r\n  set \"arg2=%3\"\r\n  set \"arg3=%4\"\r\n)\r\nif \"%cmd%\"==\"--allow-stale\" (\r\n  set \"cmd=%2\"\r\n  set \"arg2=%3\"\r\n  set \"arg3=%4\"\r\n)\r\nif \"%cmd%\"==\"show\" (\r\n  echo []\r\n  exit /b 0\r\n)\r\nif \"%cmd%\"==\"create\" (\r\n  set \"id=\"\r\n  set \"title=\"\r\n  for %%A in (%*) do (\r\n    set \"arg=%%~A\"\r\n    if /i \"!arg:~0,5!\"==\"--id=\" set \"id=!arg:~5!\"\r\n    if /i \"!arg:~0,8!\"==\"--title=\" set \"title=!arg:~8!\"\r\n  )\r\n  if defined AGENT_LOG (\r\n    echo !id!>>\"%AGENT_LOG%\"\r\n  )\r\n  echo {\"id\":\"!id!\",\"title\":\"!title!\",\"description\":\"\",\"issue_type\":\"agent\"}\r\n  exit /b 0\r\n)\r\nif \"%cmd%\"==\"slot\" exit /b 0\r\nif \"%cmd%\"==\"config\" exit /b 0\r\nexit /b 1\r\n"

	binDir := writeFakeBD(t, script, windowsScript)
	agentLog := filepath.Join(t.TempDir(), "agents.log")
	beadsDirLog := filepath.Join(t.TempDir(), "beads-dir.log")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGENT_LOG", agentLog)
	t.Setenv("BEADS_DIR_LOG", beadsDirLog)
	t.Setenv("BEADS_DIR", "") // Clear any existing BEADS_DIR

	manager := &Manager{townRoot: townRoot}
	if err := manager.initAgentBeads(rigPath, "demo", "gt"); err != nil {
		t.Fatalf("initAgentBeads: %v", err)
	}

	// Verify the expected rig-level agents were created
	data, err := os.ReadFile(agentLog)
	if err != nil {
		t.Fatalf("reading agent log: %v", err)
	}
	createdAgents = strings.Split(strings.TrimSpace(string(data)), "\n")

	// Should create witness and refinery for the rig
	expectedAgents := map[string]bool{
		"gt-demo-witness":  false,
		"gt-demo-refinery": false,
	}

	for _, id := range createdAgents {
		if _, ok := expectedAgents[id]; ok {
			expectedAgents[id] = true
		}
	}

	for id, found := range expectedAgents {
		if !found {
			t.Errorf("expected agent %s was not created", id)
		}
	}
	assertBeadsDirLog(t, beadsDirLog, rigBeadsDir)
}

func TestIsValidBeadsPrefix(t *testing.T) {
	tests := []struct {
		prefix string
		want   bool
	}{
		// Valid prefixes
		{"gt", true},
		{"bd", true},
		{"hq", true},
		{"gastown", true},
		{"myProject", true},
		{"my-project", true},
		{"a", true},
		{"A", true},
		{"test123", true},
		{"a1b2c3", true},
		{"a-b-c", true},

		// Invalid prefixes
		{"", false},                      // empty
		{"1abc", false},                  // starts with number
		{"-abc", false},                  // starts with hyphen
		{"abc def", false},               // contains space
		{"abc;ls", false},                // shell injection attempt
		{"$(whoami)", false},             // command substitution
		{"`id`", false},                  // backtick command
		{"abc|cat", false},               // pipe
		{"../etc/passwd", false},         // path traversal
		{"aaaaaaaaaaaaaaaaaaaaa", false}, // too long (21 chars, >20 limit)
		{"valid-but-with-$var", false},   // variable reference
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := isValidBeadsPrefix(tt.prefix)
			if got != tt.want {
				t.Errorf("isValidBeadsPrefix(%q) = %v, want %v", tt.prefix, got, tt.want)
			}
		})
	}
}

func TestInitBeadsRejectsInvalidPrefix(t *testing.T) {
	rigPath := t.TempDir()
	manager := &Manager{}

	tests := []string{
		"",
		"$(whoami)",
		"abc;rm -rf /",
		"../etc",
		"123",
	}

	for _, prefix := range tests {
		t.Run(prefix, func(t *testing.T) {
			err := manager.initBeads(rigPath, prefix)
			if err == nil {
				t.Errorf("initBeads(%q) should have failed", prefix)
			}
			if !strings.Contains(err.Error(), "invalid beads prefix") {
				t.Errorf("initBeads(%q) error = %q, want error containing 'invalid beads prefix'", prefix, err.Error())
			}
		})
	}
}

func TestInitBeads_InheritsDoltBackendFromTown(t *testing.T) {
	// Cannot use t.Parallel() due to t.Setenv
	// When town uses Dolt backend, new rigs should inherit it.
	// This test verifies that initBeads passes --backend dolt when town uses Dolt.

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "newrig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	// Create town-level .beads with Dolt backend (non-server mode)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town beads: %v", err)
	}
	// Set storage-backend: dolt in config.yaml (non-server mode)
	configYAML := "prefix: hq\nstorage-backend: dolt\n"
	if err := os.WriteFile(filepath.Join(townBeadsDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write town config.yaml: %v", err)
	}

	// Use fake bd that captures the init arguments
	argsLog := filepath.Join(t.TempDir(), "bd-args.log")
	script := `#!/usr/bin/env bash
set -e
echo "$@" >> "` + argsLog + `"
exit 0
`
	windowsScript := "@echo off\r\necho %* >> \"" + argsLog + "\"\r\nexit /b 0\r\n"
	binDir := writeFakeBD(t, script, windowsScript)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manager := &Manager{townRoot: townRoot}
	if err := manager.initBeads(rigPath, "nr"); err != nil {
		t.Fatalf("initBeads: %v", err)
	}

	// Verify --backend dolt was passed to bd init
	argsData, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("reading args log: %v", err)
	}
	argsContent := string(argsData)
	if !strings.Contains(argsContent, "--backend dolt") {
		t.Errorf("bd init should include --backend dolt when town uses Dolt; got: %s", argsContent)
	}
}

func TestInitBeads_DoltServerMode_CreatesRedirect(t *testing.T) {
	t.Parallel()
	// When town uses Dolt server mode, new rigs should create a redirect
	// to the town-level .beads instead of a local database.

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "newrig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}

	// Create town-level .beads with Dolt server mode enabled
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town beads: %v", err)
	}
	// Set up metadata.json with Dolt server mode
	metadata := `{
  "backend": "dolt",
  "dolt_server_enabled": true,
  "dolt_server_host": "127.0.0.1",
  "dolt_server_port": 3306
}`
	if err := os.WriteFile(filepath.Join(townBeadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write town metadata.json: %v", err)
	}

	manager := &Manager{townRoot: townRoot}
	if err := manager.initBeads(rigPath, "nr"); err != nil {
		t.Fatalf("initBeads: %v", err)
	}

	// Verify redirect file was created pointing to town-level .beads
	redirectPath := filepath.Join(rigPath, ".beads", "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("reading redirect file: %v", err)
	}

	// Should contain a relative path to town .beads (../.beads)
	got := strings.TrimSpace(string(content))
	if !strings.HasSuffix(got, ".beads") {
		t.Errorf("redirect should point to .beads; got: %s", got)
	}
}

func TestDeriveBeadsPrefix(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		// Compound words with common suffixes should split
		{"gastown", "gt"},     // gas + town
		{"nashville", "nv"},   // nash + ville
		{"bridgeport", "bp"},  // bridge + port
		{"someplace", "sp"},   // some + place
		{"greenland", "gl"},   // green + land
		{"springfield", "sf"}, // spring + field
		{"hollywood", "hw"},   // holly + wood
		{"oxford", "of"},      // ox + ford

		// Hyphenated names
		{"my-project", "mp"},
		{"gas-town", "gt"},
		{"some-long-name", "sln"},

		// Underscored names
		{"my_project", "mp"},

		// Short single words (use the whole name)
		{"foo", "foo"},
		{"bar", "bar"},
		{"ab", "ab"},

		// Longer single words without known suffixes (first 2 chars)
		{"myrig", "my"},
		{"awesome", "aw"},
		{"coolrig", "co"},

		// With language suffixes stripped
		{"myproject-py", "my"},
		{"myproject-go", "my"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBeadsPrefix(tt.name)
			if got != tt.want {
				t.Errorf("deriveBeadsPrefix(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSplitCompoundWord(t *testing.T) {
	tests := []struct {
		word string
		want []string
	}{
		// Known suffixes
		{"gastown", []string{"gas", "town"}},
		{"nashville", []string{"nash", "ville"}},
		{"bridgeport", []string{"bridge", "port"}},
		{"someplace", []string{"some", "place"}},
		{"greenland", []string{"green", "land"}},
		{"springfield", []string{"spring", "field"}},
		{"hollywood", []string{"holly", "wood"}},
		{"oxford", []string{"ox", "ford"}},

		// Just the suffix (should not split)
		{"town", []string{"town"}},
		{"ville", []string{"ville"}},

		// No known suffix
		{"myrig", []string{"myrig"}},
		{"awesome", []string{"awesome"}},

		// Empty prefix would result (should not split)
		// Note: "town" itself shouldn't split to ["", "town"]
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := splitCompoundWord(tt.word)
			if len(got) != len(tt.want) {
				t.Errorf("splitCompoundWord(%q) = %v, want %v", tt.word, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCompoundWord(%q)[%d] = %q, want %q", tt.word, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestConvertToSSH(t *testing.T) {
	tests := []struct {
		name    string
		https   string
		wantSSH string
	}{
		{
			name:    "GitHub with .git suffix",
			https:   "https://github.com/owner/repo.git",
			wantSSH: "git@github.com:owner/repo.git",
		},
		{
			name:    "GitHub without .git suffix",
			https:   "https://github.com/owner/repo",
			wantSSH: "git@github.com:owner/repo.git",
		},
		{
			name:    "GitHub with org/subpath",
			https:   "https://github.com/myorg/myproject.git",
			wantSSH: "git@github.com:myorg/myproject.git",
		},
		{
			name:    "GitLab with .git suffix",
			https:   "https://gitlab.com/owner/repo.git",
			wantSSH: "git@gitlab.com:owner/repo.git",
		},
		{
			name:    "GitLab without .git suffix",
			https:   "https://gitlab.com/owner/repo",
			wantSSH: "git@gitlab.com:owner/repo.git",
		},
		{
			name:    "Unknown host returns empty",
			https:   "https://bitbucket.org/owner/repo.git",
			wantSSH: "",
		},
		{
			name:    "Non-HTTPS URL returns empty",
			https:   "git@github.com:owner/repo.git",
			wantSSH: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToSSH(tt.https)
			if got != tt.wantSSH {
				t.Errorf("convertToSSH(%q) = %q, want %q", tt.https, got, tt.wantSSH)
			}
		})
	}
}
