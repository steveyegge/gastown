package capacity

import (
	"errors"
	"testing"
	"time"
)

func TestDispatchCycle_Plan(t *testing.T) {
	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 5, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{
				{ID: "a", WorkBeadID: "wa"},
				{ID: "b", WorkBeadID: "wb"},
				{ID: "c", WorkBeadID: "wc"},
			}, nil
		},
		BatchSize: 2,
	}

	plan, err := cycle.Plan()
	if err != nil {
		t.Fatalf("Plan() error: %v", err)
	}
	if len(plan.ToDispatch) != 2 {
		t.Errorf("ToDispatch = %d, want 2", len(plan.ToDispatch))
	}
	if plan.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", plan.Skipped)
	}
}

func TestDispatchCycle_Plan_CapacityError(t *testing.T) {
	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 0, errors.New("tmux gone") },
		QueryPending:      func() ([]PendingBead, error) { return nil, nil },
		BatchSize:         1,
	}

	_, err := cycle.Plan()
	if err == nil {
		t.Fatal("Plan() should return error when AvailableCapacity fails")
	}
}

func TestDispatchCycle_Plan_QueryError(t *testing.T) {
	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 5, nil },
		QueryPending:      func() ([]PendingBead, error) { return nil, errors.New("bd failed") },
		BatchSize:         1,
	}

	_, err := cycle.Plan()
	if err == nil {
		t.Fatal("Plan() should return error when QueryPending fails")
	}
}

func TestDispatchCycle_Run_AllSuccess(t *testing.T) {
	dispatched := []string{}
	successCalled := []string{}

	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 100, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{
				{ID: "a", WorkBeadID: "wa"},
				{ID: "b", WorkBeadID: "wb"},
			}, nil
		},
		Execute: func(b PendingBead) error {
			dispatched = append(dispatched, b.ID)
			return nil
		},
		OnSuccess: func(b PendingBead) error {
			successCalled = append(successCalled, b.ID)
			return nil
		},
		OnFailure: func(b PendingBead, err error) {
			t.Errorf("OnFailure should not be called, called for %s", b.ID)
		},
		BatchSize: 10,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if report.Dispatched != 2 {
		t.Errorf("Dispatched = %d, want 2", report.Dispatched)
	}
	if report.Failed != 0 {
		t.Errorf("Failed = %d, want 0", report.Failed)
	}
	if len(dispatched) != 2 || dispatched[0] != "a" || dispatched[1] != "b" {
		t.Errorf("dispatched = %v, want [a b]", dispatched)
	}
	if len(successCalled) != 2 {
		t.Errorf("successCalled = %v, want 2 entries", successCalled)
	}
}

func TestDispatchCycle_Run_WithFailures(t *testing.T) {
	failuresCalled := []string{}

	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 100, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{
				{ID: "a"},
				{ID: "b"},
				{ID: "c"},
			}, nil
		},
		Execute: func(b PendingBead) error {
			if b.ID == "b" {
				return errors.New("dispatch failed")
			}
			return nil
		},
		OnSuccess: func(b PendingBead) error { return nil },
		OnFailure: func(b PendingBead, err error) {
			failuresCalled = append(failuresCalled, b.ID)
		},
		BatchSize: 10,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if report.Dispatched != 2 {
		t.Errorf("Dispatched = %d, want 2", report.Dispatched)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
	if len(failuresCalled) != 1 || failuresCalled[0] != "b" {
		t.Errorf("failuresCalled = %v, want [b]", failuresCalled)
	}
}

func TestDispatchCycle_Run_NoBeads(t *testing.T) {
	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 5, nil },
		QueryPending:      func() ([]PendingBead, error) { return nil, nil },
		Execute:           func(b PendingBead) error { t.Error("Execute should not be called"); return nil },
		BatchSize:         10,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if report.Dispatched != 0 {
		t.Errorf("Dispatched = %d, want 0", report.Dispatched)
	}
	if report.Reason != "none" {
		t.Errorf("Reason = %q, want %q", report.Reason, "none")
	}
}

