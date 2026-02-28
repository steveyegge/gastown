// Package beads provides merge request and gate utilities.
package beads

import (
	"strings"
)

// FindMRForBranch searches for an open merge-request bead for the given branch.
// Returns the MR bead if found, nil if not found.
// This enables idempotent `gt done` - if an MR already exists, we skip creation.
func (b *Beads) FindMRForBranch(branch string) (*Issue, error) {
	return b.findMRForBranch(branch, true)
}

// FindMRForBranchAny searches for a merge-request bead for the given branch
// across all statuses (open and closed). Used by recovery checks to determine
// if work was ever submitted to the merge queue. See #1035.
func (b *Beads) FindMRForBranchAny(branch string) (*Issue, error) {
	return b.findMRForBranch(branch, false)
}

// findMRForBranch searches both the issues table (Dolt) and wisps table
// (SQLite) for a merge-request bead matching the given branch.
// Uses status=all which covers both tables with full descriptions.
// When skipClosed is true, closed beads are excluded (for open-MR checks).
func (b *Beads) findMRForBranch(branch string, skipClosed bool) (*Issue, error) {
	branchPrefix := "branch: " + branch + "\n"

	issues, err := b.List(ListOptions{
		Status: "all",
		Label:  "gt:merge-request",
	})
	if err != nil {
		return nil, err
	}
	for _, issue := range issues {
		if skipClosed && issue.Status == "closed" {
			continue
		}
		if strings.HasPrefix(issue.Description, branchPrefix) {
			return issue, nil
		}
	}

	return nil, nil
}
