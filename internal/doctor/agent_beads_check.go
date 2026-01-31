package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// AgentBeadsCheck verifies that agent beads exist for all agents.
// This includes:
// - Global agents (deacon, mayor) - stored in town beads with hq- prefix
// - Per-rig agents (witness, refinery) - stored in each rig's beads
// - Crew workers - stored in each rig's beads
//
// Agent beads are created by gt rig add (see gt-h3hak, gt-pinkq) and gt crew add.
// Each rig uses its configured prefix (e.g., "gt-" for gastown, "bd-" for beads).
type AgentBeadsCheck struct {
	FixableCheck
}

// NewAgentBeadsCheck creates a new agent beads check.
func NewAgentBeadsCheck() *AgentBeadsCheck {
	return &AgentBeadsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "agent-beads-exist",
				CheckDescription: "Verify agent beads exist for all agents",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// rigInfo holds the rig name and its beads path from routes.
type rigInfo struct {
	name      string // rig name (first component of path)
	beadsPath string // full path to beads directory relative to town root
}

// Run checks if agent beads exist for all expected agents.
func (c *AgentBeadsCheck) Run(ctx *CheckContext) *CheckResult {
	// Load routes to get prefixes (routes.jsonl is source of truth for prefixes)
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
	// Routes have format: prefix "gt-" -> path "gastown/mayor/rig" or "my-saas"
	prefixToRig := make(map[string]rigInfo) // prefix (without hyphen) -> rigInfo
	for _, r := range routes {
		// Extract rig name from path (first component)
		parts := strings.Split(r.Path, "/")
		if len(parts) >= 1 && parts[0] != "." {
			rigName := parts[0]
			prefix := strings.TrimSuffix(r.Prefix, "-")
			prefixToRig[prefix] = rigInfo{
				name:      rigName,
				beadsPath: r.Path, // Use the full route path
			}
		}
	}

	var missing []string
	var missingLabels []string // Beads that exist but lack gt:agent label
	var checked int

	// Check global agents (Mayor, Deacon) in town beads
	// These use hq- prefix and are stored in ~/gt/.beads/
	townBeadsPath := beads.GetTownBeadsPath(ctx.TownRoot)
	townBd := beads.New(townBeadsPath)

	deaconID := beads.DeaconBeadIDTown()
	mayorID := beads.MayorBeadIDTown()

	if issue, err := townBd.Show(deaconID); err != nil {
		missing = append(missing, deaconID)
	} else if !beads.HasLabel(issue, "gt:agent") {
		missingLabels = append(missingLabels, deaconID)
	}
	checked++

	if issue, err := townBd.Show(mayorID); err != nil {
		missing = append(missing, mayorID)
	} else if !beads.HasLabel(issue, "gt:agent") {
		missingLabels = append(missingLabels, mayorID)
	}
	checked++

	if len(prefixToRig) == 0 {
		// No rigs to check, but we still checked global agents
		if len(missing) == 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: fmt.Sprintf("All %d agent beads exist", checked),
			}
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d agent bead(s) missing", len(missing)),
			Details: missing,
			FixHint: "Run 'gt doctor --fix' to create missing agent beads",
		}
	}

	// Check each rig for its agents
	for prefix, info := range prefixToRig {
		// Get beads client for this rig using the route path directly
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		rigName := info.name

		// Check rig-specific agents (using canonical naming: prefix-rig-role-name)
		witnessID := beads.WitnessBeadIDWithPrefix(prefix, rigName)
		refineryID := beads.RefineryBeadIDWithPrefix(prefix, rigName)

		if issue, err := bd.Show(witnessID); err != nil {
			missing = append(missing, witnessID)
		} else if !beads.HasLabel(issue, "gt:agent") {
			missingLabels = append(missingLabels, witnessID)
		}
		checked++

		if issue, err := bd.Show(refineryID); err != nil {
			missing = append(missing, refineryID)
		} else if !beads.HasLabel(issue, "gt:agent") {
			missingLabels = append(missingLabels, refineryID)
		}
		checked++

		// Check crew worker agents
		crewWorkers := listCrewWorkers(ctx.TownRoot, rigName)
		for _, workerName := range crewWorkers {
			crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, workerName)
			if issue, err := bd.Show(crewID); err != nil {
				missing = append(missing, crewID)
			} else if !beads.HasLabel(issue, "gt:agent") {
				missingLabels = append(missingLabels, crewID)
			}
			checked++
		}
	}

	if len(missing) == 0 && len(missingLabels) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d agent beads exist with gt:agent label", checked),
		}
	}

	// Build combined details
	var details []string
	details = append(details, missing...)
	for _, id := range missingLabels {
		details = append(details, id+" (missing gt:agent label)")
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("%d agent bead(s) missing, %d missing gt:agent label", len(missing), len(missingLabels)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to create missing agent beads and add missing labels",
	}
}

