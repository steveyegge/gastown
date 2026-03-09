package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/rig"
)

// RigConfigSyncCheck verifies that all registered rigs have a config.json file,
// Dolt database, and rig identity bead. This prevents issues where the daemon
// can't find the beads prefix to check docked/parked status.
type RigConfigSyncCheck struct {
	FixableCheck
	missingConfig    []string          // Rig names missing config.json
	prefixMismatches []prefixMismatch  // Prefix mismatches between config.json and registry
	missingRigBeads  []rigBeadInfo     // Rigs missing identity beads
	missingDoltDB    []string          // Rigs missing Dolt database
	missingPrefixCfg []string          // Rigs missing issue-prefix in config.yaml
}

type prefixMismatch struct {
	rigName        string
	configPrefix   string
	registryPrefix string
}

type rigBeadInfo struct {
	rigName string
	prefix  string
	gitURL  string
}

// NewRigConfigSyncCheck creates a new rig config sync check.
func NewRigConfigSyncCheck() *RigConfigSyncCheck {
	return &RigConfigSyncCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "rig-config-sync",
				CheckDescription: "Verify registered rigs have config.json, Dolt DB, and identity beads",
				CheckCategory:    CategoryConfig,
			},
		},
	}
}

// Run checks if all registered rigs have proper configuration.
func (c *RigConfigSyncCheck) Run(ctx *CheckContext) *CheckResult {
	rigsConfigPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not load rigs registry",
			Details: []string{err.Error()},
		}
	}

	c.missingConfig = nil
	c.prefixMismatches = nil
	c.missingRigBeads = nil
	c.missingDoltDB = nil
	c.missingPrefixCfg = nil
	var details []string

	for rigName, entry := range rigsConfig.Rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		configPath := filepath.Join(rigPath, "config.json")

		// Check if rig directory exists
		if _, err := os.Stat(rigPath); os.IsNotExist(err) {
			details = append(details, fmt.Sprintf("Registered rig %s directory does not exist", rigName))
			continue
		}

		// Get expected prefix
		expectedPrefix := ""
		if entry.BeadsConfig != nil {
			expectedPrefix = entry.BeadsConfig.Prefix
		}

		// Check if config.json exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			c.missingConfig = append(c.missingConfig, rigName)
			details = append(details, fmt.Sprintf("Rig %s is registered but missing config.json", rigName))
			continue
		}

		// Check if config.json has correct prefix
		rigCfg, err := rig.LoadRigConfig(rigPath)
		if err != nil {
			details = append(details, fmt.Sprintf("Rig %s has unreadable config.json: %v", rigName, err))
			continue
		}

		configPrefix := ""
		if rigCfg.Beads != nil {
			configPrefix = rigCfg.Beads.Prefix
		}

		// Compare prefixes
		if expectedPrefix != "" && configPrefix != "" && expectedPrefix != configPrefix {
			c.prefixMismatches = append(c.prefixMismatches, prefixMismatch{
				rigName:        rigName,
				configPrefix:   configPrefix,
				registryPrefix: expectedPrefix,
			})
			details = append(details, fmt.Sprintf(
				"Rig %s prefix mismatch: config.json has %q, registry has %q",
				rigName, configPrefix, expectedPrefix))
		}

		// Check beads configuration at mayor/rig/.beads
		mayorRigBeads := filepath.Join(rigPath, "mayor", "rig", ".beads")
		if _, err := os.Stat(mayorRigBeads); os.IsNotExist(err) {
			details = append(details, fmt.Sprintf("Rig %s is missing mayor/rig/.beads directory", rigName))
			continue
		}

		// Check issue-prefix in config.yaml
		configYamlPath := filepath.Join(mayorRigBeads, "config.yaml")
		if data, err := os.ReadFile(configYamlPath); err == nil {
			if !strings.Contains(string(data), "issue-prefix:") && expectedPrefix != "" {
				c.missingPrefixCfg = append(c.missingPrefixCfg, rigName)
				details = append(details, fmt.Sprintf("Rig %s .beads/config.yaml missing issue-prefix", rigName))
			}
		}

		// Check metadata.json for Dolt database
		metadataPath := filepath.Join(mayorRigBeads, "metadata.json")
		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			details = append(details, fmt.Sprintf("Rig %s is missing .beads/metadata.json", rigName))
			continue
		}

		// Check if Dolt database exists
		if configPrefix != "" {
			if !c.doltDatabaseExists(ctx, configPrefix) {
				c.missingDoltDB = append(c.missingDoltDB, rigName)
				details = append(details, fmt.Sprintf("Rig %s Dolt database '%s' not found on server", rigName, configPrefix))
			}
		}

		// Check if rig identity bead exists
		if configPrefix != "" {
			rigBeadID := fmt.Sprintf("%s-rig-%s", configPrefix, rigName)
			if !c.rigBeadExists(ctx, rigBeadID, rigPath) {
				c.missingRigBeads = append(c.missingRigBeads, rigBeadInfo{
					rigName: rigName,
					prefix:  configPrefix,
					gitURL:  entry.GitURL,
				})
				details = append(details, fmt.Sprintf("Rig %s is missing identity bead %s", rigName, rigBeadID))
			}
		}
	}

	// Check for summary
	issueCount := len(c.missingConfig) + len(c.prefixMismatches) + len(c.missingRigBeads) + len(c.missingDoltDB) + len(c.missingPrefixCfg)
	if issueCount == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All registered rigs have valid configuration",
		}
	}

	var parts []string
	if len(c.missingConfig) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing config.json", len(c.missingConfig)))
	}
	if len(c.prefixMismatches) > 0 {
		parts = append(parts, fmt.Sprintf("%d prefix mismatch(es)", len(c.prefixMismatches)))
	}
	if len(c.missingRigBeads) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing identity bead(s)", len(c.missingRigBeads)))
	}
	if len(c.missingDoltDB) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing Dolt DB(s)", len(c.missingDoltDB)))
	}
	if len(c.missingPrefixCfg) > 0 {
		parts = append(parts, fmt.Sprintf("%d missing issue-prefix", len(c.missingPrefixCfg)))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: strings.Join(parts, ", "),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to create missing config files and databases",
	}
}

