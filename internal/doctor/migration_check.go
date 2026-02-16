package doctor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/util"
)

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
	return loadRigNames(rigsPath)
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
	// Find rigs configured for server mode, grouped by server address
	rigsByAddr := c.findServerModeRigsByAddr(ctx.TownRoot)
	if len(rigsByAddr) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No rigs configured for Dolt server mode",
			Category: c.CheckCategory,
		}
	}

	// Check connectivity to each unique server address
	var unreachable []string
	var details []string
	totalRigs := 0
	unreachableRigs := 0
	for addr, rigs := range rigsByAddr {
		totalRigs += len(rigs)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			unreachable = append(unreachable, addr)
			unreachableRigs += len(rigs)
			details = append(details, fmt.Sprintf("Server %s unreachable (rigs: %s)", addr, strings.Join(rigs, ", ")))
		} else {
			_ = conn.Close()
		}
	}

	if len(unreachable) > 0 {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: fmt.Sprintf("SPLIT-BRAIN RISK: %d rig(s) configured for Dolt server mode but server unreachable at %s",
				unreachableRigs, strings.Join(unreachable, ", ")),
			Details: append(details,
				"bd commands will fail or create isolated local databases",
				"This is the split-brain scenario — data written now may be invisible to the server later",
			),
			FixHint:  "Check dolt server connectivity or run 'gt dolt start' for local server",
			Category: c.CheckCategory,
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusOK,
		Message:  fmt.Sprintf("Dolt server reachable (%d rig(s) in server mode)", totalRigs),
		Category: c.CheckCategory,
	}
}

// findServerModeRigsByAddr returns rig names grouped by their configured server address.
// Rigs without explicit host/port fall back to the default local server (127.0.0.1:3307).
func (c *DoltServerReachableCheck) findServerModeRigsByAddr(townRoot string) map[string][]string {
	result := make(map[string][]string)

	// Check town-level beads (hq)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if addr, ok := c.getServerAddr(townBeadsDir); ok {
		result[addr] = append(result[addr], "hq")
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
		if addr, ok := c.getServerAddr(beadsDir); ok {
			result[addr] = append(result[addr], rigName)
		}
	}

	return result
}

// getServerAddr reads metadata.json and returns the configured server address if dolt_mode is "server".
// Returns the address string (host:port) and true if server mode is configured.
func (c *DoltServerReachableCheck) getServerAddr(beadsDir string) (string, bool) {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", false
	}
	var metadata struct {
		DoltMode       string `json:"dolt_mode"`
		DoltServerHost string `json:"dolt_server_host"`
		DoltServerPort int    `json:"dolt_server_port"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", false
	}
	if metadata.DoltMode != "server" {
		return "", false
	}

	host := metadata.DoltServerHost
	port := metadata.DoltServerPort
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = doltserver.DefaultPort
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), true
}

// DoltOrphanedDatabaseCheck detects databases in .dolt-data/ that are not
// referenced by any rig's metadata.json. These orphans waste disk space and
// are served unnecessarily by the Dolt server.
type DoltOrphanedDatabaseCheck struct {
	FixableCheck
	orphanNames []string // Cached during Run for use in Fix
}

// NewDoltOrphanedDatabaseCheck creates a new orphaned database check.
func NewDoltOrphanedDatabaseCheck() *DoltOrphanedDatabaseCheck {
	return &DoltOrphanedDatabaseCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "dolt-orphaned-databases",
				CheckDescription: "Detect orphaned databases in .dolt-data/",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks for orphaned databases.
func (c *DoltOrphanedDatabaseCheck) Run(ctx *CheckContext) *CheckResult {
	c.orphanNames = nil

	orphans, err := doltserver.FindOrphanedDatabases(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Could not check for orphaned databases: %v", err),
			Category: c.CheckCategory,
		}
	}

	if len(orphans) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No orphaned databases in .dolt-data/",
			Category: c.CheckCategory,
		}
	}

	details := make([]string, len(orphans))
	for i, o := range orphans {
		details[i] = fmt.Sprintf("Orphaned: %s (%s)", o.Name, formatBytes(o.SizeBytes))
		c.orphanNames = append(c.orphanNames, o.Name)
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d orphaned database(s) in .dolt-data/", len(orphans)),
		Details:  details,
		FixHint:  "Run 'gt dolt cleanup' to remove orphaned databases",
		Category: c.CheckCategory,
	}
}

// Fix removes orphaned databases.
func (c *DoltOrphanedDatabaseCheck) Fix(ctx *CheckContext) error {
	for _, name := range c.orphanNames {
		if err := doltserver.RemoveDatabase(ctx.TownRoot, name); err != nil {
			return fmt.Errorf("removing orphaned database %s: %w", name, err)
		}
	}
	return nil
}

// formatBytes returns a human-readable size string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
