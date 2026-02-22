package capacity

import "strings"

// PendingBead represents a bead that is scheduled and ready for dispatch evaluation.
type PendingBead struct {
	ID          string
	Title       string
	TargetRig   string
	Description string
	Labels      []string
	Meta        *SchedulerMetadata // pre-parsed, nil if missing
}

// DispatchPlan is the output of PlanDispatch — what to dispatch and why.
type DispatchPlan struct {
	ToDispatch []PendingBead
	Skipped    int
	Reason     string // "capacity" | "batch" | "ready" | "none"
}

// FailureAction indicates what to do after a dispatch failure.
type FailureAction int

const (
	// FailureRetry means the bead should be retried on the next cycle.
	FailureRetry FailureAction = iota
	// FailureQuarantine means the bead should be marked as permanently failed.
	FailureQuarantine
)

// ReadinessFilter is a function that filters pending beads to those ready for dispatch.
type ReadinessFilter func(pending []PendingBead) []PendingBead

// FailurePolicy is a function that determines what to do after N failures.
type FailurePolicy func(failures int) FailureAction

// AllReady is a ReadinessFilter that passes all beads through (no filtering).
func AllReady(pending []PendingBead) []PendingBead {
	return pending
}

// BlockerAware returns a ReadinessFilter that only passes beads whose IDs
// appear in the readyIDs set (i.e., beads that have no unresolved blockers).
func BlockerAware(readyIDs map[string]bool) ReadinessFilter {
	return func(pending []PendingBead) []PendingBead {
		var result []PendingBead
		for _, b := range pending {
			if readyIDs[b.ID] {
				result = append(result, b)
			}
		}
		return result
	}
}

// PlanDispatch computes which beads to dispatch given capacity constraints.
// maxPol: max concurrent polecats (0 = unlimited)
// batchSize: max beads per dispatch cycle
// activePolecats: currently running polecats
// ready: beads that passed readiness filtering
func PlanDispatch(maxPol, batchSize, activePolecats int, ready []PendingBead) DispatchPlan {
	if len(ready) == 0 {
		return DispatchPlan{Reason: "none"}
	}

	// Compute available capacity
	capacity := 0
	if maxPol > 0 {
		capacity = maxPol - activePolecats
		if capacity <= 0 {
			return DispatchPlan{
				Skipped: len(ready),
				Reason:  "capacity",
			}
		}
	}

	// Dispatch up to the smallest of capacity, batchSize, and readyBeads count
	toDispatch := batchSize
	if capacity > 0 && capacity < toDispatch {
		toDispatch = capacity
	}
	if len(ready) < toDispatch {
		toDispatch = len(ready)
	}

	reason := "batch"
	if capacity > 0 && capacity < batchSize && capacity < len(ready) {
		reason = "capacity"
	}
	if len(ready) < batchSize && (capacity == 0 || len(ready) < capacity) {
		reason = "ready"
	}

	return DispatchPlan{
		ToDispatch: ready[:toDispatch],
		Skipped:    len(ready) - toDispatch,
		Reason:     reason,
	}
}

// NoRetryPolicy returns a FailurePolicy that always quarantines on first failure.
func NoRetryPolicy() FailurePolicy {
	return func(failures int) FailureAction {
		return FailureQuarantine
	}
}

// CircuitBreakerPolicy returns a FailurePolicy that retries up to maxFailures
// times, then quarantines.
func CircuitBreakerPolicy(maxFailures int) FailurePolicy {
	return func(failures int) FailureAction {
		if failures >= maxFailures {
			return FailureQuarantine
		}
		return FailureRetry
	}
}

// FilterCircuitBroken removes beads that have exceeded the maximum dispatch
// failures threshold. Returns the filtered list and the count of removed beads.
func FilterCircuitBroken(beads []PendingBead, maxFailures int) ([]PendingBead, int) {
	var result []PendingBead
	removed := 0
	for _, b := range beads {
		if b.Meta != nil && b.Meta.DispatchFailures >= maxFailures {
			removed++
			continue
		}
		result = append(result, b)
	}
	return result, removed
}

// DispatchParams captures what the scheduler needs to tell the dispatcher.
// Mirrors the relevant fields from cmd.SlingParams but is scheduler-owned.
type DispatchParams struct {
	BeadID      string
	FormulaName string
	RigName     string
	Args        string
	Vars        []string
	Merge       string
	BaseBranch  string
	Account     string
	Agent       string
	Mode        string
	NoMerge     bool
	HookRawBead bool
}

// ReconstructDispatchParams builds DispatchParams from scheduler metadata.
func ReconstructDispatchParams(meta *SchedulerMetadata, beadID string) DispatchParams {
	p := DispatchParams{
		BeadID:      beadID,
		RigName:     meta.TargetRig,
		FormulaName: meta.Formula,
		Args:        meta.Args,
		Merge:       meta.Merge,
		BaseBranch:  meta.BaseBranch,
		Account:     meta.Account,
		Agent:       meta.Agent,
		Mode:        meta.Mode,
		NoMerge:     meta.NoMerge,
		HookRawBead: meta.HookRawBead,
	}
	if meta.Vars != "" {
		p.Vars = splitVars(meta.Vars)
	}
	return p
}

// splitVars splits a newline-separated vars string into individual key=value pairs.
func splitVars(vars string) []string {
	if vars == "" {
		return nil
	}
	var result []string
	for _, line := range strings.Split(vars, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
