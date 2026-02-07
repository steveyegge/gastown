package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MigrationReadiness represents the overall migration readiness status.
// This struct is designed for machine-parseable JSON output.
type MigrationReadiness struct {
	Ready    bool              `json:"ready"`    // YES/NO overall verdict
	Version  MigrationVersions `json:"version"`  // Version compatibility info
	Rigs     []RigMigration    `json:"rigs"`     // Per-rig migration status
	Blockers []string          `json:"blockers"` // List of blocking issues
}

// MigrationVersions captures version compatibility information.
type MigrationVersions struct {
	GT           string `json:"gt"`            // gt version
	BD           string `json:"bd"`            // bd version
	BDSupportsDolt bool   `json:"bd_supports_dolt"` // bd version supports Dolt
}

// RigMigration represents migration status for a single rig.
type RigMigration struct {
	Name           string `json:"name"`
	Backend        string `json:"backend"`         // "sqlite", "dolt", or "unknown"
	NeedsMigration bool   `json:"needs_migration"` // true if still on SQLite
	GitClean       bool   `json:"git_clean"`       // true if git working tree is clean
	BeadsDir       string `json:"beads_dir"`       // Path to .beads directory
}

// MigrationReadinessCheck verifies that the workspace is ready for migration.
// It checks:
// 1. Unmigrated rig detection (metadata.json backend field)
// 2. Version compatibility (gt/bd version support for Dolt)
// 3. Pre-migration health (git state clean)
type MigrationReadinessCheck struct {
	BaseCheck
	readiness *MigrationReadiness // Cached result for JSON output
}

// NewMigrationReadinessCheck creates a new migration readiness check.
func NewMigrationReadinessCheck() *MigrationReadinessCheck {
	return &MigrationReadinessCheck{
		BaseCheck: BaseCheck{
			CheckName:        "migration-readiness",
			CheckDescription: "Check if workspace is ready for SQLite to Dolt migration",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks migration readiness across all rigs.
func (c *MigrationReadinessCheck) Run(ctx *CheckContext) *CheckResult {
	readiness := &MigrationReadiness{
		Ready:    true,
		Blockers: []string{},
		Rigs:     []RigMigration{},
	}
	c.readiness = readiness

	// Check versions
	readiness.Version = c.checkVersions()
	if !readiness.Version.BDSupportsDolt {
		readiness.Ready = false
		readiness.Blockers = append(readiness.Blockers, "bd version does not support Dolt backend")
	}

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); err == nil {
		rigMigration := c.checkRigBeads("town-root", townBeadsDir, ctx.TownRoot)
		readiness.Rigs = append(readiness.Rigs, rigMigration)
		if rigMigration.NeedsMigration {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Town root beads uses %s backend", rigMigration.Backend))
		}
		if !rigMigration.GitClean {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, "Town root has uncommitted changes")
		}
	}

	// Find all rigs and check their beads
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigs := c.loadRigs(rigsPath)
	for rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)

		// Check main rig beads (in mayor/rig/.beads)
		rigBeadsDir := filepath.Join(rigPath, "mayor", "rig", ".beads")
		if _, err := os.Stat(rigBeadsDir); err == nil {
			rigMigration := c.checkRigBeads(rigName, rigBeadsDir, rigPath)
			readiness.Rigs = append(readiness.Rigs, rigMigration)
			if rigMigration.NeedsMigration {
				readiness.Ready = false
				readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Rig %s beads uses %s backend", rigName, rigMigration.Backend))
			}
			if !rigMigration.GitClean {
				readiness.Ready = false
				readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Rig %s has uncommitted changes", rigName))
			}
		}
	}

	// Build result
	if readiness.Ready {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Workspace ready for migration (all rigs on Dolt)",
		}
	}

	var needsMigration int
	for _, rig := range readiness.Rigs {
		if rig.NeedsMigration {
			needsMigration++
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d rig(s) need migration, %d blocker(s)", needsMigration, len(readiness.Blockers)),
		Details: readiness.Blockers,
		FixHint: "Run 'bd migrate' in each rig to migrate from SQLite to Dolt",
	}
}

// Readiness returns the cached migration readiness result for JSON output.
func (c *MigrationReadinessCheck) Readiness() *MigrationReadiness {
	return c.readiness
}

// checkVersions checks gt and bd version compatibility.
func (c *MigrationReadinessCheck) checkVersions() MigrationVersions {
	versions := MigrationVersions{
		GT:             "unknown",
		BD:             "unknown",
		BDSupportsDolt: false,
	}

	// Get gt version
	if output, err := exec.Command("gt", "version").Output(); err == nil {
		versions.GT = strings.TrimSpace(string(output))
	}

	// Get bd version
	if output, err := exec.Command("bd", "version").Output(); err == nil {
		versionStr := strings.TrimSpace(string(output))
		versions.BD = versionStr
		// Check if bd supports Dolt (version 0.40.0+ supports Dolt)
		versions.BDSupportsDolt = c.bdSupportsDolt(versionStr)
	}

	return versions
}

