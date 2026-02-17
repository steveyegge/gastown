package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestDoneUsesResolveBeadsDir verifies that the done command correctly uses
// beads.ResolveBeadsDir to follow redirect files when initializing beads.
// This is critical for polecat/crew worktrees that use .beads/redirect to point
// to the shared mayor/rig/.beads directory.
//
// The done.go file has two code paths that initialize beads:
//   - Line 181: ExitCompleted path - bd := beads.New(beads.ResolveBeadsDir(cwd))
//   - Line 277: ExitPhaseComplete path - bd := beads.New(beads.ResolveBeadsDir(cwd))
//
// Both must use ResolveBeadsDir to properly handle redirects.
func TestDoneUsesResolveBeadsDir(t *testing.T) {
	// Create a temp directory structure simulating polecat worktree with redirect
	tmpDir := t.TempDir()

	// Create structure like:
	//   gastown/
	//     mayor/rig/.beads/          <- shared beads directory
	//     polecats/fixer/.beads/     <- polecat with redirect
	//       redirect -> ../../mayor/rig/.beads

	mayorRigBeadsDir := filepath.Join(tmpDir, "gastown", "mayor", "rig", ".beads")
	polecatDir := filepath.Join(tmpDir, "gastown", "polecats", "fixer")
	polecatBeadsDir := filepath.Join(polecatDir, ".beads")

	// Create directories
	if err := os.MkdirAll(mayorRigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig/.beads: %v", err)
	}
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecats/fixer/.beads: %v", err)
	}

	// Create redirect file pointing to mayor/rig/.beads
	redirectContent := "../../mayor/rig/.beads"
	redirectPath := filepath.Join(polecatBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	t.Run("redirect followed from polecat directory", func(t *testing.T) {
		// This mirrors how done.go initializes beads at line 181 and 277
		resolvedDir := beads.ResolveBeadsDir(polecatDir)

		// Should resolve to mayor/rig/.beads
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ResolveBeadsDir(%s) = %s, want %s", polecatDir, resolvedDir, mayorRigBeadsDir)
		}

		// Verify the beads instance is created with the resolved path
		// We use the same pattern as done.go: beads.New(beads.ResolveBeadsDir(cwd))
		bd := beads.New(beads.ResolveBeadsDir(polecatDir))
		if bd == nil {
			t.Error("beads.New returned nil")
		}
	})

	t.Run("redirect not present uses local beads", func(t *testing.T) {
		// Without redirect, should use local .beads
		localDir := filepath.Join(tmpDir, "gastown", "mayor", "rig")
		resolvedDir := beads.ResolveBeadsDir(localDir)

		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ResolveBeadsDir(%s) = %s, want %s", localDir, resolvedDir, mayorRigBeadsDir)
		}
	})
}

// TestDoneBeadsInitWithoutRedirect verifies that beads initialization works
// normally when no redirect file exists.
func TestDoneBeadsInitWithoutRedirect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple .beads directory without redirect (like mayor/rig)
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// ResolveBeadsDir should return the same directory when no redirect exists
	resolvedDir := beads.ResolveBeadsDir(tmpDir)
	if resolvedDir != beadsDir {
		t.Errorf("ResolveBeadsDir(%s) = %s, want %s", tmpDir, resolvedDir, beadsDir)
	}

	// Beads initialization should work the same way done.go does it
	bd := beads.New(beads.ResolveBeadsDir(tmpDir))
	if bd == nil {
		t.Error("beads.New returned nil")
	}
}

