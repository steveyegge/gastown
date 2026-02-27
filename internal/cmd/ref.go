package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var refCmd = &cobra.Command{
	Use:   "ref",
	Short: "Manage the shared ref pool (cross-town references)",
	Long: `Manage the shared reference pool that provides cross-town visibility.

Refs added to the pool are available to all polecats across all rigs and towns
(excluding self-references).

Requires GT_REF_POOL environment variable to be set.`,
}

var refListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all pool refs with status",
	Args:  cobra.NoArgs,
	RunE:  runRefList,
}

var refAddCmd = &cobra.Command{
	Use:   "add <git-url>",
	Short: "Add a ref to the shared pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runRefAdd,
}

var refRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a ref from the shared pool",
	Args:  cobra.ExactArgs(1),
	RunE:  runRefRemove,
}

var refSyncCmd = &cobra.Command{
	Use:   "sync [alias]",
	Short: "Pull latest for pool refs (one or all)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRefSync,
}

var (
	refAddName   string
	refAddBranch string
)

func init() {
	refAddCmd.Flags().StringVar(&refAddName, "name", "", "Alias for the ref (defaults to repo name)")
	refAddCmd.Flags().StringVar(&refAddBranch, "branch", "", "Branch to track")

	refCmd.AddCommand(refListCmd)
	refCmd.AddCommand(refAddCmd)
	refCmd.AddCommand(refRemoveCmd)
	refCmd.AddCommand(refSyncCmd)

	rootCmd.AddCommand(refCmd)
}

func requirePool() (string, error) {
	poolPath := rig.ResolvePoolPath()
	if poolPath == "" {
		return "", fmt.Errorf("GT_REF_POOL not set — shared ref pool not configured")
	}
	return poolPath, nil
}

func runRefList(cmd *cobra.Command, args []string) error {
	poolPath, err := requirePool()
	if err != nil {
		return err
	}

	reg, err := rig.LoadPoolRegistry(poolPath)
	if err != nil {
		return err
	}

	if len(reg.Refs) == 0 {
		fmt.Println("No refs in pool")
		fmt.Printf("\nAdd one with: %s\n", style.Dim.Render("gt ref add <git-url> --name <alias>"))
		return nil
	}

	// Sort aliases for stable output
	aliases := make([]string, 0, len(reg.Refs))
	for alias := range reg.Refs {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	fmt.Printf("Shared ref pool (%s):\n\n", style.Dim.Render(poolPath))
	for _, alias := range aliases {
		entry := reg.Refs[alias]
		icon := style.Success.Render("●")

		// Check if clone exists on disk
		dest := rig.PoolRefPath(poolPath, alias)
		if _, err := os.Stat(dest); err != nil {
			icon = style.Warning.Render("○")
		}

		branchInfo := ""
		if entry.Branch != "" {
			branchInfo = fmt.Sprintf(" [%s]", entry.Branch)
		}
		townInfo := ""
		if entry.AddedByTown != "" {
			townInfo = fmt.Sprintf(" (from %s)", entry.AddedByTown)
		}

		fmt.Printf("  %s %s  %s%s%s\n",
			icon,
			style.Bold.Render(alias),
			style.Dim.Render(entry.GitURL),
			branchInfo,
			townInfo)
	}
	fmt.Println()

	return nil
}

func runRefAdd(cmd *cobra.Command, args []string) error {
	poolPath, err := requirePool()
	if err != nil {
		return err
	}

	gitURL := args[0]
	alias := refAddName
	if alias == "" {
		alias = repoNameFromURL(gitURL)
	}
	if err := rig.ValidateRefAlias(alias); err != nil {
		return err
	}

	// Get town name for attribution
	townName := ""
	if tn, err := workspace.GetTownNameFromCwd(); err == nil {
		townName = tn
	}

	fmt.Printf("Adding %s as %s to pool...\n", style.Dim.Render(gitURL), style.Bold.Render(alias))

	if err := rig.RegisterPoolRef(poolPath, alias, gitURL, refAddBranch, townName); err != nil {
		return fmt.Errorf("registering pool ref: %w", err)
	}

	fmt.Printf("%s Added %s to pool at %s\n",
		style.Success.Render("✓"), alias, rig.PoolRefPath(poolPath, alias))
	return nil
}

func runRefRemove(cmd *cobra.Command, args []string) error {
	poolPath, err := requirePool()
	if err != nil {
		return err
	}

	alias := args[0]
	if err := rig.RemovePoolRef(poolPath, alias); err != nil {
		return err
	}

	fmt.Printf("%s Removed %s from pool\n", style.Success.Render("✓"), alias)
	return nil
}

func runRefSync(cmd *cobra.Command, args []string) error {
	poolPath, err := requirePool()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		alias := args[0]
		fmt.Printf("Syncing %s...\n", style.Bold.Render(alias))
		if err := rig.SyncPoolRef(poolPath, alias); err != nil {
			return err
		}
		fmt.Printf("%s Synced %s\n", style.Success.Render("✓"), alias)
		return nil
	}

	fmt.Println("Syncing all pool refs...")
	if err := rig.SyncAllPoolRefs(poolPath); err != nil {
		return err
	}
	fmt.Printf("%s All pool refs synced\n", style.Success.Render("✓"))
	return nil
}
