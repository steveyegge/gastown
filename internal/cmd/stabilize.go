package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	stabilizeNoRestart bool
	stabilizeCrew      bool
	stabilizeVerbose   bool
)

var stabilizeCmd = &cobra.Command{
	Use:     "stabilize",
	GroupID: GroupDiag,
	Short:   "Stabilize Gas Town after building from source",
	Long: `Stabilize runs post-update recovery after building gastown from source.

This command:
1. Syncs gastown worktrees (mayor/rig, crew dirs) with origin
2. Restarts the daemon to pick up the new binary
3. Runs 'gt doctor --fix' to catch and repair issues
4. Optionally spawns a crew agent for issues that can't be auto-fixed

Use after 'make install' to ensure everything is consistent.

Requires gastown-src to be configured (automatically set by 'make install').
The $GASTOWN_SRC environment variable takes precedence if set.`,
	Example: `  # After building from source
  make install && gt stabilize

  # Testing a fix (don't restart daemon)
  make install && gt stabilize --no-restart

  # Full stabilization with crew assistance
  gt stabilize --crew

  # Just run doctor (for go install users)
  gt doctor --fix`,
	RunE: runStabilize,
}

func init() {
	stabilizeCmd.Flags().BoolVar(&stabilizeNoRestart, "no-restart", false,
		"Don't restart daemon (use when testing against current broken state)")
	stabilizeCmd.Flags().BoolVar(&stabilizeCrew, "crew", false,
		"Spawn crew agent to resolve issues doctor can't auto-fix")
	stabilizeCmd.Flags().BoolVarP(&stabilizeVerbose, "verbose", "v", false,
		"Show detailed output")
	rootCmd.AddCommand(stabilizeCmd)
}

func runStabilize(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// 1. Resolve gastown source directory
	gastownSrc, err := resolveGastownSrc(townRoot)
	if err != nil {
		return err
	}

	fmt.Printf("%s Using gastown source: %s\n", style.Success.Render("✓"), gastownSrc)

	// 2. Detect version change
	installedVersion, err := getInstalledGtVersion()
	if err != nil {
		fmt.Printf("%s Could not detect installed version: %v\n", style.Warning.Render("⚠"), err)
	} else {
		currentVersion := Version // From version.go
		// Compare just the core version (strip -dirty suffix for comparison)
		installedCore := strings.TrimSuffix(installedVersion, "-dirty")
		currentCore := strings.TrimSuffix(currentVersion, "-dirty")

		if installedCore != currentCore {
			fmt.Printf("%s Version change: %s → %s\n",
				style.Info.Render("ℹ"), currentVersion, installedVersion)
		} else {
			fmt.Printf("%s Version: %s\n", style.Success.Render("✓"), installedVersion)
		}
	}

	// 3. Sync worktrees
	if err := syncGastownWorktrees(townRoot, gastownSrc, stabilizeVerbose); err != nil {
		fmt.Printf("%s Worktree sync warning: %v\n", style.Warning.Render("⚠"), err)
		// Continue anyway - this is non-fatal
	} else {
		fmt.Printf("%s Worktrees synced\n", style.Success.Render("✓"))
	}

	// 4. Restart daemon (unless --no-restart)
	if stabilizeNoRestart {
		fmt.Printf("%s Skipping daemon restart (--no-restart)\n", style.Info.Render("ℹ"))
		fmt.Println("  Run 'gt daemon restart' when ready to pick up new binary")
	} else {
		if err := restartDaemonForStabilize(); err != nil {
			fmt.Printf("%s Daemon restart failed: %v\n", style.Warning.Render("⚠"), err)
			// Continue anyway
		} else {
			fmt.Printf("%s Daemon restarted\n", style.Success.Render("✓"))
		}
	}

	// 5. Run doctor --fix
	fmt.Println("\nRunning gt doctor --fix...")
	issues, err := runDoctorFixForStabilize(stabilizeVerbose)
	if err != nil {
		fmt.Printf("%s Doctor encountered errors: %v\n", style.Warning.Render("⚠"), err)
		// Continue to show issues
	}

	// 6. Handle remaining issues
	if len(issues) == 0 {
		fmt.Printf("\n%s Stabilization complete\n", style.Success.Render("✓"))
		return nil
	}

	fmt.Printf("\n%s %d issue(s) may need attention:\n", style.Warning.Render("⚠"), len(issues))
	for i, issue := range issues {
		fmt.Printf("  %d. %s\n", i+1, issue)
	}

	if stabilizeCrew {
		fmt.Println("\nSpawning crew agent to resolve remaining issues...")
		return spawnCrewForStabilization(townRoot, gastownSrc, issues)
	}

	fmt.Println("\nRun 'gt stabilize --crew' to spawn a crew agent for assistance.")
	return nil
}

