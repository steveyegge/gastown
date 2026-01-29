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

    // Verify parallel flag is set on the right steps
    parallelSteps := []string{"survey-workers", "check-timer-gates", "check-swarm-completion", "ping-deacon"}
    for _, id := range parallelSteps {
        step := f.GetStep(id)
        if step == nil {
            t.Errorf("Step %s not found", id)
            continue
        }
        if !step.Parallel {
            t.Errorf("Step %s should have parallel=true", id)
        }
    }

    // Test that after check-refinery, all 4 parallel steps are ready
    completed := map[string]bool{
        "inbox-check": true,
        "process-cleanups": true,
        "check-refinery": true,
    }
    
    parallel, sequential := f.ParallelReadySteps(completed)
    
    if len(parallel) != 4 {
        t.Errorf("Expected 4 parallel steps, got %d: %v", len(parallel), parallel)
    }
    
    if sequential != "" {
        t.Errorf("Expected no sequential step, got %s", sequential)
    }

    // Verify patrol-cleanup needs all 4 parallel steps
    patrolCleanup := f.GetStep("patrol-cleanup")
    if patrolCleanup == nil {
        t.Fatal("patrol-cleanup step not found")
    }
    if len(patrolCleanup.Needs) != 4 {
        t.Errorf("patrol-cleanup should need 4 steps, got %d: %v", len(patrolCleanup.Needs), patrolCleanup.Needs)
    }
}
