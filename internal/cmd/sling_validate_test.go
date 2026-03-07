package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
		errMsg  string // substring that must appear in error
	}{
		// Valid targets
		{name: "empty target", target: "", wantErr: false},
		{name: "self target", target: ".", wantErr: false},
		{name: "bare rig name", target: "gastown", wantErr: false},
		{name: "role shortcut mayor", target: "mayor", wantErr: false},
		{name: "role shortcut deacon", target: "deacon", wantErr: false},
		{name: "rig/polecats/name", target: "gastown/polecats/nux", wantErr: false},
		{name: "rig/crew/name", target: "gastown/crew/burke", wantErr: false},
		{name: "rig/witness", target: "gastown/witness", wantErr: false},
		{name: "rig/refinery", target: "gastown/refinery", wantErr: false},
		{name: "deacon/dogs", target: "deacon/dogs", wantErr: false},
		{name: "deacon/dogs/name", target: "deacon/dogs/rex", wantErr: false},
		{name: "polecat shorthand", target: "gastown/nux", wantErr: false},
		{name: "crew shorthand", target: "gastown/max", wantErr: false},

		// Invalid targets — empty segments
		{name: "trailing slash", target: "gastown/", wantErr: true, errMsg: "empty path segment"},
		{name: "double slash", target: "gastown//polecats", wantErr: true, errMsg: "empty path segment"},
		{name: "leading slash", target: "/polecats", wantErr: true, errMsg: "empty path segment"},

		// Invalid targets — unknown role (only rejected with 3+ segments)
		{name: "unknown role 3-seg", target: "gastown/badrole/name", wantErr: true, errMsg: "unknown role"},
		{name: "typo in role 3-seg", target: "gastown/polecat/name", wantErr: true, errMsg: "unknown role"},

		// Invalid targets — missing name
		{name: "crew no name", target: "gastown/crew", wantErr: true, errMsg: "requires a worker name"},
		{name: "polecats no name", target: "gastown/polecats", wantErr: true, errMsg: "requires a polecat name"},

		// Invalid targets — witness/refinery with sub-agents
		{name: "witness with name", target: "gastown/witness/extra", wantErr: true, errMsg: "does not have named sub-agents"},
		{name: "refinery with name", target: "gastown/refinery/extra", wantErr: true, errMsg: "does not have named sub-agents"},

		// Invalid targets — too many segments
		{name: "too many segments", target: "gastown/crew/burke/extra", wantErr: true, errMsg: "too many path segments"},

		// Invalid targets — mayor sub-paths
		{name: "mayor sub-agent", target: "mayor/something", wantErr: true, errMsg: "does not have sub-agents"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTarget(tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateTarget(%q) = nil, want error containing %q", tc.target, tc.errMsg)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateTarget(%q) = %v, want nil", tc.target, err)
			}
			if tc.wantErr && err != nil && tc.errMsg != "" {
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("ValidateTarget(%q) error = %q, want it to contain %q", tc.target, err.Error(), tc.errMsg)
				}
			}
		})
	}
}

func TestSlingChecklistRender(t *testing.T) {
	// Capture stdout
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	cl := &slingChecklist{}
	cl.pass("bead exists", "gt-abc")
	cl.pass("bead status", "open")
	cl.info("batch size", "3 beads")
	cl.warn("respawn count", "at limit")
	cl.render()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify section header
	if !strings.Contains(output, "Validation") {
		t.Errorf("expected 'Validation' header in output, got:\n%s", output)
	}

	// Verify checks appear
	if !strings.Contains(output, "bead exists") {
		t.Errorf("expected 'bead exists' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "gt-abc") {
		t.Errorf("expected 'gt-abc' detail in output, got:\n%s", output)
	}

	// Verify summary: 2 passes out of 4 total checks
	if !strings.Contains(output, "2/4 checks passed") {
		t.Errorf("expected '2/4 checks passed' in output, got:\n%s", output)
	}
}

func TestBuildSlingValidation(t *testing.T) {
	info := &beadInfo{
		Title:    "Fix the bug",
		Status:   "open",
		Assignee: "",
	}
	cl := buildSlingValidation("gt-xyz", info, "gastown/polecats/test", "mol-polecat-work", t.TempDir(), false)

	// Should have: bead exists, bead status, title valid, target resolved, formula exists, cross-rig guard, respawn count
	if len(cl.checks) < 6 {
		t.Errorf("expected at least 6 checks, got %d", len(cl.checks))
	}

	// All should pass for a normal bead
	for _, chk := range cl.checks {
		if chk.Status == "warn" {
			t.Errorf("unexpected warning for check %q: %s", chk.Name, chk.Detail)
		}
	}
}

func TestBuildDryRunPlan(t *testing.T) {
	info := &beadInfo{Title: "Test issue", Status: "open"}
	plan := buildDryRunPlan("gt-abc", info, "gastown/polecats/test", "%99", "mol-polecat-work", dryRunPlanOpts{
		args:  "focus on security",
		merge: "mr",
	})

	found := map[string]bool{}
	for _, line := range plan {
		if strings.Contains(line, "Cook formula") {
			found["cook"] = true
		}
		if strings.Contains(line, "Hook bead") {
			found["hook"] = true
		}
		if strings.Contains(line, "Nudge pane") {
			found["nudge"] = true
		}
		if strings.Contains(line, "Store args") {
			found["args"] = true
		}
		if strings.Contains(line, "Merge strategy") {
			found["merge"] = true
		}
	}

	for _, key := range []string{"cook", "hook", "nudge", "args", "merge"} {
		if !found[key] {
			t.Errorf("expected %q in plan, got: %v", key, plan)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
	}
	for _, tc := range tests {
		got := truncate(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestRenderDryRunPlan(t *testing.T) {
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	renderDryRunPlan([]string{"Step 1: do thing", "Step 2: do other thing"})

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Plan") {
		t.Errorf("expected 'Plan' header in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Step 1") {
		t.Errorf("expected 'Step 1' in output, got:\n%s", output)
	}
}

func TestBuildScheduleValidation(t *testing.T) {
	info := &beadInfo{
		Title:  "Test task",
		Status: "open",
	}
	cl := buildScheduleValidation("gt-abc", info, "gastown", "mol-polecat-work")

	// Should have: bead exists, bead status, target rig, formula exists, cross-rig guard, no duplicate
	if len(cl.checks) < 5 {
		t.Errorf("expected at least 5 checks, got %d", len(cl.checks))
	}

	// Find formula check
	foundFormula := false
	for _, chk := range cl.checks {
		if chk.Name == "formula exists" {
			foundFormula = true
			if chk.Detail != "mol-polecat-work" {
				t.Errorf("formula detail = %q, want %q", chk.Detail, "mol-polecat-work")
			}
		}
	}
	if !foundFormula {
		t.Error("expected 'formula exists' check")
	}
}