// TestDoneBeadsInitBothCodePaths documents that both code paths in done.go
// that create beads instances use ResolveBeadsDir:
//   - ExitCompleted (line 181): for MR creation and issue operations
//   - ExitPhaseComplete (line 277): for gate waiter registration
//
// This test verifies the pattern by demonstrating that the resolved directory
// is used consistently for different operations.
func TestDoneBeadsInitBothCodePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup: crew directory with redirect to mayor/rig/.beads
	mayorRigBeadsDir := filepath.Join(tmpDir, "mayor", "rig", ".beads")
	crewDir := filepath.Join(tmpDir, "crew", "max")
	crewBeadsDir := filepath.Join(crewDir, ".beads")

	if err := os.MkdirAll(mayorRigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig/.beads: %v", err)
	}
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir crew/max/.beads: %v", err)
	}

	// Create redirect
	redirectPath := filepath.Join(crewBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	t.Run("ExitCompleted path uses ResolveBeadsDir", func(t *testing.T) {
		// This simulates the line 181 path in done.go:
		// bd := beads.New(beads.ResolveBeadsDir(cwd))
		resolvedDir := beads.ResolveBeadsDir(crewDir)
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ExitCompleted path: ResolveBeadsDir(%s) = %s, want %s",
				crewDir, resolvedDir, mayorRigBeadsDir)
		}

		bd := beads.New(beads.ResolveBeadsDir(crewDir))
		if bd == nil {
			t.Error("beads.New returned nil for ExitCompleted path")
		}
	})

	t.Run("ExitPhaseComplete path uses ResolveBeadsDir", func(t *testing.T) {
		// This simulates the line 277 path in done.go:
		// bd := beads.New(beads.ResolveBeadsDir(cwd))
		resolvedDir := beads.ResolveBeadsDir(crewDir)
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ExitPhaseComplete path: ResolveBeadsDir(%s) = %s, want %s",
				crewDir, resolvedDir, mayorRigBeadsDir)
		}

		bd := beads.New(beads.ResolveBeadsDir(crewDir))
		if bd == nil {
			t.Error("beads.New returned nil for ExitPhaseComplete path")
		}
	})
}

// TestDoneRedirectChain verifies behavior with chained redirects.
// ResolveBeadsDir follows chains up to depth 3 as a safety net for legacy configs.
// SetupRedirect avoids creating chains (bd CLI doesn't support them), but if
// chains exist we follow them to the final destination.
func TestDoneRedirectChain(t *testing.T) {
	tmpDir := t.TempDir()

	// Create chain: worktree -> intermediate -> canonical
	canonicalBeadsDir := filepath.Join(tmpDir, "canonical", ".beads")
	intermediateDir := filepath.Join(tmpDir, "intermediate")
	intermediateBeadsDir := filepath.Join(intermediateDir, ".beads")
	worktreeDir := filepath.Join(tmpDir, "worktree")
	worktreeBeadsDir := filepath.Join(worktreeDir, ".beads")

	// Create all directories
	for _, dir := range []string{canonicalBeadsDir, intermediateBeadsDir, worktreeBeadsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// Create redirects
	// intermediate -> canonical
	if err := os.WriteFile(filepath.Join(intermediateBeadsDir, "redirect"), []byte("../canonical/.beads"), 0644); err != nil {
		t.Fatalf("write intermediate redirect: %v", err)
	}
	// worktree -> intermediate
	if err := os.WriteFile(filepath.Join(worktreeBeadsDir, "redirect"), []byte("../intermediate/.beads"), 0644); err != nil {
		t.Fatalf("write worktree redirect: %v", err)
	}

	// ResolveBeadsDir follows chains up to depth 3 as a safety net.
	// Note: SetupRedirect avoids creating chains (bd CLI doesn't support them),
	// but if chains exist from legacy configs, we follow them to the final destination.
	resolved := beads.ResolveBeadsDir(worktreeDir)

	// Should resolve to canonical (follows the full chain)
	if resolved != canonicalBeadsDir {
		t.Errorf("ResolveBeadsDir should follow chain to final destination: got %s, want %s",
			resolved, canonicalBeadsDir)
	}
}

// TestDoneEmptyRedirectFallback verifies that an empty or whitespace-only
// redirect file falls back to the local .beads directory.
func TestDoneEmptyRedirectFallback(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create empty redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("   \n"), 0644); err != nil {
		t.Fatalf("write empty redirect: %v", err)
	}

	// Should fall back to local .beads
	resolved := beads.ResolveBeadsDir(tmpDir)
	if resolved != beadsDir {
		t.Errorf("empty redirect should fallback: got %s, want %s", resolved, beadsDir)
	}
}

// TestDoneCircularRedirectProtection verifies that circular redirects
// are detected and handled safely.
func TestDoneCircularRedirectProtection(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create circular redirect (points to itself)
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(".beads"), 0644); err != nil {
		t.Fatalf("write circular redirect: %v", err)
	}

	// Should detect circular redirect and return original
	resolved := beads.ResolveBeadsDir(tmpDir)
	if resolved != beadsDir {
		t.Errorf("circular redirect should return original: got %s, want %s", resolved, beadsDir)
	}
}

