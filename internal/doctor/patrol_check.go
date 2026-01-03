package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/templates"
)

// PatrolMoleculesExistCheck verifies that patrol molecules exist for each rig.
type PatrolMoleculesExistCheck struct {
	FixableCheck
	missingMols map[string][]string // rig -> missing molecule titles
}

// NewPatrolMoleculesExistCheck creates a new patrol molecules exist check.
func NewPatrolMoleculesExistCheck() *PatrolMoleculesExistCheck {
	return &PatrolMoleculesExistCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "patrol-molecules-exist",
				CheckDescription: "Check if patrol molecules exist for each rig",
			},
		},
	}
}

// patrolMolecules are the required patrol molecule titles.
var patrolMolecules = []string{
	"Deacon Patrol",
	"Witness Patrol",
	"Refinery Patrol",
}

// Run checks if patrol molecules exist.
func (c *PatrolMoleculesExistCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingMols = make(map[string][]string)

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
		missing := c.checkPatrolMolecules(rigPath)
		if len(missing) > 0 {
			c.missingMols[rigName] = missing
			details = append(details, fmt.Sprintf("%s: missing %v", rigName, missing))
		}
	}

	if len(details) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d rig(s) missing patrol molecules", len(c.missingMols)),
			Details: details,
			FixHint: "Run 'gt doctor --fix' to create missing patrol molecules",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("All %d rig(s) have patrol molecules", len(rigs)),
	}
}

// checkPatrolMolecules returns missing patrol molecule titles for a rig.
func (c *PatrolMoleculesExistCheck) checkPatrolMolecules(rigPath string) []string {
	// List molecules using bd
	cmd := exec.Command("bd", "list", "--type=molecule")
	cmd.Dir = rigPath
	output, err := cmd.Output()
	if err != nil {
		return patrolMolecules // Can't check, assume all missing
	}

	outputStr := string(output)
	var missing []string
	for _, mol := range patrolMolecules {
		if !strings.Contains(outputStr, mol) {
			missing = append(missing, mol)
		}
	}
	return missing
}

// Fix creates missing patrol molecules.
func (c *PatrolMoleculesExistCheck) Fix(ctx *CheckContext) error {
	for rigName, missing := range c.missingMols {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		for _, mol := range missing {
			desc := getPatrolMoleculeDesc(mol)
			cmd := exec.Command("bd", "create", //nolint:gosec // G204: args are constructed internally
				"--type=molecule",
				"--title="+mol,
				"--description="+desc,
				"--priority=2",
			)
			cmd.Dir = rigPath
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("creating %s in %s: %w", mol, rigName, err)
			}
		}
	}
	return nil
}

func getPatrolMoleculeDesc(title string) string {
	switch title {
	case "Deacon Patrol":
		return "Mayor's daemon patrol loop for handling callbacks, health checks, and cleanup."
	case "Witness Patrol":
		return "Per-rig worker monitor patrol loop with progressive nudging."
	case "Refinery Patrol":
		return "Merge queue processor patrol loop with verification gates."
	default:
		return "Patrol molecule"
	}
}

// PatrolHooksWiredCheck verifies that hooks trigger patrol execution.
type PatrolHooksWiredCheck struct {
	BaseCheck
}

// NewPatrolHooksWiredCheck creates a new patrol hooks wired check.
func NewPatrolHooksWiredCheck() *PatrolHooksWiredCheck {
	return &PatrolHooksWiredCheck{
		BaseCheck: BaseCheck{
			CheckName:        "patrol-hooks-wired",
			CheckDescription: "Check if hooks trigger patrol execution",
		},
	}
}

