package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	wlPostTitle       string
	wlPostDescription string
	wlPostProject     string
	wlPostType        string
	wlPostPriority    int
	wlPostEffort      string
	wlPostTags        string
)

var wlPostCmd = &cobra.Command{
	Use:   "post",
	Short: "Post a new wanted item to the commons",
	Long: `Post a new wanted item to the Wasteland commons (shared wanted board).

Creates a wanted item with a unique w-<hash> ID and inserts it into the
wl-commons database. Phase 1 (wild-west): direct write to main branch.

The posted_by field is set to the town's DoltHub org (DOLTHUB_ORG) or
falls back to the town directory name.

Examples:
  gt wl post --title "Fix auth bug" --project gastown --type bug
  gt wl post --title "Add federation sync" --type feature --priority 1 --effort large
  gt wl post --title "Update docs" --tags "docs,federation" --effort small`,
	RunE: runWlPost,
}

func init() {
	wlPostCmd.Flags().StringVar(&wlPostTitle, "title", "", "Title of the wanted item (required)")
	wlPostCmd.Flags().StringVarP(&wlPostDescription, "description", "d", "", "Detailed description")
	wlPostCmd.Flags().StringVar(&wlPostProject, "project", "", "Project name (e.g., gastown, beads)")
	wlPostCmd.Flags().StringVar(&wlPostType, "type", "", "Item type: feature, bug, design, rfc, docs")
	wlPostCmd.Flags().IntVar(&wlPostPriority, "priority", 2, "Priority: 0=critical, 1=high, 2=medium, 3=low, 4=backlog")
	wlPostCmd.Flags().StringVar(&wlPostEffort, "effort", "medium", "Effort level: trivial, small, medium, large, epic")
	wlPostCmd.Flags().StringVar(&wlPostTags, "tags", "", "Comma-separated tags (e.g., 'go,auth,federation')")

	_ = wlPostCmd.MarkFlagRequired("title")

	wlCmd.AddCommand(wlPostCmd)
}

func runWlPost(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	var tags []string
	if wlPostTags != "" {
		for _, t := range strings.Split(wlPostTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	validTypes := map[string]bool{
		"feature": true, "bug": true, "design": true, "rfc": true, "docs": true,
	}
	if wlPostType != "" && !validTypes[wlPostType] {
		return fmt.Errorf("invalid type %q: must be one of feature, bug, design, rfc, docs", wlPostType)
	}

	validEfforts := map[string]bool{
		"trivial": true, "small": true, "medium": true, "large": true, "epic": true,
	}
	if !validEfforts[wlPostEffort] {
		return fmt.Errorf("invalid effort %q: must be one of trivial, small, medium, large, epic", wlPostEffort)
	}

	if wlPostPriority < 0 || wlPostPriority > 4 {
		return fmt.Errorf("invalid priority %d: must be 0-4", wlPostPriority)
	}

	if err := doltserver.EnsureWLCommons(townRoot); err != nil {
		return fmt.Errorf("ensuring wl-commons database: %w", err)
	}

	id := doltserver.GenerateWantedID(wlPostTitle)
	handle := doltserver.GetTownHandle(townRoot)

	item := &doltserver.WantedItem{
		ID:          id,
		Title:       wlPostTitle,
		Description: wlPostDescription,
		Project:     wlPostProject,
		Type:        wlPostType,
		Priority:    wlPostPriority,
		Tags:        tags,
		PostedBy:    handle,
		EffortLevel: wlPostEffort,
	}

	if err := doltserver.InsertWanted(townRoot, item); err != nil {
		return fmt.Errorf("posting wanted item: %w", err)
	}

	fmt.Printf("%s Posted wanted item: %s\n", style.Bold.Render("✓"), style.Bold.Render(id))
	fmt.Printf("  Title:    %s\n", wlPostTitle)
	if wlPostProject != "" {
		fmt.Printf("  Project:  %s\n", wlPostProject)
	}
	if wlPostType != "" {
		fmt.Printf("  Type:     %s\n", wlPostType)
	}
	fmt.Printf("  Priority: %d\n", wlPostPriority)
	fmt.Printf("  Effort:   %s\n", wlPostEffort)
	if len(tags) > 0 {
		fmt.Printf("  Tags:     %s\n", strings.Join(tags, ", "))
	}
	fmt.Printf("  Posted by: %s\n", handle)

	return nil
}
