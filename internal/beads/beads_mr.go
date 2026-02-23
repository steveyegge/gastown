// Package beads provides merge request and gate utilities.
package beads

import (
	"strings"
)

// FindMRForBranch searches for an existing merge-request bead for the given branch.
// Returns the MR bead if found, nil if not found.
// This enables idempotent `gt done` - if an MR already exists, we skip creation.
func (b *Beads) FindMRForBranch(branch string) (*Issue, error) {
	// List all merge-request beads (open status only - closed MRs are already processed)
	issues, err := b.List(ListOptions{
		Status: "open",
		Label:  "gt:merge-request",
	})
	if err != nil {
		return nil, err
	}

	// Search for one matching this branch
	// MR description format: "branch: <branch>\ntarget: ..."
	branchPrefix := "branch: " + branch + "\n"
	for _, issue := range issues {
		if strings.HasPrefix(issue.Description, branchPrefix) {
			return issue, nil
		}
	}

	return nil, nil
}

// FindMRForBranchAny searches for a merge-request bead (open or closed) for the given branch.
// Unlike FindMRForBranch which only checks open MRs, this also checks closed MRs
// (already processed by the refinery). Used by check-recovery to verify work entered the
// merge pipeline. See #1035.
func (b *Beads) FindMRForBranchAny(branch string) (*Issue, error) {
	// Check open MRs first
	mr, err := b.FindMRForBranch(branch)
	if err != nil {
		return nil, err
	}
	if mr != nil {
		return mr, nil
	}

	// Check closed MRs (already processed by refinery)
	issues, err := b.List(ListOptions{
		Status: "closed",
		Label:  "gt:merge-request",
	})
	if err != nil {
		return nil, err
	}

	branchPrefix := "branch: " + branch + "\n"
	for _, issue := range issues {
		if strings.HasPrefix(issue.Description, branchPrefix) {
			return issue, nil
		}
	}

	return nil, nil
}

