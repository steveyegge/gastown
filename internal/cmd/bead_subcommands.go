package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// --- gt bead create ---

var beadCreateCmd = &cobra.Command{
	Use:   "create [flags]",
	Short: "Create a bead (routes by --rig prefix)",
	Long: `Create a new bead in the specified rig's database.

Uses --rig to route to the correct beads database. Without --rig,
creates in the default database (current directory).

All bd create flags are passed through.

Examples:
  gt bead create --rig nw --title "Fix login bug" --type bug
  gt bead create --rig hq --title "Cross-rig task" --type task
  gt bead create --title "Local issue"  # Uses default database`,
	DisableFlagParsing: true,
	RunE:               runBeadCreate,
}

func runBeadCreate(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	rig, filteredArgs := extractRigFlag(args)

	builder := BdCmd(append([]string{"create"}, filteredArgs...)...)
	if rig != "" {
		builder = builder.RouteForPrefix(rig + "-")
	}

	c := builder.Build()
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	return c.Run()
}

// --- gt bead update ---

var beadUpdateCmd = &cobra.Command{
	Use:   "update <bead-id> [flags]",
	Short: "Update a bead (routes by prefix)",
	Long: `Update a bead, automatically routing to the correct rig database
based on the bead ID prefix.

All bd update flags are passed through.

Examples:
  gt bead update nw-abc --status=in_progress
  gt bead update hq-xyz --notes "Investigation complete"
  gt bead update gt-def --assignee alice`,
	DisableFlagParsing: true,
	RunE:               runBeadUpdate,
}

func runBeadUpdate(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("bead ID required\n\nUsage: gt bead update <bead-id> [flags]")
	}

	beadID := extractBeadIDFromArgs(args)

	builder := BdCmd(append([]string{"update"}, args...)...)
	if beadID != "" {
		builder = builder.RouteForBead(beadID)
	}

	c := builder.Build()
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	return c.Run()
}

// --- gt bead dep ---

var beadDepCmd = &cobra.Command{
	Use:   "dep [subcommand] [args]",
	Short: "Manage bead dependencies (routes by prefix)",
	Long: `Manage dependencies between beads, routing to the correct rig database
based on bead ID prefix.

All bd dep subcommands and flags are passed through.

Examples:
  gt bead dep add nw-abc nw-def          # nw-abc depends on nw-def
  gt bead dep list hq-xyz                # List deps of hq-xyz
  gt bead dep rm gt-abc gt-def           # Remove dependency
  gt bead dep nw-abc --blocks nw-def     # nw-abc blocks nw-def`,
	DisableFlagParsing: true,
	RunE:               runBeadDep,
}

func runBeadDep(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("subcommand or bead ID required\n\nUsage: gt bead dep [subcommand] [args]")
	}

	// Find the first bead ID for routing (skip subcommands like "add", "list", "rm")
	beadID := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Skip dep subcommands
		if arg == "add" || arg == "list" || arg == "rm" || arg == "remove" || arg == "cycles" {
			continue
		}
		if strings.Contains(arg, "-") {
			beadID = arg
			break
		}
	}

	builder := BdCmd(append([]string{"dep"}, args...)...)
	if beadID != "" {
		builder = builder.RouteForBead(beadID)
	}

	c := builder.Build()
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	return c.Run()
}

// --- gt bead list ---

var beadListCmd = &cobra.Command{
	Use:   "list [flags]",
	Short: "List beads (routes by --rig prefix)",
	Long: `List beads from a specific rig's database.

Uses --rig to route to the correct beads database. Without --rig,
lists from the default database (current directory).

All bd list flags are passed through.

Examples:
  gt bead list --rig nw --status=open    # List open beads in namu_warden
  gt bead list --rig hq                  # List town-level beads
  gt bead list --status=open             # List from default database
  gt bead list --rig nw --label backend  # List with label filter`,
	DisableFlagParsing: true,
	RunE:               runBeadList,
}

func runBeadList(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	rig, filteredArgs := extractRigFlag(args)

	builder := BdCmd(append([]string{"list"}, filteredArgs...)...)
	if rig != "" {
		builder = builder.RouteForPrefix(rig + "-")
	}

	c := builder.Build()
	c.Stdout = os.Stdout
	return c.Run()
}

// --- gt bead search ---

var beadSearchCmd = &cobra.Command{
	Use:   "search <query> [flags]",
	Short: "Search beads (routes by --rig prefix)",
	Long: `Search beads in a specific rig's database.

Uses --rig to route to the correct beads database. Without --rig,
searches the default database (current directory).

All bd search flags are passed through.

Examples:
  gt bead search "auth bug" --rig nw           # Search in namu_warden
  gt bead search "login" --rig hq --status all # Search all in HQ
  gt bead search "timeout"                     # Search default database`,
	DisableFlagParsing: true,
	RunE:               runBeadSearch,
}

func runBeadSearch(cmd *cobra.Command, args []string) error {
	if helped, err := checkHelpFlag(cmd, args); helped || err != nil {
		return err
	}

	rig, filteredArgs := extractRigFlag(args)

	builder := BdCmd(append([]string{"search"}, filteredArgs...)...)
	if rig != "" {
		builder = builder.RouteForPrefix(rig + "-")
	}

	c := builder.Build()
	c.Stdout = os.Stdout
	return c.Run()
}

// --- Helpers ---

// extractRigFlag extracts --rig <prefix> from raw args, returning the prefix
// and the remaining args with --rig removed. Supports both --rig <val> and
// --rig=<val> forms.
func extractRigFlag(args []string) (string, []string) {
	var rig string
	var filtered []string
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--rig" && i+1 < len(args) {
			rig = args[i+1]
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--rig=") {
			rig = strings.TrimPrefix(arg, "--rig=")
			continue
		}
		filtered = append(filtered, arg)
	}

	return rig, filtered
}

func init() {
	beadCmd.AddCommand(beadCreateCmd)
	beadCmd.AddCommand(beadUpdateCmd)
	beadCmd.AddCommand(beadDepCmd)
	beadCmd.AddCommand(beadListCmd)
	beadCmd.AddCommand(beadSearchCmd)
}
