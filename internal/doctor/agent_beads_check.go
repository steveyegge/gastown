package doctor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	var missingLabel []string
	var checked int

	// Build combined sets of known agent beads from both issues and wisps tables.
	// Agent beads are ephemeral (stored in wisps), but we also check issues for
	// backward compatibility. The wisps list doesn't include type/labels, so we
	// track wisp IDs separately for existence checks.
	allAgentBeads := make(map[string]*beads.Issue) // from issues table (has labels)
	allWispIDs := make(map[string]bool)            // from wisps table (ID only)

	// Load global agents from town beads
	townBeadsPath := beads.GetTownBeadsPath(ctx.TownRoot)
	townBd := beads.New(townBeadsPath)
	if townAgents, err := townBd.ListAgentBeads(); err == nil {
		for id, issue := range townAgents {
			allAgentBeads[id] = issue
		}
	}
	if townWisps, _ := townBd.ListWispIDs(); townWisps != nil {
		for id := range townWisps {
			allWispIDs[id] = true
		}
	}

	// Load rig-level agents
	for _, info := range prefixToRig {
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		if rigAgents, err := bd.ListAgentBeads(); err == nil {
			for id, issue := range rigAgents {
				allAgentBeads[id] = issue
			}
		}
		if rigWisps, _ := bd.ListWispIDs(); rigWisps != nil {
			for id := range rigWisps {
				allWispIDs[id] = true
			}
		}
	}

	// checkAgentBead verifies an agent bead exists (in issues or wisps table).
	// Label checking only applies to beads found in the issues table (wisps
	// don't expose labels in their list output).
	checkAgentBead := func(id string) {
		if issue, exists := allAgentBeads[id]; exists {
			// Found in issues table — check label
			if !beads.HasLabel(issue, "gt:agent") {
				missingLabel = append(missingLabel, id)
			}
		} else if !allWispIDs[id] {
			// Not in issues or wisps
			missing = append(missing, id)
		}
		checked++
	}

	// Check global agents (Mayor, Deacon)
	deaconID := beads.DeaconBeadIDTown()
	mayorID := beads.MayorBeadIDTown()

	checkAgentBead(deaconID)
	checkAgentBead(mayorID)

	if len(prefixToRig) == 0 {
		// No rigs to check, but we still checked global agents
		if len(missing) == 0 && len(missingLabel) == 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: fmt.Sprintf("All %d agent beads exist with gt:agent label", checked),
			}
		}
		details := append(missing, missingLabel...)
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d agent bead(s) missing, %d missing gt:agent label", len(missing), len(missingLabel)),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to create missing agent beads and add labels",
		}
	}

	// Check each rig for its agents
	for prefix, info := range prefixToRig {
		rigName := info.name

		// Check rig-specific agents (using canonical naming: prefix-rig-role-name)
		witnessID := beads.WitnessBeadIDWithPrefix(prefix, rigName)
		refineryID := beads.RefineryBeadIDWithPrefix(prefix, rigName)

		checkAgentBead(witnessID)
		checkAgentBead(refineryID)

		// Check crew worker agents
		crewWorkers := listCrewWorkers(ctx.TownRoot, rigName)
		for _, workerName := range crewWorkers {
			crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, workerName)
			checkAgentBead(crewID)
		}
	}

	if len(missing) == 0 && len(missingLabel) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d agent beads exist with gt:agent label", checked),
		}
	}

	if len(missing) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d agent bead(s) missing", len(missing)),
			Details: missing,
			FixHint: "Run 'gt doctor --fix' to create missing agent beads",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d agent bead(s) missing gt:agent label", len(missingLabel)),
		Details: missingLabel,
		FixHint: "Run 'gt doctor --fix' to add missing labels",
	}
}

