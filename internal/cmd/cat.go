package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var catJSON bool

var catCmd = &cobra.Command{
	Use:     "cat <bead-id>",
	GroupID: GroupWork,
	Short:   "Display bead content",
	Long: `Display the content of a bead (issue, task, molecule, etc.).

This is a convenience wrapper around 'bd show' that integrates with gt.
Accepts any bead ID (bd-*, hq-*, mol-*).

Examples:
  gt cat bd-abc123       # Show a bead
  gt cat hq-xyz789       # Show a town-level bead
  gt cat bd-abc --json   # Output as JSON`,
	Args: cobra.ExactArgs(1),
	RunE: runCat,
}

func init() {
	rootCmd.AddCommand(catCmd)
	catCmd.Flags().BoolVar(&catJSON, "json", false, "Output as JSON")
}

func runCat(cmd *cobra.Command, args []string) error {
	beadID := args[0]

	// Validate it looks like a bead ID
	if !isBeadID(beadID) {
		return fmt.Errorf("invalid bead ID %q (expected bd-*, hq-*, or mol-* prefix)", beadID)
	}

	// Build bd show command
	bdArgs := []string{"show", beadID}
	if catJSON {
		bdArgs = append(bdArgs, "--json")
	}

	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr

	return bdCmd.Run()
}

// isBeadID checks if a string looks like a bead ID.
func isBeadID(s string) bool {
	prefixes := []string{"bd-", "hq-", "mol-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
