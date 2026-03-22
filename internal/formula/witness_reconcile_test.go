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

	// detect-reverts step must exist and depend on reconcile-idle
	var detectRevertsStep *Step
	for i := range f.Steps {
		if f.Steps[i].ID == "detect-reverts" {
			detectRevertsStep = &f.Steps[i]
			break
		}
	}
	if detectRevertsStep == nil {
		t.Fatal("witness patrol formula missing 'detect-reverts' step")
	}

	hasReconcileDep := false
	for _, dep := range detectRevertsStep.Needs {
		if dep == "reconcile-idle" {
			hasReconcileDep = true
			break
		}
	}
	if !hasReconcileDep {
		t.Error("detect-reverts step must depend on 'reconcile-idle'")
	}

	// Description must mention key concepts
	revertDesc := detectRevertsStep.Description
	if !strings.Contains(revertDesc, "Revert") {
		t.Error("detect-reverts description must mention Revert commits")
	}
	if !strings.Contains(revertDesc, "reopen") || !strings.Contains(revertDesc, "bead") {
		t.Error("detect-reverts description must mention reopening beads")
	}
	if !strings.Contains(revertDesc, "thrash") {
		t.Error("detect-reverts description must mention thrash detection")
	}

	// check-timer-gates must now depend on detect-reverts (not directly on reconcile-idle)
	for i := range f.Steps {
		if f.Steps[i].ID == "check-timer-gates" {
			hasDetectRevertsDep := false
			for _, dep := range f.Steps[i].Needs {
				if dep == "detect-reverts" {
					hasDetectRevertsDep = true
					break
				}
			}
			if !hasDetectRevertsDep {
				t.Error("check-timer-gates must depend on 'detect-reverts' (not directly on reconcile-idle)")
			}
			break
		}
	}
}