// Fix creates missing agent beads and adds gt:agent labels to beads missing them.
func (c *AgentBeadsCheck) Fix(ctx *CheckContext) error {
	// Pre-load all known agent bead IDs (from both issues and wisps tables)
	// so we can check existence without per-bead Show() calls that miss ephemeral wisps.
	allAgentBeads := make(map[string]*beads.Issue) // from issues table
	allWispIDs := make(map[string]bool)            // from wisps table

	// Collect errors instead of failing on first — one broken rig shouldn't
	// block fixes for all other rigs.
	var errs []error

	// Fix global agents (Mayor, Deacon) in town beads
	townBeadsPath := beads.GetTownBeadsPath(ctx.TownRoot)
	townBd := beads.New(townBeadsPath)

	// Load existing town agent beads
	if townAgents, err := townBd.ListAgentBeads(); err == nil {
		for id, issue := range townAgents {
			allAgentBeads[id] = issue
		}
	}
	if townWisps, _ := townBd.ListWispIDs(); townWisps != nil {
		for id := range townWisps {
			allWispIDs[id] = true
		}
	}

	// fixAgentBead creates the bead if missing (not in issues or wisps).
	// Uses CreateAgentBead which tries --ephemeral first and falls back to
	// non-ephemeral if the subprocess crashes (GH#1769: Dolt nil pointer
	// dereference when wisps table doesn't exist on fresh rigs).
	// workDir is the rig directory for direct SQL fallback when bd update
	// fails silently (e.g., legacy prefixes that can't be routed — GH#2127).
	fixAgentBead := func(bd *beads.Beads, workDir, id, desc string, fields *beads.AgentFields) error {
		if issue, exists := allAgentBeads[id]; exists {
			// In issues table — ensure it has the gt:agent label.
			if !beads.HasLabel(issue, "gt:agent") {
				// Try bd update first (works for well-routed beads).
				err := bd.Update(id, beads.UpdateOptions{AddLabels: []string{"gt:agent"}})
				if err != nil {
					// bd update failed explicitly — fall back to direct SQL.
					sqlErr := addLabelSQL(workDir, id, "gt:agent")
					if sqlErr != nil {
						return fmt.Errorf("adding gt:agent label to %s: bd update: %w; SQL fallback: %v", id, err, sqlErr)
					}
				}
				// Verify the label was actually added — bd update can exit 0
				// without modifying beads with unroutable legacy prefixes (GH#2127).
				if err == nil && !verifyLabelAdded(workDir, id, "gt:agent") {
					sqlErr := addLabelSQL(workDir, id, "gt:agent")
					if sqlErr != nil {
						return fmt.Errorf("adding gt:agent label to %s: bd update was no-op, SQL fallback: %w", id, sqlErr)
					}
				}
			}
			return nil
		}
		if allWispIDs[id] {
			// Already exists as ephemeral wisp — nothing to do
			return nil
		}
		// Bead missing — create it (CreateAgentBead handles ephemeral fallback)
		if _, err := bd.CreateAgentBead(id, desc, fields); err != nil {
			return fmt.Errorf("creating %s: %w", id, err)
		}
		return nil
	}

	deaconID := beads.DeaconBeadIDTown()
	if err := fixAgentBead(townBd, townBeadsPath, deaconID,
		"Deacon (daemon beacon) - receives mechanical heartbeats, runs town plugins and monitoring.",
		&beads.AgentFields{RoleType: "deacon", AgentState: "idle"},
	); err != nil {
		errs = append(errs, err)
	}

	mayorID := beads.MayorBeadIDTown()
	if err := fixAgentBead(townBd, townBeadsPath, mayorID,
		"Mayor - global coordinator, handles cross-rig communication and escalations.",
		&beads.AgentFields{RoleType: "mayor", AgentState: "idle"},
	); err != nil {
		errs = append(errs, err)
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
				beadsPath: r.Path,
			}
		}
	}

	if len(prefixToRig) == 0 {
		return errors.Join(errs...)
	}

	// Load existing rig-level agent beads and wisp IDs before fixing
	for _, info := range prefixToRig {
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		if rigAgents, err := bd.ListAgentBeads(); err == nil {
			for id, issue := range rigAgents {
				allAgentBeads[id] = issue
			}
		}
		if rigWisps, _ := bd.ListWispIDs(); rigWisps != nil {
			for id := range rigWisps {
				allWispIDs[id] = true
			}
		}
	}

	// Fix agents for each rig
	for prefix, info := range prefixToRig {
		rigBeadsPath := filepath.Join(ctx.TownRoot, info.beadsPath)
		bd := beads.New(rigBeadsPath)
		rigName := info.name

		witnessID := beads.WitnessBeadIDWithPrefix(prefix, rigName)
		if err := fixAgentBead(bd, rigBeadsPath, witnessID,
			fmt.Sprintf("Witness for %s - monitors polecat health and progress.", rigName),
			&beads.AgentFields{RoleType: "witness", Rig: rigName, AgentState: "idle"},
		); err != nil {
			errs = append(errs, err)
		}

		refineryID := beads.RefineryBeadIDWithPrefix(prefix, rigName)
		if err := fixAgentBead(bd, rigBeadsPath, refineryID,
			fmt.Sprintf("Refinery for %s - processes merge queue.", rigName),
			&beads.AgentFields{RoleType: "refinery", Rig: rigName, AgentState: "idle"},
		); err != nil {
			errs = append(errs, err)
		}

		crewWorkers := listCrewWorkers(ctx.TownRoot, rigName)
		for _, workerName := range crewWorkers {
			crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, workerName)
			if err := fixAgentBead(bd, rigBeadsPath, crewID,
				fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", workerName, rigName),
				&beads.AgentFields{RoleType: "crew", Rig: rigName, AgentState: "idle"},
			); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
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

// addLabelSQL adds a label to a bead via direct SQL INSERT.
// This bypasses bd's prefix routing, which silently fails for beads with
// legacy/unroutable prefixes (GH#2127).
func addLabelSQL(workDir, beadID, label string) error {
	escapedID := strings.ReplaceAll(beadID, "'", "''")
	escapedLabel := strings.ReplaceAll(label, "'", "''")
	query := fmt.Sprintf("INSERT IGNORE INTO labels (issue_id, label) VALUES ('%s', '%s')", escapedID, escapedLabel)
	return execBdSQLWrite(workDir, query)
}

// verifyLabelAdded checks whether a label exists on a bead by querying labels table.
// Returns false if the label is not found or the query fails.
func verifyLabelAdded(workDir, beadID, label string) bool {
	escapedID := strings.ReplaceAll(beadID, "'", "''")
	escapedLabel := strings.ReplaceAll(label, "'", "''")
	query := fmt.Sprintf("SELECT 1 FROM labels WHERE issue_id = '%s' AND label = '%s' LIMIT 1", escapedID, escapedLabel)
	cmd := exec.Command("bd", "sql", query) //nolint:gosec // G204: query uses escaped internal values
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// bd sql returns header + data rows; if we got more than just a header, the label exists
	return strings.Contains(string(output), "1")
}

// listPolecats returns the names of polecat directories in a rig.
func listPolecats(townRoot, rigName string) []string {
	polecatDir := filepath.Join(townRoot, rigName, "polecats")
	entries, err := os.ReadDir(polecatDir)
	if err != nil {
		return nil // No polecats directory or can't read it
	}

	var polecats []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			polecats = append(polecats, entry.Name())
		}
	}
	return polecats
}
