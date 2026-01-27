package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset Gas Town state to fresh install",
	Long: `Reset Gas Town to a clean state, as if freshly installed.

This command:
1. Stops running agents (deacon, polecats, etc. - mayor preserved by default)
2. Deletes the beads database (all issues, wisps, molecules)
3. Clears activity logs and event files
4. Preserves configuration (config.yaml, formulas, etc.)

Use --all to also stop the mayor.

Use this when you want a clean slate without reinstalling.

WARNING: This permanently deletes all work history. Use with caution.`,
	RunE: runReset,
}

var (
	resetForce bool
	resetAll   bool
)

func init() {
	resetCmd.Flags().BoolVarP(&resetForce, "force", "f", false, "Skip confirmation prompt")
	resetCmd.Flags().BoolVarP(&resetAll, "all", "a", false, "Also stop mayor (by default, mayor is preserved)")
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Confirmation prompt
	if !resetForce {
		fmt.Println("⚠️  This will permanently delete all Gas Town state:")
		fmt.Println("   - All issues, wisps, and molecules")
		fmt.Println("   - All activity history")
		fmt.Println("   - All hook and mail state")
		fmt.Println()
		fmt.Print("Type 'reset' to confirm: ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "reset" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println()

	// Step 1: Stop all agents
	fmt.Println("Stopping agents...")
	t := tmux.NewTmux()

	// Stop deacon
	if running, _ := t.HasSession("hq-deacon"); running {
		_ = t.KillSessionWithProcesses("hq-deacon")
		fmt.Printf("  %s Stopped deacon\n", style.Bold.Render("✓"))
	}

	// Stop mayor (only with --all flag)
	if resetAll {
		if running, _ := t.HasSession("hq-mayor"); running {
			_ = t.KillSessionWithProcesses("hq-mayor")
			fmt.Printf("  %s Stopped mayor\n", style.Bold.Render("✓"))
		}
	}

	// Step 2: Delete beads database
	fmt.Println("Clearing database...")
	beadsDir := filepath.Join(townRoot, ".beads")
	dbFiles := []string{
		"beads.db",
		"beads.db-shm",
		"beads.db-wal",
	}
	for _, f := range dbFiles {
		path := filepath.Join(beadsDir, f)
		if err := os.Remove(path); err == nil {
			fmt.Printf("  %s Deleted %s\n", style.Bold.Render("✓"), f)
		}
	}

	// Step 3: Clear jsonl files (but don't delete - they'll be recreated)
	fmt.Println("Clearing logs...")
	jsonlFiles := []string{
		filepath.Join(beadsDir, "issues.jsonl"),
		filepath.Join(beadsDir, "interactions.jsonl"),
		filepath.Join(townRoot, ".events.jsonl"),
	}
	for _, path := range jsonlFiles {
		if err := os.Remove(path); err == nil {
			fmt.Printf("  %s Cleared %s\n", style.Bold.Render("✓"), filepath.Base(path))
		}
	}

	// Step 4: Clear daemon activity
	activityPath := filepath.Join(townRoot, "daemon", "activity.json")
	if err := os.Remove(activityPath); err == nil {
		fmt.Printf("  %s Cleared daemon activity\n", style.Bold.Render("✓"))
	}

	fmt.Println()
	fmt.Printf("%s Gas Town reset to clean state\n", style.Bold.Render("✓"))
	fmt.Println("  Configuration preserved (config.yaml, formulas)")
	fmt.Println("  Run 'gt status' to verify")

	return nil
}