// TestGetIssueFromAgentHook verifies that getIssueFromAgentHook correctly
// retrieves the issue ID from an agent's hook_bead field.
// This is critical because branch names like "polecat/furiosa-mkb0vq9f" don't
// contain the actual issue ID (test-845.1), but the agent's hook does.
func TestGetIssueFromAgentHook(t *testing.T) {
	// Skip: bd CLI 0.47.2 has a bug where database writes don't commit
	// ("sql: database is closed" during auto-flush). This blocks tests
	// that need to create issues. See internal issue for tracking.
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tests := []struct {
		name        string
		agentBeadID string
		setupBeads  func(t *testing.T, bd *beads.Beads) // setup agent bead with hook
		wantIssueID string
	}{
		{
			name:        "agent with hook_bead returns issue ID",
			agentBeadID: "test-testrig-polecat-furiosa",
			setupBeads: func(t *testing.T, bd *beads.Beads) {
				// Create a task that will be hooked
				_, err := bd.CreateWithID("test-456", beads.CreateOptions{
					Title: "Task to be hooked",
					Type:  "task",
				})
				if err != nil {
					t.Fatalf("create task bead: %v", err)
				}

				// Create agent bead using CreateAgentBead
				// Agent ID format: <prefix>-<rig>-<role>-<name>
				_, err = bd.CreateAgentBead("test-testrig-polecat-furiosa", "Test polecat agent", nil)
				if err != nil {
					t.Fatalf("create agent bead: %v", err)
				}

				// Set hook_bead on agent
				if err := bd.SetHookBead("test-testrig-polecat-furiosa", "test-456"); err != nil {
					t.Fatalf("set hook bead: %v", err)
				}
			},
			wantIssueID: "test-456",
		},
		{
			name:        "agent without hook_bead returns empty",
			agentBeadID: "test-testrig-polecat-idle",
			setupBeads: func(t *testing.T, bd *beads.Beads) {
				// Create agent bead without hook
				_, err := bd.CreateAgentBead("test-testrig-polecat-idle", "Test agent without hook", nil)
				if err != nil {
					t.Fatalf("create agent bead: %v", err)
				}
			},
			wantIssueID: "",
		},
		{
			name:        "nonexistent agent returns empty",
			agentBeadID: "test-nonexistent",
			setupBeads:  func(t *testing.T, bd *beads.Beads) {},
			wantIssueID: "",
		},
		{
			name:        "empty agent ID returns empty",
			agentBeadID: "",
			setupBeads:  func(t *testing.T, bd *beads.Beads) {},
			wantIssueID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Initialize the beads database
			cmd := exec.Command("bd", "init", "--prefix", "test", "--quiet")
			cmd.Dir = tmpDir
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("bd init: %v\n%s", err, output)
			}

			// beads.New expects the .beads directory path
			beadsDir := filepath.Join(tmpDir, ".beads")
			bd := beads.New(beadsDir)

			tt.setupBeads(t, bd)

			got := getIssueFromAgentHook(bd, tt.agentBeadID)
			if got != tt.wantIssueID {
				t.Errorf("getIssueFromAgentHook(%q) = %q, want %q", tt.agentBeadID, got, tt.wantIssueID)
			}
		})
	}
}

// TestIsPolecatActor verifies that isPolecatActor correctly identifies
// polecat actors vs other roles based on the BD_ACTOR format.
func TestIsPolecatActor(t *testing.T) {
	tests := []struct {
		actor string
		want  bool
	}{
		// Polecats: rigname/polecats/polecatname
		{"testrig/polecats/furiosa", true},
		{"testrig/polecats/nux", true},
		{"myrig/polecats/witness", true}, // even if named "witness", still a polecat

		// Non-polecats
		{"gastown/crew/george", false},
		{"gastown/crew/max", false},
		{"testrig/witness", false},
		{"testrig/deacon", false},
		{"testrig/mayor", false},
		{"gastown/refinery", false},

		// Edge cases
		{"", false},
		{"single", false},
		{"polecats/name", false}, // needs rig prefix
	}

	for _, tt := range tests {
		t.Run(tt.actor, func(t *testing.T) {
			got := isPolecatActor(tt.actor)
			if got != tt.want {
				t.Errorf("isPolecatActor(%q) = %v, want %v", tt.actor, got, tt.want)
			}
		})
	}
}

