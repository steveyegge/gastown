package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/nudge"
)

func TestNudgeStdinConflict(t *testing.T) {
	// Save and restore package-level flags
	origMessage := nudgeMessageFlag
	origStdin := nudgeStdinFlag
	defer func() {
		nudgeMessageFlag = origMessage
		nudgeStdinFlag = origStdin
	}()

	// When both --stdin and --message are set, runNudge should return an error
	nudgeStdinFlag = true
	nudgeMessageFlag = "some message"

	err := runNudge(nudgeCmd, []string{"gastown/alpha"})
	if err == nil {
		t.Fatal("expected error when --stdin and --message are both set")
	}
	if !strings.Contains(err.Error(), "cannot use --stdin with --message/-m") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveNudgePattern(t *testing.T) {
	// Create test agent sessions (mayor/deacon use hq- prefix)
	agents := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-gastown-witness", Type: AgentWitness, Rig: "gastown"},
		{Name: "gt-gastown-refinery", Type: AgentRefinery, Rig: "gastown"},
		{Name: "gt-gastown-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-gastown-crew-jack", Type: AgentCrew, Rig: "gastown", AgentName: "jack"},
		{Name: "gt-gastown-alpha", Type: AgentPolecat, Rig: "gastown", AgentName: "alpha"},
		{Name: "gt-gastown-beta", Type: AgentPolecat, Rig: "gastown", AgentName: "beta"},
		{Name: "gt-beads-witness", Type: AgentWitness, Rig: "beads"},
		{Name: "gt-beads-gamma", Type: AgentPolecat, Rig: "beads", AgentName: "gamma"},
	}

	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{
			name:     "mayor special case",
			pattern:  "mayor",
			expected: []string{"hq-mayor"},
		},
		{
			name:     "deacon special case",
			pattern:  "deacon",
			expected: []string{"hq-deacon"},
		},
		{
			name:     "specific witness",
			pattern:  "gastown/witness",
			expected: []string{"gt-gastown-witness"},
		},
		{
			name:     "all witnesses",
			pattern:  "*/witness",
			expected: []string{"gt-gastown-witness", "gt-beads-witness"},
		},
		{
			name:     "specific refinery",
			pattern:  "gastown/refinery",
			expected: []string{"gt-gastown-refinery"},
		},
		{
			name:     "all polecats in rig",
			pattern:  "gastown/polecats/*",
			expected: []string{"gt-gastown-alpha", "gt-gastown-beta"},
		},
		{
			name:     "specific polecat",
			pattern:  "gastown/polecats/alpha",
			expected: []string{"gt-gastown-alpha"},
		},
		{
			name:     "all crew in rig",
			pattern:  "gastown/crew/*",
			expected: []string{"gt-gastown-crew-max", "gt-gastown-crew-jack"},
		},
		{
			name:     "specific crew member",
			pattern:  "gastown/crew/max",
			expected: []string{"gt-gastown-crew-max"},
		},
		{
			name:     "legacy polecat format",
			pattern:  "gastown/alpha",
			expected: []string{"gt-gastown-alpha"},
		},
		{
			name:     "no matches",
			pattern:  "nonexistent/polecats/*",
			expected: nil,
		},
		{
			name:     "invalid pattern",
			pattern:  "invalid",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNudgePattern(tt.pattern, agents)

			if len(got) != len(tt.expected) {
				t.Errorf("resolveNudgePattern(%q) returned %d results, want %d: got %v, want %v",
					tt.pattern, len(got), len(tt.expected), got, tt.expected)
				return
			}

			// Check each expected value is present
			gotMap := make(map[string]bool)
			for _, g := range got {
				gotMap[g] = true
			}
			for _, e := range tt.expected {
				if !gotMap[e] {
					t.Errorf("resolveNudgePattern(%q) missing expected %q, got %v",
						tt.pattern, e, got)
				}
			}
		})
	}
}

func TestSessionNameToAddress(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		expected    string
	}{
		{
			name:        "mayor",
			sessionName: "hq-mayor",
			expected:    "mayor",
		},
		{
			name:        "deacon",
			sessionName: "hq-deacon",
			expected:    "deacon",
		},
		{
			name:        "witness",
			sessionName: "gt-gastown-witness",
			expected:    "gastown/witness",
		},
		{
			name:        "refinery",
			sessionName: "gt-gastown-refinery",
			expected:    "gastown/refinery",
		},
		{
			name:        "crew member",
			sessionName: "gt-gastown-crew-max",
			expected:    "gastown/crew/max",
		},
		{
			name:        "polecat",
			sessionName: "gt-gastown-alpha",
			expected:    "gastown/alpha",
		},
		{
			name:        "unrecognized format",
			sessionName: "random-session",
			expected:    "",
		},
		{
			name:        "gt prefix but no rig",
			sessionName: "gt-",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionNameToAddress(tt.sessionName)
			if got != tt.expected {
				t.Errorf("sessionNameToAddress(%q) = %q, want %q", tt.sessionName, got, tt.expected)
			}
		})
	}
}

