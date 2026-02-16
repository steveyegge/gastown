package github

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
)

// CheckFailure represents a failed CI check that needs a bead.
type CheckFailure struct {
	PR    PR
	Check CheckRun
	Repo  string
}

// CheckResult is the output of a single check cycle.
type CheckResult struct {
	Repo     string
	PRCount  int            // Total PRs checked
	Failures []CheckFailure // Failed checks found
	Created  []string       // Bead IDs created
	Skipped  int            // Already-existing beads (deduped)
	Errors   []error        // Non-fatal errors during processing
}

// CheckerConfig configures the check runner.
type CheckerConfig struct {
	Repo      string   // owner/repo (auto-detected if empty)
	Authors   []string // Filter by PR author (empty = all)
	DryRun    bool     // Show actions without executing
	BeadsPath string   // Path to rig's beads directory
	Actor     string   // BD_ACTOR for bead creation
	WorkDir   string   // Working directory for repo detection
}

// RunCheck performs one check cycle: poll PRs → get checks → deduplicate → create beads.
func RunCheck(cfg CheckerConfig) (*CheckResult, error) {
	repo := cfg.Repo
	if repo == "" {
		detected, err := DetectRepo(cfg.WorkDir)
		if err != nil {
			return nil, err
		}
		repo = detected
	}

	result := &CheckResult{Repo: repo}

	// List open PRs
	prs, err := ListOpenPRs(repo, cfg.Authors)
	if err != nil {
		return nil, err
	}
	result.PRCount = len(prs)

	if len(prs) == 0 {
		return result, nil
	}

	bd := beads.New(cfg.BeadsPath)

	// Check each PR for failures
	for _, pr := range prs {
		checks, err := GetCheckRuns(repo, pr.Number)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("PR #%d: %w", pr.Number, err))
			continue
		}

		for _, check := range checks {
			if !check.IsFailed() {
				continue
			}

			failure := CheckFailure{
				PR:    pr,
				Check: check,
				Repo:  repo,
			}
			result.Failures = append(result.Failures, failure)

			// Dedup check
			dup, err := IsDuplicate(bd, pr, check)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("dedup check PR #%d/%s: %w", pr.Number, check.Name, err))
				continue
			}
			if dup {
				result.Skipped++
				continue
			}

			if cfg.DryRun {
				result.Created = append(result.Created, "(dry-run)")
				continue
			}

			// Create bead
			beadID, err := createFailureBead(bd, failure, cfg.Actor)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("creating bead for PR #%d/%s: %w", pr.Number, check.Name, err))
				continue
			}
			result.Created = append(result.Created, beadID)

			// Log event for feed visibility
			_ = events.LogFeed(events.TypeGitHubCheckFailed, cfg.Actor, map[string]interface{}{
				"repo":       repo,
				"pr":         pr.Number,
				"check":      check.Name,
				"conclusion": check.Conclusion,
				"bead_id":    beadID,
			})
		}
	}

	return result, nil
}

// BeadTitle returns the standardized bead title for a CI failure.
func BeadTitle(pr PR, check CheckRun) string {
	return fmt.Sprintf("CI failure: %s on PR #%d", check.Name, pr.Number)
}

// createFailureBead creates a bead for a CI check failure.
func createFailureBead(bd *beads.Beads, f CheckFailure, actor string) (string, error) {
	description := fmt.Sprintf(
		"CI check `%s` failed on PR #%d (%s)\n\nPR: %s\nCheck: %s\nConclusion: %s",
		f.Check.Name, f.PR.Number, f.PR.Title,
		f.PR.URL, f.Check.URL, f.Check.Conclusion,
	)

	issue, err := bd.Create(beads.CreateOptions{
		Title:       BeadTitle(f.PR, f.Check),
		Type:        "task",
		Priority:    2,
		Description: description,
		Actor:       actor,
	})
	if err != nil {
		return "", err
	}

	// Add ci-failure label
	_ = bd.Update(issue.ID, beads.UpdateOptions{
		AddLabels: []string{"ci-failure"},
	})

	return issue.ID, nil
}