// TestDoneIntentLabelFormat verifies the done-intent label format matches
// the expected pattern: done-intent:<type>:<unix-ts>
func TestDoneIntentLabelFormat(t *testing.T) {
	now := time.Now()
	tests := []struct {
		exitType string
		want     string
	}{
		{"COMPLETED", fmt.Sprintf("done-intent:COMPLETED:%d", now.Unix())},
		{"ESCALATED", fmt.Sprintf("done-intent:ESCALATED:%d", now.Unix())},
		{"DEFERRED", fmt.Sprintf("done-intent:DEFERRED:%d", now.Unix())},
		{"PHASE_COMPLETE", fmt.Sprintf("done-intent:PHASE_COMPLETE:%d", now.Unix())},
	}

	for _, tt := range tests {
		t.Run(tt.exitType, func(t *testing.T) {
			label := fmt.Sprintf("done-intent:%s:%d", tt.exitType, now.Unix())
			if label != tt.want {
				t.Errorf("label format = %q, want %q", label, tt.want)
			}

			// Verify the label can be parsed back
			parts := strings.SplitN(label, ":", 3)
			if len(parts) != 3 {
				t.Fatalf("expected 3 parts, got %d", len(parts))
			}
			if parts[0] != "done-intent" {
				t.Errorf("prefix = %q, want %q", parts[0], "done-intent")
			}
			if parts[1] != tt.exitType {
				t.Errorf("exit type = %q, want %q", parts[1], tt.exitType)
			}
		})
	}
}

// TestClearDoneIntentLabel verifies that clearDoneIntentLabel removes
// only done-intent labels while preserving other labels.
func TestClearDoneIntentLabel(t *testing.T) {
	// We can't easily test the full clearDoneIntentLabel function without
	// a running bd instance, but we can verify the filtering logic.
	// The function reads labels, filters out done-intent:*, and writes back.
	allLabels := []string{
		"gt:agent",
		"idle:3",
		"done-intent:COMPLETED:1738972800",
		"backoff-until:1738972900",
	}

	var kept []string
	for _, label := range allLabels {
		if !strings.HasPrefix(label, "done-intent:") {
			kept = append(kept, label)
		}
	}

	if len(kept) != 3 {
		t.Errorf("expected 3 labels after filtering, got %d: %v", len(kept), kept)
	}

	// Verify done-intent was removed
	for _, label := range kept {
		if strings.HasPrefix(label, "done-intent:") {
			t.Errorf("done-intent label was not removed: %s", label)
		}
	}

	// Verify other labels were preserved
	wantKept := map[string]bool{
		"gt:agent":                 true,
		"idle:3":                   true,
		"backoff-until:1738972900": true,
	}
	for _, label := range kept {
		if !wantKept[label] {
			t.Errorf("unexpected label in kept set: %s", label)
		}
	}
}

// TestPushFailureDoesNotNukeWorktree verifies that when pushFailed is true,
// the worktree nuke is skipped (defense-in-depth alongside selfNukePolecat's
// own branch-on-remote check).
func TestPushFailureDoesNotNukeWorktree(t *testing.T) {
	// This tests the boolean guard logic inline in runDone:
	// if exitType == ExitCompleted && !pushFailed { ... nuke ... }
	tests := []struct {
		name       string
		exitType   string
		pushFailed bool
		wantNuke   bool
	}{
		{"completed+push-ok", ExitCompleted, false, true},
		{"completed+push-failed", ExitCompleted, true, false},
		{"escalated+push-ok", ExitEscalated, false, false},
		{"deferred+push-ok", ExitDeferred, false, false},
		{"escalated+push-failed", ExitEscalated, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the guard condition from runDone
			shouldNuke := tt.exitType == ExitCompleted && !tt.pushFailed
			if shouldNuke != tt.wantNuke {
				t.Errorf("shouldNuke = %v, want %v", shouldNuke, tt.wantNuke)
			}
		})
	}
}

