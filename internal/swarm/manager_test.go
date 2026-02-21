package swarm

import (
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

// TestLoadSwarmNotFound tests that LoadSwarm returns error for missing epic.
func TestLoadSwarmNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	// LoadSwarm for non-existent epic should fail (no beads available)
	_, err := m.LoadSwarm("nonexistent-epic")
	if err == nil {
		t.Error("LoadSwarm should fail for non-existent epic")
	}
}

// TestGetSwarmNotFound tests that GetSwarm returns error for missing swarm.
func TestGetSwarmNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.GetSwarm("nonexistent")
	if err == nil {
		t.Error("GetSwarm for nonexistent should return error")
	}
}

// TestGetReadyTasksNotFound tests that GetReadyTasks returns error for missing swarm.
func TestGetReadyTasksNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.GetReadyTasks("nonexistent")
	if err != ErrSwarmNotFound {
		t.Errorf("GetReadyTasks = %v, want ErrSwarmNotFound", err)
	}
}

// TestIsCompleteNotFound tests that IsComplete returns error for missing swarm.
func TestIsCompleteNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.IsComplete("nonexistent")
	if err != ErrSwarmNotFound {
		t.Errorf("IsComplete = %v, want ErrSwarmNotFound", err)
	}
}

// TestSwarmE2ELifecycle documents the end-to-end swarm integration test protocol.
// This test documents the manual testing steps that were validated for gt-kc7yj.4.
//
// The test scenario creates a DAG of work:
//
//	     A
//	    / \
//	   B   C
//	    \ /
//	     D
//
// Test Results (verified 2025-12-29):
//
// 1. CREATE EPIC WITH DEPENDENCIES
//
//	bd create --type=epic --title="Test Epic"         → gt-xxxxx
//	bd create --type=task --title="Task A" --parent=gt-xxxxx  → gt-xxxxx.1
//	bd create --type=task --title="Task B" --parent=gt-xxxxx  → gt-xxxxx.2
//	bd create --type=task --title="Task C" --parent=gt-xxxxx  → gt-xxxxx.3
//	bd create --type=task --title="Task D" --parent=gt-xxxxx  → gt-xxxxx.4
//	bd dep add gt-xxxxx.2 gt-xxxxx.1  # B depends on A
//	bd dep add gt-xxxxx.3 gt-xxxxx.1  # C depends on A
//	bd dep add gt-xxxxx.4 gt-xxxxx.2  # D depends on B
//	bd dep add gt-xxxxx.4 gt-xxxxx.3  # D depends on C
//
// 2. VALIDATE SWARM STRUCTURE ✅
//
//	bd swarm validate gt-xxxxx
//	Expected output:
//	  Wave 1: 1 issue (Task A)
//	  Wave 2: 2 issues (Tasks B, C - parallel)
//	  Wave 3: 1 issue (Task D)
//	  Max parallelism: 2
//	  Swarmable: YES
//
// 3. CREATE SWARM MOLECULE ✅
//
//	bd swarm create gt-xxxxx
//	Expected: Creates molecule with mol_type=swarm linked to epic
//
// 4. VERIFY READY FRONT ✅
//
//	bd swarm status gt-xxxxx
//	Expected:
//	  Ready: Task A
//	  Blocked: Tasks B, C, D (with dependency info)
//
// 5. ISSUE COMPLETION ADVANCES FRONT ✅
//
//	bd close gt-xxxxx.1 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected:
//	  Completed: Task A
//	  Ready: Tasks B, C (now unblocked)
//	  Blocked: Task D
//
// 6. PARALLEL WORK ✅
//
//	bd close gt-xxxxx.2 gt-xxxxx.3 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected:
//	  Completed: Tasks A, B, C
//	  Ready: Task D (now unblocked)
//
// 7. FINAL COMPLETION ✅
//
//	bd close gt-xxxxx.4 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected: Progress 4/4 complete (100%)
//
// 8. SWARM AUTO-CLOSE ⚠️
//
//	The swarm and epic remain open after all tasks complete.
//	This is by design - the Witness coordinator is responsible for
//	detecting completion and closing the swarm molecule.
//	Manual close: bd close gt-xxxxx gt-yyyyy --reason "Swarm complete"
//
// KNOWN ISSUES:
//   - gt swarm status/land fail to find issues (filed as gt-594a4)
//   - bd swarm commands work correctly as the underlying implementation
//   - Auto-close requires Witness patrol (not automatic in beads)
func TestSwarmE2ELifecycle(t *testing.T) {
	// This test documents the manual E2E testing protocol.
	// The actual test requires beads infrastructure and is run manually.
	// See the docstring above for the complete test procedure.
	t.Skip("E2E test requires beads infrastructure - see docstring for manual test protocol")
}

