// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var configZombieCmd = &cobra.Command{
	Use:   "zombie",
	Short: "Manage zombie polecat auto-cleanup settings",
	Long: `Manage zombie polecat auto-cleanup settings.

When enabled, the deacon will automatically nuke polecats that have been
idle for longer than the threshold with no hooked work.

Examples:
  gt config zombie                     # Show current settings
  gt config zombie --enable            # Enable auto-cleanup
  gt config zombie --disable           # Disable auto-cleanup
  gt config zombie --threshold=2h      # Set idle threshold
  gt config zombie --protect=max,dev   # Set protected polecats`,
	RunE: runConfigZombie,
}

var (
	zombieEnable    bool
	zombieDisable   bool
	zombieThreshold string
	zombieProtect   string
)

func init() {
	configZombieCmd.Flags().BoolVar(&zombieEnable, "enable", false, "Enable zombie auto-cleanup")
	configZombieCmd.Flags().BoolVar(&zombieDisable, "disable", false, "Disable zombie auto-cleanup")
	configZombieCmd.Flags().StringVar(&zombieThreshold, "threshold", "", "Idle threshold (e.g., 2h, 30m)")
	configZombieCmd.Flags().StringVar(&zombieProtect, "protect", "", "Comma-separated list of protected polecat names")

	configCmd.AddCommand(configZombieCmd)
}

func runConfigZombie(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load mayor config
	mayorConfigPath := config.MayorConfigPath(townRoot)
	mayorConfig, err := config.LoadOrCreateMayorConfig(mayorConfigPath)
	if err != nil {
		return fmt.Errorf("loading mayor config: %w", err)
	}

	// Ensure deacon config exists
	if mayorConfig.Deacon == nil {
		mayorConfig.Deacon = &config.DeaconConfig{}
	}
	if mayorConfig.Deacon.Zombie == nil {
		mayorConfig.Deacon.Zombie = &config.ZombieConfig{}
	}

	zombie := mayorConfig.Deacon.Zombie
	modified := false

	// Handle flags
	if zombieEnable && zombieDisable {
		return fmt.Errorf("cannot use both --enable and --disable")
	}

	if zombieEnable {
		zombie.AutoCleanup = true
		modified = true
		fmt.Printf("%s Zombie auto-cleanup enabled\n", style.Green.Render("✓"))
	}

	if zombieDisable {
		zombie.AutoCleanup = false
		modified = true
		fmt.Printf("%s Zombie auto-cleanup disabled\n", style.Yellow.Render("•"))
	}

	if zombieThreshold != "" {
		// Validate threshold
		if _, err := time.ParseDuration(zombieThreshold); err != nil {
			return fmt.Errorf("invalid threshold duration: %w", err)
		}
		zombie.IdleThreshold = zombieThreshold
		modified = true
		fmt.Printf("%s Idle threshold set to %s\n", style.Green.Render("✓"), zombieThreshold)
	}

	if zombieProtect != "" {
		if zombieProtect == "-" || zombieProtect == "none" {
			zombie.Protected = nil
		} else {
			zombie.Protected = splitAndTrim(zombieProtect)
		}
		modified = true
		if len(zombie.Protected) > 0 {
			fmt.Printf("%s Protected polecats: %v\n", style.Green.Render("✓"), zombie.Protected)
		} else {
			fmt.Printf("%s Cleared protected polecats list\n", style.Yellow.Render("•"))
		}
	}

	// Save if modified
	if modified {
		if err := config.SaveMayorConfig(mayorConfigPath, mayorConfig); err != nil {
			return fmt.Errorf("saving mayor config: %w", err)
		}
	}

	// Show current settings if no flags or after modification
	if !modified || cmd.Flags().NFlag() > 0 {
		fmt.Printf("\n%s\n\n", style.Bold.Render("Zombie Auto-Cleanup Settings"))

		enabled := "disabled"
		if zombie.AutoCleanup {
			enabled = style.Green.Render("enabled")
		} else {
			enabled = style.Dim.Render("disabled")
		}
		fmt.Printf("  Auto-cleanup: %s\n", enabled)

		threshold := zombie.IdleThreshold
		if threshold == "" {
			threshold = "2h (default)"
		}
		fmt.Printf("  Idle threshold: %s\n", threshold)

		if len(zombie.Protected) > 0 {
			fmt.Printf("  Protected: %v\n", zombie.Protected)
		} else {
			fmt.Printf("  Protected: %s\n", style.Dim.Render("none"))
		}
	}

	return nil
}

func splitAndTrim(s string) []string {
	var result []string
	for _, part := range splitComma(s) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitComma(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
