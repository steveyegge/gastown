package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/util"
)

// MigrationState represents the migration classification for a rig.
type MigrationState string

const (
	// StateNeverMigrated means SQLite backend with no Dolt infrastructure.
	StateNeverMigrated MigrationState = "never-migrated"
	// StatePartiallyMigrated means some Dolt infrastructure exists but migration is incomplete.
	StatePartiallyMigrated MigrationState = "partially-migrated"
	// StateFullyMigrated means Dolt backend is active and healthy.
	StateFullyMigrated MigrationState = "fully-migrated"
	// StateNoBeads means no beads directory exists for this rig.
	StateNoBeads MigrationState = "no-beads"
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
	GT             string `json:"gt"`               // gt version
	BD             string `json:"bd"`               // bd version
	BDSupportsDolt bool   `json:"bd_supports_dolt"` // bd version supports Dolt
}

// RigMigration represents migration status for a single rig.
type RigMigration struct {
	Name           string         `json:"name"`
	Backend        string         `json:"backend"`         // "sqlite", "dolt", "none", or "unknown"
	State          MigrationState `json:"state"`           // Classification: never/partial/full/no-beads
	NeedsMigration bool           `json:"needs_migration"` // true if not fully migrated
	GitClean       bool           `json:"git_clean"`       // true if git working tree is clean
	BeadsDir       string         `json:"beads_dir"`       // Path to .beads directory
	HasDoltData    bool           `json:"has_dolt_data"`   // true if .dolt-data/<rig> exists
	HasJSONL       bool           `json:"has_jsonl"`       // true if issues.jsonl exists
	HasSQLite      bool           `json:"has_sqlite"`      // true if beads.db exists
	HasDoltDir     bool           `json:"has_dolt_dir"`    // true if dolt/ dir exists in .beads
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
		rigMigration := classifyRigMigration("town-root", "hq", townBeadsDir, ctx.TownRoot)
		// Check git cleanliness for town root
		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Dir = ctx.TownRoot
		output, err := cmd.Output()
		if err == nil {
			rigMigration.GitClean = len(strings.TrimSpace(string(output))) == 0
		}
		readiness.Rigs = append(readiness.Rigs, rigMigration)
		if rigMigration.NeedsMigration {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Town root beads: %s (backend: %s)", rigMigration.State, rigMigration.Backend))
		}
		if !rigMigration.GitClean {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, "Town root has uncommitted changes")
		}
	}

	// Find all rigs and check their beads
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigs := loadRigNames(rigsPath)
	for rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)

		// Check main rig beads (in mayor/rig/.beads)
		rigBeadsDir := filepath.Join(rigPath, "mayor", "rig", ".beads")
		rigMigration := classifyRigMigration(rigName, rigName, rigBeadsDir, ctx.TownRoot)

		// Check git cleanliness
		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Dir = rigPath
		output, err := cmd.Output()
		if err == nil {
			rigMigration.GitClean = len(strings.TrimSpace(string(output))) == 0
		}

		readiness.Rigs = append(readiness.Rigs, rigMigration)
		if rigMigration.NeedsMigration {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Rig %s: %s (backend: %s)", rigName, rigMigration.State, rigMigration.Backend))
		}
		if !rigMigration.GitClean {
			readiness.Ready = false
			readiness.Blockers = append(readiness.Blockers, fmt.Sprintf("Rig %s has uncommitted changes", rigName))
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
		Status:  StatusError,
		Message: fmt.Sprintf("%d rig(s) need migration, %d blocker(s)", needsMigration, len(readiness.Blockers)),
		Details: readiness.Blockers,
		FixHint: "Run 'bd migrate dolt' in each rig to migrate from SQLite to Dolt",
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
	if _, err := fmt.Sscanf(vParts[0], "%d", &major); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(vParts[1], "%d", &minor); err != nil {
		return false
	}

	// Dolt support added in 0.40.0
	return major > 0 || (major == 0 && minor >= 40)
}

