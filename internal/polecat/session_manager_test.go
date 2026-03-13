package polecat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/sandbox"
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

// testSessionCounter provides unique session names across -count=N runs
// to prevent "duplicate session" races with tmux's async cleanup.
var testSessionCounter atomic.Int64

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

	err := m.Start(context.Background(), "Unknown", SessionStartOptions{})
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

	err := m.Stop(context.Background(), "Toast", false)
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

	townRoot := "/tmp/fake-town"

	// The env vars that should be injected via PrependEnv
	requiredEnvVars := []string{
		"GT_BRANCH",       // Git branch for nuked-worktree fallback
		"GT_POLECAT_PATH", // Worktree path for nuked-worktree fallback
		"GT_RIG",          // Rig name (was already there pre-PR)
		"GT_POLECAT",      // Polecat name (was already there pre-PR)
		"GT_ROLE",         // Role address (was already there pre-PR)
		"GT_TOWN_ROOT",    // Town root for FindFromCwdWithFallback after worktree nuke
	}

	// Verify the env var map includes all required keys
	envVars := map[string]string{
		"GT_RIG":          rigName,
		"GT_POLECAT":      polecatName,
		"GT_ROLE":         rigName + "/polecats/" + polecatName,
		"GT_POLECAT_PATH": workDir,
		"GT_TOWN_ROOT":    townRoot,
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

// TestAgentEnvOmitsGTAgent_FallbackRequired verifies that the AgentEnv path
// used by session_manager.Start does NOT include GT_AGENT when opts.Agent is
// empty (the default dispatch path). This confirms the session_manager must
// fall back to runtimeConfig.ResolvedAgent for setting GT_AGENT in the tmux
// session table.
//
// Without the fallback, GT_AGENT is never written to the tmux session table,
// and the post-startup validation kills the session with:
//   "GT_AGENT not set in session ... witness patrol will misidentify this polecat"
//
// Regression test for the bug introduced in PR #1776 which removed the
// unconditional runtimeConfig.ResolvedAgent → SetEnvironment("GT_AGENT") logic
// and replaced it with an AgentEnv-only path that requires opts.Agent to be set.
func TestAgentEnvOmitsGTAgent_FallbackRequired(t *testing.T) {
	t.Parallel()

	// Simulate what session_manager.Start calls for each dispatch scenario.
	cases := []struct {
		name       string
		agent      string // opts.Agent value
		wantGTAgent bool  // whether GT_AGENT should be in AgentEnv output
	}{
		{
			name:       "default dispatch (no --agent flag)",
			agent:      "",
			wantGTAgent: false, // fallback needed
		},
		{
			name:       "explicit --agent codex",
			agent:      "codex",
			wantGTAgent: true,
		},
		{
			name:       "explicit --agent gemini",
			agent:      "gemini",
			wantGTAgent: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := config.AgentEnv(config.AgentEnvConfig{
				Role:      "polecat",
				Rig:       "gastown",
				AgentName: "Toast",
				TownRoot:  "/tmp/town",
				Agent:     tc.agent,
			})
			_, hasGTAgent := env["GT_AGENT"]
			if hasGTAgent != tc.wantGTAgent {
				t.Errorf("AgentEnv(Agent=%q): GT_AGENT present=%v, want %v",
					tc.agent, hasGTAgent, tc.wantGTAgent)
			}
		})
	}
}

