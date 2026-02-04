package formula

import (
	"strings"
	"testing"
)

// TestPatrolFormulasHaveBackoffLogic verifies that patrol formulas include
// await-signal backoff logic in their loop-or-exit steps.
//
// This is a regression test for a bug where the witness patrol formula's
// await-signal logic was accidentally removed by subsequent commits,
// causing a tight loop when the rig was idle.
//
// See: PR #1052 (original fix), gt-tjm9q (regression report)
func TestPatrolFormulasHaveBackoffLogic(t *testing.T) {
	// Patrol formulas that must have backoff logic
	patrolFormulas := []string{
		"mol-witness-patrol.formula.toml",
		"mol-deacon-patrol.formula.toml",
	}

	for _, formulaName := range patrolFormulas {
		t.Run(formulaName, func(t *testing.T) {
			// Read formula content directly from embedded FS
			content, err := formulasFS.ReadFile("formulas/" + formulaName)
			if err != nil {
				t.Fatalf("reading %s: %v", formulaName, err)
			}

			contentStr := string(content)

			// Verify the formula contains the loop-or-exit step
			if !strings.Contains(contentStr, `id = "loop-or-exit"`) &&
				!strings.Contains(contentStr, `id = 'loop-or-exit'`) {
				t.Fatalf("%s: loop-or-exit step not found", formulaName)
			}

			// Extract the loop-or-exit step description
			// The step is defined as [[steps]] with id = "loop-or-exit"
			// We check that the file contains the required patterns near this step
			requiredPatterns := []string{
				"await-signal",
				"backoff",
				"gt mol step await-signal",
			}

			for _, pattern := range requiredPatterns {
				if !strings.Contains(contentStr, pattern) {
					t.Errorf("%s missing required pattern %q\n"+
						"The loop-or-exit step must include await-signal with backoff logic "+
						"to prevent tight loops when the rig is idle.\n"+
						"See PR #1052 for the original fix.",
						formulaName, pattern)
				}
			}
		})
	}
}
