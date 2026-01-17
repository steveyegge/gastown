package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var beadCmd = &cobra.Command{
	Use:     "bead",
	GroupID: GroupWork,
	Short:   "Bead management utilities",
	Long:    `Utilities for managing beads across repositories.`,
}

var beadMoveCmd = &cobra.Command{
	Use:   "move <bead-id> <target-prefix>",
	Short: "Move a bead to a different repository",
	Long: `Move a bead from one repository to another.

This creates a copy of the bead in the target repository (with the new prefix)
and closes the source bead with a reference to the new location.

The target prefix determines which repository receives the bead.
Common prefixes: gt- (gastown), bd- (beads), hq- (headquarters)

Examples:
  gt bead move gt-abc123 bd-     # Move gt-abc123 to beads repo as bd-*
  gt bead move hq-xyz bd-        # Move hq-xyz to beads repo
  gt bead move bd-123 gt-        # Move bd-123 to gastown repo`,
	Args: cobra.ExactArgs(2),
	RunE: runBeadMove,
}

var beadMoveDryRun bool

var beadShowCmd = &cobra.Command{
	Use:   "show <bead-id> [flags]",
	Short: "Show details of a bead",
	Long: `Displays the full details of a bead by ID.

This is an alias for 'gt show'. All bd show flags are supported.

Examples:
  gt bead show gt-abc123          # Show a gastown issue
  gt bead show hq-xyz789          # Show a town-level bead
  gt bead show bd-def456          # Show a beads issue
  gt bead show gt-abc123 --json   # Output as JSON`,
	DisableFlagParsing: true, // Pass all flags through to bd show
	RunE: func(cmd *cobra.Command, args []string) error {
		return runShow(cmd, args)
	},
}

func init() {
	beadMoveCmd.Flags().BoolVarP(&beadMoveDryRun, "dry-run", "n", false, "Show what would be done")
	beadCmd.AddCommand(beadMoveCmd)
	beadCmd.AddCommand(beadShowCmd)
	rootCmd.AddCommand(beadCmd)
}

// moveBeadInfo holds the essential fields we need to copy when moving beads
type moveBeadInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"issue_type"`
	Priority    int      `json:"priority"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	Assignee    string   `json:"assignee"`
	Status      string   `json:"status"`
}

func runBeadMove(cmd *cobra.Command, args []string) error {
	sourceID := args[0]
	targetPrefix := args[1]

	// Normalize prefix (ensure it ends with -)
	if !strings.HasSuffix(targetPrefix, "-") {
		targetPrefix = targetPrefix + "-"
	}

	// Get source bead details
	showCmd := exec.Command("bd", "show", sourceID, "--json")
	output, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("getting bead %s: %w", sourceID, err)
	}

	// bd show --json returns an array
	var sources []moveBeadInfo
	if err := json.Unmarshal(output, &sources); err != nil {
		return fmt.Errorf("parsing bead data: %w", err)
	}
	if len(sources) == 0 {
		return fmt.Errorf("bead %s not found", sourceID)
	}
	source := sources[0]

	// Don't move closed beads
	if source.Status == "closed" {
		return fmt.Errorf("cannot move closed bead %s", sourceID)
	}

	fmt.Printf("%s Moving %s to %s...\n", style.Bold.Render("→"), sourceID, targetPrefix)
	fmt.Printf("  Title: %s\n", source.Title)
	fmt.Printf("  Type: %s\n", source.Type)

	if beadMoveDryRun {
		fmt.Printf("\nDry run - would:\n")
		fmt.Printf("  1. Create new bead with prefix %s\n", targetPrefix)
		fmt.Printf("  2. Close %s with reference to new bead\n", sourceID)
		return nil
	}

	// Build create command for target
	createArgs := []string{
		"create",
		"--prefix", targetPrefix,
		"--title", source.Title,
		"--type", source.Type,
		"--priority", fmt.Sprintf("%d", source.Priority),
		"--silent", // Only output the ID
	}

	if source.Description != "" {
		createArgs = append(createArgs, "--description", source.Description)
	}
	if source.Assignee != "" {
		createArgs = append(createArgs, "--assignee", source.Assignee)
	}
	for _, label := range source.Labels {
		createArgs = append(createArgs, "--label", label)
	}

	// Create the new bead
	createCmd := exec.Command("bd", createArgs...)
	createCmd.Stderr = os.Stderr
	newIDBytes, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("creating new bead: %w", err)
	}
	newID := strings.TrimSpace(string(newIDBytes))

	fmt.Printf("%s Created %s\n", style.Bold.Render("✓"), newID)

	// Close the source bead with reference
	closeReason := fmt.Sprintf("Moved to %s", newID)
	closeCmd := exec.Command("bd", "close", sourceID, "--reason", closeReason)
	closeCmd.Stderr = os.Stderr
	if err := closeCmd.Run(); err != nil {
		// Try to clean up the new bead if close fails
		fmt.Fprintf(os.Stderr, "Warning: failed to close source bead: %v\n", err)
		fmt.Fprintf(os.Stderr, "New bead %s was created but source %s remains open\n", newID, sourceID)
		return err
	}

	fmt.Printf("%s Closed %s (moved to %s)\n", style.Bold.Render("✓"), sourceID, newID)
	fmt.Printf("\nBead moved: %s → %s\n", sourceID, newID)

	return nil
}
