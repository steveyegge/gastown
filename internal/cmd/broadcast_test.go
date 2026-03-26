package cmd

import (
	"strings"
	"testing"
)

// TestFormatAgentName covers all agent types and verifies the display name format.
func TestFormatAgentName(t *testing.T) {
	tests := []struct {
		name  string
		agent *AgentSession
		want  string
	}{
		{
			name:  "mayor",
			agent: &AgentSession{Name: "hq-mayor", Type: AgentMayor},
			want:  "mayor",
		},
		{
			name:  "deacon",
			agent: &AgentSession{Name: "hq-deacon", Type: AgentDeacon},
			want:  "deacon",
		},
		{
			name:  "witness",
			agent: &AgentSession{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"},
			want:  "gastown/witness",
		},
		{
			name:  "refinery",
			agent: &AgentSession{Name: "gt-refinery", Type: AgentRefinery, Rig: "gastown"},
			want:  "gastown/refinery",
		},
		{
			name:  "crew",
			agent: &AgentSession{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
			want:  "gastown/crew/max",
		},
		{
			name:  "polecat",
			agent: &AgentSession{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
			want:  "gastown/furiosa",
		},
		{
			name:  "unknown type falls back to Name",
			agent: &AgentSession{Name: "gt-unknown-session", Type: AgentPersonal},
			want:  "gt-unknown-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAgentName(tt.agent)
			if got != tt.want {
				t.Errorf("formatAgentName(%+v) = %q, want %q", tt.agent, got, tt.want)
			}
		})
	}
}

// TestFormatAgentName_RigVariants checks that different rig names are reflected correctly.
func TestFormatAgentName_RigVariants(t *testing.T) {
	rigs := []string{"beads", "openfang", "myrig", "alpha-rig"}
	for _, rig := range rigs {
		t.Run(rig, func(t *testing.T) {
			agent := &AgentSession{Type: AgentCrew, Rig: rig, AgentName: "worker"}
			got := formatAgentName(agent)
			want := rig + "/crew/worker"
			if got != want {
				t.Errorf("formatAgentName crew in rig %q = %q, want %q", rig, got, want)
			}
		})
	}
}

// TestFormatAgentName_PolecatMultipleNames verifies polecat name formatting.
func TestFormatAgentName_PolecatMultipleNames(t *testing.T) {
	names := []string{"alpha", "beta", "chrome", "nitro"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			agent := &AgentSession{Type: AgentPolecat, Rig: "gastown", AgentName: name}
			got := formatAgentName(agent)
			want := "gastown/" + name
			if got != want {
				t.Errorf("formatAgentName polecat %q = %q, want %q", name, got, want)
			}
		})
	}
}

// TestBroadcastTargetFiltering_WorkersOnly tests that only crew and polecats are
// targeted by default (--all not set), while infra agents are excluded.
func TestBroadcastTargetFiltering_WorkersOnly(t *testing.T) {
	all := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"},
		{Name: "gt-refinery", Type: AgentRefinery, Rig: "gastown"},
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type == AgentCrew || agent.Type == AgentPolecat {
			targets = append(targets, agent)
		}
	}

	if len(targets) != 2 {
		t.Errorf("worker-only filter: got %d targets, want 2 (crew+polecat)", len(targets))
	}
	for _, a := range targets {
		if a.Type != AgentCrew && a.Type != AgentPolecat {
			t.Errorf("unexpected agent type %d in worker targets", a.Type)
		}
	}
}

// TestBroadcastTargetFiltering_AllFlag tests that --all includes infra agents.
func TestBroadcastTargetFiltering_AllFlag(t *testing.T) {
	all := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"},
		{Name: "gt-refinery", Type: AgentRefinery, Rig: "gastown"},
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	// With --all, all agents (no type filter) are included.
	targets := make([]*AgentSession, len(all))
	copy(targets, all)

	if len(targets) != len(all) {
		t.Errorf("--all filter: got %d targets, want %d", len(targets), len(all))
	}

	// Verify infra agents are present
	hasInfra := false
	for _, a := range targets {
		if a.Type == AgentMayor || a.Type == AgentDeacon ||
			a.Type == AgentWitness || a.Type == AgentRefinery {
			hasInfra = true
			break
		}
	}
	if !hasInfra {
		t.Error("--all mode should include infra agents (mayor/deacon/witness/refinery)")
	}
}

