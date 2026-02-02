package formulas

import "testing"

// TestFormulaValidation is an intentionally failing test to validate
// the mol-cicd-fix formula. A polecat should fix this by changing
// the expected value from "wrong" to "correct".
//
// Bug: gt-a4z5tp
func TestFormulaValidation(t *testing.T) {
	got := "correct"
	want := "wrong" // BUG: This should be "correct"
	
	if got != want {
		t.Errorf("TestFormulaValidation: got %q, want %q", got, want)
	}
}
