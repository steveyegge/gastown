package cmd

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// verifyMergeRequestPersisted loads an MR by ID and validates it is a real
// merge-request record for the expected branch.
func verifyMergeRequestPersisted(bd *beads.Beads, mrID, expectedBranch string) (*beads.Issue, error) {
	if strings.TrimSpace(mrID) == "" {
		return nil, fmt.Errorf("empty MR ID")
	}

	issue, err := bd.Show(mrID)
	if err != nil {
		return nil, fmt.Errorf("read-back failed: %w", err)
	}
	if err := verifyMergeRequestRecord(issue, expectedBranch); err != nil {
		return nil, err
	}

	return issue, nil
}

// verifyMergeRequestRecord validates the core invariants for a merge-request bead.
func verifyMergeRequestRecord(issue *beads.Issue, expectedBranch string) error {
	if issue == nil {
		return fmt.Errorf("MR read-back returned nil issue")
	}
	if !beads.HasLabel(issue, "gt:merge-request") {
		return fmt.Errorf("missing gt:merge-request label")
	}

	fields := beads.ParseMRFields(issue)
	if fields == nil || strings.TrimSpace(fields.Branch) == "" {
		return fmt.Errorf("missing branch metadata in MR description")
	}

	if expectedBranch != "" && fields.Branch != expectedBranch {
		return fmt.Errorf("branch mismatch: expected %q, got %q", expectedBranch, fields.Branch)
	}

	return nil
}
