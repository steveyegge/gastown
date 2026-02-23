package capacity

import (
	"testing"
)

func TestPlanDispatch(t *testing.T) {
	beads := func(n int) []PendingBead {
		result := make([]PendingBead, n)
		for i := range result {
			result[i] = PendingBead{ID: string(rune('a' + i))}
		}
		return result
	}

	tests := []struct {
		name              string
		availableCapacity int
		batchSize         int
		readyCount        int
		wantCount         int
		wantSkipped       int
		wantReason        string
	}{
		{"no ready beads", 5, 3, 0, 0, 0, "none"},
		{"no capacity (negative)", -1, 3, 10, 0, 10, "capacity"},
		{"no capacity (zero)", 0, 3, 10, 0, 10, "capacity"},
		{"capacity constrains", 2, 3, 10, 2, 8, "capacity"},
		{"batch constrains", 10, 3, 10, 3, 7, "batch"},
		{"ready constrains", 10, 5, 2, 2, 0, "ready"},
		{"large capacity, batch constrains", 100, 3, 10, 3, 7, "batch"},
		{"large capacity, ready constrains", 100, 5, 2, 2, 0, "ready"},
		{"all equal", 3, 3, 3, 3, 0, "batch"},
		{"single bead", 10, 3, 1, 1, 0, "ready"},
		{"capacity 1", 1, 3, 10, 1, 9, "capacity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready := beads(tt.readyCount)
			plan := PlanDispatch(tt.availableCapacity, tt.batchSize, ready)

			if len(plan.ToDispatch) != tt.wantCount {
				t.Errorf("ToDispatch count: got %d, want %d", len(plan.ToDispatch), tt.wantCount)
			}
			if plan.Skipped != tt.wantSkipped {
				t.Errorf("Skipped: got %d, want %d", plan.Skipped, tt.wantSkipped)
			}
			if plan.Reason != tt.wantReason {
				t.Errorf("Reason: got %q, want %q", plan.Reason, tt.wantReason)
			}
		})
	}
}

func TestFilterCircuitBroken(t *testing.T) {
	tests := []struct {
		name        string
		failures    []int // dispatch_failures per bead (-1 = nil context)
		maxFailures int
		wantKept    int
		wantRemoved int
	}{
		{"all healthy", []int{0, 0, 0}, 3, 3, 0},
		{"one at threshold", []int{0, 3, 1}, 3, 2, 1},
		{"one above threshold", []int{0, 5, 1}, 3, 2, 1},
		{"all broken", []int{3, 4, 5}, 3, 0, 3},
		{"nil context passes through", []int{-1, 0, 2}, 3, 3, 0},
		{"empty list", []int{}, 3, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var beads []PendingBead
			for i, f := range tt.failures {
				b := PendingBead{ID: string(rune('a' + i))}
				if f >= 0 {
					b.Context = &SlingContextFields{DispatchFailures: f}
				}
				beads = append(beads, b)
			}

			kept, removed := FilterCircuitBroken(beads, tt.maxFailures)
			if len(kept) != tt.wantKept {
				t.Errorf("kept: got %d, want %d", len(kept), tt.wantKept)
			}
			if removed != tt.wantRemoved {
				t.Errorf("removed: got %d, want %d", removed, tt.wantRemoved)
			}
		})
	}
}

