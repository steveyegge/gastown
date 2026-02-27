package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

var rigLinkCmd = &cobra.Command{
	Use:   "link <rig> <git-url>",
	Short: "Link an external repo or sibling rig as a read-only reference",
	Long: `Link a reference repository to a rig for cross-repo context.

Polecats spawned in this rig will have read-only access to the linked repo
via symlinks in their workspace.

External repo (shallow clone):
  gt rig link dma https://github.com/org/repo.git --name myref
  gt rig link dma https://github.com/org/repo.git --name myref --branch develop

Same-town rig (symlink, zero disk cost):
  gt rig link dma --from gastown
  gt rig link dma --from gastown --name gs`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runRigLink,
}

var rigUnlinkCmd = &cobra.Command{
	Use:   "unlink <rig> <name>",
	Short: "Remove a linked reference",
	Args:  cobra.ExactArgs(2),
	RunE:  runRigUnlink,
}

var rigRefsCmd = &cobra.Command{
	Use:   "refs <rig>",
	Short: "List linked references for a rig",
	Args:  cobra.ExactArgs(1),
	RunE:  runRigRefs,
}

var rigSyncCmd = &cobra.Command{
	Use:   "sync <rig> [name]",
	Short: "Pull latest for cloned refs (no-op for symlinks)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runRigSync,
}

var (
	linkName   string
	linkFrom   string
	linkBranch string
)

func init() {
	rigLinkCmd.Flags().StringVar(&linkName, "name", "", "Alias for the linked ref (defaults to repo/rig name)")
	rigLinkCmd.Flags().StringVar(&linkFrom, "from", "", "Link a same-town rig instead of an external URL")
	rigLinkCmd.Flags().StringVar(&linkBranch, "branch", "", "Branch to track (external clones only)")

	rigCmd.AddCommand(rigLinkCmd)
	rigCmd.AddCommand(rigUnlinkCmd)
	rigCmd.AddCommand(rigRefsCmd)
	rigCmd.AddCommand(rigSyncCmd)
}

