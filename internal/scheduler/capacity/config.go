package capacity

import "time"
// ResourceClass classifies a task by its CPU/resource cost.
type ResourceClass string

const (
	// ClassExpensive: tests, builds, typecheck (high CPU, high memory)
	ClassExpensive ResourceClass = "expensive"
	// ClassModerate: lint, setup/npm install (medium CPU)
	ClassModerate ResourceClass = "moderate"
	// ClassCheap: edit files, review, commit (low CPU)
	ClassCheap ResourceClass = "cheap"
)

// ResourceLimits defines per-class concurrent task limits.
type ResourceLimits struct {
	// Expensive is max concurrent expensive tasks (tests, builds).
	// Default: 2
	Expensive int `json:"expensive,omitempty"`
	// Moderate is max concurrent moderate tasks (lint, setup).
	// Default: 3
	Moderate int `json:"moderate,omitempty"`
	// Cheap is max concurrent cheap tasks (edit, review).
	// Default: equals MaxPolecats (no separate limit)
	Cheap int `json:"cheap,omitempty"`
}

// GetExpensive returns Expensive or default (2).
func (r *ResourceLimits) GetExpensive() int {
	if r == nil || r.Expensive == 0 {
		return 2
	}
	return r.Expensive
}

// GetModerate returns Moderate or default (3).
func (r *ResourceLimits) GetModerate() int {
	if r == nil || r.Moderate == 0 {
		return 3
	}
	return r.Moderate
}

// GetCheap returns Cheap or defaults to MaxPolecats (no separate limit).
func (r *ResourceLimits) GetCheap(maxPolecats int) int {
	if r == nil || r.Cheap == 0 {
		return maxPolecats
	}
	if r.Cheap > maxPolecats {
		return maxPolecats
	}
	return r.Cheap
}


// SchedulerConfig configures the capacity scheduler for polecat dispatch.
// This is a town-wide setting (not per-rig) because capacity control is host-wide:
// API rate limits, memory, and CPU are shared resources across all rigs.
//
// Behavior is driven entirely by MaxPolecats:
//   -1 (default): direct dispatch — gt sling works as before, near-zero overhead
//    0:           direct dispatch (same as -1)
//    N > 0:       deferred dispatch — labels/metadata applied, daemon dispatches
type SchedulerConfig struct {
	// MaxPolecats is the max concurrent polecats across ALL rigs.
	// Includes both scheduler-dispatched and directly-slung polecats.
	// nil/absent = default (-1, direct dispatch). 0 = direct dispatch (same as -1).
	// N > 0 = deferred dispatch with capacity control.
	MaxPolecats *int `json:"max_polecats,omitempty"`

	// BatchSize is the number of beads to dispatch per heartbeat tick.
	// Limits spawn rate per 3-minute cycle.
	// nil/absent = default (1). Explicit 0 is rejected by config setter.
	BatchSize *int `json:"batch_size,omitempty"`

	// SpawnDelay is the delay between spawns to prevent Dolt lock contention.
	// Default: "0s".
	SpawnDelay string `json:"spawn_delay,omitempty"`

	// ResourceLimits defines per-class concurrent task limits.
	ResourceLimits *ResourceLimits `json:"resource_limits,omitempty"`
}

// DefaultSchedulerConfig returns a SchedulerConfig with sensible defaults.
// MaxPolecats=-1 means direct dispatch (no scheduler overhead).
func DefaultSchedulerConfig() *SchedulerConfig {
	defaultMax := -1
	defaultBatch := 1
	return &SchedulerConfig{
		MaxPolecats: &defaultMax,
		BatchSize:   &defaultBatch,
		SpawnDelay:  "0s",
	}
}

// GetMaxPolecats returns MaxPolecats or the default (-1, direct dispatch) if unset.
func (c *SchedulerConfig) GetMaxPolecats() int {
	if c == nil || c.MaxPolecats == nil {
		return -1
	}
	return *c.MaxPolecats
}

// GetBatchSize returns BatchSize or the default (1) if unset.
func (c *SchedulerConfig) GetBatchSize() int {
	if c == nil || c.BatchSize == nil {
		return 1
	}
	return *c.BatchSize
}

// GetSpawnDelay returns SpawnDelay as a duration, defaulting to 0s.
func (c *SchedulerConfig) GetSpawnDelay() time.Duration {
	if c == nil || c.SpawnDelay == "" {
		return 0
	}
	return ParseDurationOrDefault(c.SpawnDelay, 0)
}

// IsDeferred returns true when the scheduler is configured for deferred dispatch
// (max_polecats > 0). Returns false for direct dispatch (-1) and disabled (0).
func (c *SchedulerConfig) IsDeferred() bool {
	return c.GetMaxPolecats() > 0
}

// ParseDurationOrDefault parses a Go duration string, returning fallback on error or empty input.
func ParseDurationOrDefault(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