// bdSupportsDolt checks if the bd version supports Dolt backend.
// Dolt support was added in bd 0.40.0.
func (c *MigrationReadinessCheck) bdSupportsDolt(versionStr string) bool {
	// Parse version like "bd version 0.49.3 (...)"
	parts := strings.Fields(versionStr)
	if len(parts) < 3 {
		return false
	}
	version := parts[2]

	// Parse semver
	vParts := strings.Split(version, ".")
	if len(vParts) < 2 {
		return false
	}

	var major, minor int
	fmt.Sscanf(vParts[0], "%d", &major)
	fmt.Sscanf(vParts[1], "%d", &minor)

	// Dolt support added in 0.40.0
	return major > 0 || (major == 0 && minor >= 40)
}

// checkRigBeads checks a single rig's beads directory for migration status.
func (c *MigrationReadinessCheck) checkRigBeads(rigName, beadsDir, rigPath string) RigMigration {
	result := RigMigration{
		Name:           rigName,
		Backend:        "unknown",
		NeedsMigration: false,
		GitClean:       true,
		BeadsDir:       beadsDir,
	}

	// Read metadata.json
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		// No metadata.json likely means SQLite (pre-Dolt)
		result.Backend = "sqlite"
		result.NeedsMigration = true
		return result
	}

	var metadata struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		result.Backend = "unknown"
		result.NeedsMigration = true
		return result
	}

	result.Backend = metadata.Backend
	if result.Backend == "" {
		result.Backend = "sqlite" // Default to SQLite if not specified
	}
	result.NeedsMigration = result.Backend != "dolt"

	// Check git status
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = rigPath
	output, err := cmd.Output()
	if err == nil {
		result.GitClean = len(strings.TrimSpace(string(output))) == 0
	}

	return result
}

// loadRigs loads the rigs configuration from rigs.json.
func (c *MigrationReadinessCheck) loadRigs(rigsPath string) map[string]struct{} {
	rigs := make(map[string]struct{})

	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return rigs
	}

	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return rigs
	}

	for name := range config.Rigs {
		rigs[name] = struct{}{}
	}
	return rigs
}

// UnmigratedRigCheck specifically checks for rigs still on SQLite backend.
// This is a simpler check that just reports which rigs need migration.
type UnmigratedRigCheck struct {
	BaseCheck
}

// NewUnmigratedRigCheck creates a check for unmigrated rigs.
func NewUnmigratedRigCheck() *UnmigratedRigCheck {
	return &UnmigratedRigCheck{
		BaseCheck: BaseCheck{
			CheckName:        "unmigrated-rigs",
			CheckDescription: "Detect rigs still using SQLite backend",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks for rigs using SQLite backend.
func (c *UnmigratedRigCheck) Run(ctx *CheckContext) *CheckResult {
	var unmigrated []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if backend := c.getBackend(townBeadsDir); backend == "sqlite" {
		unmigrated = append(unmigrated, "town-root")
	}

	// Find all rigs and check their beads
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigs := c.loadRigs(rigsPath)
	for rigName := range rigs {
		rigBeadsDir := filepath.Join(ctx.TownRoot, rigName, "mayor", "rig", ".beads")
		if backend := c.getBackend(rigBeadsDir); backend == "sqlite" {
			unmigrated = append(unmigrated, rigName)
		}
	}

	if len(unmigrated) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All rigs using Dolt backend",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d rig(s) still on SQLite backend", len(unmigrated)),
		Details: unmigrated,
		FixHint: "Run 'bd migrate' in each rig to migrate from SQLite to Dolt",
	}
}

// getBackend returns the backend type from metadata.json.
func (c *UnmigratedRigCheck) getBackend(beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		// No metadata.json likely means SQLite or no beads
		if _, statErr := os.Stat(beadsDir); statErr == nil {
			return "sqlite" // Dir exists but no metadata = SQLite
		}
		return "" // No beads dir
	}

	var metadata struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "unknown"
	}

	if metadata.Backend == "" {
		return "sqlite" // Default to SQLite if not specified
	}
	return metadata.Backend
}

// loadRigs loads the rigs configuration from rigs.json.
func (c *UnmigratedRigCheck) loadRigs(rigsPath string) map[string]struct{} {
	rigs := make(map[string]struct{})

	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return rigs
	}

	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return rigs
	}

	for name := range config.Rigs {
		rigs[name] = struct{}{}
	}
	return rigs
}

// DoltMetadataCheck verifies that all rig .beads/metadata.json files have
// proper Dolt server configuration (backend, dolt_mode, dolt_database).
// Missing or incomplete metadata causes the split-brain problem where bd
// opens isolated local databases instead of the centralized Dolt server.
type DoltMetadataCheck struct {
	FixableCheck
	missingMetadata []string // Cached during Run for use in Fix
}

