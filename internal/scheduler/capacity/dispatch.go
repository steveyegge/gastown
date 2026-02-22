package capacity

import (
	"fmt"
	"time"
)

// ErrOnSuccessFailed wraps dispatch-succeeded-but-cleanup-failed errors.
// Used to distinguish "polecat launched, context close failed" from
// "polecat never launched" in the OnFailure callback.
type ErrOnSuccessFailed struct{ Err error }

func (e *ErrOnSuccessFailed) Error() string {
	return "dispatch succeeded but OnSuccess failed: " + e.Err.Error()
}
func (e *ErrOnSuccessFailed) Unwrap() error { return e.Err }

// DispatchCycle is a capacity-controlled dispatch orchestrator.
// The core loop is generic — all domain logic is injected via callbacks.
type DispatchCycle struct {
	// AvailableCapacity returns the number of free dispatch slots.
	// Positive = that many slots available. Zero or negative = no capacity.
	AvailableCapacity func() (int, error)

	// QueryPending returns work items eligible for dispatch.
	// The implementation handles querying, readiness checks, and filtering.
	QueryPending func() ([]PendingBead, error)

	// Execute dispatches a single item. Called for each planned item.
	Execute func(PendingBead) error

	// OnSuccess is called after successful dispatch.
	OnSuccess func(PendingBead) error

	// OnFailure is called after failed dispatch.
	OnFailure func(PendingBead, error)

	// BatchSize caps items dispatched per cycle.
	BatchSize int

	// SpawnDelay between dispatches.
	SpawnDelay time.Duration
}

// DispatchReport summarizes the result of one dispatch cycle.
type DispatchReport struct {
	Dispatched int
	Failed     int
	Skipped    int
	Reason     string // "capacity" | "batch" | "ready" | "none"
}

// Plan returns the dispatch plan without executing. Used for dry-run.
func (c *DispatchCycle) Plan() (DispatchPlan, error) {
	cap, err := c.AvailableCapacity()
	if err != nil {
		return DispatchPlan{}, fmt.Errorf("checking capacity: %w", err)
	}

	pending, err := c.QueryPending()
	if err != nil {
		return DispatchPlan{}, fmt.Errorf("querying pending: %w", err)
	}

	return PlanDispatch(cap, c.BatchSize, pending), nil
}

// onSuccessRetries is the number of times to retry OnSuccess before giving up.
const onSuccessRetries = 2

// Run executes one dispatch cycle: query → plan → execute → report.
func (c *DispatchCycle) Run() (DispatchReport, error) {
	plan, err := c.Plan()
	if err != nil {
		return DispatchReport{}, err
	}

	report := DispatchReport{
		Skipped: plan.Skipped,
		Reason:  plan.Reason,
	}

	for i, b := range plan.ToDispatch {
		if err := c.Execute(b); err != nil {
			report.Failed++
			if c.OnFailure != nil {
				c.OnFailure(b, err)
			}
			continue
		}

		// OnSuccess must succeed (e.g., closing the sling context) to prevent
		// re-dispatch on the next cycle. Retry before giving up.
		if c.OnSuccess != nil {
			var successErr error
			for attempt := 0; attempt <= onSuccessRetries; attempt++ {
				successErr = c.OnSuccess(b)
				if successErr == nil {
					break
				}
				if attempt < onSuccessRetries {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				}
			}
			if successErr != nil {
				// OnSuccess failed after retries — do NOT count as dispatched.
				// The dispatch ran but we couldn't close the context, so treat
				// it as a failure to prevent double-dispatch on the next cycle.
				report.Failed++
				if c.OnFailure != nil {
					c.OnFailure(b, &ErrOnSuccessFailed{Err: successErr})
				}
				continue
			}
		}

		report.Dispatched++

		// Inter-spawn delay (skip after last item)
		if c.SpawnDelay > 0 && i < len(plan.ToDispatch)-1 {
			time.Sleep(c.SpawnDelay)
		}
	}

	return report, nil
}
