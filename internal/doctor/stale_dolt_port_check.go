package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StaleDoltPortCheck detects stale Dolt port files that point to wrong ports.
// This can cause bd commands to fail with "database not found" errors when
// they connect to the wrong Dolt server.
type StaleDoltPortCheck struct {
	FixableCheck
	stalePorts []stalePortInfo
}

type stalePortInfo struct {
	path      string
	port      int
	correctPort int
}

// NewStaleDoltPortCheck creates a new stale Dolt port check.
func NewStaleDoltPortCheck() *StaleDoltPortCheck {
	return &StaleDoltPortCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stale-dolt-port",
				CheckDescription: "Detect stale Dolt port files pointing to wrong ports",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks for stale Dolt port files.
func (c *StaleDoltPortCheck) Run(ctx *CheckContext) *CheckResult {
	c.stalePorts = nil
	
	// Get the correct port from the main Dolt config
	correctPort := c.getCorrectPort(ctx)
	if correctPort == 0 {
		correctPort = 3307 // default
	}

	// Find all dolt-server.port files
	portFiles := c.findPortFiles(ctx.TownRoot)
	
	var details []string
	for _, portFile := range portFiles {
		data, err := os.ReadFile(portFile)
		if err != nil {
			continue
		}

		portStr := strings.TrimSpace(string(data))
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		// Check if port matches the correct port
		if port != correctPort {
			c.stalePorts = append(c.stalePorts, stalePortInfo{
				path:       portFile,
				port:       port,
				correctPort: correctPort,
			})
			relPath, _ := filepath.Rel(ctx.TownRoot, portFile)
			details = append(details, fmt.Sprintf("Stale port file %s has port %d (should be %d)", relPath, port, correctPort))
		}
	}

	// Also check for stale dolt config directories with wrong ports
	staleConfigs := c.findStaleDoltConfigs(ctx.TownRoot, correctPort)
	for _, config := range staleConfigs {
		relPath, _ := filepath.Rel(ctx.TownRoot, config)
		details = append(details, fmt.Sprintf("Stale Dolt config directory: %s (contains wrong port configuration)", relPath))
	}

	if len(c.stalePorts) == 0 && len(staleConfigs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All Dolt port files are consistent",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d stale Dolt port file(s), %d stale config dir(s)", len(c.stalePorts), len(staleConfigs)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove stale port files and config directories",
	}
}

// Fix removes stale Dolt port files and config directories.
func (c *StaleDoltPortCheck) Fix(ctx *CheckContext) error {
	for _, info := range c.stalePorts {
		if err := os.Remove(info.path); err != nil {
			return fmt.Errorf("could not remove stale port file %s: %w", info.path, err)
		}
	}
	return nil
}

// getCorrectPort returns the port from the main Dolt server config.
func (c *StaleDoltPortCheck) getCorrectPort(ctx *CheckContext) int {
	// Check the main Dolt server config
	configPath := filepath.Join(ctx.TownRoot, ".dolt-data", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}

	// Parse port from config.yaml
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "port:" && i+1 < len(lines) {
			portStr := strings.TrimSpace(strings.TrimPrefix(lines[i+1], "port:"))
			port, err := strconv.Atoi(portStr)
			if err == nil {
				return port
			}
		}
		if strings.HasPrefix(line, "  port:") {
			portStr := strings.TrimSpace(strings.TrimPrefix(line, "  port:"))
			port, err := strconv.Atoi(portStr)
			if err == nil {
				return port
			}
		}
	}

	return 0
}

// findPortFiles finds all dolt-server.port files.
func (c *StaleDoltPortCheck) findPortFiles(townRoot string) []string {
	var files []string

	// Common locations for port files
	locations := []string{
		filepath.Join(townRoot, ".beads", "dolt-server.port"),
		filepath.Join(townRoot, ".dolt-data", ".beads", "dolt-server.port"),
		filepath.Join(townRoot, "daemon", "dolt.port"),
	}

	// Also find port files in rig .beads directories
	rigsDir := filepath.Join(townRoot, "mayor", "rigs.json")
	if _, err := os.Stat(rigsDir); err == nil {
		// Walk through directories looking for .beads/dolt-server.port
		filepath.Walk(townRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.Name() == "dolt-server.port" {
				files = append(files, path)
			}
			return nil
		})
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			files = append(files, loc)
		}
	}

	return files
}

// findStaleDoltConfigs finds stale Dolt config directories with wrong ports.
func (c *StaleDoltPortCheck) findStaleDoltConfigs(townRoot string, correctPort int) []string {
	var staleConfigs []string

	// Check for .beads/dolt/ directory which shouldn't exist when using shared Dolt server
	staleDir := filepath.Join(townRoot, ".beads", "dolt")
	if _, err := os.Stat(staleDir); err == nil {
		// Check if it has a config.yaml with wrong port
		configPath := filepath.Join(staleDir, "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			if strings.Contains(string(data), fmt.Sprintf("port: %d", 13761)) {
				staleConfigs = append(staleConfigs, staleDir)
			}
		}
	}

	return staleConfigs
}