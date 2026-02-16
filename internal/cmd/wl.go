package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

// wl command flags
var (
	wlJoinHandle      string
	wlJoinDisplayName string
)

var wlCmd = &cobra.Command{
	Use:     "wl",
	GroupID: GroupWork,
	Short:   "Wasteland federation commands",
	RunE:    requireSubcommand,
	Long: `Manage Wasteland federation — join communities, post work, earn reputation.

The Wasteland is a federation of Gas Towns via DoltHub. Each town has a
sovereign fork of a shared commons database containing the wanted board
(open work), town registry, and validated completions.

Getting started:
  gt wl join steveyegge/wl-commons   # Join the default wasteland

See https://github.com/steveyegge/gastown for more information.`,
}

var wlJoinCmd = &cobra.Command{
	Use:   "join <upstream>",
	Short: "Join a wasteland by forking its commons",
	Long: `Join a wasteland community by forking its shared commons database.

This command:
  1. Forks the upstream commons to your DoltHub org
  2. Clones the fork locally
  3. Registers your town in the towns table
  4. Pushes the registration to your fork
  5. Saves wasteland configuration locally

The upstream argument is a DoltHub path like 'steveyegge/wl-commons'.

Required environment variables:
  DOLTHUB_TOKEN  - Your DoltHub API token
  DOLTHUB_ORG    - Your DoltHub organization name

Examples:
  gt wl join steveyegge/wl-commons
  gt wl join steveyegge/wl-commons --handle my-town
  gt wl join steveyegge/wl-commons --display-name "Alice's Workshop"`,
	Args: cobra.ExactArgs(1),
	RunE: runWlJoin,
}

func init() {
	wlJoinCmd.Flags().StringVar(&wlJoinHandle, "handle", "", "Town handle for registration (default: town name from config)")
	wlJoinCmd.Flags().StringVar(&wlJoinDisplayName, "display-name", "", "Display name for the town registry")

	wlCmd.AddCommand(wlJoinCmd)
	rootCmd.AddCommand(wlCmd)
}

func runWlJoin(cmd *cobra.Command, args []string) error {
	upstream := args[0]

	// Parse upstream path
	upstreamOrg, upstreamDB, err := wasteland.ParseUpstream(upstream)
	if err != nil {
		return err
	}

	// Require DoltHub credentials
	token := doltserver.DoltHubToken()
	if token == "" {
		return fmt.Errorf("DOLTHUB_TOKEN environment variable is required\n\nGet your token from https://www.dolthub.com/settings/tokens")
	}

	forkOrg := doltserver.DoltHubOrg()
	if forkOrg == "" {
		return fmt.Errorf("DOLTHUB_ORG environment variable is required\n\nSet this to your DoltHub organization name")
	}

	// Find town root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Check if already joined
	if existing, err := wasteland.LoadConfig(townRoot); err == nil {
		fmt.Printf("%s Already joined wasteland: %s\n", style.Dim.Render("⚠"), existing.Upstream)
		fmt.Printf("  Fork: %s/%s\n", existing.ForkOrg, existing.ForkDB)
		fmt.Printf("  Local: %s\n", existing.LocalDir)
		return nil
	}

	// Load town config for identity
	townConfigPath := filepath.Join(townRoot, workspace.PrimaryMarker)
	townCfg, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return fmt.Errorf("loading town config: %w", err)
	}

	// Determine town handle
	handle := wlJoinHandle
	if handle == "" {
		handle = forkOrg // default to DoltHub org as handle
	}

	displayName := wlJoinDisplayName
	if displayName == "" {
		if townCfg.PublicName != "" {
			displayName = townCfg.PublicName
		} else {
			displayName = townCfg.Name
		}
	}

	ownerEmail := townCfg.Owner

	// Get gt version (best-effort from build info)
	gtVersion := "dev"

	localDir := wasteland.LocalCloneDir(townRoot, upstreamOrg, upstreamDB)

	// Step 1: Fork the commons
	fmt.Printf("Forking %s to %s/%s...\n", upstream, forkOrg, upstreamDB)
	if err := wasteland.ForkDoltHubRepo(upstreamOrg, upstreamDB, forkOrg, token); err != nil {
		return fmt.Errorf("forking commons: %w", err)
	}
	fmt.Printf("  %s Fork created (or already exists)\n", style.Bold.Render("✓"))

	// Step 2: Clone the fork locally
	fmt.Printf("Cloning fork to %s...\n", localDir)
	if err := wasteland.CloneLocally(forkOrg, upstreamDB, localDir); err != nil {
		return fmt.Errorf("cloning fork: %w", err)
	}
	fmt.Printf("  %s Clone complete\n", style.Bold.Render("✓"))

	// Step 3: Add upstream remote
	fmt.Printf("Adding upstream remote...\n")
	if err := wasteland.AddUpstreamRemote(localDir, upstreamOrg, upstreamDB); err != nil {
		return fmt.Errorf("adding upstream remote: %w", err)
	}
	fmt.Printf("  %s Upstream remote configured\n", style.Bold.Render("✓"))

	// Step 4: Register town in the towns table
	fmt.Printf("Registering town '%s' in the commons...\n", handle)
	if err := wasteland.RegisterTown(localDir, handle, forkOrg, displayName, ownerEmail, gtVersion); err != nil {
		return fmt.Errorf("registering town: %w", err)
	}
	fmt.Printf("  %s Town registered\n", style.Bold.Render("✓"))

	// Step 5: Push to origin (the fork)
	fmt.Printf("Pushing registration to fork...\n")
	if err := wasteland.PushToOrigin(localDir); err != nil {
		return fmt.Errorf("pushing to fork: %w", err)
	}
	fmt.Printf("  %s Registration pushed\n", style.Bold.Render("✓"))

	// Step 6: Save wasteland config
	cfg := &wasteland.Config{
		Upstream:   upstream,
		ForkOrg:    forkOrg,
		ForkDB:     upstreamDB,
		LocalDir:   localDir,
		TownHandle: handle,
		JoinedAt:   time.Now(),
	}
	if err := wasteland.SaveConfig(townRoot, cfg); err != nil {
		return fmt.Errorf("saving wasteland config: %w", err)
	}

	fmt.Printf("\n%s Joined wasteland: %s\n", style.Bold.Render("✓"), upstream)
	fmt.Printf("  Handle: %s\n", handle)
	fmt.Printf("  Fork: %s/%s\n", forkOrg, upstreamDB)
	fmt.Printf("  Local: %s\n", localDir)
	fmt.Printf("\n  %s\n", style.Dim.Render("Next: gt wl browse  — browse the wanted board"))
	return nil
}