// classifyRigMigration determines the migration state of a single rig's beads.
// rigName is the display name, doltDataName is the key in .dolt-data/ (e.g., "hq" for town root).
func classifyRigMigration(rigName, doltDataName, beadsDir, townRoot string) RigMigration {
	result := RigMigration{
		Name:     rigName,
		Backend:  "none",
		State:    StateNoBeads,
		GitClean: true,
		BeadsDir: beadsDir,
	}

	// Check if beads dir exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return result
	}

	// Read metadata.json
	backend := readBeadsBackend(beadsDir)
	result.Backend = backend

	// Check for dolt directory in .beads/
	if _, err := os.Stat(filepath.Join(beadsDir, "dolt")); err == nil {
		result.HasDoltDir = true
	}

	// Check for centralized dolt data in .dolt-data/<rig>/
	doltDataDir := filepath.Join(townRoot, ".dolt-data", doltDataName)
	if _, err := os.Stat(filepath.Join(doltDataDir, ".dolt")); err == nil {
		result.HasDoltData = true
	}

	// Check for SQLite database
	if _, err := os.Stat(filepath.Join(beadsDir, "beads.db")); err == nil {
		result.HasSQLite = true
	}

	// Check for JSONL bead files
	if _, err := os.Stat(filepath.Join(beadsDir, "issues.jsonl")); err == nil {
		result.HasJSONL = true
	}

	// Classify migration state
	hasDolt := result.HasDoltDir || result.HasDoltData

	switch {
	case backend == "dolt" && hasDolt:
		// Metadata says dolt and dolt database exists — fully migrated
		result.State = StateFullyMigrated
		result.NeedsMigration = false

	case backend == "dolt" && !hasDolt:
		// Metadata says dolt but no dolt database found — broken/incomplete
		result.State = StatePartiallyMigrated
		result.NeedsMigration = true

	case backend != "dolt" && hasDolt:
		// Dolt infrastructure exists but metadata not updated — partial
		result.State = StatePartiallyMigrated
		result.NeedsMigration = true

	case (result.HasSQLite || result.HasJSONL) && !hasDolt:
		// Data exists only in SQLite/JSONL, no dolt at all
		result.State = StateNeverMigrated
		result.NeedsMigration = true

	default:
		// Beads dir exists but no recognizable data
		result.State = StateNeverMigrated
		result.NeedsMigration = true
	}

	return result
}

// readBeadsBackend reads the backend field from metadata.json in a beads directory.
func readBeadsBackend(beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		// No metadata.json — check if dir has any content indicating sqlite
		if _, statErr := os.Stat(filepath.Join(beadsDir, "beads.db")); statErr == nil {
			return "sqlite"
		}
		return "unknown"
	}

	var metadata struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "unknown"
	}

	if metadata.Backend == "" {
		return "sqlite" // Default to SQLite if backend not specified
	}
	return metadata.Backend
}