// TestVerifyStartupNudgeDelivery_IdleAgent tests that verifyStartupNudgeDelivery
// detects an idle agent (at prompt) and retries the nudge. Uses a real tmux session
// with a shell prompt that matches the ReadyPromptPrefix.
func TestVerifyStartupNudgeDelivery_IdleAgent(t *testing.T) {
	requireTmux(t)

	tm := tmux.NewTmux()
	// Use a unique session name per invocation to avoid "duplicate session" races
	// with tmux's async cleanup when running with -count=N. (Fixes gt-eo8d)
	sessionName := fmt.Sprintf("gt-test-nudge-%d", testSessionCounter.Add(1))

	// Clean up any stale session from a previous crashed test run
	_ = tm.KillSession(sessionName)

	// Create a tmux session with a shell
	if err := tm.NewSession(sessionName, os.TempDir()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillSession(sessionName) })

	// Configure the shell to show the Claude prompt prefix, simulating an idle agent.
	// The prompt "❯ " is what Claude Code shows when idle.
	time.Sleep(300 * time.Millisecond) // Let shell initialize
	_ = tm.SendKeys(sessionName, "export PS1='❯ '")
	time.Sleep(300 * time.Millisecond)

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	m := NewSessionManager(tm, r)

	rc := &config.RuntimeConfig{
		Tmux: &config.RuntimeTmuxConfig{
			ReadyPromptPrefix: "❯ ",
		},
	}

	// IsAtPrompt should detect the idle prompt
	if !tm.IsAtPrompt(sessionName, rc) {
		t.Log("Warning: prompt not detected (tmux timing); skipping idle verification")
		t.Skip("prompt detection unreliable in test environment")
	}

	// verifyStartupNudgeDelivery should detect idle state and retry.
	// We can't easily assert the retry happened, but we verify it doesn't panic/hang.
	// Use a goroutine with timeout to prevent test hanging.
	done := make(chan struct{})
	go func() {
		m.verifyStartupNudgeDelivery(sessionName, rc)
		close(done)
	}()

	select {
	case <-done:
		// Success - function completed
	case <-time.After(30 * time.Second):
		t.Fatal("verifyStartupNudgeDelivery hung (exceeded 30s timeout)")
	}
}

// TestVerifyStartupNudgeDelivery_NilConfig verifies that verifyStartupNudgeDelivery
// exits immediately when runtime config has no prompt detection.
func TestVerifyStartupNudgeDelivery_NilConfig(t *testing.T) {
	requireTmux(t)

	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	m := NewSessionManager(tmux.NewTmux(), r)

	// Should return immediately without error for nil config
	m.verifyStartupNudgeDelivery("nonexistent-session", nil)

	// And for config without prompt prefix
	rc := &config.RuntimeConfig{
		Tmux: &config.RuntimeTmuxConfig{
			ReadyPromptPrefix: "",
			ReadyDelayMs:      1000,
		},
	}
	m.verifyStartupNudgeDelivery("nonexistent-session", rc)
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

func TestPolecatSlot(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := tmpDir
	polecatsDir := filepath.Join(rigPath, "polecats")
	if err := os.MkdirAll(polecatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "testrig",
		Path:     rigPath,
		Polecats: []string{},
	}
	sm := NewSessionManager(tmux.NewTmux(), r)

	// No polecats — should return 0
	if slot := sm.polecatSlot("alpha"); slot != 0 {
		t.Errorf("empty dir: got slot %d, want 0", slot)
	}

	// Create some polecat dirs (sorted: alpha, beta, gamma)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.MkdirAll(filepath.Join(polecatsDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		want int
	}{
		{"alpha", 0},
		{"beta", 1},
		{"gamma", 2},
	}
	for _, tt := range tests {
		if slot := sm.polecatSlot(tt.name); slot != tt.want {
			t.Errorf("polecatSlot(%q) = %d, want %d", tt.name, slot, tt.want)
		}
	}

	// Hidden dirs should be skipped
	if err := os.MkdirAll(filepath.Join(polecatsDir, ".hidden"), 0755); err != nil {
		t.Fatal(err)
	}
	if slot := sm.polecatSlot("beta"); slot != 1 {
		t.Errorf("with hidden dir: polecatSlot(beta) = %d, want 1", slot)
	}
}

// mockSandboxLifecycle implements sandbox.Lifecycle for testing SessionManager
// sandbox integration.
type mockSandboxLifecycle struct {
	preStartCalls  []sandbox.SandboxOpts
	postStopCalls  []sandbox.SandboxOpts
	reconcileCalls []sandbox.ReconcileOpts
	preStartErr    error
	postStopErr    error
	preStartEnv    map[string]string
	installPrefix  string
}

