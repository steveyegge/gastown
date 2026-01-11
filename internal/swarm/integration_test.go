package swarm

import (
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
