package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/checkpoint"
	"github.com/steveyegge/gastown/internal/constants"
)

func writeTestRoutes(t *testing.T, townRoot string, routes []beads.Route) {
	t.Helper()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}
}

func TestGetAgentBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	cases := []struct {
		name string
		ctx  RoleContext
		want string
	}{
		{
			name: "mayor",
			ctx: RoleContext{
				Role:     RoleMayor,
				TownRoot: townRoot,
			},
			want: "hq-mayor",
		},
		{
			name: "deacon",
			ctx: RoleContext{
				Role:     RoleDeacon,
				TownRoot: townRoot,
			},
			want: "hq-deacon",
		},
		{
			name: "witness",
			ctx: RoleContext{
				Role:     RoleWitness,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-witness",
		},
		{
			name: "refinery",
			ctx: RoleContext{
				Role:     RoleRefinery,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-refinery",
		},
		{
			name: "polecat",
			ctx: RoleContext{
				Role:     RolePolecat,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			// Polecats use town-level beads with hq- prefix (fix for gt-myc)
			want: "hq-beads-polecat-lex",
		},
		{
			name: "crew",
			ctx: RoleContext{
				Role:     RoleCrew,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			want: "bd-beads-crew-lex",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getAgentBeadID(tc.ctx)
			if got != tc.want {
				t.Fatalf("getAgentBeadID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrimeFlagCombinations(t *testing.T) {
	// Find the gt binary - we need to test CLI flag validation
	gtBin, err := exec.LookPath("gt")
	if err != nil {
		t.Skip("gt binary not found in PATH")
	}

	cases := []struct {
		name      string
		args      []string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "state_alone_is_valid",
			args:      []string{"prime", "--state"},
			wantError: false, // May fail for other reasons (not in workspace), but not flag validation
		},
		{
			name:      "state_with_hook_errors",
			args:      []string{"prime", "--state", "--hook"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "state_with_dry_run_errors",
			args:      []string{"prime", "--state", "--dry-run"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "state_with_explain_errors",
			args:      []string{"prime", "--state", "--explain"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "dry_run_and_explain_valid",
			args:      []string{"prime", "--dry-run", "--explain"},
			wantError: false, // May fail for other reasons, but not flag validation
		},
		{
			name:      "hook_and_dry_run_valid",
			args:      []string{"prime", "--hook", "--dry-run"},
			wantError: false, // May fail for other reasons, but not flag validation
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(gtBin, tc.args...)
			output, err := cmd.CombinedOutput()

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got success with output: %s", output)
				}
				if tc.errorMsg != "" && !strings.Contains(string(output), tc.errorMsg) {
					t.Fatalf("expected error containing %q, got: %s", tc.errorMsg, output)
				}
			}
			// For non-error cases, we don't fail on other errors (like "not in workspace")
			// because we're only testing flag validation
			if !tc.wantError && tc.errorMsg != "" && strings.Contains(string(output), tc.errorMsg) {
				t.Fatalf("unexpected error message %q in output: %s", tc.errorMsg, output)
			}
		})
	}
}

// TestCheckHandoffMarkerDryRun tests that dry-run mode doesn't remove the handoff marker.
func TestCheckHandoffMarkerDryRun(t *testing.T) {
	workDir := t.TempDir()

	// Create .runtime directory and handoff marker
	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
	prevSession := "test-session-123"
	if err := os.WriteFile(markerPath, []byte(prevSession), 0644); err != nil {
		t.Fatalf("write handoff marker: %v", err)
	}

	// Capture stdout to verify explain output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Enable explain mode for this test
	oldExplain := primeExplain
	primeExplain = true
	defer func() { primeExplain = oldExplain }()

	// Call dry-run version
	checkHandoffMarkerDryRun(workDir)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	output := buf.String()

	// Verify marker still exists (not removed in dry-run)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Fatalf("handoff marker was removed in dry-run mode")
	}

	// Verify marker content unchanged
	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read handoff marker: %v", err)
	}
	if string(data) != prevSession {
		t.Fatalf("marker content changed: got %q, want %q", string(data), prevSession)
	}

	// Verify explain output mentions dry-run
	if !strings.Contains(output, "dry-run") {
		t.Fatalf("expected explain output to mention dry-run, got: %s", output)
	}
}

// TestCheckHandoffMarkerDryRun_NoMarker tests dry-run when no marker exists.
func TestCheckHandoffMarkerDryRun_NoMarker(t *testing.T) {
	workDir := t.TempDir()

	// Create .runtime directory but no marker
	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Enable explain mode
	oldExplain := primeExplain
	primeExplain = true
	defer func() { primeExplain = oldExplain }()

	// Should not panic when marker doesn't exist
	checkHandoffMarkerDryRun(workDir)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = oldStdout
	output := buf.String()

	// Verify explain output indicates no marker
	if !strings.Contains(output, "no handoff marker") {
		t.Fatalf("expected explain output to indicate no marker, got: %s", output)
	}
}

// TestDetectSessionState tests detectSessionState for all states.
func TestDetectSessionState(t *testing.T) {
	t.Run("normal_state", func(t *testing.T) {
		workDir := t.TempDir()
		ctx := RoleContext{
			Role:    RoleMayor,
			WorkDir: workDir,
		}

		state := detectSessionState(ctx)

		if state.State != "normal" {
			t.Fatalf("expected state 'normal', got %q", state.State)
		}
		if state.Role != RoleMayor {
			t.Fatalf("expected role Mayor, got %q", state.Role)
		}
	})

	t.Run("post_handoff_state", func(t *testing.T) {
		workDir := t.TempDir()

		// Create handoff marker
		runtimeDir := filepath.Join(workDir, constants.DirRuntime)
		if err := os.MkdirAll(runtimeDir, 0755); err != nil {
			t.Fatalf("create runtime dir: %v", err)
		}
		prevSession := "predecessor-session-abc"
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		if err := os.WriteFile(markerPath, []byte(prevSession), 0644); err != nil {
			t.Fatalf("write handoff marker: %v", err)
		}

		ctx := RoleContext{
			Role:    RolePolecat,
			Rig:     "beads",
			Polecat: "jade",
			WorkDir: workDir,
		}

		state := detectSessionState(ctx)

		if state.State != "post-handoff" {
			t.Fatalf("expected state 'post-handoff', got %q", state.State)
		}
		if state.PrevSession != prevSession {
			t.Fatalf("expected prev_session %q, got %q", prevSession, state.PrevSession)
		}
	})

	t.Run("crash_recovery_state", func(t *testing.T) {
		workDir := t.TempDir()

		// Create a checkpoint (simulating a crashed session)
		cp := &checkpoint.Checkpoint{
			SessionID:  "crashed-session",
			HookedBead: "bd-test123",
			StepTitle:  "Working on feature X",
			Timestamp:  time.Now().Add(-1 * time.Hour), // 1 hour old
		}
		if err := checkpoint.Write(workDir, cp); err != nil {
			t.Fatalf("write checkpoint: %v", err)
		}

		ctx := RoleContext{
			Role:    RolePolecat,
			Rig:     "beads",
			Polecat: "jade",
			WorkDir: workDir,
		}

		state := detectSessionState(ctx)

		if state.State != "crash-recovery" {
			t.Fatalf("expected state 'crash-recovery', got %q", state.State)
		}
		if state.CheckpointAge == "" {
			t.Fatalf("expected checkpoint_age to be set")
		}
	})

	t.Run("crash_recovery_only_for_workers", func(t *testing.T) {
		workDir := t.TempDir()

		// Create a checkpoint
		cp := &checkpoint.Checkpoint{
			SessionID:  "crashed-session",
			HookedBead: "bd-test123",
			StepTitle:  "Working on feature X",
			Timestamp:  time.Now().Add(-1 * time.Hour),
		}
		if err := checkpoint.Write(workDir, cp); err != nil {
			t.Fatalf("write checkpoint: %v", err)
		}

		// Mayor should NOT enter crash-recovery (only polecat/crew)
		ctx := RoleContext{
			Role:    RoleMayor,
			WorkDir: workDir,
		}

		state := detectSessionState(ctx)

		// Mayor should see normal state, not crash-recovery
		if state.State != "normal" {
			t.Fatalf("expected Mayor to have 'normal' state despite checkpoint, got %q", state.State)
		}
	})

	t.Run("autonomous_state_hooked_bead", func(t *testing.T) {
		// Skip: bd CLI 0.47.2 has a bug where database writes don't commit
		// ("sql: database is closed" during auto-flush). This blocks tests
		// that need to create issues. See internal issue for tracking.
		t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

		// Skip if bd CLI is not available
		if _, err := exec.LookPath("bd"); err != nil {
			t.Skip("bd binary not found in PATH")
		}

		workDir := t.TempDir()
		townRoot := workDir

		// Initialize beads database
		initCmd := exec.Command("bd", "init", "--prefix=bd-")
		initCmd.Dir = workDir
		if output, err := initCmd.CombinedOutput(); err != nil {
			t.Fatalf("bd init failed: %v\n%s", err, output)
		}

		// Write routes file
		beadsDir := filepath.Join(workDir, ".beads")
		routes := []beads.Route{{Prefix: "bd-", Path: "."}}
		if err := beads.WriteRoutes(beadsDir, routes); err != nil {
			t.Fatalf("write routes: %v", err)
		}

		// Create a hooked bead assigned to beads/polecats/jade
		b := beads.New(workDir)
		issue, err := b.Create(beads.CreateOptions{
			Title:    "Test hooked bead",
			Priority: 2,
		})
		if err != nil {
			t.Fatalf("create bead: %v", err)
		}

		// Update bead to set status and assignee
		status := beads.StatusHooked
		assignee := "beads/polecats/jade"
		if err := b.Update(issue.ID, beads.UpdateOptions{
			Status:   &status,
			Assignee: &assignee,
		}); err != nil {
			t.Fatalf("update bead: %v", err)
		}

		ctx := RoleContext{
			Role:     RolePolecat,
			Rig:      "beads",
			Polecat:  "jade",
			WorkDir:  workDir,
			TownRoot: townRoot,
		}

		state := detectSessionState(ctx)

		if state.State != "autonomous" {
			t.Fatalf("expected state 'autonomous', got %q", state.State)
		}
		if state.HookedBead != issue.ID {
			t.Fatalf("expected hooked_bead %q, got %q", issue.ID, state.HookedBead)
		}
	})
}

// TestOutputState tests outputState function output formats.
func TestOutputState(t *testing.T) {
	t.Run("text_output", func(t *testing.T) {
		workDir := t.TempDir()
		ctx := RoleContext{
			Role:    RoleMayor,
			WorkDir: workDir,
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		outputState(ctx, false)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		if !strings.Contains(output, "state: normal") {
			t.Fatalf("expected 'state: normal' in output, got: %s", output)
		}
		if !strings.Contains(output, "role: mayor") {
			t.Fatalf("expected 'role: mayor' in output, got: %s", output)
		}
	})

	t.Run("json_output", func(t *testing.T) {
		workDir := t.TempDir()
		ctx := RoleContext{
			Role:    RolePolecat,
			Rig:     "beads",
			Polecat: "jade",
			WorkDir: workDir,
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		outputState(ctx, true)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		// Parse JSON output
		var state SessionState
		if err := json.Unmarshal([]byte(output), &state); err != nil {
			t.Fatalf("failed to parse JSON output: %v, output was: %s", err, output)
		}

		if state.State != "normal" {
			t.Fatalf("expected state 'normal', got %q", state.State)
		}
		if state.Role != RolePolecat {
			t.Fatalf("expected role 'polecat', got %q", state.Role)
		}
	})

	t.Run("json_output_post_handoff", func(t *testing.T) {
		workDir := t.TempDir()

		// Create handoff marker
		runtimeDir := filepath.Join(workDir, constants.DirRuntime)
		if err := os.MkdirAll(runtimeDir, 0755); err != nil {
			t.Fatalf("create runtime dir: %v", err)
		}
		prevSession := "prev-session-xyz"
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		if err := os.WriteFile(markerPath, []byte(prevSession), 0644); err != nil {
			t.Fatalf("write marker: %v", err)
		}

		ctx := RoleContext{
			Role:    RolePolecat,
			Rig:     "beads",
			Polecat: "jade",
			WorkDir: workDir,
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		outputState(ctx, true)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		// Parse JSON
		var state SessionState
		if err := json.Unmarshal([]byte(output), &state); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}

		if state.State != "post-handoff" {
			t.Fatalf("expected state 'post-handoff', got %q", state.State)
		}
		if state.PrevSession != prevSession {
			t.Fatalf("expected prev_session %q, got %q", prevSession, state.PrevSession)
		}
	})
}

// TestExplain tests the explain function output.
func TestExplain(t *testing.T) {
	t.Run("explain_enabled_condition_true", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Enable explain mode
		oldExplain := primeExplain
		primeExplain = true
		defer func() { primeExplain = oldExplain }()

		explain(true, "This is a test explanation")

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		if !strings.Contains(output, "[EXPLAIN]") {
			t.Fatalf("expected [EXPLAIN] tag in output, got: %s", output)
		}
		if !strings.Contains(output, "This is a test explanation") {
			t.Fatalf("expected explanation text in output, got: %s", output)
		}
	})

	t.Run("explain_enabled_condition_false", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Enable explain mode
		oldExplain := primeExplain
		primeExplain = true
		defer func() { primeExplain = oldExplain }()

		explain(false, "This should not appear")

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		if strings.Contains(output, "[EXPLAIN]") {
			t.Fatalf("expected no [EXPLAIN] tag when condition is false, got: %s", output)
		}
	})

	t.Run("explain_disabled", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Disable explain mode
		oldExplain := primeExplain
		primeExplain = false
		defer func() { primeExplain = oldExplain }()

		explain(true, "This should not appear either")

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		if strings.Contains(output, "[EXPLAIN]") {
			t.Fatalf("expected no [EXPLAIN] tag when explain mode disabled, got: %s", output)
		}
	})
}

// TestDryRunSkipsSideEffects tests that --dry-run skips various side effects via CLI.
// TestBuildAgentID tests the agent ID construction for advice matching.
func TestBuildAgentID(t *testing.T) {
	cases := []struct {
		name string
		ctx  RoleInfo
		want string
	}{
		{
			name: "polecat",
			ctx:  RoleInfo{Role: RolePolecat, Rig: "gastown", Polecat: "alpha"},
			want: "gastown/polecats/alpha",
		},
		{
			name: "crew",
			ctx:  RoleInfo{Role: RoleCrew, Rig: "gastown", Polecat: "decision_notify"},
			want: "gastown/crew/decision_notify",
		},
		{
			name: "witness",
			ctx:  RoleInfo{Role: RoleWitness, Rig: "beads"},
			want: "beads/witness",
		},
		{
			name: "refinery",
			ctx:  RoleInfo{Role: RoleRefinery, Rig: "beads"},
			want: "beads/refinery",
		},
		{
			name: "mayor",
			ctx:  RoleInfo{Role: RoleMayor},
			want: "mayor",
		},
		{
			name: "deacon",
			ctx:  RoleInfo{Role: RoleDeacon},
			want: "deacon",
		},
		{
			name: "polecat_no_rig",
			ctx:  RoleInfo{Role: RolePolecat, Polecat: "alpha"},
			want: "",
		},
		{
			name: "polecat_no_name",
			ctx:  RoleInfo{Role: RolePolecat, Rig: "gastown"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAgentID(tc.ctx)
			if got != tc.want {
				t.Fatalf("buildAgentID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMatchesAdvice tests the advice matching logic.
func TestMatchesAdvice(t *testing.T) {
	cases := []struct {
		name     string
		bead     AdviceBead
		agentID  string
		roleType string
		rigName  string
		want     bool
	}{
		{
			name:     "global_advice_matches_all",
			bead:     AdviceBead{ID: "test1", Title: "Global"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     true,
		},
		{
			name:     "rig_advice_matches_same_rig",
			bead:     AdviceBead{ID: "test2", Title: "Rig", AdviceTargetRig: "gastown"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     true,
		},
		{
			name:     "rig_advice_rejects_different_rig",
			bead:     AdviceBead{ID: "test3", Title: "Rig", AdviceTargetRig: "beads"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     false,
		},
		{
			name:     "role_advice_matches_same_role",
			bead:     AdviceBead{ID: "test4", Title: "Role", AdviceTargetRole: "polecat"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     true,
		},
		{
			name:     "role_advice_rejects_different_role",
			bead:     AdviceBead{ID: "test5", Title: "Role", AdviceTargetRole: "crew"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     false,
		},
		{
			name:     "agent_advice_matches_same_agent",
			bead:     AdviceBead{ID: "test6", Title: "Agent", AdviceTargetAgent: "gastown/polecats/alpha"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     true,
		},
		{
			name:     "agent_advice_rejects_different_agent",
			bead:     AdviceBead{ID: "test7", Title: "Agent", AdviceTargetAgent: "gastown/polecats/beta"},
			agentID:  "gastown/polecats/alpha",
			roleType: "polecat",
			rigName:  "gastown",
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesAdvice(tc.bead, tc.agentID, tc.roleType, tc.rigName)
			if got != tc.want {
				t.Fatalf("matchesAdvice() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFilterApplicableAdvice tests filtering of advice beads.
func TestFilterApplicableAdvice(t *testing.T) {
	beads := []AdviceBead{
		{ID: "global1", Title: "Global Advice 1"},
		{ID: "global2", Title: "Global Advice 2"},
		{ID: "rig1", Title: "Gastown Rig Advice", AdviceTargetRig: "gastown"},
		{ID: "rig2", Title: "Beads Rig Advice", AdviceTargetRig: "beads"},
		{ID: "role1", Title: "Polecat Role Advice", AdviceTargetRole: "polecat"},
		{ID: "role2", Title: "Crew Role Advice", AdviceTargetRole: "crew"},
		{ID: "agent1", Title: "Alpha Agent Advice", AdviceTargetAgent: "gastown/polecats/alpha"},
		{ID: "agent2", Title: "Beta Agent Advice", AdviceTargetAgent: "gastown/polecats/beta"},
	}

	t.Run("gastown_polecat_alpha", func(t *testing.T) {
		result := filterApplicableAdvice(beads, "gastown/polecats/alpha", "polecat", "gastown")

		// Should get: global1, global2, rig1 (gastown), role1 (polecat), agent1 (alpha)
		if len(result) != 5 {
			t.Fatalf("expected 5 applicable beads, got %d", len(result))
		}

		// Check specific IDs are present
		ids := make(map[string]bool)
		for _, b := range result {
			ids[b.ID] = true
		}

		expected := []string{"global1", "global2", "rig1", "role1", "agent1"}
		for _, id := range expected {
			if !ids[id] {
				t.Errorf("expected %q to be in result", id)
			}
		}
	})

	t.Run("beads_crew_worker", func(t *testing.T) {
		result := filterApplicableAdvice(beads, "beads/crew/worker", "crew", "beads")

		// Should get: global1, global2, rig2 (beads), role2 (crew)
		if len(result) != 4 {
			t.Fatalf("expected 4 applicable beads, got %d", len(result))
		}
	})
}

// TestGetAdviceScope tests the scope label generation.
func TestGetAdviceScope(t *testing.T) {
	cases := []struct {
		name string
		bead AdviceBead
		want string
	}{
		{
			name: "global",
			bead: AdviceBead{ID: "test"},
			want: "Global",
		},
		{
			name: "rig",
			bead: AdviceBead{ID: "test", AdviceTargetRig: "gastown"},
			want: "gastown",
		},
		{
			name: "role",
			bead: AdviceBead{ID: "test", AdviceTargetRole: "polecat"},
			want: "Polecat",
		},
		{
			name: "agent",
			bead: AdviceBead{ID: "test", AdviceTargetAgent: "gastown/polecats/alpha"},
			want: "Agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getAdviceScope(tc.bead)
			if got != tc.want {
				t.Fatalf("getAdviceScope() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDryRunSkipsSideEffects(t *testing.T) {
	// Find the gt binary
	gtBin, err := exec.LookPath("gt")
	if err != nil {
		t.Skip("gt binary not found in PATH")
	}

	// Create a temp workspace
	townRoot := t.TempDir()

	// Set up minimal workspace structure
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}

	// Write routes
	routes := []beads.Route{{Prefix: "bd-", Path: "."}}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Create handoff marker that should NOT be removed in dry-run
	runtimeDir := filepath.Join(townRoot, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
	if err := os.WriteFile(markerPath, []byte("prev-session"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Run gt prime --dry-run --explain
	cmd := exec.Command(gtBin, "prime", "--dry-run", "--explain")
	cmd.Dir = townRoot
	output, _ := cmd.CombinedOutput()

	// The command may fail for other reasons (not fully configured workspace)
	// but we can check:
	// 1. Marker still exists
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Fatalf("handoff marker was removed in dry-run mode")
	}

	// 2. Output mentions skipped operations
	outputStr := string(output)
	// Check for explain output about dry-run (if workspace was valid enough to get there)
	if strings.Contains(outputStr, "bd prime") && !strings.Contains(outputStr, "skipped") {
		t.Logf("Note: output doesn't explicitly mention skipping bd prime: %s", outputStr)
	}
}
