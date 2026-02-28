package doctor

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// PatrolMoleculesExistCheck verifies that patrol formulas are accessible.
// Patrols use `bd mol wisp <formula-name>` to spawn workflows, so the formulas
// must exist in the formula search path (.beads/formulas/, ~/.beads/formulas/, or $GT_ROOT/.beads/formulas/).
type PatrolMoleculesExistCheck struct {
	BaseCheck
	missingFormulas map[string][]string // rig -> missing formula names
}

// NewPatrolMoleculesExistCheck creates a new patrol formulas exist check.
func NewPatrolMoleculesExistCheck() *PatrolMoleculesExistCheck {
	return &PatrolMoleculesExistCheck{
		BaseCheck: BaseCheck{
			CheckName:        "patrol-molecules-exist",
			CheckDescription: "Check if patrol formulas are accessible",
			CheckCategory:    CategoryPatrol,
		},
	}
}

// patrolFormulas are the required patrol formula names.
var patrolFormulas = []string{
	"mol-deacon-patrol",
	"mol-witness-patrol",
	"mol-refinery-patrol",
}

// Run checks if patrol formulas are accessible.
func (c *PatrolMoleculesExistCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingFormulas = make(map[string][]string)

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
		// If rigPath doesn't exist, fall back to TownRoot. This handles the case
		// where gt doctor runs from a mayor's canonical clone, where TownRoot
		// resolves to the clone itself (e.g. gastown/mayor/rig) rather than the
		// actual town root. The rig directory won't be a subdirectory of the clone,
		// but patrol formulas are town-level and accessible from TownRoot itself.
		if _, statErr := os.Stat(rigPath); os.IsNotExist(statErr) {
			rigPath = ctx.TownRoot
		}
		missing := c.checkPatrolFormulas(rigPath, ctx.TownRoot)
		if len(missing) > 0 {
			c.missingFormulas[rigName] = missing
			details = append(details, fmt.Sprintf("%s: missing %v", rigName, missing))
		}
	}

	if len(details) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d rig(s) missing patrol formulas", len(c.missingFormulas)),
			Details: details,
			FixHint: "Formulas should exist in .beads/formulas/ at town or rig level, or in ~/.beads/formulas/",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("All %d rig(s) have patrol formulas accessible", len(rigs)),
	}
}

// checkPatrolFormulas returns missing patrol formula names for a rig.
func (c *PatrolMoleculesExistCheck) checkPatrolFormulas(rigPath string, townRoot string) []string {
	// Check for formula files directly on the filesystem rather than shelling
	// out to `bd formula list`, which may not be available in all environments
	// (e.g., CI). Formulas are provisioned as .formula.toml files in .beads/formulas/.
	//
	// Search the full formula path: rig-level → town-level → user-level,
	// matching the beads SDK's formula resolution order.
	homeDir, _ := os.UserHomeDir()
	searchDirs := []string{
		filepath.Join(rigPath, ".beads", "formulas"),
		filepath.Join(townRoot, ".beads", "formulas"),
	}
	if homeDir != "" {
		searchDirs = append(searchDirs, filepath.Join(homeDir, ".beads", "formulas"))
	}

	var missing []string
	for _, formulaName := range patrolFormulas {
		found := false
		for _, dir := range searchDirs {
			formulaPath := filepath.Join(dir, formulaName+".formula.toml")
			if _, err := os.Stat(formulaPath); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, formulaName)
		}
	}
	return missing
}

// PatrolHooksWiredCheck verifies that hooks trigger patrol execution.
type PatrolHooksWiredCheck struct {
	FixableCheck
}

// NewPatrolHooksWiredCheck creates a new patrol hooks wired check.
func NewPatrolHooksWiredCheck() *PatrolHooksWiredCheck {
	return &PatrolHooksWiredCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "patrol-hooks-wired",
				CheckDescription: "Check if hooks trigger patrol execution",
				CheckCategory:    CategoryPatrol,
			},
		},
	}
}

// Run checks if patrol hooks are wired.
func (c *PatrolHooksWiredCheck) Run(ctx *CheckContext) *CheckResult {
	daemonConfigPath := config.DaemonPatrolConfigPath(ctx.TownRoot)
	relPath, _ := filepath.Rel(ctx.TownRoot, daemonConfigPath)

	if _, err := os.Stat(daemonConfigPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%s not found", relPath),
			FixHint: "Run 'gt doctor --fix' to create default config, or 'gt daemon start' to start the daemon",
		}
	}

	cfg, err := config.LoadDaemonPatrolConfig(daemonConfigPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Failed to read daemon config",
			Details: []string{err.Error()},
		}
	}

	if len(cfg.Patrols) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("Daemon configured with %d patrol(s)", len(cfg.Patrols)),
		}
	}

	if cfg.Heartbeat != nil && cfg.Heartbeat.Enabled {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Daemon heartbeat enabled (triggers patrols)",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Configure patrols in %s or run 'gt daemon start'", relPath),
		FixHint: "Run 'gt doctor --fix' to create default config",
	}
}

// Fix creates the daemon patrol config with defaults.
func (c *PatrolHooksWiredCheck) Fix(ctx *CheckContext) error {
	return config.EnsureDaemonPatrolConfig(ctx.TownRoot)
}

// PatrolNotStuckCheck detects wisps that have been in_progress too long.
type PatrolNotStuckCheck struct {
	BaseCheck
	stuckThreshold time.Duration
}

