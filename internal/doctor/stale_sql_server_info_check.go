package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// StaleSQLServerInfoCheck detects stale sql-server.info files left by crashed
// or stopped local Dolt servers. When running in dolt_mode=server, these files
// cause bd to connect to a dead local server instead of the central Dolt server,
// resulting in "database not found" errors. See GH#2770.
type StaleSQLServerInfoCheck struct {
	FixableCheck
	staleFiles []string
}

// NewStaleSQLServerInfoCheck creates a new stale sql-server.info check.
func NewStaleSQLServerInfoCheck() *StaleSQLServerInfoCheck {
	return &StaleSQLServerInfoCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-sql-server-info",
				CheckDescription: "Detect stale Dolt sql-server.info files from dead local servers",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks for stale sql-server.info files across all beads directories.
func (c *StaleSQLServerInfoCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleFiles = nil

	// Find all sql-server.info files under the town root
	var details []string
	_ = filepath.Walk(ctx.TownRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip .git directories and node_modules
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if info.Name() != "sql-server.info" {
			return nil
		}
		// Only care about files inside .dolt directories
		if !strings.Contains(path, ".dolt") {
			return nil
		}

		if c.isStale(path) {
			c.staleFiles = append(c.staleFiles, path)
			relPath, _ := filepath.Rel(ctx.TownRoot, path)
			details = append(details, fmt.Sprintf("Stale sql-server.info: %s", relPath))
		}
		return nil
	})

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
		Message: fmt.Sprintf("%d stale sql-server.info file(s) from dead Dolt servers", len(c.staleFiles)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove stale sql-server.info files",
	}
}

// Fix removes all detected stale sql-server.info files.
func (c *StaleSQLServerInfoCheck) Fix(ctx *CheckContext) error {
	for _, path := range c.staleFiles {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("could not remove stale sql-server.info %s: %w", path, err)
		}
	}
	return nil
}

// isStale checks if the sql-server.info file references a dead process.
// The file format is "PID:port:UUID" (one line).
func (c *StaleSQLServerInfoCheck) isStale(path string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from filepath.Walk
	if err != nil {
		return false
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return true // Empty file is stale
	}

	// Parse PID from "PID:port:UUID" format
	parts := strings.SplitN(content, ":", 3)
	if len(parts) < 1 {
		return true
	}

	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid <= 0 {
		return true // Corrupt or invalid PID
	}

	// Check if the process is alive using signal 0 (no-op probe)
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return true // Process is dead
	}

	return false // Process is alive, not stale
}
