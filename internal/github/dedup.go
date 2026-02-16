package github

import (
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// IsDuplicate checks if a bead already exists for this check failure.
// It searches open beads with the ci-failure label for a matching title.
func IsDuplicate(bd *beads.Beads, pr PR, check CheckRun) (bool, error) {
	expectedTitle := BeadTitle(pr, check)

	issues, err := bd.List(beads.ListOptions{
		Status: "open",
		Label:  "ci-failure",
	})
	if err != nil {
		return false, err
	}

	for _, issue := range issues {
		if strings.TrimSpace(issue.Title) == expectedTitle {
			return true, nil
		}
	}
	return false, nil
}
