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
// See: gt-0hzeo (refinery stall bug — missing await-signal)
func TestPatrolFormulasHaveBackoffLogic(t *testing.T) {
	// Patrol formulas that must have backoff logic.
	// The loopStepID is the step that contains the await-signal logic;
	// witness/deacon use "loop-or-exit", refinery uses "burn-or-loop".
	type patrolFormula struct {
		name       string
		loopStepID string
	}

	patrolFormulas := []patrolFormula{
		{"mol-witness-patrol.formula.toml", "loop-or-exit"},
		{"mol-deacon-patrol.formula.toml", "loop-or-exit"},
		{"mol-refinery-patrol.formula.toml", "burn-or-loop"},
	}

	for _, pf := range patrolFormulas {
		t.Run(pf.name, func(t *testing.T) {
			// Read formula content directly from embedded FS
			content, err := formulasFS.ReadFile("formulas/" + pf.name)
			if err != nil {
				t.Fatalf("reading %s: %v", pf.name, err)
			}

			contentStr := string(content)

			// Verify the formula contains the loop/decision step
			doubleQuoted := `id = "` + pf.loopStepID + `"`
			singleQuoted := `id = '` + pf.loopStepID + `'`
			if !strings.Contains(contentStr, doubleQuoted) &&
				!strings.Contains(contentStr, singleQuoted) {
				t.Fatalf("%s: %s step not found", pf.name, pf.loopStepID)
			}

			// Verify the formula contains the required backoff patterns
			requiredPatterns := []string{
				"await-signal",
				"backoff",
				"gt mol step await-signal",
			}

			for _, pattern := range requiredPatterns {
				if !strings.Contains(contentStr, pattern) {
					t.Errorf("%s missing required pattern %q\n"+
						"The %s step must include await-signal with backoff logic "+
						"to prevent tight loops when the rig is idle.\n"+
						"See PR #1052 for the original fix.",
						pf.name, pattern, pf.loopStepID)
				}
			}
		})
	}
}
