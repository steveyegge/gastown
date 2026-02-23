package formula

import (
	"testing"
)

func TestForkFormulaValidation(t *testing.T) {
	f, err := ParseFile("formulas/mol-refinery-patrol-fork.formula.toml")
	if err != nil {
		t.Fatalf("Error parsing formula: %v", err)
	}

	t.Logf("Formula parsed: %s v%d", f.Name, f.Version)
	t.Logf("Steps: %d, Vars: %d", len(f.Steps), len(f.Vars))

	// Validate
	if err := f.Validate(); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check key steps exist
	expectedSteps := []string{"inbox-check", "queue-scan", "process-branch", "run-tests", "handle-failures", "submit-pr", "poll-status"}
	for _, stepID := range expectedSteps {
		if step := f.GetStep(stepID); step == nil {
			t.Errorf("Missing step: %s", stepID)
		}
	}

	// Check required vars
	if upstreamRepo, ok := f.Vars["upstream_repo"]; !ok {
		t.Error("Missing required var: upstream_repo")
	} else if !upstreamRepo.Required {
		t.Error("upstream_repo should be required")
	}

	// Check target_branch has default
	if targetBranch, ok := f.Vars["target_branch"]; !ok {
		t.Error("Missing var: target_branch")
	} else if targetBranch.Default != "main" {
		t.Errorf("target_branch default should be 'main', got '%s'", targetBranch.Default)
	}

	t.Log("✅ Fork formula validation passed!")
}
