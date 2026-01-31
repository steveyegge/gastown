package doctor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// beadsMetadata represents the metadata.json file in .beads directory.
type beadsMetadata struct {
	Database    string `json:"database,omitempty"`
	JSONLExport string `json:"jsonl_export,omitempty"`
	Backend     string `json:"backend,omitempty"` // "sqlite" (default) or "dolt"
	Prefix      string `json:"prefix,omitempty"`
}

// detectBackend reads metadata.json and returns the backend type.
// Returns "sqlite" (default), "dolt", or error if metadata cannot be read.
func detectBackend(beadsDir string) (string, error) {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No metadata.json means SQLite (default)
			return "sqlite", nil
		}
		return "", err
	}

	var meta beadsMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}

	// Default to SQLite if backend not specified
	if meta.Backend == "" {
		return "sqlite", nil
	}

	return meta.Backend, nil
}

// BeadsDatabaseCheck verifies that the beads database is properly initialized.
// It detects when issues.db is empty or missing critical columns, and can
// auto-fix by triggering a re-import from the JSONL file.
type BeadsDatabaseCheck struct {
	FixableCheck
}

// NewBeadsDatabaseCheck creates a new beads database check.
func NewBeadsDatabaseCheck() *BeadsDatabaseCheck {
	return &BeadsDatabaseCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "beads-database",
				CheckDescription: "Verify beads database is properly initialized",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if the beads database is properly initialized.
func (c *BeadsDatabaseCheck) Run(ctx *CheckContext) *CheckResult {
	// Check town-level beads
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No .beads directory found at town root",
			FixHint: "Run 'bd init' to initialize beads",
		}
	}

	// Detect backend type (SQLite or Dolt)
	backend, err := detectBackend(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not detect backend: %v", err),
		}
	}

	// Check based on backend type
	if backend == "dolt" {
		return c.checkDoltBackend(beadsDir, ctx)
	}
	return c.checkSqliteBackend(beadsDir, ctx)
}

// checkDoltBackend checks Dolt-specific database health.
func (c *BeadsDatabaseCheck) checkDoltBackend(beadsDir string, _ *CheckContext) *CheckResult {
	doltDir := filepath.Join(beadsDir, "dolt")
	if _, err := os.Stat(doltDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Dolt backend configured but dolt/ directory not found",
			FixHint: "Run 'bd init --backend dolt' to initialize Dolt backend",
		}
	}

	// Check if dolt CLI is available
	if _, err := exec.LookPath("dolt"); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Dolt backend configured but dolt CLI not found",
			FixHint: "Install dolt: https://docs.dolthub.com/introduction/installation",
		}
	}

	// TODO: Add more Dolt-specific health checks (e.g., check for .dolt/noms directory, check dolt status)

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Dolt backend properly initialized",
	}
}

// checkSqliteBackend checks SQLite-specific database health.
func (c *BeadsDatabaseCheck) checkSqliteBackend(beadsDir string, ctx *CheckContext) *CheckResult {
	// Check if issues.db exists and has content
	issuesDB := filepath.Join(beadsDir, "issues.db")
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")

	dbInfo, dbErr := os.Stat(issuesDB)
	jsonlInfo, jsonlErr := os.Stat(issuesJSONL)

	// If no database file, that's OK - beads will create it
	if os.IsNotExist(dbErr) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No issues.db file (will be created on first use)",
		}
	}

	// If database file is empty but JSONL has content, this is the bug
	// Note: This check is for SQLite backend; Dolt backend doesn't use these files
	if dbErr == nil && dbInfo.Size() == 0 {
		if jsonlErr == nil && jsonlInfo.Size() > 0 {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusError,
				Message: "issues.db is empty but issues.jsonl has content",
				Details: []string{
					"This can cause 'table issues has no column named pinned' errors",
					"The database needs to be rebuilt from the JSONL file",
				},
				FixHint: "Run 'gt doctor --fix' or 'bd repair' to rebuild the database",
			}
		}
	}

	// Also check rig-level beads if a rig is specified (SQLite only)
	// Follows redirect if present (rig root may redirect to mayor/rig/.beads)
	if ctx.RigName != "" {
		rigBeadsDir := beads.ResolveBeadsDir(ctx.RigPath())
		if _, err := os.Stat(rigBeadsDir); err == nil {
			// Detect backend for rig
			rigBackend, err := detectBackend(rigBeadsDir)
			if err == nil && rigBackend == "sqlite" {
				rigDB := filepath.Join(rigBeadsDir, "issues.db")
				rigJSONL := filepath.Join(rigBeadsDir, "issues.jsonl")

				rigDBInfo, rigDBErr := os.Stat(rigDB)
				rigJSONLInfo, rigJSONLErr := os.Stat(rigJSONL)

				if rigDBErr == nil && rigDBInfo.Size() == 0 {
					if rigJSONLErr == nil && rigJSONLInfo.Size() > 0 {
						return &CheckResult{
							Name:    c.Name(),
							Status:  StatusError,
							Message: "Rig issues.db is empty but issues.jsonl has content",
							Details: []string{
								"Rig: " + ctx.RigName,
								"This can cause 'table issues has no column named pinned' errors",
							},
							FixHint: "Run 'gt doctor --fix' or delete the rig's issues.db",
						}
					}
				}
			}
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "Beads database is properly initialized",
	}
}

