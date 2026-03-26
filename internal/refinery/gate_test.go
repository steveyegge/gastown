package refinery

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

func TestRunGatesOnBranch_NoGatesConfigured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	// Set up a git repo with a branch
	repoDir := initTestRepoWithBranch(t, "test-branch")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	e.output = io.Discard
	e.config.RunTests = false
	e.config.Gates = nil

	result := e.RunGatesOnBranch(context.Background(), "test-branch")
	if !result.Success {
		t.Errorf("expected success when no gates configured, got: %s", result.Error)
	}
}

func TestRunGatesOnBranch_GatesPass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	repoDir := initTestRepoWithBranch(t, "integration")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	e.output = io.Discard
	e.config.Gates = map[string]*GateConfig{
		"lint": {Cmd: "true", Phase: GatePhasePreMerge},
		"test": {Cmd: "true", Phase: GatePhasePreMerge},
	}

	result := e.RunGatesOnBranch(context.Background(), "integration")
	if !result.Success {
		t.Errorf("expected gates to pass, got: %s", result.Error)
	}
}

func TestRunGatesOnBranch_GatesFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	repoDir := initTestRepoWithBranch(t, "integration")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	e.output = io.Discard
	e.config.Gates = map[string]*GateConfig{
		"lint": {Cmd: "true", Phase: GatePhasePreMerge},
		"test": {Cmd: "false", Phase: GatePhasePreMerge}, // fails
	}

	result := e.RunGatesOnBranch(context.Background(), "integration")
	if result.Success {
		t.Error("expected gates to fail")
	}
	if !result.TestsFailed {
		t.Error("expected TestsFailed flag to be set")
	}
}

func TestRunGatesOnBranch_BranchNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	repoDir := initTestRepoWithBranch(t, "existing-branch")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	e.output = io.Discard

	result := e.RunGatesOnBranch(context.Background(), "nonexistent-branch")
	if result.Success {
		t.Error("expected failure for nonexistent branch")
	}
	if !result.BranchNotFound {
		t.Error("expected BranchNotFound flag to be set")
	}
}

func TestRunGatesOnBranch_LegacyTestCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	repoDir := initTestRepoWithBranch(t, "integration")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	e.output = io.Discard
	e.config.Gates = nil
	e.config.RunTests = true
	e.config.TestCommand = "true"

	result := e.RunGatesOnBranch(context.Background(), "integration")
	if !result.Success {
		t.Errorf("expected legacy test command to pass, got: %s", result.Error)
	}
}

func TestRunGatesOnBranch_OnlyRunsPreMergeGates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	repoDir := initTestRepoWithBranch(t, "integration")

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.git = git.NewGit(repoDir)
	e.workDir = repoDir
	var buf bytes.Buffer
	e.output = &buf
	e.config.Gates = map[string]*GateConfig{
		"pre-lint":   {Cmd: "true", Phase: GatePhasePreMerge},
		"post-build": {Cmd: "false", Phase: GatePhasePostSquash}, // Would fail if run
	}

	result := e.RunGatesOnBranch(context.Background(), "integration")
	if !result.Success {
		t.Errorf("expected success (post-squash should not run), got: %s", result.Error)
	}
	output := buf.String()
	if strings.Contains(output, "post-build") {
		t.Error("post-squash gate should not run in integration branch gating")
	}
}

