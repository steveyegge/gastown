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
			want: "bd-beads-polecat-lex",
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

// TestIsCompactResume tests the isCompactResume detection logic including
// compaction-triggered handoff cycles (GH#1965).
func TestIsCompactResume(t *testing.T) {
	// Save and restore package-level state
	origSource := primeHookSource
	origReason := primeHandoffReason
	defer func() {
		primeHookSource = origSource
		primeHandoffReason = origReason
	}()

	cases := []struct {
		name           string
		hookSource     string
		handoffReason  string
		wantCompact    bool
	}{
		{
			name:       "fresh_startup",
			hookSource: "startup",
			wantCompact: false,
		},
		{
			name:       "compact_source",
			hookSource: "compact",
			wantCompact: true,
		},
		{
			name:       "resume_source",
			hookSource: "resume",
			wantCompact: true,
		},
		{
			name:       "clear_source",
			hookSource: "clear",
			wantCompact: false,
		},
		{
			name:          "compaction_handoff_cycle",
			hookSource:    "startup",
			handoffReason: "compaction",
			wantCompact:   true,
		},
		{
			name:          "normal_handoff_not_compact",
			hookSource:    "startup",
			handoffReason: "",
			wantCompact:   false,
		},
		{
			name:          "idle_handoff_not_compact",
			hookSource:    "startup",
			handoffReason: "idle",
			wantCompact:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			primeHookSource = tc.hookSource
			primeHandoffReason = tc.handoffReason

			got := isCompactResume()
			if got != tc.wantCompact {
				t.Fatalf("isCompactResume() = %v, want %v (source=%q, reason=%q)",
					got, tc.wantCompact, tc.hookSource, tc.handoffReason)
			}
		})
	}
}

// TestCheckHandoffMarkerParsesReason tests that checkHandoffMarker correctly
// parses the reason field from the marker file (GH#1965).
func TestCheckHandoffMarkerParsesReason(t *testing.T) {
	// Save and restore package-level state
	origReason := primeHandoffReason
	defer func() { primeHandoffReason = origReason }()

	t.Run("marker_with_reason", func(t *testing.T) {
		primeHandoffReason = ""
		workDir := t.TempDir()

		runtimeDir := filepath.Join(workDir, constants.DirRuntime)
		if err := os.MkdirAll(runtimeDir, 0755); err != nil {
			t.Fatalf("create runtime dir: %v", err)
		}

		// Write marker with session ID and reason
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		if err := os.WriteFile(markerPath, []byte("test-session-456\ncompaction"), 0644); err != nil {
			t.Fatalf("write marker: %v", err)
		}

		// Capture stdout (checkHandoffMarker outputs the warning)
		oldStdout := os.Stdout
		_, w, _ := os.Pipe()
		os.Stdout = w

		checkHandoffMarker(workDir)

		w.Close()
		os.Stdout = oldStdout

		// Verify reason was parsed
		if primeHandoffReason != "compaction" {
			t.Fatalf("primeHandoffReason = %q, want %q", primeHandoffReason, "compaction")
		}

		// Verify marker was removed
		if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
			t.Fatalf("handoff marker was not removed")
		}
	})

	t.Run("marker_without_reason", func(t *testing.T) {
		primeHandoffReason = ""
		workDir := t.TempDir()

		runtimeDir := filepath.Join(workDir, constants.DirRuntime)
		if err := os.MkdirAll(runtimeDir, 0755); err != nil {
			t.Fatalf("create runtime dir: %v", err)
		}

		// Write marker with session ID only (backward compat)
		markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
		if err := os.WriteFile(markerPath, []byte("test-session-789"), 0644); err != nil {
			t.Fatalf("write marker: %v", err)
		}

		oldStdout := os.Stdout
		_, w, _ := os.Pipe()
		os.Stdout = w

		checkHandoffMarker(workDir)

		w.Close()
		os.Stdout = oldStdout

		// Verify reason is empty (backward compatible)
		if primeHandoffReason != "" {
			t.Fatalf("primeHandoffReason = %q, want empty", primeHandoffReason)
		}
	})

	t.Run("no_marker", func(t *testing.T) {
		primeHandoffReason = ""
		workDir := t.TempDir()

		checkHandoffMarker(workDir)

		// Verify reason is still empty
		if primeHandoffReason != "" {
			t.Fatalf("primeHandoffReason = %q, want empty", primeHandoffReason)
		}
	})
}

// TestOutputContinuationDirective tests that the continuation directive
// outputs the expected content without the full autonomous mode block (GH#1965).
func TestOutputContinuationDirective(t *testing.T) {
	t.Run("basic_bead", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		bead := &beads.Issue{
			ID:    "gt-test123",
			Title: "Test bead title",
		}
		outputContinuationDirective(bead, false)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		// Should contain continuation directive
		if !strings.Contains(output, "CONTINUE HOOKED WORK") {
			t.Fatalf("expected 'CONTINUE HOOKED WORK' in output, got: %s", output)
		}
		if !strings.Contains(output, "gt-test123") {
			t.Fatalf("expected bead ID in output, got: %s", output)
		}

		// Should NOT contain autonomous mode language
		if strings.Contains(output, "AUTONOMOUS WORK MODE") {
			t.Fatalf("continuation directive should NOT contain 'AUTONOMOUS WORK MODE', got: %s", output)
		}
		if strings.Contains(output, "Announce:") {
			t.Fatalf("continuation directive should NOT contain 'Announce:', got: %s", output)
		}
	})

	t.Run("bead_with_molecule", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		bead := &beads.Issue{
			ID:    "gt-mol456",
			Title: "Molecule bead",
		}
		outputContinuationDirective(bead, true)

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout
		output := buf.String()

		if !strings.Contains(output, "bd mol current") {
			t.Fatalf("expected molecule hint in output, got: %s", output)
		}
	})
}