// Fix attempts to rebuild the database from JSONL.
// Note: This fix is for SQLite backend. With Dolt backend, this is a no-op.
func (c *BeadsDatabaseCheck) Fix(ctx *CheckContext) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")
	issuesDB := filepath.Join(beadsDir, "issues.db")
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")

	// Check if we need to fix town-level database
	dbInfo, dbErr := os.Stat(issuesDB)
	jsonlInfo, jsonlErr := os.Stat(issuesJSONL)

	if dbErr == nil && dbInfo.Size() == 0 && jsonlErr == nil && jsonlInfo.Size() > 0 {
		// Delete the empty database file
		if err := os.Remove(issuesDB); err != nil {
			return err
		}

		// Run bd import to rebuild from JSONL
		cmd := exec.Command("bd", "import")
		cmd.Dir = ctx.TownRoot
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	// Also fix rig-level if specified (follows redirect if present)
	if ctx.RigName != "" {
		rigBeadsDir := beads.ResolveBeadsDir(ctx.RigPath())
		rigDB := filepath.Join(rigBeadsDir, "issues.db")
		rigJSONL := filepath.Join(rigBeadsDir, "issues.jsonl")

		rigDBInfo, rigDBErr := os.Stat(rigDB)
		rigJSONLInfo, rigJSONLErr := os.Stat(rigJSONL)

		if rigDBErr == nil && rigDBInfo.Size() == 0 && rigJSONLErr == nil && rigJSONLInfo.Size() > 0 {
			if err := os.Remove(rigDB); err != nil {
				return err
			}

			cmd := exec.Command("bd", "import")
			cmd.Dir = ctx.RigPath()
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
	}

	return nil
}

// PrefixConflictCheck detects duplicate prefixes across rigs in routes.jsonl.
// Duplicate prefixes break prefix-based routing.
type PrefixConflictCheck struct {
	BaseCheck
}

// NewPrefixConflictCheck creates a new prefix conflict check.
func NewPrefixConflictCheck() *PrefixConflictCheck {
	return &PrefixConflictCheck{
		BaseCheck: BaseCheck{
			CheckName:        "prefix-conflict",
			CheckDescription: "Check for duplicate beads prefixes across rigs",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks for duplicate prefixes in routes.jsonl.
func (c *PrefixConflictCheck) Run(ctx *CheckContext) *CheckResult {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")

	// Check if routes.jsonl exists
	routesPath := filepath.Join(beadsDir, beads.RoutesFileName)
	if _, err := os.Stat(routesPath); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No routes.jsonl file (prefix routing not configured)",
		}
	}

	// Find conflicts
	conflicts, err := beads.FindConflictingPrefixes(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not check routes.jsonl: %v", err),
		}
	}

	if len(conflicts) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No prefix conflicts found",
		}
	}

	// Build details
	var details []string
	for prefix, paths := range conflicts {
		details = append(details, fmt.Sprintf("Prefix %q used by: %s", prefix, strings.Join(paths, ", ")))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("%d prefix conflict(s) found in routes.jsonl", len(conflicts)),
		Details: details,
		FixHint: "Use 'bd rename-prefix <new-prefix>' in one of the conflicting rigs to resolve",
	}
}