// NewDoltMetadataCheck creates a new dolt metadata check.
func NewDoltMetadataCheck() *DoltMetadataCheck {
	return &DoltMetadataCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "dolt-metadata",
				CheckDescription: "Check that metadata.json has Dolt server config",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if all rig metadata.json files have dolt server config.
func (c *DoltMetadataCheck) Run(ctx *CheckContext) *CheckResult {
	c.missingMetadata = nil

	// Check if dolt data directory exists (no point checking if dolt isn't in use)
	doltDataDir := filepath.Join(ctx.TownRoot, ".dolt-data")
	if _, err := os.Stat(doltDataDir); os.IsNotExist(err) {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No Dolt data directory (dolt not in use)",
			Category: c.CheckCategory,
		}
	}

	var missing []string
	var ok int

	// Check town-level beads (hq database)
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(filepath.Join(doltDataDir, "hq")); err == nil {
		if !c.hasDoltMetadata(townBeadsDir, "hq") {
			missing = append(missing, "hq (town root .beads/)")
			c.missingMetadata = append(c.missingMetadata, "hq")
		} else {
			ok++
		}
	}

	// Check rig-level beads
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigs := c.loadRigs(rigsPath)
	for rigName := range rigs {
		// Only check rigs that have a dolt database
		if _, err := os.Stat(filepath.Join(doltDataDir, rigName)); os.IsNotExist(err) {
			continue
		}

		beadsDir := c.findRigBeadsDir(ctx.TownRoot, rigName)
		if beadsDir == "" {
			missing = append(missing, rigName+" (no .beads directory)")
			c.missingMetadata = append(c.missingMetadata, rigName)
			continue
		}

		if !c.hasDoltMetadata(beadsDir, rigName) {
			relPath, _ := filepath.Rel(ctx.TownRoot, beadsDir)
			missing = append(missing, rigName+" ("+relPath+")")
			c.missingMetadata = append(c.missingMetadata, rigName)
		} else {
			ok++
		}
	}

	if len(missing) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  fmt.Sprintf("All %d rig(s) have Dolt server metadata", ok),
			Category: c.CheckCategory,
		}
	}

	details := make([]string, len(missing))
	for i, m := range missing {
		details[i] = "Missing dolt config: " + m
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d rig(s) missing Dolt server metadata", len(missing)),
		Details:  details,
		FixHint:  "Run 'gt dolt fix-metadata' to update all metadata.json files",
		Category: c.CheckCategory,
	}
}

// Fix updates metadata.json for all rigs with missing dolt config.
func (c *DoltMetadataCheck) Fix(ctx *CheckContext) error {
	if len(c.missingMetadata) == 0 {
		return nil
	}

	// Import doltserver package via the fix path
	for _, rigName := range c.missingMetadata {
		if err := c.writeDoltMetadata(ctx.TownRoot, rigName); err != nil {
			return fmt.Errorf("fixing %s: %w", rigName, err)
		}
	}

	return nil
}

// hasDoltMetadata checks if a beads directory has proper dolt server config.
func (c *DoltMetadataCheck) hasDoltMetadata(beadsDir, expectedDB string) bool {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return false
	}

	var metadata struct {
		Backend      string `json:"backend"`
		DoltMode     string `json:"dolt_mode"`
		DoltDatabase string `json:"dolt_database"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return false
	}

	return metadata.Backend == "dolt" &&
		metadata.DoltMode == "server" &&
		metadata.DoltDatabase == expectedDB
}

// writeDoltMetadata writes dolt server config to a rig's metadata.json.
func (c *DoltMetadataCheck) writeDoltMetadata(townRoot, rigName string) error {
	beadsDir := c.findRigBeadsDir(townRoot, rigName)
	if beadsDir == "" {
		return fmt.Errorf("could not find .beads directory for rig %q", rigName)
	}

	metadataPath := filepath.Join(beadsDir, "metadata.json")

	// Load existing metadata if present
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	// Set dolt server fields
	existing["database"] = "dolt"
	existing["backend"] = "dolt"
	existing["dolt_mode"] = "server"
	existing["dolt_database"] = rigName

	if _, ok := existing["jsonl_export"]; !ok {
		existing["jsonl_export"] = "issues.jsonl"
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating beads directory: %w", err)
	}

	if err := os.WriteFile(metadataPath, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("writing metadata.json: %w", err)
	}

	return nil
}

// findRigBeadsDir returns the canonical .beads directory for a rig.
func (c *DoltMetadataCheck) findRigBeadsDir(townRoot, rigName string) string {
	if rigName == "hq" {
		return filepath.Join(townRoot, ".beads")
	}

	// Prefer mayor/rig/.beads (canonical)
	mayorBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if _, err := os.Stat(mayorBeads); err == nil {
		return mayorBeads
	}

	// Fall back to rig-root .beads
	rigBeads := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		return rigBeads
	}

	return ""
}

// loadRigs loads the rigs configuration from rigs.json.
func (c *DoltMetadataCheck) loadRigs(rigsPath string) map[string]struct{} {
	rigs := make(map[string]struct{})

	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return rigs
	}

	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return rigs
	}

	for name := range config.Rigs {
		rigs[name] = struct{}{}
	}
	return rigs
}