// loadRigNames loads rig names from rigs.json.
func loadRigNames(rigsPath string) map[string]struct{} {
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
		JsonlExport  string `json:"jsonl_export"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return false
	}

	return metadata.Backend == "dolt" &&
		metadata.DoltMode == "server" &&
		metadata.DoltDatabase == expectedDB &&
		metadata.JsonlExport == "issues.jsonl"
}

// writeDoltMetadata writes dolt server config to a rig's metadata.json.
func (c *DoltMetadataCheck) writeDoltMetadata(townRoot, rigName string) error {
	// Use FindOrCreateRigBeadsDir to atomically resolve and create the directory,
	// avoiding the TOCTOU race in the stat-then-use pattern.
	beadsDir, err := c.findOrCreateRigBeadsDir(townRoot, rigName)
	if err != nil {
		return fmt.Errorf("resolving beads directory for rig %q: %w", rigName, err)
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

	// Always set jsonl_export to the canonical filename.
	// Historical migrations may have left stale values (e.g., "beads.jsonl").
	existing["jsonl_export"] = "issues.jsonl"

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	if err := util.AtomicWriteFile(metadataPath, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("writing metadata.json: %w", err)
	}

	return nil
}

// findRigBeadsDir delegates to the canonical read-only implementation in doltserver.
func (c *DoltMetadataCheck) findRigBeadsDir(townRoot, rigName string) string {
	return doltserver.FindRigBeadsDir(townRoot, rigName)
}

// findOrCreateRigBeadsDir delegates to the atomic resolve-and-create implementation.
func (c *DoltMetadataCheck) findOrCreateRigBeadsDir(townRoot, rigName string) (string, error) {
	return doltserver.FindOrCreateRigBeadsDir(townRoot, rigName)
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

// RigBackendStatusCheck detects rigs' backend status and classifies them as:
// - fully-migrated: Dolt backend, healthy
// - partially-migrated: some Dolt infrastructure but migration incomplete
// - never-migrated: SQLite backend, no Dolt presence
// Each non-OK finding includes actionable remediation steps.
type RigBackendStatusCheck struct {
	BaseCheck
}

// NewRigBackendStatusCheck creates a check that classifies rig backend status.
func NewRigBackendStatusCheck() *RigBackendStatusCheck {
	return &RigBackendStatusCheck{
		BaseCheck: BaseCheck{
			CheckName:        "rig-backend-status",
			CheckDescription: "Classify rig storage backends and detect unmigrated rigs",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks all rigs and classifies their migration state.
func (c *RigBackendStatusCheck) Run(ctx *CheckContext) *CheckResult {
	var neverMigrated []string
	var partiallyMigrated []string
	var fullyMigrated []string
	var details []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	if _, err := os.Stat(townBeadsDir); err == nil {
		rig := classifyRigMigration("town-root", "hq", townBeadsDir, ctx.TownRoot)
		switch rig.State {
		case StateNeverMigrated:
			neverMigrated = append(neverMigrated, rig.Name)
			details = append(details, formatRigFinding(rig))
		case StatePartiallyMigrated:
			partiallyMigrated = append(partiallyMigrated, rig.Name)
			details = append(details, formatRigFinding(rig))
		case StateFullyMigrated:
			fullyMigrated = append(fullyMigrated, rig.Name)
		}
	}

	// Find all rigs and check their beads
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigs := loadRigNames(rigsPath)
	for rigName := range rigs {
		rigBeadsDir := filepath.Join(ctx.TownRoot, rigName, "mayor", "rig", ".beads")
		rig := classifyRigMigration(rigName, rigName, rigBeadsDir, ctx.TownRoot)
		switch rig.State {
		case StateNeverMigrated:
			neverMigrated = append(neverMigrated, rig.Name)
			details = append(details, formatRigFinding(rig))
		case StatePartiallyMigrated:
			partiallyMigrated = append(partiallyMigrated, rig.Name)
			details = append(details, formatRigFinding(rig))
		case StateFullyMigrated:
			fullyMigrated = append(fullyMigrated, rig.Name)
		case StateNoBeads:
			// Skip — no beads to migrate
		}
	}

	totalIssues := len(neverMigrated) + len(partiallyMigrated)
	if totalIssues == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d rig(s) fully migrated to Dolt", len(fullyMigrated)),
		}
	}

	// Build summary message
	var parts []string
	if len(neverMigrated) > 0 {
		parts = append(parts, fmt.Sprintf("%d never-migrated", len(neverMigrated)))
	}
	if len(partiallyMigrated) > 0 {
		parts = append(parts, fmt.Sprintf("%d partially-migrated", len(partiallyMigrated)))
	}
	if len(fullyMigrated) > 0 {
		parts = append(parts, fmt.Sprintf("%d fully-migrated", len(fullyMigrated)))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: strings.Join(parts, ", "),
		Details: details,
		FixHint: "Run 'bd migrate dolt' in each rig to migrate from SQLite to Dolt",
	}
}

// formatRigFinding formats a single rig's migration finding with actionable detail.
func formatRigFinding(rig RigMigration) string {
	switch rig.State {
	case StateNeverMigrated:
		evidence := rig.Backend
		if rig.HasSQLite {
			evidence = "sqlite (beads.db present)"
		} else if rig.HasJSONL {
			evidence = "JSONL only (no database backend)"
		}
		return fmt.Sprintf("%s: never migrated — %s → run 'bd migrate dolt' in rig", rig.Name, evidence)
	case StatePartiallyMigrated:
		var reason string
		switch {
		case rig.Backend == "dolt" && !rig.HasDoltDir && !rig.HasDoltData:
			reason = "metadata says dolt but no dolt database found"
		case rig.Backend != "dolt" && rig.HasDoltData:
			reason = fmt.Sprintf("dolt-data exists but metadata says %s", rig.Backend)
		case rig.Backend != "dolt" && rig.HasDoltDir:
			reason = fmt.Sprintf("dolt/ dir exists but metadata says %s", rig.Backend)
		default:
			reason = "migration incomplete"
		}
		return fmt.Sprintf("%s: partially migrated — %s → run 'bd migrate dolt' to complete", rig.Name, reason)
	default:
		return fmt.Sprintf("%s: %s", rig.Name, rig.State)
	}
}

// NewUnmigratedRigCheck is kept for backward compatibility. It delegates to RigBackendStatusCheck.
func NewUnmigratedRigCheck() *RigBackendStatusCheck {
	return NewRigBackendStatusCheck()
}

// DoltServerReachableCheck detects the split-brain risk: metadata.json says
// dolt_mode=server but the Dolt server is not actually accepting connections.
// In this state, bd commands may silently create isolated local databases
// instead of connecting to the centralized server.
type DoltServerReachableCheck struct {
	BaseCheck
}

// NewDoltServerReachableCheck creates a check for split-brain risk detection.
func NewDoltServerReachableCheck() *DoltServerReachableCheck {
	return &DoltServerReachableCheck{
		BaseCheck: BaseCheck{
			CheckName:        "dolt-server-reachable",
			CheckDescription: "Check that Dolt server is reachable when server mode is configured",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if any rig has server-mode metadata but the server is unreachable.
func (c *DoltServerReachableCheck) Run(ctx *CheckContext) *CheckResult {
	// Find rigs configured for server mode
	serverRigs := c.findServerModeRigs(ctx.TownRoot)
	if len(serverRigs) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No rigs configured for Dolt server mode",
			Category: c.CheckCategory,
		}
	}

	// Server mode is configured — check if the server is actually reachable
	port := 3307 // default Dolt server port
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: fmt.Sprintf("SPLIT-BRAIN RISK: %d rig(s) configured for Dolt server mode but server unreachable at %s",
				len(serverRigs), addr),
			Details: []string{
				fmt.Sprintf("Rigs expecting server: %s", strings.Join(serverRigs, ", ")),
				"bd commands will fail or create isolated local databases",
				"This is the split-brain scenario — data written now may be invisible to the server later",
			},
			FixHint:  "Run 'gt dolt start' to start the Dolt server",
			Category: c.CheckCategory,
		}
	}
	_ = conn.Close()

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusOK,
		Message:  fmt.Sprintf("Dolt server reachable (%d rig(s) in server mode)", len(serverRigs)),
		Category: c.CheckCategory,
	}
}

// findServerModeRigs returns rig names whose metadata.json has dolt_mode=server.
func (c *DoltServerReachableCheck) findServerModeRigs(townRoot string) []string {
	var serverRigs []string

	// Check town-level beads (hq)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if c.hasServerModeMetadata(townBeadsDir) {
		serverRigs = append(serverRigs, "hq")
	}

	// Check rig-level beads
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigs := loadRigNames(rigsPath)
	for rigName := range rigs {
		// Check mayor/rig/.beads first (canonical), then rig/.beads
		beadsDir := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
		if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
			beadsDir = filepath.Join(townRoot, rigName, ".beads")
		}
		if c.hasServerModeMetadata(beadsDir) {
			serverRigs = append(serverRigs, rigName)
		}
	}

	return serverRigs
}

// hasServerModeMetadata reads metadata.json and checks if dolt_mode is "server".
func (c *DoltServerReachableCheck) hasServerModeMetadata(beadsDir string) bool {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return false
	}
	var metadata struct {
		DoltMode string `json:"dolt_mode"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return false
	}
	return metadata.DoltMode == "server"
}