func TestDispatchCycle_Run_OnSuccessError(t *testing.T) {
	// When Execute succeeds but OnSuccess fails (even after retries),
	// the item should NOT be counted as dispatched — it should be failed.
	// This prevents double-dispatch when context close fails.
	failureCalled := []string{}
	var failureErrors []error

	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 100, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{
				{ID: "a", WorkBeadID: "wa"},
				{ID: "b", WorkBeadID: "wb"},
			}, nil
		},
		Execute: func(b PendingBead) error {
			return nil // All executions succeed
		},
		OnSuccess: func(b PendingBead) error {
			if b.ID == "a" {
				return errors.New("context close failed")
			}
			return nil
		},
		OnFailure: func(b PendingBead, err error) {
			failureCalled = append(failureCalled, b.ID)
			failureErrors = append(failureErrors, err)
		},
		BatchSize: 10,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// "a" failed OnSuccess → counted as failed, not dispatched
	// "b" succeeded both Execute and OnSuccess → dispatched
	if report.Dispatched != 1 {
		t.Errorf("Dispatched = %d, want 1 (only 'b' should count)", report.Dispatched)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1 ('a' OnSuccess failed)", report.Failed)
	}
	if len(failureCalled) != 1 || failureCalled[0] != "a" {
		t.Errorf("failureCalled = %v, want [a]", failureCalled)
	}

	// Verify the error is an ErrOnSuccessFailed sentinel (not a string-based protocol)
	if len(failureErrors) == 1 {
		var onSuccessErr *ErrOnSuccessFailed
		if !errors.As(failureErrors[0], &onSuccessErr) {
			t.Errorf("OnFailure error should be *ErrOnSuccessFailed, got %T: %v", failureErrors[0], failureErrors[0])
		}
	}
}

func TestDispatchCycle_Run_OnSuccessRetry(t *testing.T) {
	// OnSuccess fails once then succeeds on retry — should count as dispatched.
	attempts := map[string]int{}

	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 100, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{{ID: "a", WorkBeadID: "wa"}}, nil
		},
		Execute: func(b PendingBead) error { return nil },
		OnSuccess: func(b PendingBead) error {
			attempts[b.ID]++
			if attempts[b.ID] <= 1 {
				return errors.New("transient failure")
			}
			return nil
		},
		OnFailure: func(b PendingBead, err error) {
			t.Errorf("OnFailure should not be called, called for %s: %v", b.ID, err)
		},
		BatchSize: 10,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if report.Dispatched != 1 {
		t.Errorf("Dispatched = %d, want 1", report.Dispatched)
	}
	if attempts["a"] != 2 {
		t.Errorf("OnSuccess attempts = %d, want 2 (1 fail + 1 success)", attempts["a"])
	}
}

func TestDispatchCycle_Run_SpawnDelay(t *testing.T) {
	start := time.Now()
	cycle := &DispatchCycle{
		AvailableCapacity: func() (int, error) { return 100, nil },
		QueryPending: func() ([]PendingBead, error) {
			return []PendingBead{{ID: "a"}, {ID: "b"}, {ID: "c"}}, nil
		},
		Execute:   func(b PendingBead) error { return nil },
		OnSuccess: func(b PendingBead) error { return nil },
		BatchSize: 10,
		// Use a very small delay so the test isn't slow, but verifiable
		SpawnDelay: 10 * time.Millisecond,
	}

	report, err := cycle.Run()
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	elapsed := time.Since(start)
	if report.Dispatched != 3 {
		t.Errorf("Dispatched = %d, want 3", report.Dispatched)
	}
	// 2 delays between 3 items = at least 20ms
	if elapsed < 15*time.Millisecond {
		t.Errorf("elapsed = %v, expected at least ~20ms for 2 delays", elapsed)
	}
}