// Run checks if patrol hooks are wired.
func (c *PatrolHooksWiredCheck) Run(ctx *CheckContext) *CheckResult {
	// Check for daemon config which manages patrols
	daemonConfigPath := filepath.Join(ctx.TownRoot, "mayor", "daemon.json")
	if _, err := os.Stat(daemonConfigPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Daemon config not found",
			FixHint: "Run 'gt daemon init' to configure daemon",
		}
	}

	// Check daemon config for patrol configuration
	data, err := os.ReadFile(daemonConfigPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Failed to read daemon config",
			Details: []string{err.Error()},
		}
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Invalid daemon config format",
			Details: []string{err.Error()},
		}
	}

	// Check for patrol entries
	if patrols, ok := config["patrols"]; ok {
		if patrolMap, ok := patrols.(map[string]interface{}); ok && len(patrolMap) > 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: fmt.Sprintf("Daemon configured with %d patrol(s)", len(patrolMap)),
			}
		}
	}

	// Check if heartbeat is enabled (triggers deacon patrol)
	if heartbeat, ok := config["heartbeat"]; ok {
		if hb, ok := heartbeat.(map[string]interface{}); ok {
			if enabled, ok := hb["enabled"].(bool); ok && enabled {
				return &CheckResult{
					Name:    c.Name(),
					Status:  StatusOK,
					Message: "Daemon heartbeat enabled (triggers patrols)",
				}
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Patrol hooks not configured in daemon",
		FixHint: "Configure patrols in mayor/daemon.json or run 'gt daemon init'",
	}
}

// PatrolNotStuckCheck detects wisps that have been in_progress too long.
type PatrolNotStuckCheck struct {
	BaseCheck
	stuckThreshold time.Duration
}

// NewPatrolNotStuckCheck creates a new patrol not stuck check.
func NewPatrolNotStuckCheck() *PatrolNotStuckCheck {
	return &PatrolNotStuckCheck{
		BaseCheck: BaseCheck{
			CheckName:        "patrol-not-stuck",
			CheckDescription: "Check for stuck patrol wisps (>1h in_progress)",
		},
		stuckThreshold: 1 * time.Hour,
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
		// Check main beads database for wisps (issues with Wisp=true)
		// Follows redirect if present (rig root may redirect to mayor/rig/.beads)
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		beadsDir := beads.ResolveBeadsDir(rigPath)
		beadsPath := filepath.Join(beadsDir, "issues.jsonl")
		stuck := c.checkStuckWisps(beadsPath, rigName)
		stuckWisps = append(stuckWisps, stuck...)
	}

	if len(stuckWisps) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d stuck patrol wisp(s) found (>1h)", len(stuckWisps)),
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

// checkStuckWisps returns descriptions of stuck wisps in a rig.
func (c *PatrolNotStuckCheck) checkStuckWisps(issuesPath string, rigName string) []string {
	file, err := os.Open(issuesPath)
	if err != nil {
		return nil // No issues file
	}
	defer file.Close()

	var stuck []string
	cutoff := time.Now().Add(-c.stuckThreshold)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var issue struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Status    string    `json:"status"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}

		// Check for in_progress issues older than threshold
		if issue.Status == "in_progress" && !issue.UpdatedAt.IsZero() && issue.UpdatedAt.Before(cutoff) {
			stuck = append(stuck, fmt.Sprintf("%s: %s (%s) - stale since %s",
				rigName, issue.ID, issue.Title, issue.UpdatedAt.Format("2006-01-02 15:04")))
		}
	}

	return stuck
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

// PatrolRolesHavePromptsCheck verifies that internal/templates/roles/*.md.tmpl exist for each rig.
// Checks at <town>/<rig>/mayor/rig/internal/templates/roles/*.md.tmpl
// Fix copies embedded templates to missing locations.
type PatrolRolesHavePromptsCheck struct {
	FixableCheck
	// missingByRig tracks missing templates per rig: rigName -> []missingFiles
	missingByRig map[string][]string
}

// NewPatrolRolesHavePromptsCheck creates a new patrol roles have prompts check.
func NewPatrolRolesHavePromptsCheck() *PatrolRolesHavePromptsCheck {
	return &PatrolRolesHavePromptsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "patrol-roles-have-prompts",
				CheckDescription: "Check if internal/templates/roles/*.md.tmpl exist for each patrol role",
			},
		},
	}
}

var requiredRolePrompts = []string{
	"deacon.md.tmpl",
	"witness.md.tmpl",
	"refinery.md.tmpl",
}

func (c *PatrolRolesHavePromptsCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingByRig = make(map[string][]string)

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

	var missingPrompts []string
	for _, rigName := range rigs {
		// Check in mayor's clone (canonical for the rig)
		mayorRig := filepath.Join(ctx.TownRoot, rigName, "mayor", "rig")
		templatesDir := filepath.Join(mayorRig, "internal", "templates", "roles")

		var rigMissing []string
		for _, roleFile := range requiredRolePrompts {
			promptPath := filepath.Join(templatesDir, roleFile)
			if _, err := os.Stat(promptPath); os.IsNotExist(err) {
				missingPrompts = append(missingPrompts, fmt.Sprintf("%s: %s", rigName, roleFile))
				rigMissing = append(rigMissing, roleFile)
			}
		}
		if len(rigMissing) > 0 {
			c.missingByRig[rigName] = rigMissing
		}
	}

	if len(missingPrompts) > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d role prompt template(s) missing", len(missingPrompts)),
			Details: missingPrompts,
			FixHint: "Run 'gt doctor --fix' to copy embedded templates to rig repos",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "All patrol role prompt templates found",
	}
}

func (c *PatrolRolesHavePromptsCheck) Fix(ctx *CheckContext) error {
	allTemplates, err := templates.GetAllRoleTemplates()
	if err != nil {
		return fmt.Errorf("getting embedded templates: %w", err)
	}

	for rigName, missingFiles := range c.missingByRig {
		mayorRig := filepath.Join(ctx.TownRoot, rigName, "mayor", "rig")
		templatesDir := filepath.Join(mayorRig, "internal", "templates", "roles")

		if err := os.MkdirAll(templatesDir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", templatesDir, err)
		}

		for _, roleFile := range missingFiles {
			content, ok := allTemplates[roleFile]
			if !ok {
				continue
			}

			destPath := filepath.Join(templatesDir, roleFile)
			if err := os.WriteFile(destPath, content, 0644); err != nil {
				return fmt.Errorf("writing %s in %s: %w", roleFile, rigName, err)
			}
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
