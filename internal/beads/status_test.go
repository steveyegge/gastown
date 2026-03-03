package beads

import "testing"

func TestAgentStateProtectsFromCleanup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state AgentState
		want  bool
	}{
		{AgentStateStuck, true},
		{AgentStateAwaitingGate, true},
		{AgentStateWorking, false},
		{AgentStateIdle, false},
		{AgentStateDone, false},
		{AgentStateSpawning, false},
		{AgentStateNuked, false},
		{AgentStateRunning, false},
		{AgentStateEscalated, false},
		{AgentState(""), false},
	}
	for _, tt := range tests {
		if got := tt.state.ProtectsFromCleanup(); got != tt.want {
			t.Errorf("AgentState(%q).ProtectsFromCleanup() = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestAgentStateIsActive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state AgentState
		want  bool
	}{
		{AgentStateWorking, true},
		{AgentStateRunning, true},
		{AgentStateSpawning, true},
		{AgentStateIdle, false},
		{AgentStateDone, false},
		{AgentStateStuck, false},
		{AgentStateNuked, false},
	}
	for _, tt := range tests {
		if got := tt.state.IsActive(); got != tt.want {
			t.Errorf("AgentState(%q).IsActive() = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestIssueStatusBlocksRemoval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status IssueStatus
		want   bool
	}{
		{StatusOpen, true},
		{StatusClosed, false},
		{IssueStatusHooked, false},
		{IssueStatusPinned, false},
		{StatusInProgress, false},
		{StatusTombstone, false},
	}
	for _, tt := range tests {
		if got := tt.status.BlocksRemoval(); got != tt.want {
			t.Errorf("IssueStatus(%q).BlocksRemoval() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIssueStatusIsTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status IssueStatus
		want   bool
	}{
		{StatusClosed, true},
		{StatusTombstone, true},
		{StatusOpen, false},
		{IssueStatusHooked, false},
		{StatusInProgress, false},
		{IssueStatusPinned, false},
	}
	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.want {
			t.Errorf("IssueStatus(%q).IsTerminal() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestIssueStatusIsAssigned(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status IssueStatus
		want   bool
	}{
		{IssueStatusHooked, true},
		{StatusInProgress, true},
		{StatusOpen, false},
		{StatusClosed, false},
		{IssueStatusPinned, false},
	}
	for _, tt := range tests {
		if got := tt.status.IsAssigned(); got != tt.want {
			t.Errorf("IssueStatus(%q).IsAssigned() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestAgentStateConstants(t *testing.T) {
	t.Parallel()
	// Verify all expected agent states match their string values
	states := map[AgentState]string{
		AgentStateSpawning:     "spawning",
		AgentStateWorking:      "working",
		AgentStateDone:         "done",
		AgentStateStuck:        "stuck",
		AgentStateEscalated:    "escalated",
		AgentStateIdle:         "idle",
		AgentStateRunning:      "running",
		AgentStateNuked:        "nuked",
		AgentStateAwaitingGate: "awaiting-gate",
	}
	for state, expected := range states {
		if string(state) != expected {
			t.Errorf("AgentState constant %q has value %q, want %q", expected, string(state), expected)
		}
	}
}

func TestIssueStatusConstants(t *testing.T) {
	t.Parallel()
	statuses := map[IssueStatus]string{
		StatusOpen:        "open",
		StatusClosed:      "closed",
		StatusInProgress:  "in_progress",
		StatusTombstone:   "tombstone",
		StatusBlocked:     "blocked",
		IssueStatusPinned: "pinned",
		IssueStatusHooked: "hooked",
	}
	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("IssueStatus constant %q has value %q, want %q", expected, string(status), expected)
		}
	}
}