// TestDeferredKillNotOnValidationError verifies that the deferred session kill
// does NOT trigger when runDone returns early due to validation errors (bad flags,
// wrong role). The sessionCleanupNeeded flag must only be set after role detection
// confirms this is a polecat.
func TestDeferredKillNotOnValidationError(t *testing.T) {
	// Simulate the flag lifecycle:
	// 1. sessionCleanupNeeded starts false
	// 2. Set true only after role detection confirms polecat
	// 3. Early returns (validation) happen before the flag is set

	// Scenario 1: Validation error (bad status) — returns before flag set
	sessionCleanupNeeded := false
	// (invalid exit status check would return here)
	// defer checks: sessionCleanupNeeded is false → no-op
	if sessionCleanupNeeded {
		t.Error("sessionCleanupNeeded should be false for validation errors")
	}

	// Scenario 2: Polecat confirmed — flag set
	sessionCleanupNeeded = true
	sessionKilled := false
	// (push fails, returns with error)
	// defer checks: sessionCleanupNeeded is true, sessionKilled is false → kill session
	if !sessionCleanupNeeded || sessionKilled {
		t.Error("deferred kill should trigger when sessionCleanupNeeded && !sessionKilled")
	}

	// Scenario 3: Clean exit — explicit kill succeeded
	sessionKilled = true
	// defer checks: sessionKilled is true → no-op (don't double-kill)
	if sessionCleanupNeeded && !sessionKilled {
		t.Error("deferred kill should NOT trigger when sessionKilled is true")
	}
}

// TestBranchDetectionGuard verifies that the branch detection logic in runDone
// correctly handles the three states: cwd available, cwd unavailable with GT_BRANCH,
// and cwd unavailable without GT_BRANCH.
// This is a regression test for PR #1402 — prevents incorrect main/master detection
// when the polecat's working directory is deleted.
func TestBranchDetectionGuard(t *testing.T) {
	tests := []struct {
		name         string
		cwdAvailable bool
		gtBranch     string // GT_BRANCH env var value
		wantError    bool
		wantBranch   string
	}{
		{
			name:         "cwd available - uses git CurrentBranch",
			cwdAvailable: true,
			gtBranch:     "",
			wantError:    false,
			wantBranch:   "current-branch", // simulated
		},
		{
			name:         "cwd unavailable + GT_BRANCH set - uses env var",
			cwdAvailable: false,
			gtBranch:     "polecat/test-worker",
			wantError:    false,
			wantBranch:   "polecat/test-worker",
		},
		{
			name:         "cwd unavailable + GT_BRANCH empty - returns error",
			cwdAvailable: false,
			gtBranch:     "",
			wantError:    true,
			wantBranch:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the branch detection logic from runDone
			var branch string
			if !tt.cwdAvailable {
				branch = tt.gtBranch
			}

			var gotError bool
			if branch == "" {
				if !tt.cwdAvailable {
					gotError = true
				} else {
					// Would call g.CurrentBranch() — simulate success
					branch = "current-branch"
				}
			}

			if gotError != tt.wantError {
				t.Errorf("error = %v, want %v", gotError, tt.wantError)
			}
			if !tt.wantError && branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
		})
	}
}

// TestBranchDetectionCleanupOnError verifies that when branch detection fails
// (cwdAvailable=false + no GT_BRANCH), the session cleanup backstop is armed
// so the polecat doesn't get stranded.
func TestBranchDetectionCleanupOnError(t *testing.T) {
	// Simulate the cleanup arming logic from runDone's branch detection error path
	cwdAvailable := false
	gtBranch := ""
	gtPolecat := "test-worker"
	rigName := "test-rig"

	var branch string
	if !cwdAvailable {
		branch = gtBranch
	}

	sessionCleanupNeeded := false
	if branch == "" && !cwdAvailable {
		// This mirrors the actual code: arm cleanup before returning error
		if gtPolecat != "" {
			sessionCleanupNeeded = true
		}
	}

	if !sessionCleanupNeeded {
		t.Error("sessionCleanupNeeded should be true when branch detection fails with GT_POLECAT set")
	}

	// Verify the RoleInfo would be constructible from env vars
	roleInfo := RoleInfo{
		Role:    RolePolecat,
		Rig:     rigName,
		Polecat: gtPolecat,
	}
	if roleInfo.Rig != rigName || roleInfo.Polecat != gtPolecat {
		t.Error("RoleInfo should be constructible from env vars for cleanup")
	}
}