func TestNudgeInvalidMode(t *testing.T) {
	// Save and restore package-level flags
	origMode := nudgeModeFlag
	origPriority := nudgePriorityFlag
	origMessage := nudgeMessageFlag
	origStdin := nudgeStdinFlag
	defer func() {
		nudgeModeFlag = origMode
		nudgePriorityFlag = origPriority
		nudgeMessageFlag = origMessage
		nudgeStdinFlag = origStdin
	}()

	nudgeStdinFlag = false
	nudgeMessageFlag = "test"

	tests := []struct {
		name     string
		mode     string
		wantErr  string
	}{
		{"bogus mode", "bogus", `invalid --mode "bogus"`},
		{"empty mode", "", `invalid --mode ""`},
		{"typo immediate", "imediate", `invalid --mode "imediate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudgeModeFlag = tt.mode
			nudgePriorityFlag = "normal"
			err := runNudge(nudgeCmd, []string{"gastown/alpha", "hello"})
			if err == nil {
				t.Fatal("expected error for invalid mode")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNudgeInvalidPriority(t *testing.T) {
	// Save and restore package-level flags
	origMode := nudgeModeFlag
	origPriority := nudgePriorityFlag
	origMessage := nudgeMessageFlag
	origStdin := nudgeStdinFlag
	defer func() {
		nudgeModeFlag = origMode
		nudgePriorityFlag = origPriority
		nudgeMessageFlag = origMessage
		nudgeStdinFlag = origStdin
	}()

	nudgeStdinFlag = false
	nudgeMessageFlag = "test"
	nudgeModeFlag = NudgeModeImmediate

	tests := []struct {
		name     string
		priority string
		wantErr  string
	}{
		{"bogus priority", "bogus", `invalid --priority "bogus"`},
		{"empty priority", "", `invalid --priority ""`},
		{"high priority", "high", `invalid --priority "high"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nudgePriorityFlag = tt.priority
			err := runNudge(nudgeCmd, []string{"gastown/alpha", "hello"})
			if err == nil {
				t.Fatal("expected error for invalid priority")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNudgeValidModesAccepted(t *testing.T) {
	// Verify all valid modes pass the validation check (they'll fail later
	// on tmux operations, but should NOT fail on mode validation).
	origMode := nudgeModeFlag
	origPriority := nudgePriorityFlag
	origMessage := nudgeMessageFlag
	origStdin := nudgeStdinFlag
	origTimeout := waitIdleTimeout
	defer func() {
		nudgeModeFlag = origMode
		nudgePriorityFlag = origPriority
		nudgeMessageFlag = origMessage
		nudgeStdinFlag = origStdin
		waitIdleTimeout = origTimeout
	}()

	// Shorten wait-idle timeout to avoid 15s test delay
	waitIdleTimeout = 200 * time.Millisecond

	nudgeStdinFlag = false
	nudgeMessageFlag = "test"
	nudgePriorityFlag = "normal"

	for _, mode := range []string{NudgeModeImmediate, NudgeModeQueue, NudgeModeWaitIdle} {
		t.Run(mode, func(t *testing.T) {
			nudgeModeFlag = mode
			err := runNudge(nudgeCmd, []string{"gastown/alpha", "hello"})
			// The error should NOT be about invalid mode — it will fail on
			// tmux or workspace, which is fine.
			if err != nil && strings.Contains(err.Error(), "invalid --mode") {
				t.Errorf("valid mode %q was rejected: %v", mode, err)
			}
		})
	}
}

func TestIfFreshMaxAge(t *testing.T) {
	// Verify the constant is 60 seconds as specified in the design.
	if ifFreshMaxAge != 60*time.Second {
		t.Errorf("ifFreshMaxAge = %v, want 60s", ifFreshMaxAge)
	}
}

func TestIfFreshSessionAgeCheck(t *testing.T) {
	// Test the age comparison logic used by --if-fresh.
	// A session created 10 seconds ago should be "fresh" (nudge allowed).
	// A session created 120 seconds ago should be "stale" (nudge suppressed).
	now := time.Now()

	tests := []struct {
		name        string
		createdAt   time.Time
		shouldNudge bool
	}{
		{
			name:        "fresh session (10s old)",
			createdAt:   now.Add(-10 * time.Second),
			shouldNudge: true,
		},
		{
			name:        "borderline session (59s old)",
			createdAt:   now.Add(-59 * time.Second),
			shouldNudge: true,
		},
		{
			name:        "stale session (61s old)",
			createdAt:   now.Add(-61 * time.Second),
			shouldNudge: false,
		},
		{
			name:        "very stale session (5min old)",
			createdAt:   now.Add(-5 * time.Minute),
			shouldNudge: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := time.Since(tt.createdAt)
			shouldNudge := age <= ifFreshMaxAge
			if shouldNudge != tt.shouldNudge {
				t.Errorf("age=%v: shouldNudge=%v, want %v", age, shouldNudge, tt.shouldNudge)
			}
		})
	}
}

func TestValidModeMapsMatchConstants(t *testing.T) {
	// Ensure the validation maps cover all defined mode constants.
	modes := []string{NudgeModeImmediate, NudgeModeQueue, NudgeModeWaitIdle}
	for _, m := range modes {
		if !validNudgeModes[m] {
			t.Errorf("mode constant %q missing from validNudgeModes", m)
		}
	}
	priorities := []string{nudge.PriorityNormal, nudge.PriorityUrgent}
	for _, p := range priorities {
		if !validNudgePriorities[p] {
			t.Errorf("priority constant %q missing from validNudgePriorities", p)
		}
	}
}
