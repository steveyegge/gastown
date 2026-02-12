package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// StaleAgentBeadsCheck detects agent beads that exist in the database but have
// no corresponding agent on disk. This catches beads inherited from upstream or
// left over after crew members are removed.
//
// Checks crew worker beads and polecat agent beads. Polecats have persistent
// identity (agent beads survive nuke cycles), so stale detection applies to them too.
// The fix closes stale beads so they no longer pollute bd ready output.
type StaleAgentBeadsCheck struct {
	FixableCheck
}

// NewStaleAgentBeadsCheck creates a new stale agent beads check.
func NewStaleAgentBeadsCheck() *StaleAgentBeadsCheck {
	return &StaleAgentBeadsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-agent-beads",
				CheckDescription: "Detect agent beads for removed workers (crew and polecats)",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run checks for agent beads that have no matching agent on disk.
func (c *StaleAgentBeadsCheck) Run(ctx *CheckContext) *CheckResult {
	// Load routes to get prefixes
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not load routes.jsonl",
		}
	}

	// Build prefix -> rigInfo map from routes
	prefixToRig := make(map[string]rigInfo)
	for _, r := range routes {
		parts := strings.Split(r.Path, "/")
		if len(parts) >= 1 && parts[0] != "." {
			rigName := parts[0]
			prefix := strings.TrimSuffix(r.Prefix, "-")
			prefixToRig[prefix] = rigInfo{
				name:      rigName,
				beadsPath: r.Path,
			}
		}
	}

	if len(prefixToRig) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs to check",
		}
	}

	var stale []string

	for prefix, info := range prefixToRig {
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		rigName := info.name

		// Get actual crew workers on disk
		crewDiskWorkers := listCrewWorkers(ctx.TownRoot, rigName)
		crewDiskSet := make(map[string]bool, len(crewDiskWorkers))
		for _, w := range crewDiskWorkers {
			crewDiskSet[w] = true
		}

		// Get actual polecats on disk
		polecatDiskWorkers := listPolecats(ctx.TownRoot, rigName)
		polecatDiskSet := make(map[string]bool, len(polecatDiskWorkers))
		for _, w := range polecatDiskWorkers {
			polecatDiskSet[w] = true
		}

		// Agent bead ID patterns:
		// Crew:    prefix-rig-crew-name
		// Polecat: prefix-rig-polecat-name
		crewPrefix := fmt.Sprintf("%s-%s-crew-", prefix, rigName)
		polecatPrefix := fmt.Sprintf("%s-%s-polecat-", prefix, rigName)
		allBeads, err := bd.List(beads.ListOptions{
			Status:   "open",
			Priority: -1,
			Label:    "gt:agent",
		})
		if err != nil {
			continue
		}

		for _, issue := range allBeads {
			switch {
			case strings.HasPrefix(issue.ID, crewPrefix):
				workerName := strings.TrimPrefix(issue.ID, crewPrefix)
				if workerName != "" && !crewDiskSet[workerName] {
					stale = append(stale, issue.ID)
				}
			case strings.HasPrefix(issue.ID, polecatPrefix):
				workerName := strings.TrimPrefix(issue.ID, polecatPrefix)
				if workerName != "" && !polecatDiskSet[workerName] {
					stale = append(stale, issue.ID)
				}
			}
		}
	}

	if len(stale) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale agent beads found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d stale agent bead(s) for removed workers", len(stale)),
		Details: stale,
		FixHint: "Run 'gt doctor --fix' to close stale agent beads",
	}
}

// Fix closes stale agent beads for crew members that no longer exist on disk.
func (c *StaleAgentBeadsCheck) Fix(ctx *CheckContext) error {
	// Re-run detection to get current stale list
	result := c.Run(ctx)
	if result.Status == StatusOK {
		return nil
	}

	// Load routes to get beads paths
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		return fmt.Errorf("loading routes.jsonl: %w", err)
	}

	// Build prefix -> beads path map
	prefixToPath := make(map[string]string)
	for _, r := range routes {
		parts := strings.Split(r.Path, "/")
		if len(parts) >= 1 && parts[0] != "." {
			prefix := strings.TrimSuffix(r.Prefix, "-")
			prefixToPath[prefix] = filepath.Join(ctx.TownRoot, r.Path)
		}
	}

	// Close each stale bead
	closedStatus := "closed"
	for _, beadID := range result.Details {
		// Determine which rig's beads client to use based on bead ID prefix
		var bd *beads.Beads
		for prefix, path := range prefixToPath {
			if strings.HasPrefix(beadID, prefix+"-") {
				bd = beads.New(path)
				break
			}
		}
		if bd == nil {
			continue
		}

		if err := bd.Update(beadID, beads.UpdateOptions{
			Status: &closedStatus,
		}); err != nil {
			return fmt.Errorf("closing stale bead %s: %w", beadID, err)
		}
	}

	return nil
}
