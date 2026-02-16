package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlClaimCmd = &cobra.Command{
	Use:   "claim <wanted-id>",
	Short: "Claim a wanted item",
	Long: `Claim a wanted item on the shared wanted board.

Updates the wanted row: claimed_by=<your town handle>, status='claimed'.
The item must exist and have status='open'.

In wild-west mode (Phase 1), this writes directly to the local wl-commons
database. In PR mode, this will create a DoltHub PR instead.

Examples:
  gt wl claim w-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runWlClaim,
}

func init() {
	wlCmd.AddCommand(wlClaimCmd)
}

func runWlClaim(cmd *cobra.Command, args []string) error {
	wantedID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	townName, err := workspace.GetTownName(townRoot)
	if err != nil {
		return fmt.Errorf("getting town handle: %w", err)
	}

	if !doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		return fmt.Errorf("database %q not found\nJoin a wasteland first with: gt wl join <org/db>", doltserver.WLCommonsDB)
	}

	item, err := doltserver.QueryWanted(townRoot, wantedID)
	if err != nil {
		return fmt.Errorf("querying wanted item: %w", err)
	}

	if item.Status != "open" {
		return fmt.Errorf("wanted item %s is not open (status: %s)", wantedID, item.Status)
	}

	if err := doltserver.ClaimWanted(townRoot, wantedID, townName); err != nil {
		return fmt.Errorf("claiming wanted item: %w", err)
	}

	fmt.Printf("%s Claimed %s\n", style.Bold.Render("✓"), wantedID)
	fmt.Printf("  Claimed by: %s\n", townName)
	fmt.Printf("  Title: %s\n", item.Title)

	return nil
}
