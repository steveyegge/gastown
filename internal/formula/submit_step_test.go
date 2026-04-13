package formula

import (
	"strings"
	"testing"
)

// TestPolecatWorkSubmitStep_UsesGHPRCreate verifies that the mol-polecat-work
// submit-and-exit step instructs polecats to create a GitHub PR via gh CLI,
// not submit to gt's merge queue.
func TestPolecatWorkSubmitStep_UsesGHPRCreate(t *testing.T) {
	content, err := GetEmbeddedFormulaContent("mol-polecat-work")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Find the submit-and-exit step
	var submitStep *Step
	for i := range f.Steps {
		if f.Steps[i].ID == "submit-and-exit" {
			submitStep = &f.Steps[i]
			break
		}
	}
	if submitStep == nil {
		t.Fatal("submit-and-exit step not found in mol-polecat-work")
	}

	desc := submitStep.Description

	// Must instruct polecats to create a PR via gh CLI
	if !strings.Contains(desc, "gh pr create") {
		t.Error("submit-and-exit step should contain 'gh pr create' instruction")
	}

	// Must NOT reference gt done for code task submission (MQ model)
	// gt done --status DEFERRED for report-only tasks is acceptable
	lines := strings.Split(desc, "\n")
	for _, line := range lines {
		if strings.Contains(line, "gt done") && !strings.Contains(line, "DEFERRED") {
			t.Errorf("submit-and-exit step should not reference 'gt done' for code tasks, found: %s", strings.TrimSpace(line))
		}
	}

	// Must NOT reference merge queue or MR bead for code submission
	if strings.Contains(desc, "MR bead") {
		t.Error("submit-and-exit step should not reference 'MR bead'")
	}
	if strings.Contains(desc, "merge queue") {
		t.Error("submit-and-exit step should not reference 'merge queue'")
	}
}
