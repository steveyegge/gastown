// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "gt",
	Short:   "Gas Town - Multi-agent workspace manager",
	Version: Version,
	Long: `Gas Town (gt) manages multi-agent workspaces called rigs.

It coordinates agent spawning, work distribution, and communication
across distributed teams of AI agents working on shared codebases.`,
	PersistentPreRunE: checkBeadsDependency,
}

// Commands that don't require beads to be installed/checked.
// These are basic utility commands that should work without beads.
var beadsExemptCommands = map[string]bool{
	"version":    true,
	"help":       true,
	"completion": true,
}

// checkBeadsDependency verifies beads meets minimum version requirements.
// Skips check for exempt commands (version, help, completion).
func checkBeadsDependency(cmd *cobra.Command, args []string) error {
	// Get the root command name being run
	cmdName := cmd.Name()

	// Skip check for exempt commands
	if beadsExemptCommands[cmdName] {
		return nil
	}

	// Check beads version
	return CheckBeadsVersion()
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