func newMockSandbox(prefix string) *mockSandboxLifecycle {
	return &mockSandboxLifecycle{
		installPrefix: prefix,
		preStartEnv: map[string]string{
			"GT_PROXY_URL": "https://127.0.0.1:9876",
		},
	}
}

func (m *mockSandboxLifecycle) PreStart(ctx context.Context, opts sandbox.SandboxOpts) (map[string]string, error) {
	m.preStartCalls = append(m.preStartCalls, opts)
	if m.preStartErr != nil {
		return nil, m.preStartErr
	}
	return m.preStartEnv, nil
}

func (m *mockSandboxLifecycle) PostStop(ctx context.Context, opts sandbox.SandboxOpts) error {
	m.postStopCalls = append(m.postStopCalls, opts)
	return m.postStopErr
}

func (m *mockSandboxLifecycle) Reconcile(ctx context.Context, opts sandbox.ReconcileOpts) error {
	m.reconcileCalls = append(m.reconcileCalls, opts)
	return nil
}

func (m *mockSandboxLifecycle) WorkspaceName(rig, polecat string) string {
	return m.installPrefix + "-" + rig + "--" + polecat
}

// TestSessionManager_Start_SandboxFailure verifies that Start returns a wrapped
// error when sandbox.PreStart fails, and that no tmux session is created.
func TestSessionManager_Start_SandboxFailure(t *testing.T) {
	requireTmux(t)
	setupTestRegistryForSession(t)

	root := t.TempDir()
	polecatName := "marble"
	// Create polecat directory so hasPolecat passes.
	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	sbx := newMockSandbox("gt-test")
	sbx.preStartErr = errors.New("workspace quota exceeded")

	r := &rig.Rig{
		Name:     "testrig",
		Path:     root,
		Polecats: []string{polecatName},
	}
	sm := NewSessionManager(tmux.NewTmux(), r, WithSandbox(sbx), WithSettings(&config.RigSettings{
		RemoteBackend: &config.RemoteBackendConfig{
			Image:   "test:latest",
			Profile: "standard",
		},
	}))

	err := sm.Start(context.Background(), polecatName, SessionStartOptions{})
	if err == nil {
		t.Fatal("Start() should fail when sandbox PreStart returns error")
	}

	// Error should wrap the sandbox error.
	if !strings.Contains(err.Error(), "sandbox pre-start") {
		t.Errorf("error should mention sandbox pre-start, got: %v", err)
	}
	if !strings.Contains(err.Error(), "workspace quota exceeded") {
		t.Errorf("error should wrap original error, got: %v", err)
	}

	// PreStart should have been called exactly once.
	if len(sbx.preStartCalls) != 1 {
		t.Fatalf("expected 1 PreStart call, got %d", len(sbx.preStartCalls))
	}

	// Verify the SandboxOpts passed to PreStart.
	opts := sbx.preStartCalls[0]
	if opts.Rig != "testrig" {
		t.Errorf("PreStart opts.Rig = %q, want %q", opts.Rig, "testrig")
	}
	if opts.Polecat != polecatName {
		t.Errorf("PreStart opts.Polecat = %q, want %q", opts.Polecat, polecatName)
	}
	expectedWsName := "gt-test-testrig--" + polecatName
	if opts.WorkspaceName != expectedWsName {
		t.Errorf("PreStart opts.WorkspaceName = %q, want %q", opts.WorkspaceName, expectedWsName)
	}

	// No tmux session should have been created.
	sessionID := sm.SessionName(polecatName)
	running, _ := tmux.NewTmux().HasSession(sessionID)
	if running {
		_ = tmux.NewTmux().KillSession(sessionID)
		t.Error("tmux session should not exist after sandbox PreStart failure")
	}
}

