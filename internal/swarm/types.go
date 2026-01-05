// Package swarm provides types and management for multi-agent swarms.
package swarm

import "time"

// SwarmState represents the lifecycle state of a swarm.
type SwarmState string

const (
	// SwarmCreated is the initial state after swarm creation.
	SwarmCreated SwarmState = "created"

	// SwarmActive means workers are actively working on tasks.
	SwarmActive SwarmState = "active"

	// SwarmMerging means all work is done and merging is in progress.
	SwarmMerging SwarmState = "merging"

	// SwarmLanded means all work has been merged to the target branch.
	SwarmLanded SwarmState = "landed"

	// SwarmFailed means the swarm failed and cannot be recovered.
	SwarmFailed SwarmState = "failed"

	// SwarmCanceled means the swarm was explicitly canceled.
	SwarmCanceled SwarmState = "canceled"
)

// IsTerminal returns true if the swarm is in a terminal state.
func (s SwarmState) IsTerminal() bool {
	return s == SwarmLanded || s == SwarmFailed || s == SwarmCanceled
}

// IsActive returns true if the swarm is actively running.
func (s SwarmState) IsActive() bool {
	return s == SwarmCreated || s == SwarmActive || s == SwarmMerging
}

// Swarm represents a coordinated multi-agent work unit.
// The swarm references a beads epic that tracks all swarm work.
type Swarm struct {
	// ID is the unique swarm identifier (matches beads epic ID).
	ID string `json:"id"`

	// RigName is the rig this swarm operates in.
	RigName string `json:"rig_name"`

	// EpicID is the beads epic tracking this swarm's work.
	EpicID string `json:"epic_id"`

	// BaseCommit is the git SHA all workers branch from.
	BaseCommit string `json:"base_commit"`

	// Integration is the integration branch name for merging work.
	Integration string `json:"integration"`

	// TargetBranch is the branch to merge into when complete (e.g., "main").
	TargetBranch string `json:"target_branch"`

	// State is the current lifecycle state.
	State SwarmState `json:"state"`

	// CreatedAt is when the swarm was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the swarm was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// Workers is the list of polecat names assigned to this swarm.
	Workers []string `json:"workers"`

	// Tasks is the list of tasks in this swarm.
	Tasks []SwarmTask `json:"tasks"`

	// Error contains error details if State is SwarmFailed.
	Error string `json:"error,omitempty"`
}

// SwarmTask represents a single task in the swarm.
// Each task maps to a beads issue and is assigned to a worker.
type SwarmTask struct {
	// IssueID is the beads issue ID for this task.
	IssueID string `json:"issue_id"`

	// Title is the task title (copied from beads issue).
	Title string `json:"title"`

	// Assignee is the polecat name working on this task.
	Assignee string `json:"assignee,omitempty"`

	// Branch is the worker's branch name for this task.
	Branch string `json:"branch,omitempty"`

	// State mirrors the beads issue status.
	State TaskState `json:"state"`

	// MergedAt is when the task branch was merged (if merged).
	MergedAt *time.Time `json:"merged_at,omitempty"`
}

// TaskState represents the state of a swarm task.
type TaskState string

const (
	// TaskPending means the task is not yet started.
	TaskPending TaskState = "pending"

	// TaskAssigned means the task is assigned but not started.
	TaskAssigned TaskState = "assigned"

	// TaskInProgress means the task is actively being worked on.
	TaskInProgress TaskState = "in_progress"

	// TaskReview means the task is ready for review/merge.
	TaskReview TaskState = "review"

	// TaskMerged means the task has been merged.
	TaskMerged TaskState = "merged"

	// TaskFailed means the task failed.
	TaskFailed TaskState = "failed"
)

// IsComplete returns true if the task is in a terminal state.
func (s TaskState) IsComplete() bool {
	return s == TaskMerged || s == TaskFailed
}

// SwarmSummary provides a high-level overview of swarm progress.
type SwarmSummary struct {
	ID           string     `json:"id"`
	State        SwarmState `json:"state"`
	TotalTasks   int        `json:"total_tasks"`
	PendingTasks int        `json:"pending_tasks"`
	ActiveTasks  int        `json:"active_tasks"`
	MergedTasks  int        `json:"merged_tasks"`
	FailedTasks  int        `json:"failed_tasks"`
	WorkerCount  int        `json:"worker_count"`
}

// Summary returns a SwarmSummary for this swarm.
func (s *Swarm) Summary() SwarmSummary {
	summary := SwarmSummary{
		ID:          s.ID,
		State:       s.State,
		TotalTasks:  len(s.Tasks),
		WorkerCount: len(s.Workers),
	}

	for _, task := range s.Tasks {
		switch task.State {
		case TaskPending, TaskAssigned:
			summary.PendingTasks++
		case TaskInProgress, TaskReview:
			summary.ActiveTasks++
		case TaskMerged:
			summary.MergedTasks++
		case TaskFailed:
			summary.FailedTasks++
		}
	}

	return summary
}

// Progress returns the completion percentage (0-100).
func (s *Swarm) Progress() int {
	if len(s.Tasks) == 0 {
		return 0
	}

	merged := 0
	for _, task := range s.Tasks {
		if task.State == TaskMerged {
			merged++
		}
	}

	return (merged * 100) / len(s.Tasks)
}