// TestIsValidTransition tests all valid and invalid state transitions.
func TestIsValidTransition(t *testing.T) {
	tests := []struct {
		name string
		from SwarmState
		to   SwarmState
		want bool
	}{
		// Valid transitions from Created
		{"created->active", SwarmCreated, SwarmActive, true},
		{"created->canceled", SwarmCreated, SwarmCanceled, true},

		// Valid transitions from Active
		{"active->merging", SwarmActive, SwarmMerging, true},
		{"active->failed", SwarmActive, SwarmFailed, true},
		{"active->canceled", SwarmActive, SwarmCanceled, true},

		// Valid transitions from Merging
		{"merging->landed", SwarmMerging, SwarmLanded, true},
		{"merging->failed", SwarmMerging, SwarmFailed, true},
		{"merging->canceled", SwarmMerging, SwarmCanceled, true},

		// Terminal states: no transitions allowed
		{"landed->active", SwarmLanded, SwarmActive, false},
		{"landed->merging", SwarmLanded, SwarmMerging, false},
		{"landed->failed", SwarmLanded, SwarmFailed, false},
		{"landed->canceled", SwarmLanded, SwarmCanceled, false},
		{"failed->active", SwarmFailed, SwarmActive, false},
		{"failed->created", SwarmFailed, SwarmCreated, false},
		{"canceled->active", SwarmCanceled, SwarmActive, false},
		{"canceled->created", SwarmCanceled, SwarmCreated, false},

		// Invalid transitions (skipping states)
		{"created->merging", SwarmCreated, SwarmMerging, false},
		{"created->landed", SwarmCreated, SwarmLanded, false},
		{"created->failed", SwarmCreated, SwarmFailed, false},
		{"active->created", SwarmActive, SwarmCreated, false},
		{"active->landed", SwarmActive, SwarmLanded, false},
		{"merging->created", SwarmMerging, SwarmCreated, false},
		{"merging->active", SwarmMerging, SwarmActive, false},

		// Self-transitions (not allowed)
		{"created->created", SwarmCreated, SwarmCreated, false},
		{"active->active", SwarmActive, SwarmActive, false},
		{"merging->merging", SwarmMerging, SwarmMerging, false},

		// Unknown state
		{"unknown->active", SwarmState("unknown"), SwarmActive, false},
		{"active->unknown", SwarmActive, SwarmState("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("isValidTransition(%q, %q) = %v, want %v",
					tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// TestAppendUnique tests appending unique strings to a slice.
func TestAppendUnique(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		s     string
		want  []string
	}{
		{
			name:  "add to empty slice",
			slice: []string{},
			s:     "alpha",
			want:  []string{"alpha"},
		},
		{
			name:  "add to nil slice",
			slice: nil,
			s:     "alpha",
			want:  []string{"alpha"},
		},
		{
			name:  "add unique element",
			slice: []string{"alpha", "bravo"},
			s:     "charlie",
			want:  []string{"alpha", "bravo", "charlie"},
		},
		{
			name:  "add duplicate element",
			slice: []string{"alpha", "bravo"},
			s:     "alpha",
			want:  []string{"alpha", "bravo"},
		},
		{
			name:  "add duplicate last element",
			slice: []string{"alpha", "bravo"},
			s:     "bravo",
			want:  []string{"alpha", "bravo"},
		},
		{
			name:  "add empty string to empty slice",
			slice: []string{},
			s:     "",
			want:  []string{""},
		},
		{
			name:  "add empty string duplicate",
			slice: []string{""},
			s:     "",
			want:  []string{""},
		},
		{
			name:  "add empty string to non-empty slice",
			slice: []string{"alpha"},
			s:     "",
			want:  []string{"alpha", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendUnique(tt.slice, tt.s)
			if len(got) != len(tt.want) {
				t.Fatalf("appendUnique(%v, %q) length = %d, want %d",
					tt.slice, tt.s, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("appendUnique(%v, %q)[%d] = %q, want %q",
						tt.slice, tt.s, i, got[i], tt.want[i])
				}
			}
		})
	}
}
