package formula

import (
	"strings"
	"testing"
)

// TestWitnessPatrolHasReconcileStep verifies that the witness patrol formula
// includes a reconcile-idle step that runs after survey-workers.
func TestWitnessPatrolHasReconcileStep(t *testing.T) {
	content, err := formulasFS.ReadFile("formulas/mol-witness-patrol.formula.toml")
	if err != nil {
		t.Fatalf("reading witness patrol formula: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("parsing witness patrol formula: %v", err)
	}

	// Find the reconcile-idle step
	var reconcileStep *Step
	for i := range f.Steps {
		if f.Steps[i].ID == "reconcile-idle" {
			reconcileStep = &f.Steps[i]
			break
		}
	}

	if reconcileStep == nil {
		t.Fatal("witness patrol formula missing 'reconcile-idle' step")
	}

	// Must depend on survey-workers
	hasDep := false
	for _, dep := range reconcileStep.Needs {
		if dep == "survey-workers" {
			hasDep = true
			break
		}
	}
	if !hasDep {
		t.Error("reconcile-idle step must depend on 'survey-workers'")
	}

	// Description must mention key concepts
	desc := reconcileStep.Description
	if !strings.Contains(desc, "MERGE_READY") {
		t.Error("reconcile-idle description must mention MERGE_READY")
	}
	if !strings.Contains(desc, "ReconcileIdlePolecats") {
		t.Error("reconcile-idle description must reference ReconcileIdlePolecats function")
	}

	// check-timer-gates must now depend on reconcile-idle (not survey-workers)
	for i := range f.Steps {
		if f.Steps[i].ID == "check-timer-gates" {
			hasReconcileDep := false
			for _, dep := range f.Steps[i].Needs {
				if dep == "reconcile-idle" {
					hasReconcileDep = true
					break
				}
			}
			if !hasReconcileDep {
				t.Error("check-timer-gates must depend on 'reconcile-idle' (not directly on survey-workers)")
			}
			break
		}
	}
}
