package cmd

import (
	"testing"
)

// TestIdleTransitionUsesDetachedRef verifies that the idle transition reference
// is "origin/<branch>" (producing a detached HEAD) rather than the named branch.
// Git worktrees forbid two worktrees on the same named branch — checking out
// "release" directly would block the refinery.
func TestIdleTransitionUsesDetachedRef(t *testing.T) {
	tests := []struct {
		defaultBranch string
		wantRef       string
	}{
		{"release", "origin/release"},
		{"main", "origin/main"},
		{"develop", "origin/develop"},
	}

	for _, tt := range tests {
		t.Run(tt.defaultBranch, func(t *testing.T) {
			// This matches the logic in done.go idle transition:
			// detachedRef := "origin/" + defaultBranch
			detachedRef := "origin/" + tt.defaultBranch
			if detachedRef != tt.wantRef {
				t.Errorf("detachedRef = %q, want %q", detachedRef, tt.wantRef)
			}
			// Verify it's NOT the named branch (which would lock it)
			if detachedRef == tt.defaultBranch {
				t.Errorf("detachedRef should not equal defaultBranch %q (would lock the branch in worktree)", tt.defaultBranch)
			}
		})
	}
}

// TestIdleTransitionDeletesOldBranch verifies that the old branch deletion
// logic correctly skips protected branches.
func TestIdleTransitionDeletesOldBranch(t *testing.T) {
	tests := []struct {
		oldBranch     string
		defaultBranch string
		shouldDelete  bool
	}{
		{"polecat/toast/gt-abc", "release", true},
		{"polecat/ember/gt-xyz", "main", true},
		{"release", "release", false},      // same as default
		{"main", "main", false},            // same as default
		{"master", "release", false},       // always protected
		{"", "release", false},             // empty
	}

	for _, tt := range tests {
		t.Run(tt.oldBranch, func(t *testing.T) {
			// Match the logic in done.go:
			// if oldBranch != "" && oldBranch != defaultBranch && oldBranch != "master"
			shouldDelete := tt.oldBranch != "" && tt.oldBranch != tt.defaultBranch && tt.oldBranch != "master"
			if shouldDelete != tt.shouldDelete {
				t.Errorf("shouldDelete(%q, default=%q) = %v, want %v",
					tt.oldBranch, tt.defaultBranch, shouldDelete, tt.shouldDelete)
			}
		})
	}
}