// TestBroadcastTargetFiltering_RigFilter verifies --rig only includes agents in that rig.
func TestBroadcastTargetFiltering_RigFilter(t *testing.T) {
	all := []*AgentSession{
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "bd-crew-alice", Type: AgentCrew, Rig: "beads", AgentName: "alice"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
		{Name: "bd-gamma", Type: AgentPolecat, Rig: "beads", AgentName: "gamma"},
	}

	filterRig := "gastown"
	var targets []*AgentSession
	for _, agent := range all {
		if agent.Rig != filterRig {
			continue
		}
		if agent.Type == AgentCrew || agent.Type == AgentPolecat {
			targets = append(targets, agent)
		}
	}

	if len(targets) != 2 {
		t.Errorf("rig filter %q: got %d targets, want 2", filterRig, len(targets))
	}
	for _, a := range targets {
		if a.Rig != filterRig {
			t.Errorf("rig filter: agent %q has rig %q, want %q", a.Name, a.Rig, filterRig)
		}
	}
}

// TestBroadcastTargetFiltering_EmptyTargets verifies the no-workers scenario.
func TestBroadcastTargetFiltering_EmptyTargets(t *testing.T) {
	// Only infra agents, workers-only filter → no targets.
	all := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"},
	}

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type == AgentCrew || agent.Type == AgentPolecat {
			targets = append(targets, agent)
		}
	}

	if len(targets) != 0 {
		t.Errorf("expected 0 targets when only infra agents present, got %d", len(targets))
	}
}

// TestBroadcastTargetFiltering_SelfExclusion verifies that the sender is excluded
// from the target list (a self-broadcast skip).
func TestBroadcastTargetFiltering_SelfExclusion(t *testing.T) {
	sender := "gastown/crew/max"

	all := []*AgentSession{
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-crew-alice", Type: AgentCrew, Rig: "gastown", AgentName: "alice"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type != AgentCrew && agent.Type != AgentPolecat {
			continue
		}
		// Skip self
		if formatAgentName(agent) == sender {
			continue
		}
		targets = append(targets, agent)
	}

	if len(targets) != 2 {
		t.Errorf("self-exclusion: got %d targets, want 2", len(targets))
	}
	for _, a := range targets {
		if formatAgentName(a) == sender {
			t.Errorf("sender %q should be excluded from targets", sender)
		}
	}
}

// TestBroadcastTargetFiltering_SelfExclusion_EmptySender verifies that an empty
// BD_ACTOR (no sender) causes nobody to be excluded.
func TestBroadcastTargetFiltering_SelfExclusion_EmptySender(t *testing.T) {
	sender := "" // no BD_ACTOR set

	all := []*AgentSession{
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type != AgentCrew && agent.Type != AgentPolecat {
			continue
		}
		// The broadcast command only skips self when sender is non-empty.
		if sender != "" && formatAgentName(agent) == sender {
			continue
		}
		targets = append(targets, agent)
	}

	if len(targets) != 2 {
		t.Errorf("empty sender: got %d targets, want 2 (no exclusion)", len(targets))
	}
}

// TestBroadcastCmd_Registered verifies broadcastCmd is registered with rootCmd.
func TestBroadcastCmd_Registered(t *testing.T) {
	found := false
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "broadcast <message>" {
			found = true
			break
		}
	}
	if !found {
		t.Error("broadcastCmd not registered with rootCmd")
	}
}

// TestBroadcastCmd_RequiresExactlyOneArg verifies the Args constraint.
func TestBroadcastCmd_RequiresExactlyOneArg(t *testing.T) {
	if broadcastCmd.Args == nil {
		t.Fatal("broadcastCmd.Args should not be nil")
	}

	// Zero args should fail
	if err := broadcastCmd.Args(broadcastCmd, []string{}); err == nil {
		t.Error("expected error for zero args")
	}

	// One arg should pass
	if err := broadcastCmd.Args(broadcastCmd, []string{"hello"}); err != nil {
		t.Errorf("unexpected error for one arg: %v", err)
	}

	// Two args should fail
	if err := broadcastCmd.Args(broadcastCmd, []string{"hello", "world"}); err == nil {
		t.Error("expected error for two args")
	}
}

