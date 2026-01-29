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

	// The formula has a linear structure (each step depends only on the previous one)
	// Verify the linear dependency chain
	linearChain := []struct {
		id    string
		needs string // empty for first step
	}{
		{"inbox-check", ""},
		{"process-cleanups", "inbox-check"},
		{"check-refinery", "process-cleanups"},
		{"survey-workers", "check-refinery"},
		{"check-timer-gates", "survey-workers"},
		{"check-swarm-completion", "check-timer-gates"},
		{"ping-deacon", "check-swarm-completion"},
		{"patrol-cleanup", "ping-deacon"},
		{"context-check", "patrol-cleanup"},
		{"loop-or-exit", "context-check"},
	}

	for _, tc := range linearChain {
		step := f.GetStep(tc.id)
		if step == nil {
			t.Errorf("Step %s not found", tc.id)
			continue
		}

		if tc.needs == "" {
			if len(step.Needs) != 0 {
				t.Errorf("Step %s should have no dependencies, got %v", tc.id, step.Needs)
			}
		} else {
			if len(step.Needs) != 1 || step.Needs[0] != tc.needs {
				t.Errorf("Step %s should need only [%s], got %v", tc.id, tc.needs, step.Needs)
			}
		}
	}

	// Test ParallelReadySteps with a linear formula
	// After completing check-refinery, only survey-workers should be ready (linear chain)
	completed := map[string]bool{
		"inbox-check":      true,
		"process-cleanups": true,
		"check-refinery":   true,
	}

	parallel, sequential := f.ParallelReadySteps(completed)

	// In a linear formula, there should be no parallel steps
	if len(parallel) != 0 {
		t.Errorf("Expected 0 parallel steps in linear formula, got %d: %v", len(parallel), parallel)
	}

	// The next sequential step should be survey-workers
	if sequential != "survey-workers" {
		t.Errorf("Expected sequential step 'survey-workers', got %q", sequential)
	}
}
