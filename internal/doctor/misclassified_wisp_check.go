package doctor

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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
	var totalProbeErrors int

	for _, rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		found, probeErrors := c.findMisclassifiedWisps(rigPath, rigName)
		totalProbeErrors += probeErrors
		if len(found) > 0 {
			c.misclassified = append(c.misclassified, found...)
			c.misclassifiedRigs[rigName] = len(found)
			details = append(details, fmt.Sprintf("%s: %d misclassified wisp(s)", rigName, len(found)))
		}
	}

	// Also check town-level beads
	townFound, townProbeErrors := c.findMisclassifiedWisps(ctx.TownRoot, "town")
	totalProbeErrors += townProbeErrors
	if len(townFound) > 0 {
		c.misclassified = append(c.misclassified, townFound...)
		c.misclassifiedRigs["town"] = len(townFound)
		details = append(details, fmt.Sprintf("town: %d misclassified wisp(s)", len(townFound)))
	}

	if totalProbeErrors > 0 {
		details = append(details, fmt.Sprintf("%d DB probe(s) failed — some candidates may have been skipped", totalProbeErrors))
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

	if totalProbeErrors > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No misclassified wisps found (some DB probes failed)",
			Details: details,
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No misclassified wisps found",
	}
}

// misclassifiedWispsQuery selects non-ephemeral, non-closed issues for misclassification detection.
const misclassifiedWispsQuery = `SELECT id, title, status, issue_type, labels, ephemeral FROM issues WHERE status != 'closed' AND (ephemeral IS NULL OR ephemeral = 0)`

// findMisclassifiedWisps queries Dolt for issues that should be wisps but aren't.
// Returns the found misclassified wisps and the number of DB probe errors encountered.
func (c *CheckMisclassifiedWisps) findMisclassifiedWisps(path string, rigName string) ([]misclassifiedWisp, int) {
	cmd := exec.Command("bd", "sql", "--csv", misclassifiedWispsQuery) //nolint:gosec // G204: query is a constant
	cmd.Dir = path
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Dolt query failed — count as a probe error but don't block the check.
		return nil, 1
	}

	r := csv.NewReader(strings.NewReader(string(output)))
	records, err := r.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, 0 // No results or parse error
	}

	var found []misclassifiedWisp
	for _, rec := range records[1:] { // Skip CSV header
		if len(rec) < 6 {
			continue
		}
		id := strings.TrimSpace(rec[0])
		title := strings.TrimSpace(rec[1])
		issueType := strings.TrimSpace(rec[3])
		labelsStr := strings.TrimSpace(rec[4])

		// Parse labels (CSV field may be comma-separated or JSON array)
		var labels []string
		if labelsStr != "" {
			if strings.HasPrefix(labelsStr, "[") {
				_ = json.Unmarshal([]byte(labelsStr), &labels)
			} else {
				labels = strings.Split(labelsStr, ",")
			}
		}

		if reason := c.shouldBeWisp(id, title, issueType, labels); reason != "" {
			found = append(found, misclassifiedWisp{
				rigName: rigName,
				id:      id,
				title:   title,
				reason:  reason,
			})
		}
	}

	return found, 0
}

// shouldBeWisp checks if an issue has characteristics indicating it should be a wisp.
// Returns the reason string if it should be a wisp, empty string otherwise.
func (c *CheckMisclassifiedWisps) shouldBeWisp(id, title, issueType string, labels []string) string {
	// Check for merge-request type - these should always be wisps
	if issueType == "merge-request" {
		return "merge-request type should be ephemeral"
	}

	// Check for agent type - agent operational state is ephemeral (gt-bewatn.9)
	if issueType == "agent" {
		return "agent type should be ephemeral"
	}

	// Check for patrol-related labels
	for _, label := range labels {
		if strings.Contains(label, "patrol") {
			return "patrol label indicates ephemeral workflow"
		}
		if label == "gt:mail" || label == "gt:handoff" {
			return "mail/handoff label indicates ephemeral message"
		}
		if label == "gt:agent" {
			return "agent label indicates ephemeral operational state"
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

// Fix marks misclassified issues as ephemeral wisps via bd update --ephemeral.
// This preserves the issue for audit rather than permanently closing it.
func (c *CheckMisclassifiedWisps) Fix(ctx *CheckContext) error {
	if len(c.misclassified) == 0 {
		return nil
	}

	var lastErr error

	for _, wisp := range c.misclassified {
		// Determine working directory: town-level or rig-level
		var workDir string
		if wisp.rigName == "town" {
			workDir = ctx.TownRoot
		} else {
			workDir = filepath.Join(ctx.TownRoot, wisp.rigName)
		}

		// Mark as ephemeral (wisp) rather than closing - preserves the issue for audit
		// bd update --ephemeral is supported as of bd 0.52.0+
		cmd := exec.Command("bd", "update", wisp.id, "--ephemeral")
		cmd.Dir = workDir
		if output, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s/%s: %v (%s)", wisp.rigName, wisp.id, err, string(output))
		}
	}

	return lastErr
}
