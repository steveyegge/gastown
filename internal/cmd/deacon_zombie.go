// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var deaconCleanupZombiesCmd = &cobra.Command{
	Use:   "cleanup-zombies",
	Short: "Auto-cleanup zombie polecats based on config",
	Long: `Automatically cleanup zombie polecats based on configured thresholds.

This command checks all polecats across all rigs and nukes those that:
1. Have been idle longer than the configured threshold
2. Have no hooked work (no active bead assignment)
3. Are not in the protected list

Configure settings with: gt config zombie --enable --threshold=2h

Examples:
  gt deacon cleanup-zombies           # Run cleanup
  gt deacon cleanup-zombies --dry-run # Show what would be cleaned`,
	RunE: runDeaconCleanupZombies,
}

var cleanupZombiesDryRun bool

func init() {
	deaconCleanupZombiesCmd.Flags().BoolVar(&cleanupZombiesDryRun, "dry-run", false, "Show what would be cleaned without actually cleaning")
	deaconCmd.AddCommand(deaconCleanupZombiesCmd)
}

func runDeaconCleanupZombies(cmd *cobra.Command, args []string) error {
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

	// Check if auto-cleanup is enabled
	if mayorConfig.Deacon == nil || mayorConfig.Deacon.Zombie == nil || !mayorConfig.Deacon.Zombie.AutoCleanup {
		fmt.Printf("%s Zombie auto-cleanup is disabled\n", style.Dim.Render("•"))
		fmt.Println("Enable with: gt config zombie --enable")
		return nil
	}

	zombie := mayorConfig.Deacon.Zombie

	// Parse threshold
	threshold := 2 * time.Hour // default
	if zombie.IdleThreshold != "" {
		parsed, err := time.ParseDuration(zombie.IdleThreshold)
		if err != nil {
			return fmt.Errorf("invalid idle threshold: %w", err)
		}
		threshold = parsed
	}

	// Build protected map
	protected := make(map[string]bool)
	for _, name := range zombie.Protected {
		protected[name] = true
	}

	fmt.Printf("%s Scanning for zombie polecats (threshold: %s)\n\n",
		style.Bold.Render("🧟"),
		threshold)

	// Find all rigs
	rigEntries, err := findRigs(townRoot)
	if err != nil {
		return fmt.Errorf("finding rigs: %w", err)
	}

	cleaned := 0
	skipped := 0

	for _, rigPath := range rigEntries {
		rigName := filepath.Base(rigPath)
		polecatsDir := filepath.Join(rigPath, "polecats")

		entries, err := os.ReadDir(polecatsDir)
		if err != nil {
			continue // No polecats directory
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			polecatName := entry.Name()
			polecatPath := filepath.Join(polecatsDir, polecatName)

			// Check if protected
			if protected[polecatName] {
				fmt.Printf("  %s %s/%s (protected)\n",
					style.Dim.Render("•"),
					rigName,
					polecatName)
				skipped++
				continue
			}

			// Check if has hooked work
			hookFile := filepath.Join(polecatPath, ".beads", "hook.json")
			if _, err := os.Stat(hookFile); err == nil {
				// Has hooked work, skip
				skipped++
				continue
			}

			// Check idle time (using activity file or directory mtime)
			activityFile := filepath.Join(polecatPath, ".beads", "activity.json")
			var lastActivity time.Time

			if info, err := os.Stat(activityFile); err == nil {
				lastActivity = info.ModTime()
			} else if info, err := os.Stat(polecatPath); err == nil {
				lastActivity = info.ModTime()
			} else {
				continue
			}

			idleTime := time.Since(lastActivity)
			if idleTime < threshold {
				skipped++
				continue
			}

			// This is a zombie - cleanup
			fmt.Printf("  %s %s/%s (idle %s)\n",
				style.Yellow.Render("→"),
				rigName,
				polecatName,
				formatDuration(idleTime))

			if !cleanupZombiesDryRun {
				// Nuke the polecat
				pm := polecat.NewManager(rigPath)
				if err := pm.Nuke(polecatName, true); err != nil {
					fmt.Printf("    %s Failed to nuke: %v\n", style.Red.Render("✗"), err)
				} else {
					fmt.Printf("    %s Nuked\n", style.Green.Render("✓"))
					cleaned++
				}
			} else {
				fmt.Printf("    %s Would nuke (dry-run)\n", style.Dim.Render("•"))
				cleaned++
			}
		}
	}

	fmt.Println()
	if cleanupZombiesDryRun {
		fmt.Printf("%s Dry run: %d zombies found, %d skipped\n",
			style.Yellow.Render("⚠"),
			cleaned,
			skipped)
	} else if cleaned > 0 {
		fmt.Printf("%s Cleaned %d zombies, %d skipped\n",
			style.Green.Render("✓"),
			cleaned,
			skipped)
	} else {
		fmt.Printf("%s No zombies found (%d polecats checked)\n",
			style.Green.Render("✓"),
			skipped)
	}

	return nil
}

func findRigs(townRoot string) ([]string, error) {
	var rigs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if it looks like a rig (has polecats or refinery dir)
		rigPath := filepath.Join(townRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(rigPath, "polecats")); err == nil {
			rigs = append(rigs, rigPath)
		} else if _, err := os.Stat(filepath.Join(rigPath, "refinery")); err == nil {
			rigs = append(rigs, rigPath)
		}
	}

	return rigs, nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