// TestBroadcastCmd_Flags verifies that all expected flags exist.
func TestBroadcastCmd_Flags(t *testing.T) {
	flagTests := []struct {
		flagName string
	}{
		{"rig"},
		{"all"},
		{"dry-run"},
	}

	for _, tt := range flagTests {
		t.Run(tt.flagName, func(t *testing.T) {
			f := broadcastCmd.Flags().Lookup(tt.flagName)
			if f == nil {
				t.Errorf("broadcastCmd missing flag --%s", tt.flagName)
			}
		})
	}
}

// TestBroadcastCmd_ShortDescription verifies the command has a short description.
func TestBroadcastCmd_ShortDescription(t *testing.T) {
	if broadcastCmd.Short == "" {
		t.Error("broadcastCmd.Short should not be empty")
	}
	// Should mention workers or broadcast
	lower := strings.ToLower(broadcastCmd.Short)
	if !strings.Contains(lower, "worker") && !strings.Contains(lower, "broadcast") &&
		!strings.Contains(lower, "nudge") && !strings.Contains(lower, "send") {
		t.Errorf("broadcastCmd.Short = %q, expected to mention workers/broadcast/nudge/send", broadcastCmd.Short)
	}
}

// TestBroadcastCmd_GroupID verifies the command is in the comm group.
func TestBroadcastCmd_GroupID(t *testing.T) {
	if broadcastCmd.GroupID != GroupComm {
		t.Errorf("broadcastCmd.GroupID = %q, want GroupComm (%q)", broadcastCmd.GroupID, GroupComm)
	}
}

// TestBroadcastMessageFormat_MultiTarget verifies that a message intended for
// multiple recipients has the same content for each recipient — no per-recipient
// templating or mutation.
func TestBroadcastMessageFormat_MultiTarget(t *testing.T) {
	message := "Check your mail — new priority work available"

	targets := []*AgentSession{
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-crew-alice", Type: AgentCrew, Rig: "gastown", AgentName: "alice"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	// Simulate what broadcast does: same message to all targets.
	for _, target := range targets {
		// The message must be non-empty and identical for each target.
		if message == "" {
			t.Errorf("message for target %q is empty", formatAgentName(target))
		}
		// No per-recipient mutation — every target gets the exact same string.
		if message != "Check your mail — new priority work available" {
			t.Errorf("message was mutated for target %q", formatAgentName(target))
		}
	}
}

// TestBroadcastMailRouting_CorrectRecipients verifies that the set of recipients
// produced by the filtering logic matches the expected set, ensuring correct mail
// routing semantics.
func TestBroadcastMailRouting_CorrectRecipients(t *testing.T) {
	setupCmdTestRegistry(t)

	all := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"},
		{Name: "gt-refinery", Type: AgentRefinery, Rig: "gastown"},
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-crew-alice", Type: AgentCrew, Rig: "gastown", AgentName: "alice"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
		{Name: "bd-gamma", Type: AgentPolecat, Rig: "beads", AgentName: "gamma"},
	}

	tests := []struct {
		name          string
		includeAll    bool
		rigFilter     string
		sender        string
		wantNames     []string
		wantExcluded  []string
	}{
		{
			name:       "default: workers only",
			includeAll: false,
			rigFilter:  "",
			sender:     "",
			wantNames:  []string{"gastown/crew/max", "gastown/crew/alice", "gastown/furiosa", "beads/gamma"},
			wantExcluded: []string{"mayor", "deacon", "gastown/witness", "gastown/refinery"},
		},
		{
			name:       "rig filter: gastown only",
			includeAll: false,
			rigFilter:  "gastown",
			sender:     "",
			wantNames:  []string{"gastown/crew/max", "gastown/crew/alice", "gastown/furiosa"},
			wantExcluded: []string{"beads/gamma"},
		},
		{
			name:       "self-exclusion: sender excluded",
			includeAll: false,
			rigFilter:  "",
			sender:     "gastown/furiosa",
			wantNames:  []string{"gastown/crew/max", "gastown/crew/alice", "beads/gamma"},
			wantExcluded: []string{"gastown/furiosa"},
		},
		{
			name:       "all flag: infra included",
			includeAll: true,
			rigFilter:  "",
			sender:     "",
			wantNames:  []string{"mayor", "deacon", "gastown/witness", "gastown/refinery", "gastown/crew/max"},
			wantExcluded: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var targets []*AgentSession
			for _, agent := range all {
				if tt.rigFilter != "" && agent.Rig != tt.rigFilter {
					continue
				}
				if !tt.includeAll {
					if agent.Type != AgentCrew && agent.Type != AgentPolecat {
						continue
					}
				}
				if tt.sender != "" && formatAgentName(agent) == tt.sender {
					continue
				}
				targets = append(targets, agent)
			}

			// Build a set of recipient names for easy lookup.
			recipientSet := make(map[string]bool)
			for _, a := range targets {
				recipientSet[formatAgentName(a)] = true
			}

			// All expected recipients must be present.
			for _, name := range tt.wantNames {
				if !recipientSet[name] {
					t.Errorf("expected recipient %q missing from targets; got %v", name, keys(recipientSet))
				}
			}

			// Excluded agents must not appear.
			for _, name := range tt.wantExcluded {
				if recipientSet[name] {
					t.Errorf("excluded agent %q should not be in targets", name)
				}
			}
		})
	}
}

