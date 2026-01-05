package rig

import (
	"os"
	"path/filepath"
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

func writeFakeBD(t *testing.T, script string) string {
	t.Helper()
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	return binDir
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

func TestInitBeadsWritesConfigOnFailure(t *testing.T) {
	rigPath := t.TempDir()
	beadsDir := filepath.Join(rigPath, ".beads")

	script := `#!/usr/bin/env bash
set -e
cmd="$1"
shift
if [[ "$cmd" == "init" ]]; then
  echo "bd init failed" >&2
  exit 1
fi
echo "unexpected command: $cmd" >&2
exit 1
`

	binDir := writeFakeBD(t, script)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("EXPECT_BEADS_DIR", beadsDir)

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
}

func TestInitAgentBeadsUsesRigBeadsDir(t *testing.T) {
	// Rig-level agent beads (witness, refinery) are stored in rig beads.
	// Town-level agents (mayor, deacon) are created by gt install in town beads.
	// This test verifies that rig agent beads are created in the rig directory,
	// without an explicit BEADS_DIR override (uses cwd-based discovery).
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
if [[ "$1" == "--no-daemon" ]]; then
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
  *)
    echo "unexpected command: $cmd" >&2
    exit 1
    ;;
esac
`

	binDir := writeFakeBD(t, script)
	agentLog := filepath.Join(t.TempDir(), "agents.log")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AGENT_LOG", agentLog)
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
		{"", false},                    // empty
		{"1abc", false},                // starts with number
		{"-abc", false},                // starts with hyphen
		{"abc def", false},             // contains space
		{"abc;ls", false},              // shell injection attempt
		{"$(whoami)", false},           // command substitution
		{"`id`", false},                // backtick command
		{"abc|cat", false},             // pipe
		{"../etc/passwd", false},       // path traversal
		{"aaaaaaaaaaaaaaaaaaaaa", false}, // too long (21 chars, >20 limit)
		{"valid-but-with-$var", false}, // variable reference
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