// Fix creates missing config.json files, Dolt databases, and rig identity beads.
func (c *RigConfigSyncCheck) Fix(ctx *CheckContext) error {
	rigsConfigPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return fmt.Errorf("could not load rigs registry: %w", err)
	}

	// Fix missing config.json files
	for _, rigName := range c.missingConfig {
		entry, ok := rigsConfig.Rigs[rigName]
		if !ok {
			continue
		}

		rigPath := filepath.Join(ctx.TownRoot, rigName)
		configPath := filepath.Join(rigPath, "config.json")

		prefix := ""
		if entry.BeadsConfig != nil {
			prefix = entry.BeadsConfig.Prefix
		}

		rigCfg := &rig.RigConfig{
			Type:      "rig",
			Version:   1,
			Name:      rigName,
			GitURL:    entry.GitURL,
			CreatedAt: entry.AddedAt,
		}
		if prefix != "" {
			rigCfg.Beads = &rig.BeadsConfig{Prefix: prefix}
		}

		data, err := json.MarshalIndent(rigCfg, "", "  ")
		if err != nil {
			return fmt.Errorf("could not serialize config for %s: %w", rigName, err)
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("could not write config.json for %s: %w", rigName, err)
		}
	}

	// Fix missing issue-prefix in config.yaml
	for _, rigName := range c.missingPrefixCfg {
		entry, ok := rigsConfig.Rigs[rigName]
		if !ok || entry.BeadsConfig == nil {
			continue
		}

		rigPath := filepath.Join(ctx.TownRoot, rigName)
		configYamlPath := filepath.Join(rigPath, "mayor", "rig", ".beads", "config.yaml")

		// Read existing config
		data, err := os.ReadFile(configYamlPath)
		if err != nil {
			continue
		}

		// Add issue-prefix line if missing
		content := string(data)
		if !strings.Contains(content, "issue-prefix:") {
			newLine := fmt.Sprintf("\nissue-prefix: %q\n", entry.BeadsConfig.Prefix)
			// Find a good place to insert it
			if strings.Contains(content, "# issue-prefix:") {
				content = strings.Replace(content, "# issue-prefix: \"\"", fmt.Sprintf("issue-prefix: %q", entry.BeadsConfig.Prefix), 1)
			} else {
				content = content + newLine
			}
			if err := os.WriteFile(configYamlPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("could not update config.yaml for %s: %w", rigName, err)
			}
		}
	}

	// Fix missing Dolt databases by running bd init
	for _, rigName := range c.missingDoltDB {
		entry, ok := rigsConfig.Rigs[rigName]
		if !ok || entry.BeadsConfig == nil {
			continue
		}

		rigPath := filepath.Join(ctx.TownRoot, rigName)
		mayorRigPath := filepath.Join(rigPath, "mayor", "rig")

		// Run bd init --prefix <prefix> --force to create the database
		cmd := exec.Command("bd", "init", "--prefix", entry.BeadsConfig.Prefix, "--force")
		cmd.Dir = mayorRigPath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("could not initialize Dolt DB for %s: %w\n%s", rigName, err, string(output))
		}
	}

	// Fix missing rig identity beads
	for _, info := range c.missingRigBeads {
		rigPath := filepath.Join(ctx.TownRoot, info.rigName)
		mayorRigPath := filepath.Join(rigPath, "mayor", "rig")

		bd := beads.New(mayorRigPath)
		fields := &beads.RigFields{
			Repo:   info.gitURL,
			Prefix: info.prefix,
			State:  beads.RigStateActive,
		}

		if _, err := bd.CreateRigBead(info.rigName, fields); err != nil {
			return fmt.Errorf("could not create rig bead for %s: %w", info.rigName, err)
		}

		// Add status:docked label if the rig should be docked
		rigBeadID := fmt.Sprintf("%s-rig-%s", info.prefix, info.rigName)
		cmd := exec.Command("bd", "label", rigBeadID, "--add", "status:docked")
		cmd.Dir = mayorRigPath
		cmd.Run() // Best effort
	}

	return nil
}

// doltDatabaseExists checks if a Dolt database exists on the server.
func (c *RigConfigSyncCheck) doltDatabaseExists(ctx *CheckContext, dbName string) bool {
	// Check by trying to use bd dolt status
	cmd := exec.Command("bd", "dolt", "status", "--json")
	cmd.Dir = ctx.TownRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// Check if database name appears in the output
	return strings.Contains(string(output), dbName)
}

// rigBeadExists checks if a rig identity bead exists.
func (c *RigConfigSyncCheck) rigBeadExists(ctx *CheckContext, rigBeadID, rigPath string) bool {
	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")

	// Try to show the bead using bd
	cmd := exec.Command("bd", "show", rigBeadID, "--json")
	cmd.Dir = mayorRigPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// Check if the output contains the bead ID
	return strings.Contains(string(output), rigBeadID)
}