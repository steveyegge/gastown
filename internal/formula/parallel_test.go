package formula

import (
	"testing"
)

func TestParallelReadySteps(t *testing.T) {
	// Parse the witness patrol formula
	f, err := ParseFile("formulas/mol-witness-patrol.formula.toml")
	if err != nil {
		t.Fatalf("Failed to parse patrol formula: %v", err)
	}

	// Verify parallel flag is not set on sequential steps
	sequentialSteps := []string{"survey-workers", "check-timer-gates", "check-swarm-completion", "check-deacon"}
	for _, id := range sequentialSteps {
		step := f.GetStep(id)
		if step == nil {
			t.Errorf("Step %s not found", id)
			continue
		}
		if step.Parallel {
			t.Errorf("Step %s should have parallel=false", id)
		}
	}

	// Test that after check-refinery, the next sequential step is ready
	completed := map[string]bool{
		"inbox-check":      true,
		"process-cleanups": true,
		"check-refinery":   true,
	}

	parallel, sequential := f.ParallelReadySteps(completed)

	if len(parallel) != 0 {
		t.Errorf("Expected 0 parallel steps, got %d: %v", len(parallel), parallel)
	}

	if sequential != "survey-workers" {
		t.Errorf("Expected sequential step survey-workers, got %s", sequential)
	}

	// Verify patrol-cleanup needs check-deacon
	patrolCleanup := f.GetStep("patrol-cleanup")
	if patrolCleanup == nil {
		t.Fatal("patrol-cleanup step not found")
	}
	if len(patrolCleanup.Needs) != 1 {
		t.Errorf("patrol-cleanup should need 1 step, got %d: %v", len(patrolCleanup.Needs), patrolCleanup.Needs)
	}
	if len(patrolCleanup.Needs) == 1 && patrolCleanup.Needs[0] != "check-deacon" {
		t.Errorf("patrol-cleanup should need check-deacon, got %v", patrolCleanup.Needs)
	}
}
