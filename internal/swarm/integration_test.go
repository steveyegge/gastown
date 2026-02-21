package swarm

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestGetWorkerBranch(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	branch := m.GetWorkerBranch("sw-1", "Toast", "task-123")
	expected := "sw-1/Toast/task-123"
	if branch != expected {
		t.Errorf("branch = %q, want %q", branch, expected)
	}
}

// Note: Integration tests that require git operations and beads
// are covered by the E2E test (gt-kc7yj.4).

// TestSwarmGitErrorWithStderr tests Error() when Stderr is populated.
func TestSwarmGitErrorWithStderr(t *testing.T) {
	err := &SwarmGitError{
		Command: "merge",
		Stderr:  "CONFLICT (content): Merge conflict in main.go",
		Err:     fmt.Errorf("exit status 1"),
	}

	got := err.Error()
	want := "merge: CONFLICT (content): Merge conflict in main.go"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestSwarmGitErrorWithoutStderr tests Error() when Stderr is empty.
func TestSwarmGitErrorWithoutStderr(t *testing.T) {
	err := &SwarmGitError{
		Command: "checkout",
		Stderr:  "",
		Err:     fmt.Errorf("exit status 128"),
	}

	got := err.Error()
	want := "checkout: exit status 128"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestSwarmGitErrorImplementsError verifies SwarmGitError implements the error interface.
func TestSwarmGitErrorImplementsError(t *testing.T) {
	var err error = &SwarmGitError{
		Command: "push",
		Stderr:  "rejected",
		Err:     fmt.Errorf("exit status 1"),
	}

	if err.Error() != "push: rejected" {
		t.Errorf("Error() via interface = %q, want %q", err.Error(), "push: rejected")
	}
}
