package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// WispGCCheck detects and cleans orphaned wisps that are older than a threshold.
// Wisps are ephemeral issues (Wisp: true flag) used for patrol cycles and
// operational workflows that shouldn't accumulate.
type WispGCCheck struct {
	FixableCheck
	threshold     time.Duration
	abandonedRigs map[string]int // rig -> count of abandoned wisps
}

// NewWispGCCheck creates a new wisp GC check with 1 hour threshold.
func NewWispGCCheck() *WispGCCheck {
	return &WispGCCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "wisp-gc",
				CheckDescription: "Detect and clean orphaned wisps (>1h old)",
				CheckCategory:    CategoryCleanup,
			},
		},
		threshold:     1 * time.Hour,
		abandonedRigs: make(map[string]int),
	}
}

// Run checks for abandoned wisps in each rig.
func (c *WispGCCheck) Run(ctx *CheckContext) *CheckResult {
	c.abandonedRigs = make(map[string]int)

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
	totalAbandoned := 0

	for _, rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		count := c.countAbandonedWisps(rigPath)
		if count > 0 {
			c.abandonedRigs[rigName] = count
			totalAbandoned += count
			details = append(details, fmt.Sprintf("%s: %d abandoned wisp(s)", rigName, count))
		}
	}

	if totalAbandoned > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d abandoned wisp(s) found (>1h old)", totalAbandoned),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to garbage collect orphaned wisps",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No abandoned wisps found",
	}
}

// countAbandonedWisps counts wisps older than the threshold in a rig.
func (c *WispGCCheck) countAbandonedWisps(rigPath string) int {
	// Check the beads database for wisps (follows redirect if present)
	beadsDir := beads.ResolveBeadsDir(rigPath)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	file, err := os.Open(issuesPath)
	if err != nil {
		return 0 // No issues file
	}
	defer file.Close()

	cutoff := time.Now().Add(-c.threshold)
	count := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var issue struct {
			ID        string    `json:"id"`
			Status    string    `json:"status"`
			Wisp      bool      `json:"wisp"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}

		// Count wisps that are not closed and older than threshold
		if issue.Wisp && issue.Status != "closed" && !issue.UpdatedAt.IsZero() && issue.UpdatedAt.Before(cutoff) {
			count++
		}
	}

	return count
}

// Fix runs bd mol wisp gc in each rig with abandoned wisps.
func (c *WispGCCheck) Fix(ctx *CheckContext) error {
	var lastErr error

	for rigName := range c.abandonedRigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)

		// Run bd --no-daemon mol wisp gc
		cmd := exec.Command("bd", "--no-daemon", "mol", "wisp", "gc")
		cmd.Dir = rigPath
		if output, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("%s: %v (%s)", rigName, err, string(output))
		}
	}

	return lastErr
}