// DefaultStuckThreshold is the fallback when no role bead config exists.
// Per ZFC: "Let agents decide thresholds. 'Stuck' is a judgment call."
const DefaultStuckThreshold = 1 * time.Hour

// NewPatrolNotStuckCheck creates a new patrol not stuck check.
func NewPatrolNotStuckCheck() *PatrolNotStuckCheck {
	return &PatrolNotStuckCheck{
		BaseCheck: BaseCheck{
			CheckName:        "patrol-not-stuck",
			CheckDescription: "Check for stuck patrol wisps (>1h in_progress)",
			CheckCategory:    CategoryPatrol,
		},
		stuckThreshold: DefaultStuckThreshold,
	}
}

// Run checks for stuck patrol wisps.
func (c *PatrolNotStuckCheck) Run(ctx *CheckContext) *CheckResult {

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

	var stuckWisps []string
	for _, rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)

		// Query Dolt database (the only supported backend).
		stuck, err := c.checkStuckWispsDolt(rigPath, rigName)
		if err != nil {
			// Dolt query failed — report as error rather than silently skipping.
			stuckWisps = append(stuckWisps, fmt.Sprintf("%s: Dolt query failed: %v", rigName, err))
			continue
		}
		stuckWisps = append(stuckWisps, stuck...)
	}

	thresholdStr := c.stuckThreshold.String()
	if len(stuckWisps) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d stuck patrol wisp(s) found (>%s)", len(stuckWisps), thresholdStr),
			Details: stuckWisps,
			FixHint: "Manual review required - wisps may need to be burned or sessions restarted",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "No stuck patrol wisps found",
	}
}

// stuckWispsQuery selects in_progress issues for stuck-wisp detection via Dolt.
const stuckWispsQuery = `SELECT id, title, status, updated_at FROM issues WHERE status = 'in_progress' ORDER BY updated_at ASC`

// checkStuckWispsDolt queries the Dolt database for stuck wisps using bd sql.
// Returns an error if the query fails (caller should fall back to JSONL).
func (c *PatrolNotStuckCheck) checkStuckWispsDolt(rigPath string, rigName string) ([]string, error) {
	cmd := exec.Command("bd", "sql", "--csv", stuckWispsQuery) //nolint:gosec // G204: query is a constant
	cmd.Dir = rigPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bd sql: %w", err)
	}

	r := csv.NewReader(strings.NewReader(string(output)))
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse: %w", err)
	}
	if len(records) < 2 {
		return nil, nil // No results (header only or empty)
	}

	var stuck []string
	cutoff := time.Now().Add(-c.stuckThreshold)

	for _, rec := range records[1:] { // Skip CSV header
		if len(rec) < 4 {
			continue
		}
		id := strings.TrimSpace(rec[0])
		title := strings.TrimSpace(rec[1])
		updatedAt := strings.TrimSpace(rec[3])

		t, err := time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			// Try RFC3339 as fallback
			t, err = time.Parse(time.RFC3339, updatedAt)
			if err != nil {
				continue
			}
		}

		if !t.IsZero() && t.Before(cutoff) {
			stuck = append(stuck, fmt.Sprintf("%s: %s (%s) - stale since %s",
				rigName, id, title, t.Format("2006-01-02 15:04")))
		}
	}

	return stuck, nil
}

// PatrolPluginsAccessibleCheck verifies plugin directories exist and are readable.
type PatrolPluginsAccessibleCheck struct {
	FixableCheck
	missingDirs []string
}

// NewPatrolPluginsAccessibleCheck creates a new patrol plugins accessible check.
func NewPatrolPluginsAccessibleCheck() *PatrolPluginsAccessibleCheck {
	return &PatrolPluginsAccessibleCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "patrol-plugins-accessible",
				CheckDescription: "Check if plugin directories exist and are readable",
				CheckCategory:    CategoryPatrol,
			},
		},
	}
}

// Run checks if plugin directories are accessible.
func (c *PatrolPluginsAccessibleCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingDirs = nil

	// Check town-level plugins directory
	townPluginsDir := filepath.Join(ctx.TownRoot, "plugins")
	if _, err := os.Stat(townPluginsDir); os.IsNotExist(err) {
		c.missingDirs = append(c.missingDirs, townPluginsDir)
	}

	// Check rig-level plugins directories
	rigs, err := discoverRigs(ctx.TownRoot)
	if err == nil {
		for _, rigName := range rigs {
			rigPluginsDir := filepath.Join(ctx.TownRoot, rigName, "plugins")
			if _, err := os.Stat(rigPluginsDir); os.IsNotExist(err) {
				c.missingDirs = append(c.missingDirs, rigPluginsDir)
			}
		}
	}

	if len(c.missingDirs) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d plugin directory(ies) missing", len(c.missingDirs)),
			Details: c.missingDirs,
			FixHint: "Run 'gt doctor --fix' to create missing directories",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "All plugin directories accessible",
	}
}

// Fix creates missing plugin directories.
func (c *PatrolPluginsAccessibleCheck) Fix(ctx *CheckContext) error {
	for _, dir := range c.missingDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// discoverRigs finds all registered rigs.
func discoverRigs(townRoot string) ([]string, error) {
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No rigs configured
		}
		return nil, err
	}

	var rigsConfig config.RigsConfig
	if err := json.Unmarshal(data, &rigsConfig); err != nil {
		return nil, err
	}

	var rigs []string
	for name := range rigsConfig.Rigs {
		rigs = append(rigs, name)
	}
	return rigs, nil
}