// resolveGastownSrc returns the gastown source directory path.
// Resolution order: $GASTOWN_SRC env var > gt config gastown-src
func resolveGastownSrc(townRoot string) (string, error) {
	// 1. Check environment variable
	if envSrc := os.Getenv("GASTOWN_SRC"); envSrc != "" {
		if err := validateGastownSrc(envSrc); err != nil {
			return "", fmt.Errorf("$GASTOWN_SRC is invalid: %w", err)
		}
		return envSrc, nil
	}

	// 2. Check config
	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return "", err
	}
	if settings != nil && settings.GastownSrc != "" {
		if err := validateGastownSrc(settings.GastownSrc); err != nil {
			return "", fmt.Errorf("gastown-src config is invalid: %w", err)
		}
		return settings.GastownSrc, nil
	}

	// 3. Not configured
	return "", fmt.Errorf(`gastown source directory not configured

If you build gastown from source, 'make install' sets this automatically.
To set it manually:
  gt config gastown-src /path/to/your/gastown

If you installed via 'go install', you don't need 'gt stabilize'.
Just run: gt doctor --fix`)
}

// validateGastownSrc checks that the path looks like a gastown source directory.
func validateGastownSrc(path string) error {
	if _, err := os.Stat(filepath.Join(path, "cmd", "gt", "main.go")); err != nil {
		return fmt.Errorf("cmd/gt/main.go not found in %s", path)
	}
	if _, err := os.Stat(filepath.Join(path, "Makefile")); err != nil {
		return fmt.Errorf("Makefile not found in %s", path)
	}
	return nil
}