// PrefixMismatchCheck detects when rigs.json has a different prefix than what
// routes.jsonl actually uses for a rig. This can happen when:
// - deriveBeadsPrefix() generates a different prefix than what's in the beads DB
// - Someone manually edited rigs.json with the wrong prefix
// - The beads were initialized before auto-derive existed with a different prefix
type PrefixMismatchCheck struct {
	FixableCheck
}

// NewPrefixMismatchCheck creates a new prefix mismatch check.
func NewPrefixMismatchCheck() *PrefixMismatchCheck {
	return &PrefixMismatchCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "prefix-mismatch",
				CheckDescription: "Check for prefix mismatches between rigs.json and routes.jsonl",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks for prefix mismatches between rigs.json and routes.jsonl.
func (c *PrefixMismatchCheck) Run(ctx *CheckContext) *CheckResult {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")

	// Load routes.jsonl
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not load routes.jsonl: %v", err),
		}
	}
	if len(routes) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No routes configured (nothing to check)",
		}
	}

	// Load rigs.json
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsConfig, err := loadRigsConfig(rigsPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs.json found (nothing to check)",
		}
	}

	// Build map of route path -> prefix from routes.jsonl
	routePrefixByPath := make(map[string]string)
	for _, r := range routes {
		// Normalize: strip trailing hyphen from prefix for comparison
		prefix := strings.TrimSuffix(r.Prefix, "-")
		routePrefixByPath[r.Path] = prefix
	}

	// Check each rig in rigs.json against routes.jsonl
	var mismatches []string
	mismatchData := make(map[string][2]string) // rigName -> [rigsJsonPrefix, routesPrefix]

	for rigName, rigEntry := range rigsConfig.Rigs {
		// Skip rigs without beads config
		if rigEntry.BeadsConfig == nil || rigEntry.BeadsConfig.Prefix == "" {
			continue
		}

		rigsJsonPrefix := rigEntry.BeadsConfig.Prefix
		expectedPath := rigName + "/mayor/rig"

		// Find the route for this rig
		routePrefix, hasRoute := routePrefixByPath[expectedPath]
		if !hasRoute {
			// No route for this rig - routes-config check handles this
			continue
		}

		// Compare prefixes (both should be without trailing hyphen)
		if rigsJsonPrefix != routePrefix {
			mismatches = append(mismatches, rigName)
			mismatchData[rigName] = [2]string{rigsJsonPrefix, routePrefix}
		}
	}

	if len(mismatches) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No prefix mismatches found",
		}
	}

	// Build details
	var details []string
	for _, rigName := range mismatches {
		data := mismatchData[rigName]
		details = append(details, fmt.Sprintf("Rig '%s': rigs.json says '%s', routes.jsonl uses '%s'",
			rigName, data[0], data[1]))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d prefix mismatch(es) between rigs.json and routes.jsonl", len(mismatches)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to update rigs.json with correct prefixes",
	}
}

// Fix updates rigs.json to match the prefixes in routes.jsonl.
func (c *PrefixMismatchCheck) Fix(ctx *CheckContext) error {
	beadsDir := filepath.Join(ctx.TownRoot, ".beads")

	// Load routes.jsonl
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil || len(routes) == 0 {
		return nil // Nothing to fix
	}

	// Load rigs.json
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsConfig, err := loadRigsConfig(rigsPath)
	if err != nil {
		return nil // Nothing to fix
	}

	// Build map of route path -> prefix from routes.jsonl
	routePrefixByPath := make(map[string]string)
	for _, r := range routes {
		prefix := strings.TrimSuffix(r.Prefix, "-")
		routePrefixByPath[r.Path] = prefix
	}

	// Update each rig's prefix to match routes.jsonl
	modified := false
	for rigName, rigEntry := range rigsConfig.Rigs {
		expectedPath := rigName + "/mayor/rig"
		routePrefix, hasRoute := routePrefixByPath[expectedPath]
		if !hasRoute {
			continue
		}

		// Ensure BeadsConfig exists
		if rigEntry.BeadsConfig == nil {
			rigEntry.BeadsConfig = &rigsConfigBeadsConfig{}
		}

		if rigEntry.BeadsConfig.Prefix != routePrefix {
			rigEntry.BeadsConfig.Prefix = routePrefix
			rigsConfig.Rigs[rigName] = rigEntry
			modified = true
		}
	}

	if modified {
		return saveRigsConfig(rigsPath, rigsConfig)
	}

	return nil
}

