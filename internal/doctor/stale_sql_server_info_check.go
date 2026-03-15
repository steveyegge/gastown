package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/steveyegge/gastown/internal/config"
)

// StaleSQLServerInfoCheck detects stale sql-server.info files in .beads/dolt/.dolt/
// directories. These files are written by Dolt when starting a SQL server and contain
// "PID:PORT:UUID". In server mode, a stale copy causes bd to refuse to connect,
// producing "database not found" errors even though the central Dolt server is healthy.
type StaleSQLServerInfoCheck struct {
	FixableCheck
	staleFiles []staleInfoFile
}

type staleInfoFile struct {
	path string
	pid  int
}

// NewStaleSQLServerInfoCheck creates a new stale sql-server.info check.
func NewStaleSQLServerInfoCheck() *StaleSQLServerInfoCheck {
	return &StaleSQLServerInfoCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-sql-server-info",
				CheckDescription: "Detect stale sql-server.info files in beads directories",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks for stale sql-server.info files across all beads directories.
func (c *StaleSQLServerInfoCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleFiles = nil

	var details []string

	// Collect all beads directories to check
	beadsDirs := findAllBeadsDirs(ctx.TownRoot)

	for _, beadsDir := range beadsDirs {
		infoPath := filepath.Join(beadsDir, "dolt", ".dolt", "sql-server.info")
		data, err := os.ReadFile(infoPath) //nolint:gosec // G304: path is constructed internally
		if err != nil {
			continue // No info file
		}

		// Format: "PID:PORT:UUID"
		parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 3)
		if len(parts) < 1 {
			c.staleFiles = append(c.staleFiles, staleInfoFile{path: infoPath, pid: 0})
			relPath, _ := filepath.Rel(ctx.TownRoot, infoPath)
			details = append(details, fmt.Sprintf("Malformed sql-server.info: %s", relPath))
			continue
		}

		pid, err := strconv.Atoi(parts[0])
		if err != nil || pid <= 0 {
			c.staleFiles = append(c.staleFiles, staleInfoFile{path: infoPath, pid: 0})
			relPath, _ := filepath.Rel(ctx.TownRoot, infoPath)
			details = append(details, fmt.Sprintf("Malformed sql-server.info: %s", relPath))
			continue
		}

		// Check if the process is alive
		proc, err := os.FindProcess(pid)
		if err != nil {
			c.staleFiles = append(c.staleFiles, staleInfoFile{path: infoPath, pid: pid})
			relPath, _ := filepath.Rel(ctx.TownRoot, infoPath)
			details = append(details, fmt.Sprintf("Stale sql-server.info (PID %d, process not found): %s", pid, relPath))
			continue
		}

		if err := proc.Signal(syscall.Signal(0)); err != nil {
			c.staleFiles = append(c.staleFiles, staleInfoFile{path: infoPath, pid: pid})
			relPath, _ := filepath.Rel(ctx.TownRoot, infoPath)
			details = append(details, fmt.Sprintf("Stale sql-server.info (PID %d, process dead): %s", pid, relPath))
		}
	}

	if len(c.staleFiles) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale sql-server.info files found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d stale sql-server.info file(s)", len(c.staleFiles)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove stale sql-server.info files",
	}
}

// Fix removes stale sql-server.info files.
func (c *StaleSQLServerInfoCheck) Fix(ctx *CheckContext) error {
	for _, info := range c.staleFiles {
		if err := os.Remove(info.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove %s: %w", info.path, err)
		}
	}
	return nil
}

// findAllBeadsDirs returns all .beads directories in the town.
func findAllBeadsDirs(townRoot string) []string {
	var dirs []string

	// Town-level .beads
	townBeads := filepath.Join(townRoot, ".beads")
	if info, err := os.Stat(townBeads); err == nil && info.IsDir() {
		dirs = append(dirs, townBeads)
	}

	// Rig-level .beads directories
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return dirs
	}

	for rigName := range rigsConfig.Rigs {
		rigBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
		if info, err := os.Stat(rigBeads); err == nil && info.IsDir() {
			dirs = append(dirs, rigBeads)
		}
	}

	return dirs
}
