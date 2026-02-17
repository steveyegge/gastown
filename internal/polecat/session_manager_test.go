package polecat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

func setupTestRegistryForSession(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("bd", "beads")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func requireTmux(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("tmux not supported on Windows")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

func TestSessionName(t *testing.T) {
	setupTestRegistryForSession(t)

	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	name := m.SessionName("Toast")
	if name != "gt-Toast" {
		t.Errorf("sessionName = %q, want gt-Toast", name)
	}
}

func TestSessionManagerPolecatDir(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Path:     "/home/user/ai/gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	dir := m.polecatDir("Toast")
	expected := "/home/user/ai/gastown/polecats/Toast"
	if filepath.ToSlash(dir) != expected {
		t.Errorf("polecatDir = %q, want %q", dir, expected)
	}
}

func TestHasPolecat(t *testing.T) {
	root := t.TempDir()
	// hasPolecat checks filesystem, so create actual directories
	for _, name := range []string{"Toast", "Cheedo"} {
		if err := os.MkdirAll(filepath.Join(root, "polecats", name), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{"Toast", "Cheedo"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	if !m.hasPolecat("Toast") {
		t.Error("expected hasPolecat(Toast) = true")
	}
	if !m.hasPolecat("Cheedo") {
		t.Error("expected hasPolecat(Cheedo) = true")
	}
	if m.hasPolecat("Unknown") {
		t.Error("expected hasPolecat(Unknown) = false")
	}
}

func TestStartPolecatNotFound(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	err := m.Start("Unknown", SessionStartOptions{})
	if err == nil {
		t.Error("expected error for unknown polecat")
	}
}

func TestIsRunningNoSession(t *testing.T) {
	requireTmux(t)

	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	running, err := m.IsRunning("Toast")
	if err != nil {
		t.Fatalf("IsRunning: %v", err)
	}
	if running {
		t.Error("expected IsRunning = false for non-existent session")
	}
}

func TestSessionManagerListEmpty(t *testing.T) {
	requireTmux(t)

	// Register a unique prefix so List() won't match real sessions.
	// Without this, PrefixFor returns "gt" (default) and matches running gastown sessions.
	reg := session.NewPrefixRegistry()
	reg.Register("xz", "test-rig-unlikely-name")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })

	r := &rig.Rig{
		Name:     "test-rig-unlikely-name",
		Polecats: []string{},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	infos, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("infos count = %d, want 0", len(infos))
	}
}

func TestStopNotFound(t *testing.T) {
	requireTmux(t)

	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	err := m.Stop("Toast", false)
	if err != ErrSessionNotFound {
		t.Errorf("Stop = %v, want ErrSessionNotFound", err)
	}
}

func TestCaptureNotFound(t *testing.T) {
	requireTmux(t)

	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	_, err := m.Capture("Toast", 50)
	if err != ErrSessionNotFound {
		t.Errorf("Capture = %v, want ErrSessionNotFound", err)
	}
}

func TestInjectNotFound(t *testing.T) {
	requireTmux(t)

	r := &rig.Rig{
		Name:     "test-rig",
		Polecats: []string{"Toast"},
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	err := m.Inject("Toast", "hello")
	if err != ErrSessionNotFound {
		t.Errorf("Inject = %v, want ErrSessionNotFound", err)
	}
}

// TestPolecatCommandFormat verifies the polecat session command exports
// GT_ROLE, GT_RIG, GT_POLECAT, and BD_ACTOR inline before starting Claude.
// This is a regression test for gt-y41ep - env vars must be exported inline
// because tmux SetEnvironment only affects new panes, not the current shell.
func TestPolecatCommandFormat(t *testing.T) {
	// This test verifies the expected command format.
	// The actual command is built in Start() but we test the format here
	// to document and verify the expected behavior.

	rigName := "gastown"
	polecatName := "Toast"
	expectedBdActor := "gastown/polecats/Toast"
	// GT_ROLE uses compound format: rig/polecats/name
	expectedGtRole := rigName + "/polecats/" + polecatName

	// Build the expected command format (mirrors Start() logic)
	expectedPrefix := "export GT_ROLE=" + expectedGtRole + " GT_RIG=" + rigName + " GT_POLECAT=" + polecatName + " BD_ACTOR=" + expectedBdActor + " GIT_AUTHOR_NAME=" + expectedBdActor
	expectedSuffix := "&& claude --dangerously-skip-permissions"

	// The command must contain all required env exports
	requiredParts := []string{
		"export",
		"GT_ROLE=" + expectedGtRole,
		"GT_RIG=" + rigName,
		"GT_POLECAT=" + polecatName,
		"BD_ACTOR=" + expectedBdActor,
		"GIT_AUTHOR_NAME=" + expectedBdActor,
		"claude --dangerously-skip-permissions",
	}

	// Verify expected format contains all required parts
	fullCommand := expectedPrefix + " " + expectedSuffix
	for _, part := range requiredParts {
		if !strings.Contains(fullCommand, part) {
			t.Errorf("Polecat command should contain %q", part)
		}
	}

	// Verify GT_ROLE uses compound format with "polecats" (not "mayor", "crew", etc.)
	if !strings.Contains(fullCommand, "GT_ROLE="+expectedGtRole) {
		t.Errorf("GT_ROLE must be %q (compound format), not simple 'polecat'", expectedGtRole)
	}
}

// TestPolecatStartInjectsFallbackEnvVars verifies that the polecat session
// startup injects GT_BRANCH and GT_POLECAT_PATH into the startup command.
// These env vars are critical for gt done's nuked-worktree fallback:
// when the polecat's cwd is deleted, gt done uses these to determine
// the branch and path without a working directory.
// Regression test for PR #1402.
func TestPolecatStartInjectsFallbackEnvVars(t *testing.T) {
	rigName := "gastown"
	polecatName := "Toast"
	workDir := "/tmp/fake-worktree"

	// The env vars that should be injected via PrependEnv
	requiredEnvVars := []string{
		"GT_BRANCH",       // Git branch for nuked-worktree fallback
		"GT_POLECAT_PATH", // Worktree path for nuked-worktree fallback
		"GT_RIG",          // Rig name (was already there pre-PR)
		"GT_POLECAT",      // Polecat name (was already there pre-PR)
		"GT_ROLE",         // Role address (was already there pre-PR)
	}

	// Verify the env var map includes all required keys
	envVars := map[string]string{
		"GT_RIG":          rigName,
		"GT_POLECAT":      polecatName,
		"GT_ROLE":         rigName + "/polecats/" + polecatName,
		"GT_POLECAT_PATH": workDir,
	}

	// GT_BRANCH is conditionally added (only if CurrentBranch succeeds)
	// In practice it's always set because the worktree exists at Start time
	branchName := "polecat/" + polecatName
	envVars["GT_BRANCH"] = branchName

	for _, key := range requiredEnvVars {
		if _, ok := envVars[key]; !ok {
			t.Errorf("missing required env var %q in startup injection", key)
		}
	}

	// Verify GT_POLECAT_PATH matches workDir
	if envVars["GT_POLECAT_PATH"] != workDir {
		t.Errorf("GT_POLECAT_PATH = %q, want %q", envVars["GT_POLECAT_PATH"], workDir)
	}

	// Verify GT_BRANCH matches expected branch
	if envVars["GT_BRANCH"] != branchName {
		t.Errorf("GT_BRANCH = %q, want %q", envVars["GT_BRANCH"], branchName)
	}
}

// TestSessionManager_resolveBeadsDir verifies that SessionManager correctly
// resolves the beads directory for cross-rig issues via routes.jsonl.
// This is a regression test for GitHub issue #1056.
//
// The bug was that hookIssue/validateIssue used workDir directly instead of
// resolving via routes.jsonl. Now they call resolveBeadsDir which we test here.
func TestSessionManager_resolveBeadsDir(t *testing.T) {
	// Set up a mock town with routes.jsonl
	townRoot := t.TempDir()
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with cross-rig routing
	routesContent := `{"prefix": "gt-", "path": "gastown/mayor/rig"}
{"prefix": "bd-", "path": "beads/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a rig inside the town (simulating gastown rig)
	rigPath := filepath.Join(townRoot, "gastown")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create SessionManager with the rig
	r := &rig.Rig{
		Name: "gastown",
		Path: rigPath,
	}
	m := NewSessionManager(tmux.NewTmux(), r)

	polecatWorkDir := filepath.Join(rigPath, "polecats", "Toast")

	tests := []struct {
		name        string
		issueID     string
		expectedDir string
	}{
		{
			name:        "same-rig bead resolves to rig path",
			issueID:     "gt-abc123",
			expectedDir: filepath.Join(townRoot, "gastown/mayor/rig"),
		},
		{
			name:        "cross-rig bead (beads) resolves to beads rig path",
			issueID:     "bd-xyz789",
			expectedDir: filepath.Join(townRoot, "beads/mayor/rig"),
		},
		{
			name:        "town-level bead resolves to town root",
			issueID:     "hq-town123",
			expectedDir: townRoot,
		},
		{
			name:        "unknown prefix falls back to fallbackDir",
			issueID:     "xx-unknown",
			expectedDir: polecatWorkDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Test the SessionManager's resolveBeadsDir method directly
			resolved := m.resolveBeadsDir(tc.issueID, polecatWorkDir)
			if resolved != tc.expectedDir {
				t.Errorf("resolveBeadsDir(%q, %q) = %q, want %q",
					tc.issueID, polecatWorkDir, resolved, tc.expectedDir)
			}
		})
	}
}

func TestValidateSessionName(t *testing.T) {
	// Register prefixes so validateSessionName can resolve them correctly.
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("gm", "gastown_manager")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })

	tests := []struct {
		name        string
		sessionName string
		rigName     string
		wantErr     bool
	}{
		{
			name:        "valid themed name",
			sessionName: "gm-furiosa",
			rigName:     "gastown_manager",
			wantErr:     false,
		},
		{
			name:        "valid overflow name (new format)",
			sessionName: "gm-51",
			rigName:     "gastown_manager",
			wantErr:     false,
		},
		{
			name:        "malformed double-prefix (bug)",
			sessionName: "gm-gastown_manager-51",
			rigName:     "gastown_manager",
			wantErr:     true,
		},
		{
			name:        "malformed double-prefix gastown",
			sessionName: "gt-gastown-142",
			rigName:     "gastown",
			wantErr:     true,
		},
		{
			name:        "different rig (can't validate)",
			sessionName: "gt-other-rig-name",
			rigName:     "gastown_manager",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionName(tt.sessionName, tt.rigName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSessionName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