// Fix creates missing agent beads and adds missing gt:agent labels.
func (c *AgentBeadsCheck) Fix(ctx *CheckContext) error {
	// Create global agents (Mayor, Deacon) in town beads
	// These use hq- prefix and are stored in ~/gt/.beads/
	townBeadsPath := beads.GetTownBeadsPath(ctx.TownRoot)
	townBd := beads.New(townBeadsPath)

	deaconID := beads.DeaconBeadIDTown()
	if issue, err := townBd.Show(deaconID); err != nil {
		fields := &beads.AgentFields{
			RoleType:   "deacon",
			Rig:        "",
			AgentState: "idle",
		}
		desc := "Deacon (daemon beacon) - receives mechanical heartbeats, runs town plugins and monitoring."
		if _, err := townBd.CreateAgentBead(deaconID, desc, fields); err != nil {
			return fmt.Errorf("creating %s: %w", deaconID, err)
		}
	} else if !beads.HasLabel(issue, "gt:agent") {
		if err := townBd.AddLabel(deaconID, "gt:agent"); err != nil {
			return fmt.Errorf("adding gt:agent label to %s: %w", deaconID, err)
		}
	}

	mayorID := beads.MayorBeadIDTown()
	if issue, err := townBd.Show(mayorID); err != nil {
		fields := &beads.AgentFields{
			RoleType:   "mayor",
			Rig:        "",
			AgentState: "idle",
		}
		desc := "Mayor - global coordinator, handles cross-rig communication and escalations."
		if _, err := townBd.CreateAgentBead(mayorID, desc, fields); err != nil {
			return fmt.Errorf("creating %s: %w", mayorID, err)
		}
	} else if !beads.HasLabel(issue, "gt:agent") {
		if err := townBd.AddLabel(mayorID, "gt:agent"); err != nil {
			return fmt.Errorf("adding gt:agent label to %s: %w", mayorID, err)
		}
	}

	// Load routes to get prefixes for rig-level agents
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		return fmt.Errorf("loading routes.jsonl: %w", err)
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
				beadsPath: r.Path, // Use the full route path
			}
		}
	}

	if len(prefixToRig) == 0 {
		return nil // No rigs to process
	}

	// Create missing agents for each rig
	for prefix, info := range prefixToRig {
		// Use the route path directly instead of hardcoding /mayor/rig
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		rigName := info.name

		// Create rig-specific agents if missing or add label if missing (using canonical naming: prefix-rig-role-name)
		witnessID := beads.WitnessBeadIDWithPrefix(prefix, rigName)
		if issue, err := bd.Show(witnessID); err != nil {
			fields := &beads.AgentFields{
				RoleType:   "witness",
				Rig:        rigName,
				AgentState: "idle",
			}
			desc := fmt.Sprintf("Witness for %s - monitors polecat health and progress.", rigName)
			if _, err := bd.CreateAgentBead(witnessID, desc, fields); err != nil {
				return fmt.Errorf("creating %s: %w", witnessID, err)
			}
		} else if !beads.HasLabel(issue, "gt:agent") {
			if err := bd.AddLabel(witnessID, "gt:agent"); err != nil {
				return fmt.Errorf("adding gt:agent label to %s: %w", witnessID, err)
			}
		}

		refineryID := beads.RefineryBeadIDWithPrefix(prefix, rigName)
		if issue, err := bd.Show(refineryID); err != nil {
			fields := &beads.AgentFields{
				RoleType:   "refinery",
				Rig:        rigName,
				AgentState: "idle",
			}
			desc := fmt.Sprintf("Refinery for %s - processes merge queue.", rigName)
			if _, err := bd.CreateAgentBead(refineryID, desc, fields); err != nil {
				return fmt.Errorf("creating %s: %w", refineryID, err)
			}
		} else if !beads.HasLabel(issue, "gt:agent") {
			if err := bd.AddLabel(refineryID, "gt:agent"); err != nil {
				return fmt.Errorf("adding gt:agent label to %s: %w", refineryID, err)
			}
		}

		// Create crew worker agents if missing or add label if missing
		crewWorkers := listCrewWorkers(ctx.TownRoot, rigName)
		for _, workerName := range crewWorkers {
			crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, workerName)
			if issue, err := bd.Show(crewID); err != nil {
				fields := &beads.AgentFields{
					RoleType:   "crew",
					Rig:        rigName,
					AgentState: "idle",
				}
				desc := fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", workerName, rigName)
				if _, err := bd.CreateAgentBead(crewID, desc, fields); err != nil {
					return fmt.Errorf("creating %s: %w", crewID, err)
				}
			} else if !beads.HasLabel(issue, "gt:agent") {
				if err := bd.AddLabel(crewID, "gt:agent"); err != nil {
					return fmt.Errorf("adding gt:agent label to %s: %w", crewID, err)
				}
			}
		}
	}

	return nil
}

// listCrewWorkers returns the names of all crew workers in a rig.
func listCrewWorkers(townRoot, rigName string) []string {
	crewDir := filepath.Join(townRoot, rigName, "crew")
	entries, err := os.ReadDir(crewDir)
	if err != nil {
		return nil // No crew directory or can't read it
	}

	var workers []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			workers = append(workers, entry.Name())
		}
	}
	return workers
}