func TestRunConvoyGates_LoadConfigError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell commands")
	}

	// Create a rig path with a malformed config.json
	rigPath := t.TempDir()
	os.MkdirAll(filepath.Join(rigPath, "refinery", "rig"), 0755)
	os.WriteFile(filepath.Join(rigPath, "config.json"), []byte(`{invalid json`), 0644)

	var logs []string
	logger := func(format string, args ...interface{}) {
		logs = append(logs, strings.TrimSpace(strings.Replace(format, "%s", "%v", -1)))
	}

	result := RunConvoyGates(context.Background(), rigPath, "hq-test", "convoy/test", logger)
	if result.Success {
		t.Error("expected failure with malformed config")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestFindRigWithRefinery_Found(t *testing.T) {
	townRoot := t.TempDir()

	// Create a rig directory with refinery worktree
	rigDir := filepath.Join(townRoot, "myrig")
	os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755)
	os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{}`), 0644)

	result := findRigWithRefinery(townRoot)
	if result != rigDir {
		t.Errorf("findRigWithRefinery() = %q, want %q", result, rigDir)
	}
}

func TestFindRigWithRefinery_NotFound(t *testing.T) {
	townRoot := t.TempDir()

	result := findRigWithRefinery(townRoot)
	if result != "" {
		t.Errorf("findRigWithRefinery() = %q, want empty", result)
	}
}

func TestFindRigWithRefinery_SkipsNonRigDirs(t *testing.T) {
	townRoot := t.TempDir()

	// Create .beads and docs dirs (should be skipped)
	os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755)
	os.MkdirAll(filepath.Join(townRoot, "docs"), 0755)

	// Create a rig directory with refinery worktree
	rigDir := filepath.Join(townRoot, "testrig")
	os.MkdirAll(filepath.Join(rigDir, "refinery", "rig"), 0755)
	os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(`{}`), 0644)

	result := findRigWithRefinery(townRoot)
	if result != rigDir {
		t.Errorf("findRigWithRefinery() = %q, want %q", result, rigDir)
	}
}

func TestParseGitRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS with .git suffix",
			url:       "https://github.com/steveyegge/gastown.git",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "HTTPS without .git suffix",
			url:       "https://github.com/steveyegge/gastown",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "SSH format",
			url:       "git@github.com:steveyegge/gastown.git",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "SSH without .git suffix",
			url:       "git@github.com:steveyegge/gastown",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "HTTPS with trailing whitespace",
			url:       "  https://github.com/org/repo.git  ",
			wantOwner: "org",
			wantRepo:  "repo",
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseGitRemoteURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got owner=%q repo=%q", owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestConvertDraftPR_MissingPRNumber(t *testing.T) {
	err := convertDraftPR("no pr_number here", t.TempDir(), "hq-test", nopLogger)
	if err == nil {
		t.Error("expected error for missing pr_number")
	}
	if !strings.Contains(err.Error(), "no pr_number") {
		t.Errorf("error should mention missing pr_number, got: %v", err)
	}
}

func TestConvertDraftPR_InvalidPRNumber(t *testing.T) {
	err := convertDraftPR("pr_number: abc", t.TempDir(), "hq-test", nopLogger)
	if err == nil {
		t.Error("expected error for invalid pr_number")
	}
	if !strings.Contains(err.Error(), "invalid pr_number") {
		t.Errorf("error should mention invalid pr_number, got: %v", err)
	}
}

func TestConvertDraftPR_MissingRig(t *testing.T) {
	// townRoot with no rig directories
	townRoot := t.TempDir()
	err := convertDraftPR("pr_number: 42", townRoot, "hq-test", nopLogger)
	if err == nil {
		t.Error("expected error for missing rig")
	}
	if !strings.Contains(err.Error(), "cannot find rig") {
		t.Errorf("error should mention missing rig, got: %v", err)
	}
}

func TestNotifyConvoyOwner_NilFields(t *testing.T) {
	// Should not panic with nil fields — just nudges crew/overseer.
	// We can't easily verify the nudge was sent without a real gt binary,
	// but we can verify it doesn't crash.
	notifyConvoyOwner(nil, "", t.TempDir(), "hq-test", nopLogger, "false")
}

var nopLogger = func(format string, args ...interface{}) {}

// initTestRepoWithBranch creates a temporary git repo with main and a named branch,
// backed by a bare "origin" remote so fetch/pull operations work.
func initTestRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()

	// Create a bare repo to act as "origin"
	bareDir := t.TempDir()
	runGit(t, "git init bare", "git", "init", "--bare", "-b", "main", bareDir)

	// Create working repo with origin pointing to the bare repo
	dir := t.TempDir()
	runGit(t, "git init", "git", "init", "-b", "main", dir)
	runGit(t, "git config email", "git", "-C", dir, "config", "user.email", "test@test.com")
	runGit(t, "git config name", "git", "-C", dir, "config", "user.name", "Test")
	runGit(t, "git remote add", "git", "-C", dir, "remote", "add", "origin", bareDir)

	// Create an initial commit on main and push to origin
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)
	runGit(t, "git add", "git", "-C", dir, "add", "README.md")
	runGit(t, "git commit", "git", "-C", dir, "commit", "-m", "initial commit")
	runGit(t, "git push main", "git", "-C", dir, "push", "origin", "main")

	// Create the named branch and push it
	runGit(t, "git branch", "git", "-C", dir, "branch", branchName)
	runGit(t, "git push branch", "git", "-C", dir, "push", "origin", branchName)

	return dir
}

func runGit(t *testing.T, label string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", label, err, out)
	}
}
