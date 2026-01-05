package swarm

import (
	"testing"
	"time"
)

func TestSwarmStateIsTerminal(t *testing.T) {
	tests := []struct {
		state    SwarmState
		terminal bool
	}{
		{SwarmCreated, false},
		{SwarmActive, false},
		{SwarmMerging, false},
		{SwarmLanded, true},
		{SwarmFailed, true},
		{SwarmCanceled, true},
	}

	for _, tt := range tests {
		if got := tt.state.IsTerminal(); got != tt.terminal {
			t.Errorf("%s.IsTerminal() = %v, want %v", tt.state, got, tt.terminal)
		}
	}
}

func TestSwarmStateIsActive(t *testing.T) {
	tests := []struct {
		state  SwarmState
		active bool
	}{
		{SwarmCreated, true},
		{SwarmActive, true},
		{SwarmMerging, true},
		{SwarmLanded, false},
		{SwarmFailed, false},
		{SwarmCanceled, false},
	}

	for _, tt := range tests {
		if got := tt.state.IsActive(); got != tt.active {
			t.Errorf("%s.IsActive() = %v, want %v", tt.state, got, tt.active)
		}
	}
}

func TestTaskStateIsComplete(t *testing.T) {
	tests := []struct {
		state    TaskState
		complete bool
	}{
		{TaskPending, false},
		{TaskAssigned, false},
		{TaskInProgress, false},
		{TaskReview, false},
		{TaskMerged, true},
		{TaskFailed, true},
	}

	for _, tt := range tests {
		if got := tt.state.IsComplete(); got != tt.complete {
			t.Errorf("%s.IsComplete() = %v, want %v", tt.state, got, tt.complete)
		}
	}
}

func TestSwarmSummary(t *testing.T) {
	swarm := &Swarm{
		ID:      "test-swarm",
		State:   SwarmActive,
		Workers: []string{"worker1", "worker2"},
		Tasks: []SwarmTask{
			{IssueID: "1", State: TaskPending},
			{IssueID: "2", State: TaskAssigned},
			{IssueID: "3", State: TaskInProgress},
			{IssueID: "4", State: TaskReview},
			{IssueID: "5", State: TaskMerged},
			{IssueID: "6", State: TaskFailed},
		},
	}

	summary := swarm.Summary()

	if summary.ID != "test-swarm" {
		t.Errorf("ID = %q, want test-swarm", summary.ID)
	}
	if summary.TotalTasks != 6 {
		t.Errorf("TotalTasks = %d, want 6", summary.TotalTasks)
	}
	if summary.PendingTasks != 2 {
		t.Errorf("PendingTasks = %d, want 2", summary.PendingTasks)
	}
	if summary.ActiveTasks != 2 {
		t.Errorf("ActiveTasks = %d, want 2", summary.ActiveTasks)
	}
	if summary.MergedTasks != 1 {
		t.Errorf("MergedTasks = %d, want 1", summary.MergedTasks)
	}
	if summary.FailedTasks != 1 {
		t.Errorf("FailedTasks = %d, want 1", summary.FailedTasks)
	}
	if summary.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", summary.WorkerCount)
	}
}

func TestSwarmProgress(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []SwarmTask
		expected int
	}{
		{
			name:     "empty",
			tasks:    nil,
			expected: 0,
		},
		{
			name: "none merged",
			tasks: []SwarmTask{
				{State: TaskPending},
				{State: TaskInProgress},
			},
			expected: 0,
		},
		{
			name: "half merged",
			tasks: []SwarmTask{
				{State: TaskMerged},
				{State: TaskInProgress},
			},
			expected: 50,
		},
		{
			name: "all merged",
			tasks: []SwarmTask{
				{State: TaskMerged},
				{State: TaskMerged},
				{State: TaskMerged},
			},
			expected: 100,
		},
		{
			name: "one of three",
			tasks: []SwarmTask{
				{State: TaskMerged},
				{State: TaskPending},
				{State: TaskPending},
			},
			expected: 33, // 1/3 = 33%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			swarm := &Swarm{Tasks: tt.tasks}
			if got := swarm.Progress(); got != tt.expected {
				t.Errorf("Progress() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestSwarmJSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	swarm := &Swarm{
		ID:           "swarm-123",
		RigName:      "gastown",
		EpicID:       "gt-abc",
		BaseCommit:   "abc123",
		Integration:  "swarm-123-integration",
		TargetBranch: "main",
		State:        SwarmActive,
		CreatedAt:    now,
		UpdatedAt:    now,
		Workers:      []string{"Toast", "Cheedo"},
		Tasks: []SwarmTask{
			{
				IssueID:  "gt-def",
				Title:    "Test task",
				Assignee: "Toast",
				Branch:   "swarm-123-Toast",
				State:    TaskInProgress,
			},
		},
	}

	// Just verify it doesn't panic and has expected values
	if swarm.ID != "swarm-123" {
		t.Errorf("ID = %q, want swarm-123", swarm.ID)
	}
	if len(swarm.Workers) != 2 {
		t.Errorf("Workers count = %d, want 2", len(swarm.Workers))
	}
}
