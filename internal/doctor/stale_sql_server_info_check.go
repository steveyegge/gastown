package doctor

import (
	"encoding/json"
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

	// Find sql-server.info files in known .beads/dolt/.dolt/ locations.
	// Avoids filepath.Walk over the entire town root, which is extremely slow
	// on Docker bind mounts (macOS VirtioFS).
	var details []string

	locations := []string{
		filepath.Join(ctx.TownRoot, ".beads", "dolt", ".dolt", "sql-server.info"),
	}

	// Collect rig names from rigs.json and top-level directories.
	rigNames := make(map[string]struct{})
	rigsConfig := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	if data, err := os.ReadFile(rigsConfig); err == nil {
		var rigs struct {
			Rigs map[string]struct{} `json:"rigs"`
		}
		if json.Unmarshal(data, &rigs) == nil {
			for name := range rigs.Rigs {
				rigNames[name] = struct{}{}
			}
		}
	}
	// Also scan top-level directories as fallback (handles rigs not yet in rigs.json).
	if entries, err := os.ReadDir(ctx.TownRoot); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				rigNames[e.Name()] = struct{}{}
			}
		}
	}
	for rigName := range rigNames {
		locations = append(locations,
			filepath.Join(ctx.TownRoot, rigName, ".beads", "dolt", ".dolt", "sql-server.info"),
		)
	}

	for _, path := range locations {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if c.isStale(path) {
			c.staleFiles = append(c.staleFiles, path)
			relPath, _ := filepath.Rel(ctx.TownRoot, path)
			details = append(details, fmt.Sprintf("Stale sql-server.info: %s", relPath))
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
		Message: fmt.Sprintf("%d stale sql-server.info file(s) from dead Dolt servers", len(c.staleFiles)),
		Details: details,
		FixHint: "Restart the Dolt server to clear stale sql-server.info files (Dolt writes and cleans these itself)",
	}
}

// Fix is a no-op. sql-server.info is a Dolt-internal file written and managed
// by Dolt itself (see dolt/commands/sqlserver/creds.go). Restarting the Dolt
// server will create a fresh one.
//
// WARNING: DO NOT remove, delete, or modify files inside Dolt's .dolt/
// directory — including noms/LOCK files. These are Dolt-internal files.
// Removing them WILL cause unrecoverable data corruption and data loss.
// Dolt manages these files itself; external interference is never safe.
func (c *StaleSQLServerInfoCheck) Fix(ctx *CheckContext) error {
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
