// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/ui"
	"github.com/steveyegge/gastown/internal/version"
	"github.com/steveyegge/gastown/internal/workspace"
)

var rootCmd = &cobra.Command{
	Use:     "gt",
	Short:   "Gas Town - Multi-agent workspace manager",
	Version: Version,
	Long: `Gas Town (gt) manages multi-agent workspaces called rigs.

It coordinates agent spawning, work distribution, and communication
across distributed teams of AI agents working on shared codebases.`,
	PersistentPreRunE: persistentPreRun,
}

// Commands that don't require beads to be installed/checked.
// NOTE: Gas Town has migrated to Dolt for beads storage. The bd version
// check is obsolete. Exempt all common commands.
var beadsExemptCommands = map[string]bool{
	"version":    true,
	"help":       true,
	"completion": true,
	"crew":       true,
	"polecat":    true,
	"witness":    true,
	"refinery":   true,
	"status":     true,
	"mail":       true,
	"hook":       true,
	"prime":      true,
	"nudge":      true,
	"seance":     true,
	"doctor":     true,
	"dolt":       true,
	"handoff":    true,
	"costs":      true,
	"feed":       true,
	"rig":        true,
	"config":     true,
	"install":    true,
	"tap":        true,
	"dnd":        true,
	"krc":        true, // KRC doesn't require beads
}

// Commands exempt from the town root branch warning.
// These are commands that help fix the problem or are diagnostic.
var branchCheckExemptCommands = map[string]bool{
	"version":    true,
	"help":       true,
	"completion": true,
	"doctor":     true, // Used to fix the problem
	"install":    true, // Initial setup
	"git-init":   true, // Git setup
}

// persistentPreRun runs before every command.
func persistentPreRun(cmd *cobra.Command, args []string) error {
	// Check if binary was built properly (via make build, not raw go build).
	// Raw go build produces unsigned binaries that macOS will kill.
	if BuiltProperly == "" {
		fmt.Fprintln(os.Stderr, "ERROR: This binary was built with 'go build' directly.")
		fmt.Fprintln(os.Stderr, "       Use 'make build' to create a properly signed binary.")
		if gtRoot := os.Getenv("GT_ROOT"); gtRoot != "" {
			fmt.Fprintf(os.Stderr, "       Run from: %s\n", gtRoot)
		}
		os.Exit(1)
	}

	// Initialize CLI theme (dark/light mode support)
	initCLITheme()

	// Get the root command name being run
	cmdName := cmd.Name()

	// Check for stale binary (warning only, doesn't block)
	if !beadsExemptCommands[cmdName] {
		checkStaleBinaryWarning()
	}

	// Check town root branch (warning only, non-blocking)
	if !branchCheckExemptCommands[cmdName] {
		warnIfTownRootOffMain()
	}

	// Skip beads check for exempt commands
	if beadsExemptCommands[cmdName] {
		return nil
	}

	// Check beads version
	return CheckBeadsVersion()
}

// initCLITheme initializes the CLI color theme based on settings and environment.
func initCLITheme() {
	// Try to load town settings for CLITheme config
	var configTheme string
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		settingsPath := config.TownSettingsPath(townRoot)
		if settings, err := config.LoadOrCreateTownSettings(settingsPath); err == nil {
			configTheme = settings.CLITheme
		}
	}

	// Initialize theme with config value (env var takes precedence inside InitTheme)
	ui.InitTheme(configTheme)
	ui.ApplyThemeMode()
}

// warnIfTownRootOffMain prints a warning if the town root is not on main branch.
// This is a non-blocking warning to help catch accidental branch switches.
func warnIfTownRootOffMain() {
	// Find town root (silently - don't error if not in workspace)
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return
	}

	// Check if it's a git repo
	gitDir := townRoot + "/.git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return
	}

	// Get current branch
	gitCmd := exec.Command("git", "branch", "--show-current")
	gitCmd.Dir = townRoot
	out, err := gitCmd.Output()
	if err != nil {
		return
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "main" || branch == "master" {
		return
	}

	// Town root is on wrong branch - warn the user
	fmt.Fprintf(os.Stderr, "\n%s Town root is on branch '%s' (should be 'main')\n",
		style.Bold.Render("⚠️  WARNING:"), branch)
	fmt.Fprintf(os.Stderr, "   This can cause gt commands to fail. Run: %s\n\n",
		style.Dim.Render("gt doctor --fix"))
}