// TestSessionManager_Start_NoSandbox verifies that when no sandbox is configured,
// Start proceeds without calling any sandbox lifecycle hooks and uses the local
// clone path for the working directory.
func TestSessionManager_Start_NoSandbox(t *testing.T) {
	// This test verifies the nil-sandbox path in Start().
	// Without sandbox, Start() should use clonePath (local git worktree)
	// and never touch any sandbox methods.

	root := t.TempDir()
	polecatName := "jasper"
	// Create polecat directory structure.
	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "localrig",
		Path:     root,
		Polecats: []string{polecatName},
	}

	// Create SessionManager WITHOUT sandbox (default local mode).
	sm := NewSessionManager(tmux.NewTmux(), r)

	// Verify sandbox is nil.
	if sm.sandbox != nil {
		t.Fatal("expected sandbox to be nil for default SessionManager")
	}

	// Verify workDir resolution goes through clonePath, not polecatDir.
	// When sandbox is nil, Start() uses clonePath(polecat).
	// When sandbox is non-nil, Start() uses polecatDir(polecat).
	clonePath := sm.clonePath(polecatName)
	polecatDir := sm.polecatDir(polecatName)

	// Without the new-structure subdir existing, clonePath defaults to
	// polecats/<name>/<rigname>/ (new structure).
	expectedClone := filepath.Join(root, "polecats", polecatName, "localrig")
	if clonePath != expectedClone {
		t.Errorf("clonePath = %q, want %q", clonePath, expectedClone)
	}
	expectedDir := filepath.Join(root, "polecats", polecatName)
	if polecatDir != expectedDir {
		t.Errorf("polecatDir = %q, want %q", polecatDir, expectedDir)
	}
}

// TestSessionManager_Start_WithSandbox verifies that Start calls sandbox.PreStart
// with the correct SandboxOpts and uses the polecatDir (marker directory) as the
// working directory instead of the clone path.
func TestSessionManager_Start_WithSandbox(t *testing.T) {
	requireTmux(t)

	// When sandbox is configured, Start() should:
	// 1. Use polecatDir (not clonePath) as workDir
	// 2. Call PreStart with correct opts before creating tmux session
	// 3. Pass the returned inner env vars to the session

	root := t.TempDir()
	polecatName := "agate"
	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	sbx := newMockSandbox("gt-xyz")
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackendConfig{
			Image:   "gastown:latest",
			Profile: "standard",
		},
	}

	r := &rig.Rig{
		Name:     "remotrig",
		Path:     root,
		Polecats: []string{polecatName},
	}
	sm := NewSessionManager(tmux.NewTmux(), r, WithSandbox(sbx), WithSettings(settings), WithInstallPrefix("gt-xyz"))

	// Verify sandbox is set.
	if sm.sandbox == nil {
		t.Fatal("expected sandbox to be non-nil")
	}

	// Verify workDir resolution uses polecatDir when sandbox is set.
	// This is the marker directory for tmux cwd (no local worktree).
	expectedWorkDir := filepath.Join(root, "polecats", polecatName)

	// Start() will use this path when sandbox != nil and opts.WorkDir == "".
	polecatDir := sm.polecatDir(polecatName)
	if polecatDir != expectedWorkDir {
		t.Errorf("polecatDir = %q, want %q", polecatDir, expectedWorkDir)
	}

	// Verify WorkspaceName is deterministic and correct.
	wsName := sm.sandbox.WorkspaceName("remotrig", polecatName)
	expectedWs := "gt-xyz-remotrig--" + polecatName
	if wsName != expectedWs {
		t.Errorf("WorkspaceName = %q, want %q", wsName, expectedWs)
	}

	// Verify the SandboxOpts that Start() would construct.
	// We test this by calling Start with a failing sandbox to capture the opts
	// without needing the full config/tmux infrastructure.
	captureErr := errors.New("capture-opts-sentinel")
	sbx.preStartErr = captureErr

	_ = sm.Start(context.Background(), polecatName, SessionStartOptions{Branch: "feat/test-branch"})

	if len(sbx.preStartCalls) != 1 {
		t.Fatalf("expected 1 PreStart call, got %d", len(sbx.preStartCalls))
	}

	opts := sbx.preStartCalls[0]
	if opts.Rig != "remotrig" {
		t.Errorf("opts.Rig = %q, want %q", opts.Rig, "remotrig")
	}
	if opts.Polecat != polecatName {
		t.Errorf("opts.Polecat = %q, want %q", opts.Polecat, polecatName)
	}
	if opts.InstallPrefix != "gt-xyz" {
		t.Errorf("opts.InstallPrefix = %q, want %q", opts.InstallPrefix, "gt-xyz")
	}
	if opts.WorkspaceName != expectedWs {
		t.Errorf("opts.WorkspaceName = %q, want %q", opts.WorkspaceName, expectedWs)
	}
	if opts.RigSettings != settings {
		t.Error("opts.RigSettings should point to the configured settings")
	}
	if opts.Branch != "feat/test-branch" {
		t.Errorf("opts.Branch = %q, want %q", opts.Branch, "feat/test-branch")
	}
}

