// Package autopilot provides automatic gastown configuration for user tasks.
//
// Autopilot decomposes a high-level task description into a structured plan,
// launches it as a mountain convoy, and governs resource usage dynamically.
// Three phases: Plan → Launch → Govern.
package autopilot

import "time"

// Strategy represents the execution mode for an autopilot plan.
type Strategy string

const (
	// StrategyPolecats uses independent polecat workers for focused, parallel tasks.
	StrategyPolecats Strategy = "polecats"

	// StrategyCrew uses a crew for deep-context work (architecture, multi-file refactors).
	StrategyCrew Strategy = "crew"

	// StrategyHybrid mixes crew for strategic planning with polecats for execution.
	StrategyHybrid Strategy = "hybrid"
)

// IsValid returns true if the strategy is a recognized value.
func (s Strategy) IsValid() bool {
	switch s {
	case StrategyPolecats, StrategyCrew, StrategyHybrid:
		return true
	default:
		return false
	}
}

// TaskNode represents a single task in the autopilot plan's dependency graph.
type TaskNode struct {
	// ID is a short identifier for dependency references (e.g., "t1", "t2").
	ID string `json:"id"`

	// Title is the human-readable task description.
	Title string `json:"title"`

	// Type classifies the task for bead creation (e.g., "task", "bug", "test").
	Type string `json:"type"`

	// DependsOn lists IDs of tasks that must complete before this one can start.
	DependsOn []string `json:"depends_on,omitempty"`

	// Priority is the task priority (1 = highest, 5 = lowest).
	Priority int `json:"priority,omitempty"`

	// EstimatedMinutes is the planner's rough time estimate for the task.
	EstimatedMinutes int `json:"estimated_minutes,omitempty"`
}

// AutopilotPlan is the structured output from the planner phase.
// It contains the full task decomposition and execution strategy
// produced by a claude --print call against the project context.
type AutopilotPlan struct {
	// EpicTitle is the top-level epic name for the bead hierarchy.
	EpicTitle string `json:"epic_title"`

	// Tasks is the ordered list of tasks forming the dependency graph.
	Tasks []TaskNode `json:"tasks"`

	// Strategy is the recommended execution mode (polecats, crew, or hybrid).
	Strategy Strategy `json:"strategy"`

	// MaxConcurrency is the recommended number of concurrent workers,
	// based on resource assessment and task parallelism.
	MaxConcurrency int `json:"max_concurrency"`

	// Reasoning explains why the planner chose this decomposition and strategy.
	Reasoning string `json:"reasoning,omitempty"`
}

// ResourceSnapshot captures system resource state at a point in time.
// Used by the planner to inform concurrency decisions and by the governor
// to make dynamic adjustments during execution.
type ResourceSnapshot struct {
	// Timestamp is when the snapshot was taken.
	Timestamp time.Time `json:"timestamp"`

	// MemoryTotalBytes is total physical memory.
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`

	// MemoryAvailableBytes is memory available for new processes.
	MemoryAvailableBytes uint64 `json:"memory_available_bytes"`

	// CPUUsagePercent is overall CPU utilization (0-100).
	CPUUsagePercent float64 `json:"cpu_usage_percent"`

	// NumCores is the number of logical CPU cores.
	NumCores int `json:"num_cores"`

	// LoadAvg1 is the 1-minute load average.
	LoadAvg1 float64 `json:"load_avg_1"`

	// ActiveSessions is the count of running Claude agent sessions.
	ActiveSessions int `json:"active_sessions"`
}

// MemoryAvailableGB returns available memory in gigabytes.
func (s *ResourceSnapshot) MemoryAvailableGB() float64 {
	return float64(s.MemoryAvailableBytes) / (1024 * 1024 * 1024)
}

// MemoryTotalGB returns total memory in gigabytes.
func (s *ResourceSnapshot) MemoryTotalGB() float64 {
	return float64(s.MemoryTotalBytes) / (1024 * 1024 * 1024)
}

// GovernorConfig controls the autopilot governor's resource management behavior.
// The governor runs as a deacon patrol molecule, polling at the configured interval
// and adjusting polecat concurrency based on system pressure.
type GovernorConfig struct {
	// PollInterval is how often the governor checks resource state.
	// Default: 60s.
	PollInterval time.Duration `json:"poll_interval"`

	// MemoryFloorGB is the minimum free memory before the governor pauses the mountain.
	// Default: 1.0 (GB).
	MemoryFloorGB float64 `json:"memory_floor_gb"`

	// MemoryResumeGB is the free memory threshold to resume after a pause.
	// Default: 1.5 (GB).
	MemoryResumeGB float64 `json:"memory_resume_gb"`

	// CPUCeiling is the CPU usage percent above which the governor reduces concurrency.
	// Default: 85.
	CPUCeiling float64 `json:"cpu_ceiling"`

	// CPUFloor is the CPU usage percent below which the governor may increase concurrency.
	// Default: 75.
	CPUFloor float64 `json:"cpu_floor"`

	// MaxPolecats is the upper bound on concurrent polecats (from the plan).
	MaxPolecats int `json:"max_polecats"`

	// MountainID is the convoy/mountain being governed.
	MountainID string `json:"mountain_id"`

	// EpicID is the root epic bead being tracked for completion.
	EpicID string `json:"epic_id"`
}

// DefaultGovernorConfig returns a GovernorConfig with sensible defaults.
// MaxPolecats, MountainID, and EpicID must be set by the caller.
func DefaultGovernorConfig() *GovernorConfig {
	return &GovernorConfig{
		PollInterval:   60 * time.Second,
		MemoryFloorGB:  1.0,
		MemoryResumeGB: 1.5,
		CPUCeiling:     85.0,
		CPUFloor:       75.0,
	}
}

// GovernorAction represents a decision made by the governor during a poll cycle.
type GovernorAction string

const (
	// GovernorNoop means no adjustment is needed.
	GovernorNoop GovernorAction = "noop"

	// GovernorPause pauses the mountain due to memory pressure.
	GovernorPause GovernorAction = "pause"

	// GovernorResume resumes the mountain after memory recovery.
	GovernorResume GovernorAction = "resume"

	// GovernorScaleDown reduces max_polecats by 1 due to CPU pressure.
	GovernorScaleDown GovernorAction = "scale_down"

	// GovernorScaleUp increases max_polecats by 1 due to CPU headroom.
	GovernorScaleUp GovernorAction = "scale_up"

	// GovernorComplete terminates the governor because the mountain is done.
	GovernorComplete GovernorAction = "complete"
)

// GovernorDecision captures a governor poll result with the chosen action and reasoning.
type GovernorDecision struct {
	// Action is what the governor decided to do.
	Action GovernorAction `json:"action"`

	// Reason explains why this action was chosen.
	Reason string `json:"reason"`

	// Snapshot is the resource state that informed the decision.
	Snapshot ResourceSnapshot `json:"snapshot"`

	// CurrentPolecats is the max_polecats setting before this decision.
	CurrentPolecats int `json:"current_polecats"`

	// Timestamp is when the decision was made.
	Timestamp time.Time `json:"timestamp"`
}
