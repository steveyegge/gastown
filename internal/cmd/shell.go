// ABOUTME: Shell integration management commands.
// ABOUTME: Install/remove shell hooks without full HQ setup.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/shell"
	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/style"
)

var shellCmd = &cobra.Command{
	Use:     "shell",
	GroupID: GroupConfig,
	Short:   "Manage shell integration",
	RunE:    requireSubcommand,
}

var shellInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install or update shell integration",
	Long: `Install or update the Gas Town shell integration.

This adds a hook to your shell RC file that:
  - Sets GT_TOWN_ROOT and GT_RIG when you cd into a Gas Town rig
  - Offers to add new git repos to Gas Town on first visit

Run this after upgrading gt to get the latest shell hook features.`,
	RunE: runShellInstall,
}

var shellRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove shell integration",
	RunE:  runShellRemove,
}

var shellStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show shell integration status",
	RunE:  runShellStatus,
}

func init() {
	shellCmd.AddCommand(shellInstallCmd)
	shellCmd.AddCommand(shellRemoveCmd)
	shellCmd.AddCommand(shellStatusCmd)
	rootCmd.AddCommand(shellCmd)
}

func runShellInstall(cmd *cobra.Command, args []string) error {
	if err := shell.Install(); err != nil {
		return err
	}

	if err := state.Enable(Version); err != nil {
		fmt.Printf("%s Could not enable Gas Town: %v\n", style.Dim.Render("⚠"), err)
	}

	fmt.Printf("%s Shell integration installed (%s)\n", style.Success.Render("✓"), shell.RCFilePath(shell.DetectShell()))
	fmt.Println()
	fmt.Println("Run 'source ~/.zshrc' or open a new terminal to activate.")
	return nil
}

func runShellRemove(cmd *cobra.Command, args []string) error {
	if err := shell.Remove(); err != nil {
		return err
	}

	fmt.Printf("%s Shell integration removed\n", style.Success.Render("✓"))
	return nil
}

func runShellStatus(cmd *cobra.Command, args []string) error {
	s, err := state.Load()
	if err != nil {
		fmt.Println("Gas Town: not configured")
		fmt.Println("Shell integration: not installed")
		return nil
	}

	if s.Enabled {
		fmt.Println("Gas Town: enabled")
	} else {
		fmt.Println("Gas Town: disabled")
	}

	if s.ShellIntegration != "" {
		fmt.Printf("Shell integration: %s (%s)\n", s.ShellIntegration, shell.RCFilePath(s.ShellIntegration))
	} else {
		fmt.Println("Shell integration: not installed")
	}

	return nil
}
