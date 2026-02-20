package formula

import (
	"fmt"
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

// TestPatrolFormulasHaveSquashCycle verifies that all three patrol formulas
// include the squash/create-wisp/hook cycle in their loop step.
//
// Without this cycle, closed step beads accumulate across patrol cycles,
// `bd ready` eventually returns nothing, and `findActivePatrol` can't find
// the wisp via status=hooked on session restart.
//
// Regression test for steveyegge/gastown#1371.
//
// Also enforces that squash uses `gt mol squash --jitter` to desynchronize
// concurrent Dolt lock acquisitions from deacon/witness/refinery patrol agents.
// See: hq-vytww2 (Reduce Dolt lock contention from concurrent patrol agents).
func TestPatrolFormulasHaveSquashCycle(t *testing.T) {
	type patrolFormula struct {
		name       string
		loopStepID string
		molName    string // the formula name used in "bd mol wisp <name>"
	}

	patrolFormulas := []patrolFormula{
		{"mol-witness-patrol.formula.toml", "loop-or-exit", "mol-witness-patrol"},
		{"mol-deacon-patrol.formula.toml", "loop-or-exit", "mol-deacon-patrol"},
		{"mol-refinery-patrol.formula.toml", "burn-or-loop", "mol-refinery-patrol"},
	}

	for _, pf := range patrolFormulas {
		t.Run(pf.name, func(t *testing.T) {
			content, err := formulasFS.ReadFile("formulas/" + pf.name)
			if err != nil {
				t.Fatalf("reading %s: %v", pf.name, err)
			}

			// Parse the formula and find the loop step description
			f, err := Parse(content)
			if err != nil {
				t.Fatalf("parsing %s: %v", pf.name, err)
			}

			var loopDesc string
			for _, step := range f.Steps {
				if step.ID == pf.loopStepID {
					loopDesc = step.Description
					break
				}
			}
			if loopDesc == "" {
				t.Fatalf("%s: %s step not found or has empty description", pf.name, pf.loopStepID)
			}

			// The loop step must contain all three parts of the cycle:
			// 1. Squash the current wisp (using gt mol squash --jitter to reduce lock contention)
			// 2. Create a new patrol wisp
			// 3. Hook/assign the new wisp
			requiredPatterns := []struct {
				pattern string
				reason  string
			}{
				{"gt mol squash", "squash current wisp using gt command (not bd) for jitter support"},
				{"--jitter", "jitter flag required to desynchronize concurrent Dolt lock acquisitions (hq-vytww2)"},
				{fmt.Sprintf("bd mol wisp %s", pf.molName), "create new patrol wisp for next cycle"},
				{"--status=hooked", "hook the new wisp so findActivePatrol can find it"},
			}

			for _, rp := range requiredPatterns {
				if !strings.Contains(loopDesc, rp.pattern) {
					t.Errorf("%s %s step missing %q (%s)\n"+
						"All patrol formulas must include the squash/create-wisp/hook cycle with jitter.\n"+
						"See steveyegge/gastown#1371 (squash cycle) and hq-vytww2 (jitter requirement).",
						pf.name, pf.loopStepID, rp.pattern, rp.reason)
				}
			}
		})
	}
}

// TestPatrolFormulasHaveWispGC verifies that all three patrol formulas
// include `bd mol wisp gc` in their inbox-check step to clean up stale
// wisps from abnormal exits in previous cycles.
//
// Without this, patrol agents that die/restart abnormally before reaching
// the loop-or-exit squash step leave their wisps open indefinitely.
//
// Regression test for steveyegge/gastown#1712.
func TestPatrolFormulasHaveWispGC(t *testing.T) {
	patrolFormulas := []string{
		"mol-witness-patrol.formula.toml",
		"mol-deacon-patrol.formula.toml",
		"mol-refinery-patrol.formula.toml",
	}

	for _, name := range patrolFormulas {
		t.Run(name, func(t *testing.T) {
			content, err := formulasFS.ReadFile("formulas/" + name)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}

			f, err := Parse(content)
			if err != nil {
				t.Fatalf("parsing %s: %v", name, err)
			}

			// Find the inbox-check step (first step in all patrol formulas)
			var inboxDesc string
			for _, step := range f.Steps {
				if step.ID == "inbox-check" {
					inboxDesc = step.Description
					break
				}
			}
			if inboxDesc == "" {
				t.Fatalf("%s: inbox-check step not found or has empty description", name)
			}

			if !strings.Contains(inboxDesc, "bd mol wisp gc") {
				t.Errorf("%s inbox-check step missing \"bd mol wisp gc\"\n"+
					"All patrol formulas must run wisp GC at the start of each cycle\n"+
					"to clean up stale wisps from abnormal exits.\n"+
					"See steveyegge/gastown#1712.",
					name)
			}
		})
	}
}