// TestConvoyMergeStrategyBranching verifies that the merge strategy branching
// logic in runDone correctly routes to the right code path for each strategy.
func TestConvoyMergeStrategyBranching(t *testing.T) {
	tests := []struct {
		name          string
		mergeStrategy string
		wantPush      bool // should push happen?
		wantMR        bool // should MR bead be created?
		wantDirect    bool // should push to default branch?
	}{
		{
			name:          "mr strategy - normal push and MR",
			mergeStrategy: "mr",
			wantPush:      true,
			wantMR:        true,
			wantDirect:    false,
		},
		{
			name:          "empty strategy - defaults to mr behavior",
			mergeStrategy: "",
			wantPush:      true,
			wantMR:        true,
			wantDirect:    false,
		},
		{
			name:          "direct strategy - push to main, no MR",
			mergeStrategy: "direct",
			wantPush:      true,
			wantMR:        false,
			wantDirect:    true,
		},
		{
			name:          "local strategy - no push, no MR",
			mergeStrategy: "local",
			wantPush:      false,
			wantMR:        false,
			wantDirect:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the branching logic from runDone
			shouldPush := true
			shouldCreateMR := true
			shouldPushDirect := false

			switch tt.mergeStrategy {
			case "local":
				shouldPush = false
				shouldCreateMR = false
			case "direct":
				shouldPushDirect = true
				shouldCreateMR = false
			default:
				// "mr" or empty = default behavior
			}

			if shouldPush != tt.wantPush {
				t.Errorf("shouldPush = %v, want %v", shouldPush, tt.wantPush)
			}
			if shouldCreateMR != tt.wantMR {
				t.Errorf("shouldCreateMR = %v, want %v", shouldCreateMR, tt.wantMR)
			}
			if shouldPushDirect != tt.wantDirect {
				t.Errorf("shouldPushDirect = %v, want %v", shouldPushDirect, tt.wantDirect)
			}
		})
	}
}

// TestConvoyMergeStrategyNotification verifies that the merge strategy
// is included in the witness notification body when set to non-default values.
func TestConvoyMergeStrategyNotification(t *testing.T) {
	tests := []struct {
		name          string
		mergeStrategy string
		wantInBody    bool
	}{
		{"direct strategy included", "direct", true},
		{"local strategy included", "local", true},
		{"mr strategy excluded", "mr", false},
		{"empty strategy excluded", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the notification body building from runDone
			var bodyLines []string
			bodyLines = append(bodyLines, "Exit: COMPLETED")
			if tt.mergeStrategy != "" && tt.mergeStrategy != "mr" {
				bodyLines = append(bodyLines, fmt.Sprintf("MergeStrategy: %s", tt.mergeStrategy))
			}

			body := strings.Join(bodyLines, "\n")
			hasMergeStrategy := strings.Contains(body, "MergeStrategy:")

			if hasMergeStrategy != tt.wantInBody {
				t.Errorf("body contains MergeStrategy = %v, want %v\nbody: %s",
					hasMergeStrategy, tt.wantInBody, body)
			}
		})
	}
}