func TestAllReady(t *testing.T) {
	beads := []PendingBead{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	result := AllReady(beads)
	if len(result) != 3 {
		t.Errorf("AllReady should pass all beads through, got %d", len(result))
	}
}

func TestBlockerAware(t *testing.T) {
	beads := []PendingBead{
		{ID: "ctx-a", WorkBeadID: "a"},
		{ID: "ctx-b", WorkBeadID: "b"},
		{ID: "ctx-c", WorkBeadID: "c"},
		{ID: "ctx-d", WorkBeadID: "d"},
	}

	readyIDs := map[string]bool{"a": true, "c": true}
	filter := BlockerAware(readyIDs)
	result := filter(beads)

	if len(result) != 2 {
		t.Fatalf("BlockerAware should return 2 beads, got %d", len(result))
	}
	if result[0].WorkBeadID != "a" || result[1].WorkBeadID != "c" {
		t.Errorf("BlockerAware returned wrong beads: %v, %v", result[0].WorkBeadID, result[1].WorkBeadID)
	}
}

func TestBlockerAware_EmptySet(t *testing.T) {
	beads := []PendingBead{{ID: "a", WorkBeadID: "wa"}, {ID: "b", WorkBeadID: "wb"}}
	readyIDs := map[string]bool{}
	filter := BlockerAware(readyIDs)
	result := filter(beads)
	if len(result) != 0 {
		t.Errorf("BlockerAware with empty readyIDs should return 0 beads, got %d", len(result))
	}
}

func TestCircuitBreakerPolicy(t *testing.T) {
	policy := CircuitBreakerPolicy(3)

	tests := []struct {
		failures int
		want     FailureAction
	}{
		{0, FailureRetry},
		{1, FailureRetry},
		{2, FailureRetry},
		{3, FailureQuarantine},
		{5, FailureQuarantine},
	}
	for _, tt := range tests {
		got := policy(tt.failures)
		if got != tt.want {
			t.Errorf("CircuitBreakerPolicy(3)(%d) = %v, want %v", tt.failures, got, tt.want)
		}
	}
}

func TestNoRetryPolicy(t *testing.T) {
	policy := NoRetryPolicy()
	for _, failures := range []int{0, 1, 5} {
		if got := policy(failures); got != FailureQuarantine {
			t.Errorf("NoRetryPolicy()(%d) = %v, want FailureQuarantine", failures, got)
		}
	}
}

func TestReconstructFromContext(t *testing.T) {
	ctx := &SlingContextFields{
		WorkBeadID:  "bead-123",
		TargetRig:   "prod-rig",
		Formula:     "mol-polecat-work",
		Args:        "do stuff",
		Vars:        "x=1\ny=2",
		Merge:       "mr",
		BaseBranch:  "main",
		Account:     "acme",
		Agent:       "codex",
		Mode:        "ralph",
		NoMerge:     true,
		HookRawBead: true,
	}

	params := ReconstructFromContext(ctx)

	if params.BeadID != "bead-123" {
		t.Errorf("BeadID: got %q, want %q", params.BeadID, "bead-123")
	}
	if params.RigName != "prod-rig" {
		t.Errorf("RigName: got %q, want %q", params.RigName, "prod-rig")
	}
	if params.FormulaName != "mol-polecat-work" {
		t.Errorf("FormulaName: got %q, want %q", params.FormulaName, "mol-polecat-work")
	}
	if params.Args != "do stuff" {
		t.Errorf("Args: got %q, want %q", params.Args, "do stuff")
	}
	if len(params.Vars) != 2 || params.Vars[0] != "x=1" || params.Vars[1] != "y=2" {
		t.Errorf("Vars: got %v, want [x=1 y=2]", params.Vars)
	}
	if params.Merge != "mr" {
		t.Errorf("Merge: got %q, want %q", params.Merge, "mr")
	}
	if params.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want %q", params.BaseBranch, "main")
	}
	if params.Account != "acme" {
		t.Errorf("Account: got %q, want %q", params.Account, "acme")
	}
	if params.Agent != "codex" {
		t.Errorf("Agent: got %q, want %q", params.Agent, "codex")
	}
	if params.Mode != "ralph" {
		t.Errorf("Mode: got %q, want %q", params.Mode, "ralph")
	}
	if !params.NoMerge {
		t.Error("NoMerge: expected true")
	}
	if !params.HookRawBead {
		t.Error("HookRawBead: expected true")
	}
}

func TestReconstructFromContext_EmptyVars(t *testing.T) {
	ctx := &SlingContextFields{
		WorkBeadID: "bead-1",
		TargetRig:  "rig1",
	}
	params := ReconstructFromContext(ctx)
	if params.Vars != nil {
		t.Errorf("Vars should be nil when ctx.Vars is empty, got %v", params.Vars)
	}
}

func TestSplitVars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "a=1", []string{"a=1"}},
		{"two newline-separated", "a=1\nb=2", []string{"a=1", "b=2"}},
		{"three newline-separated", "x=hello\ny=world\nz=42", []string{"x=hello", "y=world", "z=42"}},
		{"blank lines filtered", "a=1\n\nb=2\n", []string{"a=1", "b=2"}},
		{"whitespace trimmed", "  a=1  \n  b=2  ", []string{"a=1", "b=2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitVars(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("splitVars(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("splitVars(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("splitVars(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
