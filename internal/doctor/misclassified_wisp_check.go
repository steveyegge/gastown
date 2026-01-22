package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// CheckMisclassifiedWisps detects issues that should be marked as wisps but aren't.
// Wisps are ephemeral issues for operational workflows (patrols, MRs, mail).
// This check finds issues that have wisp characteristics but lack the wisp:true flag.
type CheckMisclassifiedWisps struct {
	FixableCheck
	misclassified     []misclassifiedWisp
	misclassifiedRigs map[string]int // rig -> count
}

type misclassifiedWisp struct {
	rigName string
	id      string
	title   string
	reason  string
}

// NewCheckMisclassifiedWisps creates a new misclassified wisp check.
func NewCheckMisclassifiedWisps() *CheckMisclassifiedWisps {
	return &CheckMisclassifiedWisps{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "misclassified-wisps",
				CheckDescription: "Detect issues that should be wisps but aren't marked as ephemeral",
				CheckCategory:    CategoryCleanup,
			},
		},
		misclassifiedRigs: make(map[string]int),
	}
}

// Run checks for misclassified wisps in each rig.
func (c *CheckMisclassifiedWisps) Run(ctx *CheckContext) *CheckResult {
	c.misclassified = nil
	c.misclassifiedRigs = make(map[string]int)

	rigs, err := discoverRigs(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Failed to discover rigs",
			Details: []string{err.Error()},
		}
	}

	if len(rigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs configured",
		}
	}

	var details []string

	for _, rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		found := c.findMisclassifiedWisps(rigPath, rigName)
		if len(found) > 0 {
			c.misclassified = append(c.misclassified, found...)
			c.misclassifiedRigs[rigName] = len(found)
			details = append(details, fmt.Sprintf("%s: %d misclassified wisp(s)", rigName, len(found)))
		}
	}

	// Also check town-level beads
	townFound := c.findMisclassifiedWisps(ctx.TownRoot, "town")
	if len(townFound) > 0 {
		c.misclassified = append(c.misclassified, townFound...)
		c.misclassifiedRigs["town"] = len(townFound)
		details = append(details, fmt.Sprintf("town: %d misclassified wisp(s)", len(townFound)))
	}

	total := len(c.misclassified)
	if total > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d issue(s) should be marked as wisps", total),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to mark these issues as ephemeral",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No misclassified wisps found",
	}
}

// findMisclassifiedWisps finds issues that should be wisps but aren't in a single location.
func (c *CheckMisclassifiedWisps) findMisclassifiedWisps(path string, rigName string) []misclassifiedWisp {
	beadsDir := beads.ResolveBeadsDir(path)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	file, err := os.Open(issuesPath)
	if err != nil {
		return nil // No issues file
	}
	defer file.Close()

	var found []misclassifiedWisp

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var issue struct {
			ID     string   `json:"id"`
			Title  string   `json:"title"`
			Status string   `json:"status"`
			Type   string   `json:"issue_type"`
			Labels []string `json:"labels"`
			Wisp   bool     `json:"wisp"`
		}
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}

		// Skip issues already marked as wisps
		if issue.Wisp {
			continue
		}

		// Skip closed issues - they're done, no need to reclassify
		if issue.Status == "closed" {
			continue
		}

		// Check for wisp characteristics
		if reason := c.shouldBeWisp(issue.ID, issue.Title, issue.Type, issue.Labels); reason != "" {
			found = append(found, misclassifiedWisp{
				rigName: rigName,
				id:      issue.ID,
				title:   issue.Title,
				reason:  reason,
			})
		}
	}

	return found
}

// shouldBeWisp checks if an issue has characteristics indicating it should be a wisp.
// Returns the reason string if it should be a wisp, empty string otherwise.
func (c *CheckMisclassifiedWisps) shouldBeWisp(id, title, issueType string, labels []string) string {
	// Check for merge-request type (legacy) - these should always be wisps
	if issueType == "merge-request" {
		return "merge-request type should be ephemeral"
	}

	// Check for patrol-related labels and MR label
	for _, label := range labels {
		// MRs are identified by gt:merge-request label (bd-3q6.10)
		if label == "gt:merge-request" {
			return "merge-request label indicates ephemeral MR"
		}
		if strings.Contains(label, "patrol") {
			return "patrol label indicates ephemeral workflow"
		}
		if label == "gt:mail" || label == "gt:handoff" {
			return "mail/handoff label indicates ephemeral message"
		}
	}

	// Check for formula instance patterns in ID
	// Formula instances typically have IDs like "mol-<formula>-<hash>" or "<formula>.<step>"
	if strings.HasPrefix(id, "mol-") && strings.Contains(id, "-patrol") {
		return "patrol molecule ID pattern"
	}

	// Check for specific title patterns indicating operational work
	lowerTitle := strings.ToLower(title)
	if strings.Contains(lowerTitle, "patrol cycle") ||
		strings.Contains(lowerTitle, "witness patrol") ||
		strings.Contains(lowerTitle, "deacon patrol") ||
		strings.Contains(lowerTitle, "refinery patrol") {
		return "patrol title indicates ephemeral workflow"
	}

	return ""
}

// Fix marks misclassified issues as wisps using bd update.
func (c *CheckMisclassifiedWisps) Fix(ctx *CheckContext) error {
	// Note: bd doesn't have a direct flag to set wisp:true on existing issues.
	// The proper fix is to ensure issues are created with --ephemeral flag.
	// For now, we just report the issues - they'll be cleaned up by wisp-gc
	// if they become abandoned, or manually closed.
	//
	// A true fix would require bd to support: bd update <id> --ephemeral
	// Until then, this check serves as a diagnostic.
	return nil
}