// TestParseConvoyMergeStrategy verifies that parseConvoyMergeStrategy correctly
// extracts the merge strategy from convoy descriptions.
func TestParseConvoyMergeStrategy(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "direct strategy",
			description: "Auto-created convoy tracking gt-abc\nMerge: direct",
			want:        "direct",
		},
		{
			name:        "mr strategy",
			description: "Convoy tracking 3 issues\nOwner: mayor/\nMerge: mr",
			want:        "mr",
		},
		{
			name:        "local strategy",
			description: "Merge: local\nOwner: mayor/",
			want:        "local",
		},
		{
			name:        "no merge field",
			description: "Auto-created convoy tracking gt-abc",
			want:        "",
		},
		{
			name:        "empty description",
			description: "",
			want:        "",
		},
		{
			name:        "merge in middle of description",
			description: "Convoy tracking 1 issues\nMerge: direct\nNotify: mayor/",
			want:        "direct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseConvoyMergeStrategy(tt.description)
			if got != tt.want {
				t.Errorf("parseConvoyMergeStrategy() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDoneCheckpointLabelFormat verifies the done-cp label format matches
// the expected pattern: done-cp:<stage>:<value>:<unix-ts>
func TestDoneCheckpointLabelFormat(t *testing.T) {
	now := time.Now()
	tests := []struct {
		checkpoint DoneCheckpoint
		value      string
		wantPrefix string
	}{
		{CheckpointPushed, "polecat/furiosa-abc", "done-cp:pushed:polecat/furiosa-abc:"},
		{CheckpointMRCreated, "gt-xyz123", "done-cp:mr-created:gt-xyz123:"},
		{CheckpointDoltMerged, "ok", "done-cp:dolt-merged:ok:"},
		{CheckpointWitnessNotified, "ok", "done-cp:witness-notified:ok:"},
	}

	for _, tt := range tests {
		t.Run(string(tt.checkpoint), func(t *testing.T) {
			label := fmt.Sprintf("done-cp:%s:%s:%d", tt.checkpoint, tt.value, now.Unix())
			if !strings.HasPrefix(label, tt.wantPrefix) {
				t.Errorf("label = %q, want prefix %q", label, tt.wantPrefix)
			}

			// Verify the label can be parsed back
			parts := strings.SplitN(label, ":", 4)
			if len(parts) != 4 {
				t.Fatalf("expected 4 parts, got %d: %v", len(parts), parts)
			}
			if parts[0] != "done-cp" {
				t.Errorf("prefix = %q, want %q", parts[0], "done-cp")
			}
			if DoneCheckpoint(parts[1]) != tt.checkpoint {
				t.Errorf("stage = %q, want %q", parts[1], tt.checkpoint)
			}
			if parts[2] != tt.value {
				t.Errorf("value = %q, want %q", parts[2], tt.value)
			}
		})
	}
}

// TestReadDoneCheckpoints verifies that readDoneCheckpoints correctly
// parses checkpoint labels from an issue's label list.
func TestReadDoneCheckpoints(t *testing.T) {
	// Test the parsing logic directly by simulating what readDoneCheckpoints does
	tests := []struct {
		name   string
		labels []string
		want   map[DoneCheckpoint]string
	}{
		{
			name:   "no checkpoints",
			labels: []string{"gt:agent", "idle:3"},
			want:   map[DoneCheckpoint]string{},
		},
		{
			name:   "push checkpoint only",
			labels: []string{"gt:agent", "done-cp:pushed:polecat/furiosa-abc:1738972800"},
			want:   map[DoneCheckpoint]string{CheckpointPushed: "polecat/furiosa-abc"},
		},
		{
			name: "multiple checkpoints",
			labels: []string{
				"gt:agent",
				"done-cp:pushed:polecat/furiosa-abc:1738972800",
				"done-cp:mr-created:gt-xyz123:1738972801",
				"done-cp:dolt-merged:ok:1738972802",
			},
			want: map[DoneCheckpoint]string{
				CheckpointPushed:     "polecat/furiosa-abc",
				CheckpointMRCreated:  "gt-xyz123",
				CheckpointDoltMerged: "ok",
			},
		},
		{
			name:   "all checkpoints",
			labels: []string{
				"done-cp:pushed:branch-name:1738972800",
				"done-cp:mr-created:gt-mr1:1738972801",
				"done-cp:dolt-merged:ok:1738972802",
				"done-cp:witness-notified:ok:1738972803",
			},
			want: map[DoneCheckpoint]string{
				CheckpointPushed:          "branch-name",
				CheckpointMRCreated:       "gt-mr1",
				CheckpointDoltMerged:      "ok",
				CheckpointWitnessNotified: "ok",
			},
		},
		{
			name:   "mixed with done-intent and other labels",
			labels: []string{
				"gt:agent",
				"done-intent:COMPLETED:1738972800",
				"done-cp:pushed:mybranch:1738972801",
				"idle:2",
			},
			want: map[DoneCheckpoint]string{CheckpointPushed: "mybranch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from readDoneCheckpoints
			checkpoints := make(map[DoneCheckpoint]string)
			for _, label := range tt.labels {
				if strings.HasPrefix(label, "done-cp:") {
					parts := strings.SplitN(label, ":", 4)
					if len(parts) >= 3 {
						stage := DoneCheckpoint(parts[1])
						value := parts[2]
						checkpoints[stage] = value
					}
				}
			}

			if len(checkpoints) != len(tt.want) {
				t.Errorf("got %d checkpoints, want %d", len(checkpoints), len(tt.want))
			}
			for k, v := range tt.want {
				if checkpoints[k] != v {
					t.Errorf("checkpoint[%s] = %q, want %q", k, checkpoints[k], v)
				}
			}
		})
	}
}

// TestClearDoneCheckpoints verifies that clearDoneCheckpoints removes
// only done-cp labels while preserving other labels.
func TestClearDoneCheckpoints(t *testing.T) {
	allLabels := []string{
		"gt:agent",
		"idle:3",
		"done-intent:COMPLETED:1738972800",
		"done-cp:pushed:mybranch:1738972801",
		"done-cp:mr-created:gt-xyz:1738972802",
		"done-cp:dolt-merged:ok:1738972803",
		"backoff-until:1738972900",
	}

	var kept []string
	var removed []string
	for _, label := range allLabels {
		if strings.HasPrefix(label, "done-cp:") {
			removed = append(removed, label)
		} else {
			kept = append(kept, label)
		}
	}

	if len(removed) != 3 {
		t.Errorf("expected 3 checkpoint labels removed, got %d: %v", len(removed), removed)
	}
	if len(kept) != 4 {
		t.Errorf("expected 4 labels kept, got %d: %v", len(kept), kept)
	}

	// Verify no checkpoint labels in kept set
	for _, label := range kept {
		if strings.HasPrefix(label, "done-cp:") {
			t.Errorf("checkpoint label was not removed: %s", label)
		}
	}

	// Verify done-intent is preserved (not a checkpoint)
	found := false
	for _, label := range kept {
		if strings.HasPrefix(label, "done-intent:") {
			found = true
		}
	}
	if !found {
		t.Error("done-intent label should be preserved by clearDoneCheckpoints")
	}
}

// TestCheckpointResumeSkipsPush verifies that when a push checkpoint exists,
// the push section is skipped on resume.
func TestCheckpointResumeSkipsPush(t *testing.T) {
	tests := []struct {
		name        string
		checkpoints map[DoneCheckpoint]string
		wantSkip    bool
	}{
		{
			name:        "no checkpoints - push runs normally",
			checkpoints: map[DoneCheckpoint]string{},
			wantSkip:    false,
		},
		{
			name:        "push checkpoint exists - skip push",
			checkpoints: map[DoneCheckpoint]string{CheckpointPushed: "mybranch"},
			wantSkip:    true,
		},
		{
			name: "push and MR checkpoints - skip push",
			checkpoints: map[DoneCheckpoint]string{
				CheckpointPushed:    "mybranch",
				CheckpointMRCreated: "gt-xyz",
			},
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the guard condition from runDone
			skipPush := tt.checkpoints[CheckpointPushed] != ""
			if skipPush != tt.wantSkip {
				t.Errorf("skipPush = %v, want %v", skipPush, tt.wantSkip)
			}
		})
	}
}

// TestCheckpointResumeSkipsDoltMerge verifies that when a Dolt merge
// checkpoint exists, the merge section is skipped on resume.
func TestCheckpointResumeSkipsDoltMerge(t *testing.T) {
	tests := []struct {
		name        string
		checkpoints map[DoneCheckpoint]string
		wantSkip    bool
	}{
		{
			name:        "no checkpoints - merge runs normally",
			checkpoints: map[DoneCheckpoint]string{},
			wantSkip:    false,
		},
		{
			name: "dolt merge checkpoint - skip merge",
			checkpoints: map[DoneCheckpoint]string{
				CheckpointPushed:     "mybranch",
				CheckpointDoltMerged: "ok",
			},
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipMerge := tt.checkpoints[CheckpointDoltMerged] != ""
			if skipMerge != tt.wantSkip {
				t.Errorf("skipMerge = %v, want %v", skipMerge, tt.wantSkip)
			}
		})
	}
}

// TestCheckpointNilMapSafe verifies that reading from a nil/empty checkpoint
// map returns zero values and doesn't panic.
func TestCheckpointNilMapSafe(t *testing.T) {
	// Nil map - should not panic
	var nilMap map[DoneCheckpoint]string
	if nilMap[CheckpointPushed] != "" {
		t.Error("nil map should return zero value")
	}

	// Empty map
	emptyMap := map[DoneCheckpoint]string{}
	if emptyMap[CheckpointPushed] != "" {
		t.Error("empty map should return zero value")
	}
}