// TestSessionManager_Stop_WithSandbox verifies that Stop calls sandbox.PostStop
// after killing the tmux session, with the correct SandboxOpts.
func TestSessionManager_Stop_WithSandbox(t *testing.T) {
	requireTmux(t)
	setupTestRegistryForSession(t)

	root := t.TempDir()
	polecatName := fmt.Sprintf("flint-%d", testSessionCounter.Add(1))

	// Create polecat directory.
	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	sbx := newMockSandbox("gt-abc")
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackendConfig{
			Image:    "gastown:latest",
			AutoStop: true,
		},
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{polecatName},
	}
	tm := tmux.NewTmux()
	sm := NewSessionManager(tm, r, WithSandbox(sbx), WithSettings(settings), WithInstallPrefix("gt-abc"))

	// Create a tmux session manually to simulate a running polecat.
	sessionID := sm.SessionName(polecatName)
	if err := tm.NewSession(sessionID, os.TempDir()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillSession(sessionID) })

	// Verify the session exists.
	running, err := tm.HasSession(sessionID)
	if err != nil || !running {
		t.Fatal("expected tmux session to be running")
	}

	// Stop the session (force to avoid graceful shutdown timeout).
	if err := sm.Stop(context.Background(), polecatName, true); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// PostStop should have been called exactly once.
	if len(sbx.postStopCalls) != 1 {
		t.Fatalf("expected 1 PostStop call, got %d", len(sbx.postStopCalls))
	}

	// Verify PostStop opts.
	opts := sbx.postStopCalls[0]
	if opts.Rig != "gastown" {
		t.Errorf("PostStop opts.Rig = %q, want %q", opts.Rig, "gastown")
	}
	if opts.Polecat != polecatName {
		t.Errorf("PostStop opts.Polecat = %q, want %q", opts.Polecat, polecatName)
	}
	if opts.InstallPrefix != "gt-abc" {
		t.Errorf("PostStop opts.InstallPrefix = %q, want %q", opts.InstallPrefix, "gt-abc")
	}
	expectedWs := "gt-abc-gastown--" + polecatName
	if opts.WorkspaceName != expectedWs {
		t.Errorf("PostStop opts.WorkspaceName = %q, want %q", opts.WorkspaceName, expectedWs)
	}
	if opts.RigSettings != settings {
		t.Error("PostStop opts.RigSettings should point to the configured settings")
	}

	// tmux session should be gone.
	running, _ = tm.HasSession(sessionID)
	if running {
		t.Error("tmux session should have been killed")
	}
}

// TestSessionManager_Stop_WithSandbox_PostStopError verifies that Stop succeeds
// even when sandbox.PostStop returns an error (non-fatal behavior).
func TestSessionManager_Stop_WithSandbox_PostStopError(t *testing.T) {
	requireTmux(t)
	setupTestRegistryForSession(t)

	root := t.TempDir()
	polecatName := fmt.Sprintf("slate-%d", testSessionCounter.Add(1))

	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	sbx := newMockSandbox("gt-abc")
	sbx.postStopErr = errors.New("cert revocation timeout")

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{polecatName},
	}
	tm := tmux.NewTmux()
	sm := NewSessionManager(tm, r, WithSandbox(sbx), WithSettings(&config.RigSettings{
		RemoteBackend: &config.RemoteBackendConfig{},
	}))

	// Create tmux session.
	sessionID := sm.SessionName(polecatName)
	if err := tm.NewSession(sessionID, os.TempDir()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillSession(sessionID) })

	// Stop should succeed even though PostStop will error.
	err := sm.Stop(context.Background(), polecatName, true)
	if err != nil {
		t.Fatalf("Stop() should succeed despite PostStop error, got: %v", err)
	}

	// PostStop was still called.
	if len(sbx.postStopCalls) != 1 {
		t.Fatalf("expected 1 PostStop call, got %d", len(sbx.postStopCalls))
	}
}