// checkBeadsDependency verifies beads meets minimum version requirements.
// Skips check for exempt commands (version, help, completion).
// Deprecated: Use persistentPreRun instead, which calls CheckBeadsVersion.
func checkBeadsDependency(cmd *cobra.Command, _ []string) error {
	// Get the root command name being run
	cmdName := cmd.Name()

	// Skip check for exempt commands
	if beadsExemptCommands[cmdName] {
		return nil
	}

	// Check for stale binary (warning only, doesn't block)
	checkStaleBinaryWarning()

	// Check beads version
	return CheckBeadsVersion()
}

// staleBinaryWarned tracks if we've already warned about stale binary in this session.
// We use an environment variable since the binary restarts on each command.
var staleBinaryWarned = os.Getenv("GT_STALE_WARNED") == "1"

// checkStaleBinaryWarning checks if the installed binary is stale and prints a warning.
// This is a non-blocking check - errors are silently ignored.
func checkStaleBinaryWarning() {
	// Only warn once per shell session
	if staleBinaryWarned {
		return
	}

	repoRoot, err := version.GetRepoRoot()
	if err != nil {
		// Can't find repo - silently skip (might be running from non-dev environment)
		return
	}

	info := version.CheckStaleBinary(repoRoot)
	if info.Error != nil {
		// Check failed - silently skip
		return
	}

	if info.IsStale {
		staleBinaryWarned = true
		_ = os.Setenv("GT_STALE_WARNED", "1")

		msg := fmt.Sprintf("gt binary is stale (built from %s, repo at %s)",
			version.ShortCommit(info.BinaryCommit), version.ShortCommit(info.RepoCommit))
		if info.CommitsBehind > 0 {
			msg = fmt.Sprintf("gt binary is %d commits behind (built from %s, repo at %s)",
				info.CommitsBehind, version.ShortCommit(info.BinaryCommit), version.ShortCommit(info.RepoCommit))
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", style.WarningPrefix, msg)
		fmt.Fprintf(os.Stderr, "    %s Run 'make install' in gastown repo to update\n", style.ArrowPrefix)
	}
}

// Execute runs the root command and returns an exit code.
// The caller (main) should call os.Exit with this code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		// Check for silent exit (scripting commands that signal status via exit code)
		if code, ok := IsSilentExit(err); ok {
			return code
		}
		// Other errors already printed by cobra
		return 1
	}
	return 0
}

// Command group IDs - used by subcommands to organize help output
const (
	GroupWork      = "work"
	GroupAgents    = "agents"
	GroupComm      = "comm"
	GroupServices  = "services"
	GroupWorkspace = "workspace"
	GroupConfig    = "config"
	GroupDiag      = "diag"
)

func init() {
	// Enable prefix matching for subcommands (e.g., "gt ref at" -> "gt refinery attach")
	cobra.EnablePrefixMatching = true

	// Define command groups (order determines help output order)
	rootCmd.AddGroup(
		&cobra.Group{ID: GroupWork, Title: "Work Management:"},
		&cobra.Group{ID: GroupAgents, Title: "Agent Management:"},
		&cobra.Group{ID: GroupComm, Title: "Communication:"},
		&cobra.Group{ID: GroupServices, Title: "Services:"},
		&cobra.Group{ID: GroupWorkspace, Title: "Workspace:"},
		&cobra.Group{ID: GroupConfig, Title: "Configuration:"},
		&cobra.Group{ID: GroupDiag, Title: "Diagnostics:"},
	)

	// Put help and completion in a sensible group
	rootCmd.SetHelpCommandGroupID(GroupDiag)
	rootCmd.SetCompletionCommandGroupID(GroupConfig)

	// Global flags can be added here
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")
}

// buildCommandPath walks the command hierarchy to build the full command path.
// For example: "gt mail send", "gt status", etc.
func buildCommandPath(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}

// requireSubcommand returns a RunE function for parent commands that require
// a subcommand. Without this, Cobra silently shows help and exits 0 for
// unknown subcommands like "gt mol foobar", masking errors.
func requireSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("requires a subcommand\n\nRun '%s --help' for usage", buildCommandPath(cmd))
	}
	return fmt.Errorf("unknown command %q for %q\n\nRun '%s --help' for available commands",
		args[0], buildCommandPath(cmd), buildCommandPath(cmd))
}

// checkHelpFlag checks if --help or -h is the first argument and shows help if so.
// Returns true if help was shown, false otherwise.
//
// This is needed for commands with DisableFlagParsing: true, which bypass
// Cobra's automatic help flag handling.
//
// We only check the FIRST argument to avoid false positives like:
//
//	gt commit -m "--help"  # User wants message "--help", not help output
//
// This covers the common case (gt commit --help) without breaking edge cases.
func checkHelpFlag(cmd *cobra.Command, args []string) (bool, error) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		return true, cmd.Help()
	}
	return false, nil
}