func runRigLink(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	cfg, err := rig.LoadRigConfig(r.Path)
	if err != nil {
		return fmt.Errorf("loading rig config: %w", err)
	}

	if cfg.Refs == nil {
		cfg.Refs = make(map[string]rig.RefEntry)
	}

	if linkFrom != "" {
		// Same-town rig link
		alias := linkName
		if alias == "" {
			alias = linkFrom
		}
		if err := rig.ValidateRefAlias(alias); err != nil {
			return err
		}
		if _, exists := cfg.Refs[alias]; exists {
			return fmt.Errorf("ref %q already configured — unlink first", alias)
		}

		if err := rig.LinkSameTownRef(townRoot, r.Path, alias, linkFrom); err != nil {
			return fmt.Errorf("linking rig: %w", err)
		}

		cfg.Refs[alias] = rig.RefEntry{
			FromRig: linkFrom,
			AddedAt: time.Now(),
		}

		if err := rig.SaveRigConfig(r.Path, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		fmt.Printf("%s Linked %s → %s (symlink)\n",
			style.Success.Render("✓"), alias, linkFrom)
		return nil
	}

	// External repo link
	if len(args) < 2 {
		return fmt.Errorf("git URL required (or use --from for same-town rig)")
	}
	gitURL := args[1]

	alias := linkName
	if alias == "" {
		alias = repoNameFromURL(gitURL)
	}
	if err := rig.ValidateRefAlias(alias); err != nil {
		return err
	}
	if _, exists := cfg.Refs[alias]; exists {
		return fmt.Errorf("ref %q already configured — unlink first", alias)
	}

	fmt.Printf("Cloning %s as %s...\n", style.Dim.Render(gitURL), style.Bold.Render(alias))

	if err := rig.LinkExternalRef(r.Path, alias, gitURL, linkBranch); err != nil {
		return fmt.Errorf("cloning: %w", err)
	}

	cfg.Refs[alias] = rig.RefEntry{
		GitURL:  gitURL,
		Branch:  linkBranch,
		Shallow: true,
		AddedAt: time.Now(),
	}

	if err := rig.SaveRigConfig(r.Path, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s Linked %s (shallow clone)\n",
		style.Success.Render("✓"), alias)
	return nil
}

func runRigUnlink(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	alias := args[1]

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	cfg, err := rig.LoadRigConfig(r.Path)
	if err != nil {
		return fmt.Errorf("loading rig config: %w", err)
	}

	if cfg.Refs == nil {
		return fmt.Errorf("no refs configured for rig %s", rigName)
	}
	if _, exists := cfg.Refs[alias]; !exists {
		return fmt.Errorf("ref %q not found in rig %s", alias, rigName)
	}

	if err := rig.UnlinkRef(r.Path, alias); err != nil {
		return fmt.Errorf("removing ref: %w", err)
	}

	delete(cfg.Refs, alias)
	if len(cfg.Refs) == 0 {
		cfg.Refs = nil // clean up empty map for omitempty
	}

	if err := rig.SaveRigConfig(r.Path, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s Unlinked %s from %s\n",
		style.Success.Render("✓"), alias, rigName)
	return nil
}

func runRigRefs(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	cfg, err := rig.LoadRigConfig(r.Path)
	if err != nil {
		return fmt.Errorf("loading rig config: %w", err)
	}

	if cfg.Refs == nil || len(cfg.Refs) == 0 {
		fmt.Printf("No refs linked for rig %s\n", style.Bold.Render(rigName))
		fmt.Printf("\nLink one with: %s\n", style.Dim.Render("gt rig link "+rigName+" <git-url> --name <alias>"))
		return nil
	}

	statuses := rig.ListRefs(r.Path, cfg.Refs)

	fmt.Printf("Refs for %s:\n\n", style.Bold.Render(rigName))
	for _, s := range statuses {
		icon := style.Success.Render("●")
		if s.Status != "ok" {
			icon = style.Warning.Render("○")
		}

		source := s.GitURL
		if s.FromRig != "" {
			source = fmt.Sprintf("rig:%s", s.FromRig)
		}

		fmt.Printf("  %s %s  %s  %s  %s\n",
			icon,
			style.Bold.Render(s.Alias),
			style.Dim.Render(s.Type),
			source,
			style.Dim.Render("["+s.Status+"]"))
	}
	fmt.Println()

	return nil
}

func runRigSync(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	cfg, err := rig.LoadRigConfig(r.Path)
	if err != nil {
		return fmt.Errorf("loading rig config: %w", err)
	}

	if cfg.Refs == nil || len(cfg.Refs) == 0 {
		fmt.Println("No refs to sync")
		return nil
	}

	if len(args) == 2 {
		// Sync specific ref
		alias := args[1]
		if _, exists := cfg.Refs[alias]; !exists {
			return fmt.Errorf("ref %q not found", alias)
		}
		fmt.Printf("Syncing %s...\n", style.Bold.Render(alias))
		if err := rig.SyncRef(r.Path, alias); err != nil {
			return err
		}
		fmt.Printf("%s Synced %s\n", style.Success.Render("✓"), alias)
		return nil
	}

	// Sync all cloned refs
	fmt.Printf("Syncing refs for %s...\n", style.Bold.Render(rigName))
	if err := rig.SyncAllRefs(r.Path, cfg.Refs); err != nil {
		return err
	}
	fmt.Printf("%s All refs synced\n", style.Success.Render("✓"))
	return nil
}

// repoNameFromURL extracts a short name from a git URL.
// e.g., "https://github.com/org/repo.git" → "repo"
func repoNameFromURL(url string) string {
	// Strip trailing .git
	name := url
	if len(name) > 4 && name[len(name)-4:] == ".git" {
		name = name[:len(name)-4]
	}
	// Take last path component
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' || name[i] == ':' {
			return name[i+1:]
		}
	}
	return name
}
