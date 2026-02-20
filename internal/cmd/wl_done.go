package cmd

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlDoneEvidence string

var wlDoneCmd = &cobra.Command{
	Use:   "done <wanted-id>",
	Short: "Submit completion evidence for a wanted item",
	Long: `Submit completion evidence for a claimed wanted item.

Inserts a completion record and updates the wanted item status to 'in_review'.
The item must be claimed by your rig.

The --evidence flag provides the evidence URL (PR link, commit hash, etc.).

A completion ID is generated as c-<hash> where hash is derived from the
wanted ID, rig handle, and timestamp.

Examples:
  gt wl done w-abc123 --evidence 'https://github.com/org/repo/pull/123'
  gt wl done w-abc123 --evidence 'commit abc123def'`,
	Args: cobra.ExactArgs(1),
	RunE: runWlDone,
}

func init() {
	wlDoneCmd.Flags().StringVar(&wlDoneEvidence, "evidence", "", "Evidence URL or description (required)")
	_ = wlDoneCmd.MarkFlagRequired("evidence")

	wlCmd.AddCommand(wlDoneCmd)
}

func runWlDone(cmd *cobra.Command, args []string) error {
	wantedID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	wlCfg, err := wasteland.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading wasteland config: %w", err)
	}
	rigHandle := wlCfg.RigHandle

	if !doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		return fmt.Errorf("database %q not found\nJoin a wasteland first with: gt wl join <org/db>", doltserver.WLCommonsDB)
	}

	store := doltserver.NewWLCommons(townRoot)
	completionID := generateCompletionID(wantedID, rigHandle)

	if err := submitDone(store, wantedID, rigHandle, wlDoneEvidence, completionID); err != nil {
		return err
	}

	fmt.Printf("%s Completion submitted for %s\n", style.Bold.Render("✓"), wantedID)
	fmt.Printf("  Completion ID: %s\n", completionID)
	fmt.Printf("  Completed by: %s\n", rigHandle)
	fmt.Printf("  Evidence: %s\n", wlDoneEvidence)
	fmt.Printf("  Status: in_review\n")

	return nil
}

// submitDone contains the testable business logic for submitting a completion.
func submitDone(store doltserver.WLCommonsStore, wantedID, rigHandle, evidence, completionID string) error {
	item, err := store.QueryWanted(wantedID)
	if err != nil {
		return fmt.Errorf("querying wanted item: %w", err)
	}

	if item.Status != "claimed" {
		return fmt.Errorf("wanted item %s is not claimed (status: %s)", wantedID, item.Status)
	}

	if item.ClaimedBy != rigHandle {
		return fmt.Errorf("wanted item %s is claimed by %q, not %q", wantedID, item.ClaimedBy, rigHandle)
	}

	if err := store.SubmitCompletion(completionID, wantedID, rigHandle, evidence); err != nil {
		return fmt.Errorf("submitting completion: %w", err)
	}

	return nil
}

func generateCompletionID(wantedID, rigHandle string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	h := sha256.Sum256([]byte(wantedID + "|" + rigHandle + "|" + now))
	return fmt.Sprintf("c-%x", h[:8])
}