// getInstalledGtVersion runs the installed gt binary and returns its version.
// Returns just the version string (e.g., "v0.5.0-214-g13461161") without prefix or suffix.
func getInstalledGtVersion() (string, error) {
	// Find gt in PATH (the newly installed one)
	gtPath, err := exec.LookPath("gt")
	if err != nil {
		return "", err
	}

	cmd := exec.Command(gtPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse version from output like "gt version v0.5.0-214-g13461161-dirty (dev: 13461161)"
	// Extract just the version part (v0.5.0-...)
	output := strings.TrimSpace(string(out))
	parts := strings.Fields(output)
	for _, part := range parts {
		if strings.HasPrefix(part, "v") && strings.Contains(part, ".") {
			return part, nil
		}
	}
	// Fallback: return full output if we can't parse it
	return output, nil
}

// syncGastownWorktrees syncs gastown worktrees with origin.
func syncGastownWorktrees(townRoot, gastownSrc string, verbose bool) error {
	// Get current branch in source
	cmd := exec.Command("git", "-C", gastownSrc, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	branch := strings.TrimSpace(string(branchOut))

	// Push to origin (in case local commits)
	if verbose {
		fmt.Printf("  Pushing %s to origin...\n", branch)
	}
	cmd = exec.Command("git", "-C", gastownSrc, "push", "origin", branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Non-fatal, might be up to date or no push access
		if verbose {
			fmt.Printf("  Push: %s\n", strings.TrimSpace(string(out)))
		}
	}

	// Find gastown rig if it exists
	gastownRig := filepath.Join(townRoot, "gastown")
	if _, err := os.Stat(gastownRig); err == nil {
		// Sync mayor/rig
		mayorRig := filepath.Join(gastownRig, "mayor", "rig")
		if _, err := os.Stat(mayorRig); err == nil {
			if verbose {
				fmt.Printf("  Syncing gastown/mayor/rig...\n")
			}
			cmd = exec.Command("git", "-C", mayorRig, "pull", "origin", branch, "--ff-only")
			if out, err := cmd.CombinedOutput(); err != nil {
				if verbose {
					fmt.Printf("    Warning: %s\n", strings.TrimSpace(string(out)))
				}
			}
		}

		// Sync crew worktrees
		crewDir := filepath.Join(gastownRig, "crew")
		if entries, err := os.ReadDir(crewDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				worktree := filepath.Join(crewDir, entry.Name())
				// Check if it's a git worktree/repo
				gitPath := filepath.Join(worktree, ".git")
				if _, err := os.Stat(gitPath); err != nil {
					continue
				}
				if verbose {
					fmt.Printf("  Syncing gastown/crew/%s...\n", entry.Name())
				}
				cmd = exec.Command("git", "-C", worktree, "pull", "origin", branch, "--ff-only")
				if out, err := cmd.CombinedOutput(); err != nil {
					if verbose {
						fmt.Printf("    Warning: %s\n", strings.TrimSpace(string(out)))
					}
				}
			}
		}
	}

	return nil
}

// restartDaemonForStabilize stops and starts the daemon.
func restartDaemonForStabilize() error {
	// Stop daemon
	cmd := exec.Command("gt", "daemon", "stop")
	_ = cmd.Run() // Ignore error if not running

	// Start daemon
	cmd = exec.Command("gt", "daemon", "start")
	return cmd.Run()
}

// runDoctorFixForStabilize runs gt doctor --fix and returns issues that might need attention.
func runDoctorFixForStabilize(verbose bool) ([]string, error) {
	args := []string{"doctor", "--fix"}
	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command("gt", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Always show output in verbose mode, or if there was an error
	if verbose || err != nil {
		if stdout.Len() > 0 {
			fmt.Print(stdout.String())
		}
		if stderr.Len() > 0 {
			fmt.Print(stderr.String())
		}
	}

	// Parse output for issues that might need attention
	var issues []string
	output := stdout.String()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Look for failure indicators
		if strings.Contains(line, "FAIL") ||
			strings.Contains(line, "could not") ||
			strings.Contains(line, "manual") ||
			strings.Contains(line, "✗") {
			if line != "" && !strings.HasPrefix(line, "Running") {
				issues = append(issues, line)
			}
		}
	}

	return issues, err
}

// spawnCrewForStabilization spawns a crew agent to fix remaining issues.
func spawnCrewForStabilization(townRoot, gastownSrc string, issues []string) error {
	// Check if gastown is a rig
	gastownRig := filepath.Join(townRoot, "gastown")
	if _, err := os.Stat(gastownRig); err != nil {
		return fmt.Errorf("gastown is not configured as a rig in this town; cannot spawn crew agent\n" +
			"To add gastown as a rig: gt rig add gastown <git-url>")
	}

	// Create a bead for the stabilization work
	issueList := strings.Join(issues, "\n  - ")
	description := fmt.Sprintf(`Post-update stabilization: resolve issues that gt doctor --fix could not auto-fix.

Issues to resolve:
  - %s

Context:
- This follows a gastown update (make install && gt stabilize)
- The binary has been updated and daemon restarted
- Doctor ran but these issues need manual intervention

For each issue:
1. Investigate the root cause
2. Apply the fix
3. Verify with gt doctor

When complete, run 'gt doctor' to confirm all issues are resolved.`, issueList)

	// Use bd create to make the issue
	cmd := exec.Command("bd", "create",
		"--title", "Post-update stabilization",
		"--type", "task",
		"--description", description,
	)
	cmd.Dir = gastownSrc
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("creating stabilization bead: %w", err)
	}
	beadID := strings.TrimSpace(string(out))
	if beadID == "" {
		return fmt.Errorf("bd create returned empty bead ID")
	}

	// Sling to gastown crew
	fmt.Printf("Created bead %s\n", beadID)
	fmt.Println("Spawning crew agent...")

	cmd = exec.Command("gt", "sling", beadID, "gastown")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