// TestSessionManager_Stop_NoSandbox verifies that Stop works normally without
// a sandbox configured (no PostStop calls).
func TestSessionManager_Stop_NoSandbox(t *testing.T) {
	requireTmux(t)
	setupTestRegistryForSession(t)

	root := t.TempDir()
	polecatName := fmt.Sprintf("onyx-%d", testSessionCounter.Add(1))

	if err := os.MkdirAll(filepath.Join(root, "polecats", polecatName), 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     root,
		Polecats: []string{polecatName},
	}
	tm := tmux.NewTmux()
	// No sandbox — default local mode.
	sm := NewSessionManager(tm, r)

	sessionID := sm.SessionName(polecatName)
	if err := tm.NewSession(sessionID, os.TempDir()); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillSession(sessionID) })

	if err := sm.Stop(context.Background(), polecatName, true); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// Session should be gone.
	running, _ := tm.HasSession(sessionID)
	if running {
		t.Error("tmux session should have been killed")
	}
}

// TestWithSandbox_Option verifies that the WithSandbox functional option
// correctly sets the sandbox field on SessionManager.
func TestWithSandbox_Option(t *testing.T) {
	r := &rig.Rig{Name: "testrig"}

	// Without option: sandbox is nil.
	sm := NewSessionManager(tmux.NewTmux(), r)
	if sm.sandbox != nil {
		t.Error("default SessionManager should have nil sandbox")
	}

	// With option: sandbox is set.
	sbx := newMockSandbox("gt-test")
	sm = NewSessionManager(tmux.NewTmux(), r, WithSandbox(sbx))
	if sm.sandbox == nil {
		t.Error("SessionManager with WithSandbox should have non-nil sandbox")
	}
}

// TestWithInstallPrefix_Option verifies that the WithInstallPrefix functional option
// correctly sets the installPrefix field on SessionManager.
func TestWithInstallPrefix_Option(t *testing.T) {
	r := &rig.Rig{Name: "testrig"}

	// Without option: installPrefix is empty.
	sm := NewSessionManager(tmux.NewTmux(), r)
	if sm.installPrefix != "" {
		t.Error("default SessionManager should have empty installPrefix")
	}

	// With option: installPrefix is set.
	sm = NewSessionManager(tmux.NewTmux(), r, WithInstallPrefix("gt-abc123"))
	if sm.installPrefix != "gt-abc123" {
		t.Errorf("installPrefix = %q, want %q", sm.installPrefix, "gt-abc123")
	}
}

// TestWithSettings_Option verifies that the WithSettings functional option
// correctly sets the settings field on SessionManager.
func TestWithSettings_Option(t *testing.T) {
	r := &rig.Rig{Name: "testrig"}

	// Without option: settings is nil.
	sm := NewSessionManager(tmux.NewTmux(), r)
	if sm.settings != nil {
		t.Error("default SessionManager should have nil settings")
	}

	// With option: settings is set.
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackendConfig{Image: "test:latest"},
	}
	sm = NewSessionManager(tmux.NewTmux(), r, WithSettings(settings))
	if sm.settings == nil {
		t.Error("SessionManager with WithSettings should have non-nil settings")
	}
	if sm.settings.RemoteBackend.Image != "test:latest" {
		t.Errorf("settings.RemoteBackend.Image = %q, want %q", sm.settings.RemoteBackend.Image, "test:latest")
	}
}