// rigsConfigEntry is a local type for loading rigs.json without importing config package
// to avoid circular dependencies and keep the check self-contained.
type rigsConfigEntry struct {
	GitURL      string                 `json:"git_url"`
	LocalRepo   string                 `json:"local_repo,omitempty"`
	AddedAt     string                 `json:"added_at"` // Keep as string to preserve format
	BeadsConfig *rigsConfigBeadsConfig `json:"beads,omitempty"`
}

type rigsConfigBeadsConfig struct {
	Repo   string `json:"repo"`
	Prefix string `json:"prefix"`
}

type rigsConfigFile struct {
	Version int                         `json:"version"`
	Rigs    map[string]rigsConfigEntry  `json:"rigs"`
}

func loadRigsConfig(path string) (*rigsConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg rigsConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func saveRigsConfig(path string, cfg *rigsConfigFile) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// beadShower is an interface for fetching bead information.
// Allows mocking in tests.
type beadShower interface {
	Show(id string) (*beads.Issue, error)
}

// labelAdder is an interface for adding labels to beads.
// Allows mocking in tests.
type labelAdder interface {
	AddLabel(townRoot, id, label string) error
}

// realLabelAdder implements labelAdder using bd command.
type realLabelAdder struct{}

func (r *realLabelAdder) AddLabel(townRoot, id, label string) error {
	cmd := exec.Command("bd", "label", "add", id, label)
	cmd.Dir = townRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding %s label to %s: %s", label, id, strings.TrimSpace(string(output)))
	}
	return nil
}

// RoleLabelCheck verifies that role beads have the gt:role label.
// This label is required for GetRoleConfig to recognize role beads.
// Role beads created before the label migration may be missing this label.
type RoleLabelCheck struct {
	FixableCheck
	missingLabel []string // Role bead IDs missing gt:role label
	townRoot     string   // Cached for Fix

	// Injected dependencies for testing
	beadShower beadShower
	labelAdder labelAdder
}

// NewRoleLabelCheck creates a new role label check.
func NewRoleLabelCheck() *RoleLabelCheck {
	return &RoleLabelCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "role-bead-labels",
				CheckDescription: "Check that role beads have gt:role label",
				CheckCategory:    CategoryConfig,
			},
		},
		labelAdder: &realLabelAdder{},
	}
}

// roleBeadIDs returns the list of role bead IDs to check.
func roleBeadIDs() []string {
	return []string{
		beads.MayorRoleBeadIDTown(),
		beads.DeaconRoleBeadIDTown(),
		beads.DogRoleBeadIDTown(),
		beads.WitnessRoleBeadIDTown(),
		beads.RefineryRoleBeadIDTown(),
		beads.PolecatRoleBeadIDTown(),
		beads.CrewRoleBeadIDTown(),
	}
}

// Run checks if role beads have the gt:role label.
func (c *RoleLabelCheck) Run(ctx *CheckContext) *CheckResult {
	// Check if bd command is available (skip if testing with mock)
	if c.beadShower == nil {
		if _, err := exec.LookPath("bd"); err != nil {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "beads not installed (skipped)",
			}
		}
	}

	// Check if .beads directory exists at town level
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No beads database (skipped)",
		}
	}

	// Use injected beadShower or create real one
	shower := c.beadShower
	if shower == nil {
		shower = beads.New(ctx.TownRoot)
	}

	var missingLabel []string
	for _, roleID := range roleBeadIDs() {
		issue, err := shower.Show(roleID)
		if err != nil {
			// Bead doesn't exist - that's OK, install will create it
			continue
		}

		// Check if it has the gt:role label
		if !beads.HasLabel(issue, "gt:role") {
			missingLabel = append(missingLabel, roleID)
		}
	}

	// Cache for Fix
	c.missingLabel = missingLabel
	c.townRoot = ctx.TownRoot

	if len(missingLabel) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All role beads have gt:role label",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d role bead(s) missing gt:role label", len(missingLabel)),
		Details: missingLabel,
		FixHint: "Run 'gt doctor --fix' to add missing labels",
	}
}