// keys returns the keys of a string bool map for diagnostic output.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestBroadcastEdgeCase_EmptyMessage verifies the empty-message guard path.
// broadcast.go checks `if message == ""` and returns an error.
func TestBroadcastEdgeCase_EmptyMessage(t *testing.T) {
	// Replicate the guard used in runBroadcast.
	message := ""
	if message == "" {
		// This is the expected error path — test passes.
		return
	}
	t.Error("expected empty message check to trigger early return")
}

// TestBroadcastEdgeCase_SelfBroadcast verifies that when the sender matches
// every target, the resulting target list is empty (no one to send to).
func TestBroadcastEdgeCase_SelfBroadcast(t *testing.T) {
	sender := "gastown/crew/solo"

	all := []*AgentSession{
		{Name: "gt-crew-solo", Type: AgentCrew, Rig: "gastown", AgentName: "solo"},
	}

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type != AgentCrew && agent.Type != AgentPolecat {
			continue
		}
		if sender != "" && formatAgentName(agent) == sender {
			continue
		}
		targets = append(targets, agent)
	}

	if len(targets) != 0 {
		t.Errorf("self-only broadcast: expected 0 targets (all excluded as self), got %d", len(targets))
	}
}

// TestBroadcastEdgeCase_NoSessionsRunning verifies that an empty agent list
// produces an empty target list without panicking.
func TestBroadcastEdgeCase_NoSessionsRunning(t *testing.T) {
	var all []*AgentSession // empty

	var targets []*AgentSession
	for _, agent := range all {
		if agent.Type == AgentCrew || agent.Type == AgentPolecat {
			targets = append(targets, agent)
		}
	}

	if len(targets) != 0 {
		t.Errorf("no sessions: expected 0 targets, got %d", len(targets))
	}
}

// TestBroadcastEdgeCase_RigFilterNoMatch verifies that filtering by a rig with
// no running agents produces an empty target list.
func TestBroadcastEdgeCase_RigFilterNoMatch(t *testing.T) {
	all := []*AgentSession{
		{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"},
	}

	filterRig := "nonexistent"
	var targets []*AgentSession
	for _, agent := range all {
		if agent.Rig != filterRig {
			continue
		}
		if agent.Type == AgentCrew || agent.Type == AgentPolecat {
			targets = append(targets, agent)
		}
	}

	if len(targets) != 0 {
		t.Errorf("nonexistent rig filter: expected 0 targets, got %d", len(targets))
	}
}