// Fix adds the gt:role label to role beads that are missing it.
func (c *RoleLabelCheck) Fix(ctx *CheckContext) error {
	for _, roleID := range c.missingLabel {
		if err := c.labelAdder.AddLabel(c.townRoot, roleID, "gt:role"); err != nil {
			return err
		}
	}
	return nil
}

// DatabasePrefixCheck detects when a rig's database has a different issue_prefix
// than what routes.jsonl specifies. This can happen when:
// - The database was initialized with a different prefix
// - Manual database edits changed the prefix
// - A bug in prefix derivation caused a mismatch
//
// Unlike PrefixMismatchCheck (rigs.json ↔ routes.jsonl), this check verifies
// the actual database configuration matches the routing table.
type DatabasePrefixCheck struct {
	FixableCheck
	mismatches []databasePrefixMismatch
}

type databasePrefixMismatch struct {
	rigPath      string
	beadsDir     string
	routesPrefix string // From routes.jsonl (without trailing hyphen)
	dbPrefix     string // From database config
}

// NewDatabasePrefixCheck creates a new database prefix check.
func NewDatabasePrefixCheck() *DatabasePrefixCheck {
	return &DatabasePrefixCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "database-prefix",
				CheckDescription: "Check rig database issue_prefix matches routes.jsonl",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if each rig's database issue_prefix matches routes.jsonl.
func (c *DatabasePrefixCheck) Run(ctx *CheckContext) *CheckResult {
	c.mismatches = nil // Reset

	beadsDir := filepath.Join(ctx.TownRoot, ".beads")

	// Load routes.jsonl
	routes, err := beads.LoadRoutes(beadsDir)
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No routes.jsonl found (nothing to check)",
			Category: c.Category(),
		}
	}
	if len(routes) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No routes configured (nothing to check)",
			Category: c.Category(),
		}
	}

	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "beads not installed (skipped)",
			Category: c.Category(),
		}
	}

	var problems []string

	for _, route := range routes {
		// Skip town root route
		if route.Path == "." || route.Path == "" {
			continue
		}

		// Resolve the rig's beads directory (follows redirects)
		rigPath := filepath.Join(ctx.TownRoot, route.Path)
		rigBeadsDir := beads.ResolveBeadsDir(rigPath)

		// Check if beads directory exists
		if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
			continue // No beads dir for this rig
		}

		// Query database for issue_prefix using bd config get
		dbPrefix, err := c.getDBPrefix(rigBeadsDir)
		if err != nil {
			// No issue_prefix configured - that's OK
			continue
		}

		// Normalize routes prefix (strip trailing hyphen)
		routesPrefix := strings.TrimSuffix(route.Prefix, "-")

		// Compare prefixes
		if dbPrefix != routesPrefix {
			problems = append(problems, fmt.Sprintf("Route '%s': routes.jsonl says '%s', database has '%s'",
				route.Path, routesPrefix, dbPrefix))
			c.mismatches = append(c.mismatches, databasePrefixMismatch{
				rigPath:      route.Path,
				beadsDir:     rigBeadsDir,
				routesPrefix: routesPrefix,
				dbPrefix:     dbPrefix,
			})
		}
	}

	if len(c.mismatches) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "All database prefixes match routes.jsonl",
			Category: c.Category(),
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d database prefix mismatch(es) with routes.jsonl", len(c.mismatches)),
		Details:  problems,
		FixHint:  "Run 'gt doctor --fix' to update database configs to match routes.jsonl",
		Category: c.Category(),
	}
}

// getDBPrefix queries the database for issue_prefix config value.
func (c *DatabasePrefixCheck) getDBPrefix(beadsDir string) (string, error) {
	cmd := exec.Command("bd", "config", "get", "issue_prefix", "--db", beadsDir)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Fix updates database configs to match routes.jsonl prefixes.
func (c *DatabasePrefixCheck) Fix(ctx *CheckContext) error {
	// Re-run check to populate mismatches if needed
	if len(c.mismatches) == 0 {
		result := c.Run(ctx)
		if result.Status == StatusOK {
			return nil // Nothing to fix
		}
	}

	for _, m := range c.mismatches {
		cmd := exec.Command("bd", "config", "set", "issue_prefix", m.routesPrefix, "--db", m.beadsDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("updating %s: %s", m.rigPath, strings.TrimSpace(string(output)))
		}
	}

	return nil
}
