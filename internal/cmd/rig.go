// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/wisp"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

var rigCmd = &cobra.Command{
	Use:     "rig",
	GroupID: GroupWorkspace,
	Short:   "Manage rigs in the workspace",
	RunE:    requireSubcommand,
	Long: `Manage rigs (project containers) in the Gas Town workspace.

A rig is a container for managing a project and its agents:
  - refinery/rig/  Canonical main clone (Refinery's working copy)
  - mayor/rig/     Mayor's working clone for this rig
  - crew/<name>/   Human workspace(s)
  - witness/       Witness agent (no clone)
  - polecats/      Worker directories
  - .beads/        Rig-level issue tracking`,
}

var rigAddCmd = &cobra.Command{
	Use:   "add <name> <git-url>",
	Short: "Add a new rig to the workspace",
	Long: `Add a new rig by cloning a repository.

This creates a rig container with:
  - config.json           Rig configuration
  - .beads/               Rig-level issue tracking (initialized)
  - plugins/              Rig-level plugin directory
  - refinery/rig/         Canonical main clone
  - mayor/rig/            Mayor's working clone
  - crew/                 Empty crew directory (add members with 'gt crew add')
  - witness/              Witness agent directory
  - polecats/             Worker directory (empty)

The command also:
  - Seeds patrol molecules (Deacon, Witness, Refinery)
  - Creates ~/gt/plugins/ (town-level) if it doesn't exist
  - Creates <rig>/plugins/ (rig-level)

Use --adopt to register an existing directory instead of creating new:
  - Reads existing config.json if present
  - Auto-detects git URL from origin remote (git-url argument not required)
  - Adds entry to mayor/rigs.json

Example:
  gt rig add gastown https://github.com/steveyegge/gastown
  gt rig add my-project git@github.com:user/repo.git --prefix mp
  gt rig add existing-rig --adopt`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runRigAdd,
}

var rigListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all rigs in the workspace",
	RunE:  runRigList,
}

var rigRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a rig from the registry (does not delete files)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRigRemove,
}

var rigResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset rig state (handoff content, mail, stale issues)",
	Long: `Reset various rig state.

By default, resets all resettable state. Use flags to reset specific items.

Examples:
  gt rig reset              # Reset all state
  gt rig reset --handoff    # Clear handoff content only
  gt rig reset --mail       # Clear stale mail messages only
  gt rig reset --stale      # Reset orphaned in_progress issues
  gt rig reset --stale --dry-run  # Preview what would be reset`,
	RunE: runRigReset,
}

var rigBootCmd = &cobra.Command{
	Use:   "boot <rig>",
	Short: "Start witness and refinery for a rig",
	Long: `Start the witness and refinery agents for a rig.

This is the inverse of 'gt rig shutdown'. It starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig boot greenplace`,
	Args: cobra.ExactArgs(1),
	RunE: runRigBoot,
}

var rigStartCmd = &cobra.Command{
	Use:   "start <rig>...",
	Short: "Start witness and refinery on patrol for one or more rigs",
	Long: `Start the witness and refinery agents on patrol for one or more rigs.

This is similar to 'gt rig boot' but supports multiple rigs at once.
For each rig, it starts:
- The witness (if not already running)
- The refinery (if not already running)

Polecats are NOT started by this command - they are spawned
on demand when work is assigned.

Examples:
  gt rig start gastown
  gt rig start gastown beads
  gt rig start gastown beads myproject`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigStart,
}

var rigRebootCmd = &cobra.Command{
	Use:   "reboot <rig>",
	Short: "Restart witness and refinery for a rig",
	Long: `Restart the patrol agents (witness and refinery) for a rig.

This is equivalent to 'gt rig shutdown' followed by 'gt rig boot'.
Useful after polecats complete work and land their changes.

Examples:
  gt rig reboot greenplace
  gt rig reboot beads --force`,
	Args: cobra.ExactArgs(1),
	RunE: runRigReboot,
}

var rigShutdownCmd = &cobra.Command{
	Use:   "shutdown <rig>",
	Short: "Gracefully stop all rig agents",
	Long: `Stop all agents in a rig.

This command gracefully shuts down:
- All polecat sessions
- The refinery (if running)
- The witness (if running)

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig shutdown greenplace
  gt rig shutdown greenplace --force
  gt rig shutdown greenplace --nuclear  # DANGER: loses uncommitted work`,
	Args: cobra.ExactArgs(1),
	RunE: runRigShutdown,
}

var rigStatusCmd = &cobra.Command{
	Use:   "status [rig]",
	Short: "Show detailed status for a specific rig",
	Long: `Show detailed status for a specific rig including all workers.

If no rig is specified, infers the rig from the current directory.

Displays:
- Rig information (name, path, beads prefix)
- Witness status (running/stopped, uptime)
- Refinery status (running/stopped, uptime, queue size)
- Polecats (name, state, assigned issue, session status)
- Crew members (name, branch, session status, git status)

Examples:
  gt rig status           # Infer rig from current directory
  gt rig status gastown
  gt rig status beads`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRigStatus,
}

var rigStopCmd = &cobra.Command{
	Use:   "stop <rig>...",
	Short: "Stop one or more rigs (shutdown semantics)",
	Long: `Stop all agents in one or more rigs.

This command is similar to 'gt rig shutdown' but supports multiple rigs.
For each rig, it gracefully shuts down:
- All polecat sessions
- The refinery (if running)
- The witness (if running)

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig stop gastown
  gt rig stop gastown beads
  gt rig stop --force gastown beads
  gt rig stop --nuclear gastown  # DANGER: loses uncommitted work`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigStop,
}

var rigRestartCmd = &cobra.Command{
	Use:   "restart <rig>...",
	Short: "Restart one or more rigs (stop then start)",
	Long: `Restart the patrol agents (witness and refinery) for one or more rigs.

This is equivalent to 'gt rig stop' followed by 'gt rig start' for each rig.
Useful after polecats complete work and land their changes.

Before shutdown, checks all polecats for uncommitted work:
- Uncommitted changes (modified/untracked files)
- Stashes
- Unpushed commits

Use --force to skip graceful shutdown and kill immediately.
Use --nuclear to bypass ALL safety checks (will lose work!).

Examples:
  gt rig restart gastown
  gt rig restart gastown beads
  gt rig restart --force gastown beads
  gt rig restart --nuclear gastown  # DANGER: loses uncommitted work`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRigRestart,
}

// Flags
var (
	rigAddPrefix       string
	rigAddLocalRepo    string
	rigAddBranch       string
	rigAddAdopt        bool
	rigAddAdoptURL     string
	rigAddAdoptForce   bool
	rigResetHandoff    bool
	rigResetMail       bool
	rigResetStale      bool
	rigResetDryRun     bool
	rigResetRole       string
	rigShutdownForce   bool
	rigShutdownNuclear bool
	rigStopForce       bool
	rigStopNuclear     bool
	rigRestartForce    bool
	rigRestartNuclear  bool
)

func init() {
	rootCmd.AddCommand(rigCmd)
	rigCmd.AddCommand(rigAddCmd)
	rigCmd.AddCommand(rigBootCmd)
	rigCmd.AddCommand(rigListCmd)
	rigCmd.AddCommand(rigRebootCmd)
	rigCmd.AddCommand(rigRemoveCmd)
	rigCmd.AddCommand(rigResetCmd)
	rigCmd.AddCommand(rigRestartCmd)
	rigCmd.AddCommand(rigShutdownCmd)
	rigCmd.AddCommand(rigStartCmd)
	rigCmd.AddCommand(rigStatusCmd)
	rigCmd.AddCommand(rigStopCmd)

	rigAddCmd.Flags().StringVar(&rigAddPrefix, "prefix", "", "Beads issue prefix (default: derived from name)")
	rigAddCmd.Flags().StringVar(&rigAddLocalRepo, "local-repo", "", "Local repo path to share git objects (optional)")
	rigAddCmd.Flags().StringVar(&rigAddBranch, "branch", "", "Default branch name (default: auto-detected from remote)")
	rigAddCmd.Flags().BoolVar(&rigAddAdopt, "adopt", false, "Adopt an existing directory instead of creating new")
	rigAddCmd.Flags().StringVar(&rigAddAdoptURL, "url", "", "Git remote URL for --adopt (default: auto-detected from origin)")
	rigAddCmd.Flags().BoolVar(&rigAddAdoptForce, "force", false, "With --adopt, register even if git remote cannot be detected")

	rigResetCmd.Flags().BoolVar(&rigResetHandoff, "handoff", false, "Clear handoff content")
	rigResetCmd.Flags().BoolVar(&rigResetMail, "mail", false, "Clear stale mail messages")
	rigResetCmd.Flags().BoolVar(&rigResetStale, "stale", false, "Reset orphaned in_progress issues (no active session)")
	rigResetCmd.Flags().BoolVar(&rigResetDryRun, "dry-run", false, "Show what would be reset without making changes")
	rigResetCmd.Flags().StringVar(&rigResetRole, "role", "", "Role to reset (default: auto-detect from cwd)")

	rigShutdownCmd.Flags().BoolVarP(&rigShutdownForce, "force", "f", false, "Force immediate shutdown")
	rigShutdownCmd.Flags().BoolVar(&rigShutdownNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")

	rigRebootCmd.Flags().BoolVarP(&rigShutdownForce, "force", "f", false, "Force immediate shutdown during reboot")

	rigStopCmd.Flags().BoolVarP(&rigStopForce, "force", "f", false, "Force immediate shutdown")
	rigStopCmd.Flags().BoolVar(&rigStopNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")

	rigRestartCmd.Flags().BoolVarP(&rigRestartForce, "force", "f", false, "Force immediate shutdown during restart")
	rigRestartCmd.Flags().BoolVar(&rigRestartNuclear, "nuclear", false, "DANGER: Bypass ALL safety checks (loses uncommitted work!)")
}

func runRigAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Handle --adopt mode: register existing directory
	if rigAddAdopt {
		return runRigAdopt(cmd, args)
	}

	// Normal add mode requires git URL
	if len(args) < 2 {
		return fmt.Errorf("git-url is required (or use --adopt to register an existing directory)")
	}
	gitURL := args[1]

	// Ensure beads (bd) is available before proceeding
	if err := deps.EnsureBeads(true); err != nil {
		return fmt.Errorf("beads dependency check failed: %w", err)
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		// Create new if doesn't exist
		rigsConfig = &config.RigsConfig{
			Version: 1,
			Rigs:    make(map[string]config.RigEntry),
		}
	}

	// Create rig manager
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	fmt.Printf("Creating rig %s...\n", style.Bold.Render(name))
	fmt.Printf("  Repository: %s\n", gitURL)
	if rigAddLocalRepo != "" {
		fmt.Printf("  Local repo: %s\n", rigAddLocalRepo)
	}

	startTime := time.Now()

	// Add the rig
	newRig, err := mgr.AddRig(rig.AddRigOptions{
		Name:          name,
		GitURL:        gitURL,
		BeadsPrefix:   rigAddPrefix,
		LocalRepo:     rigAddLocalRepo,
		DefaultBranch: rigAddBranch,
	})
	if err != nil {
		return fmt.Errorf("adding rig: %w", err)
	}

	// Save updated rigs config
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		return fmt.Errorf("saving rigs config: %w", err)
	}

	// Add route to town-level routes.jsonl for prefix-based routing.
	// Route points to the canonical beads location:
	// - If source repo has .beads/ tracked in git, route to mayor/rig
	// - Otherwise route to rig root (where initBeads creates the database)
	// The conditional routing is necessary because initBeads creates the database at
	// "<rig>/.beads", while repos with tracked beads have their database at mayor/rig/.beads.
	var beadsWorkDir string
	if newRig.Config.Prefix != "" {
		routePath := name
		mayorRigBeads := filepath.Join(townRoot, name, "mayor", "rig", ".beads")
		if _, err := os.Stat(mayorRigBeads); err == nil {
			// Source repo has .beads/ tracked - route to mayor/rig
			routePath = name + "/mayor/rig"
			beadsWorkDir = filepath.Join(townRoot, name, "mayor", "rig")
		} else {
			beadsWorkDir = filepath.Join(townRoot, name)
		}
		route := beads.Route{
			Prefix: newRig.Config.Prefix + "-",
			Path:   routePath,
		}
		if err := beads.AppendRoute(townRoot, route); err != nil {
			// Non-fatal: routing will still work, just not from town root
			fmt.Printf("  %s Could not update routes.jsonl: %v\n", style.Warning.Render("!"), err)
		}
	}

	// Create rig identity bead
	if newRig.Config.Prefix != "" && beadsWorkDir != "" {
		bd := beads.New(beadsWorkDir)
		rigBeadID := beads.RigBeadIDWithPrefix(newRig.Config.Prefix, name)
		fields := &beads.RigFields{
			Repo:   gitURL,
			Prefix: newRig.Config.Prefix,
			State:  "active",
		}
		if _, err := bd.CreateRigBead(rigBeadID, name, fields); err != nil {
			// Non-fatal: rig is functional without the identity bead
			fmt.Printf("  %s Could not create rig identity bead: %v\n", style.Warning.Render("!"), err)
		} else {
			fmt.Printf("  Created rig identity bead: %s\n", rigBeadID)
		}
	}

	elapsed := time.Since(startTime)

	// Read default branch from rig config
	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(filepath.Join(townRoot, name)); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	fmt.Printf("\n%s Rig created in %.1fs\n", style.Success.Render("✓"), elapsed.Seconds())
	fmt.Printf("\nStructure:\n")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  ├── config.json\n")
	fmt.Printf("  ├── .repo.git/        (shared bare repo for refinery+polecats)\n")
	fmt.Printf("  ├── .beads/           (prefix: %s)\n", newRig.Config.Prefix)
	fmt.Printf("  ├── plugins/          (rig-level plugins)\n")
	fmt.Printf("  ├── mayor/rig/        (clone: %s)\n", defaultBranch)
	fmt.Printf("  ├── refinery/rig/     (worktree: %s, sees polecat branches)\n", defaultBranch)
	fmt.Printf("  ├── crew/             (empty - add crew with 'gt crew add')\n")
	fmt.Printf("  ├── witness/\n")
	fmt.Printf("  └── polecats/\n")

	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  gt crew add <name> --rig %s   # Create your personal workspace\n", name)
	fmt.Printf("  cd %s/crew/<name>              # Start working\n", filepath.Join(townRoot, name))

	return nil
}

func runRigList(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		fmt.Println("No rigs configured.")
		return nil
	}

	if len(rigsConfig.Rigs) == 0 {
		fmt.Println("No rigs configured.")
		fmt.Printf("\nAdd one with: %s\n", style.Dim.Render("gt rig add <name> <git-url>"))
		return nil
	}

	// Create rig manager to get details
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	fmt.Printf("Rigs in %s:\n\n", townRoot)

	for name := range rigsConfig.Rigs {
		r, err := mgr.GetRig(name)
		if err != nil {
			fmt.Printf("  %s %s\n", style.Warning.Render("!"), name)
			continue
		}

		summary := r.Summary()
		fmt.Printf("  %s\n", style.Bold.Render(name))
		fmt.Printf("    Polecats: %d  Crew: %d\n", summary.PolecatCount, summary.CrewCount)

		agents := []string{}
		if summary.HasRefinery {
			agents = append(agents, "refinery")
		}
		if summary.HasWitness {
			agents = append(agents, "witness")
		}
		if r.HasMayor {
			agents = append(agents, "mayor")
		}
		if len(agents) > 0 {
			fmt.Printf("    Agents: %v\n", agents)
		}
		fmt.Println()
	}

	return nil
}

func runRigRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		return fmt.Errorf("loading rigs config: %w", err)
	}

	// Get the rig's beads prefix before removing (needed for route cleanup)
	var beadsPrefix string
	if entry, ok := rigsConfig.Rigs[name]; ok && entry.BeadsConfig != nil {
		beadsPrefix = entry.BeadsConfig.Prefix
	}

	// Create rig manager
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	if err := mgr.RemoveRig(name); err != nil {
		return fmt.Errorf("removing rig: %w", err)
	}

	// Save updated config
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		return fmt.Errorf("saving rigs config: %w", err)
	}

	// Remove route from routes.jsonl (issue #899)
	if beadsPrefix != "" {
		if err := beads.RemoveRoute(townRoot, beadsPrefix+"-"); err != nil {
			// Non-fatal: log warning but continue
			fmt.Printf("  %s Could not remove route from routes.jsonl: %v\n", style.Warning.Render("!"), err)
		}
	}

	fmt.Printf("%s Rig %s removed from registry\n", style.Success.Render("✓"), name)
	fmt.Printf("\nNote: Files at %s were NOT deleted.\n", filepath.Join(townRoot, name))
	fmt.Printf("To delete: %s\n", style.Dim.Render(fmt.Sprintf("rm -rf %s", filepath.Join(townRoot, name))))

	return nil
}

func runRigAdopt(_ *cobra.Command, args []string) error {
	name := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{
			Version: 1,
			Rigs:    make(map[string]config.RigEntry),
		}
	}

	// Create rig manager
	g := git.NewGit(townRoot)
	mgr := rig.NewManager(townRoot, rigsConfig, g)

	fmt.Printf("Adopting existing rig %s...\n", style.Bold.Render(name))

	// Register the existing rig
	result, err := mgr.RegisterRig(rig.RegisterRigOptions{
		Name:        name,
		GitURL:      rigAddAdoptURL,
		BeadsPrefix: rigAddPrefix,
		Force:       rigAddAdoptForce,
	})
	if err != nil {
		return fmt.Errorf("adopting rig: %w", err)
	}

	// Save updated config
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		return fmt.Errorf("saving rigs config: %w", err)
	}

	// Add route to town-level routes.jsonl for prefix-based routing
	if result.BeadsPrefix != "" {
		routePath := name
		mayorRigBeads := filepath.Join(townRoot, name, "mayor", "rig", ".beads")
		if _, err := os.Stat(mayorRigBeads); err == nil {
			routePath = name + "/mayor/rig"
		}
		route := beads.Route{
			Prefix: result.BeadsPrefix + "-",
			Path:   routePath,
		}
		if err := beads.AppendRoute(townRoot, route); err != nil {
			fmt.Printf("  %s Could not update routes.jsonl: %v\n", style.Warning.Render("!"), err)
		}
	}

	// Print results
	fmt.Printf("\n%s Rig %s adopted\n", style.Success.Render("✓"), name)
	if result.FromConfig {
		fmt.Printf("  %s Read configuration from existing config.json\n", style.Dim.Render("ℹ"))
	}
	fmt.Printf("  Repository: %s\n", result.GitURL)
	fmt.Printf("  Prefix: %s\n", result.BeadsPrefix)
	if result.DefaultBranch != "" {
		fmt.Printf("  Default branch: %s\n", result.DefaultBranch)
	}

	return nil
}

func runRigReset(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Determine role to reset
	roleKey := rigResetRole
	if roleKey == "" {
		// Auto-detect using env-aware role detection
		roleInfo, err := GetRoleWithContext(cwd, townRoot)
		if err != nil {
			return fmt.Errorf("detecting role: %w", err)
		}
		if roleInfo.Role == RoleUnknown {
			return fmt.Errorf("could not detect role; use --role to specify")
		}
		roleKey = string(roleInfo.Role)
	}

	// If no specific flags, reset all; otherwise only reset what's specified
	resetAll := !rigResetHandoff && !rigResetMail && !rigResetStale

	// Town beads for handoff/mail operations
	townBd := beads.New(townRoot)
	// Rig beads for issue operations (uses cwd to find .beads/)
	rigBd := beads.New(cwd)

	// Reset handoff content
	if resetAll || rigResetHandoff {
		if err := townBd.ClearHandoffContent(roleKey); err != nil {
			return fmt.Errorf("clearing handoff content: %w", err)
		}
		fmt.Printf("%s Cleared handoff content for %s\n", style.Success.Render("✓"), roleKey)
	}

	// Clear stale mail messages
	if resetAll || rigResetMail {
		result, err := townBd.ClearMail("Cleared during reset")
		if err != nil {
			return fmt.Errorf("clearing mail: %w", err)
		}
		if result.Closed > 0 || result.Cleared > 0 {
			fmt.Printf("%s Cleared mail: %d closed, %d pinned cleared\n",
				style.Success.Render("✓"), result.Closed, result.Cleared)
		} else {
			fmt.Printf("%s No mail to clear\n", style.Success.Render("✓"))
		}
	}

	// Reset stale in_progress issues
	if resetAll || rigResetStale {
		if err := runResetStale(rigBd, rigResetDryRun); err != nil {
			return fmt.Errorf("resetting stale issues: %w", err)
		}
	}

	return nil
}

// runResetStale resets in_progress issues whose assigned agent no longer has a session.
func runResetStale(bd *beads.Beads, dryRun bool) error {
	t := tmux.NewTmux()

	// Get all in_progress issues
	issues, err := bd.List(beads.ListOptions{
		Status:   "in_progress",
		Priority: -1, // All priorities
	})
	if err != nil {
		return fmt.Errorf("listing in_progress issues: %w", err)
	}

	if len(issues) == 0 {
		fmt.Printf("%s No in_progress issues found\n", style.Success.Render("✓"))
		return nil
	}

	var resetCount, skippedCount int
	var resetIssues []string

	for _, issue := range issues {
		if issue.Assignee == "" {
			continue // No assignee to check
		}

		// Parse assignee: rig/name or rig/crew/name
		sessionName, isPersistent := assigneeToSessionName(issue.Assignee)
		if sessionName == "" {
			continue // Couldn't parse assignee
		}

		// Check if session exists
		hasSession, err := t.HasSession(sessionName)
		if err != nil {
			// tmux error, skip this one
			continue
		}

		if hasSession {
			continue // Session exists, not stale
		}

		// For crew (persistent identities), only reset if explicitly checking sessions
		if isPersistent {
			skippedCount++
			if dryRun {
				fmt.Printf("  %s: %s %s\n",
					style.Dim.Render(issue.ID),
					issue.Assignee,
					style.Dim.Render("(persistent, skipped)"))
			}
			continue
		}

		// Session doesn't exist - this is stale
		if dryRun {
			fmt.Printf("  %s: %s (no session) → open\n",
				style.Bold.Render(issue.ID),
				issue.Assignee)
		} else {
			// Reset status to open and clear assignee
			openStatus := "open"
			emptyAssignee := ""
			if err := bd.Update(issue.ID, beads.UpdateOptions{
				Status:   &openStatus,
				Assignee: &emptyAssignee,
			}); err != nil {
				fmt.Printf("  %s Failed to reset %s: %v\n",
					style.Warning.Render("⚠"),
					issue.ID, err)
				continue
			}
		}
		resetCount++
		resetIssues = append(resetIssues, issue.ID)
	}

	if dryRun {
		if resetCount > 0 || skippedCount > 0 {
			fmt.Printf("\n%s Would reset %d issues, skip %d persistent\n",
				style.Dim.Render("(dry-run)"),
				resetCount, skippedCount)
		} else {
			fmt.Printf("%s No stale issues found\n", style.Success.Render("✓"))
		}
	} else {
		if resetCount > 0 {
			fmt.Printf("%s Reset %d stale issues: %v\n",
				style.Success.Render("✓"),
				resetCount, resetIssues)
		} else {
			fmt.Printf("%s No stale issues to reset\n", style.Success.Render("✓"))
		}
		if skippedCount > 0 {
			fmt.Printf("  Skipped %d persistent (crew) issues\n", skippedCount)
		}
	}

	return nil
}

// assigneeToSessionName converts an assignee (rig/name or rig/crew/name) to tmux session name.
// Returns the session name and whether this is a persistent identity (crew).
func assigneeToSessionName(assignee string) (sessionName string, isPersistent bool) {
	parts := strings.Split(assignee, "/")

	switch len(parts) {
	case 2:
		// rig/polecatName -> gt-rig-polecatName
		return fmt.Sprintf("gt-%s-%s", parts[0], parts[1]), false
	case 3:
		// rig/crew/name -> gt-rig-crew-name
		if parts[1] == "crew" {
			return fmt.Sprintf("gt-%s-crew-%s", parts[0], parts[2]), true
		}
		// Other 3-part formats not recognized
		return "", false
	default:
		return "", false
	}
}

// Helper to check if path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runRigBoot(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config and get rig
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	fmt.Printf("Booting rig %s...\n", style.Bold.Render(rigName))

	var started []string
	var skipped []string

	t := tmux.NewTmux()

	// 1. Start the witness
	// Check actual tmux session, not state file (may be stale)
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
	witnessRunning, _ := t.HasSession(witnessSession)
	if witnessRunning {
		skipped = append(skipped, "witness (already running)")
	} else {
		fmt.Printf("  Starting witness...\n")
		witMgr := witness.NewManager(r)
		if err := witMgr.Start(false, "", nil); err != nil {
			if err == witness.ErrAlreadyRunning {
				skipped = append(skipped, "witness (already running)")
			} else {
				return fmt.Errorf("starting witness: %w", err)
			}
		} else {
			started = append(started, "witness")
		}
	}

	// 2. Start the refinery
	// Check actual tmux session, not state file (may be stale)
	refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
	refineryRunning, _ := t.HasSession(refinerySession)
	if refineryRunning {
		skipped = append(skipped, "refinery (already running)")
	} else {
		fmt.Printf("  Starting refinery...\n")
		refMgr := refinery.NewManager(r)
		if err := refMgr.Start(false, ""); err != nil { // false = background mode
			return fmt.Errorf("starting refinery: %w", err)
		}
		started = append(started, "refinery")
	}

	// Report results
	if len(started) > 0 {
		fmt.Printf("%s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
	}
	if len(skipped) > 0 {
		fmt.Printf("%s Skipped: %s\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
	}

	return nil
}

func runRigStart(cmd *cobra.Command, args []string) error {
	// Find workspace once
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	t := tmux.NewTmux()

	var successRigs []string
	var failedRigs []string

	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failedRigs = append(failedRigs, rigName)
			continue
		}

		// Check if rig is parked or docked
		cfg := wisp.NewConfig(townRoot, rigName)
		status := cfg.GetString("status")
		if status == "parked" || status == "docked" {
			fmt.Printf("%s Rig '%s' is %s - skipping (use 'gt rig unpark' or 'gt rig undock' first)\n",
				style.Warning.Render("⚠"), rigName, status)
			continue
		}

		fmt.Printf("Starting rig %s...\n", style.Bold.Render(rigName))

		var started []string
		var skipped []string
		hasError := false

		// 1. Start the witness
		witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
		witnessRunning, _ := t.HasSession(witnessSession)
		if witnessRunning {
			skipped = append(skipped, "witness")
		} else {
			fmt.Printf("  Starting witness...\n")
			witMgr := witness.NewManager(r)
			if err := witMgr.Start(false, "", nil); err != nil {
				if err == witness.ErrAlreadyRunning {
					skipped = append(skipped, "witness")
				} else {
					fmt.Printf("  %s Failed to start witness: %v\n", style.Warning.Render("⚠"), err)
					hasError = true
				}
			} else {
				started = append(started, "witness")
			}
		}

		// 2. Start the refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
		refineryRunning, _ := t.HasSession(refinerySession)
		if refineryRunning {
			skipped = append(skipped, "refinery")
		} else {
			fmt.Printf("  Starting refinery...\n")
			refMgr := refinery.NewManager(r)
			if err := refMgr.Start(false, ""); err != nil {
				fmt.Printf("  %s Failed to start refinery: %v\n", style.Warning.Render("⚠"), err)
				hasError = true
			} else {
				started = append(started, "refinery")
			}
		}

		// Report results for this rig
		if len(started) > 0 {
			fmt.Printf("  %s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
		}
		if len(skipped) > 0 {
			fmt.Printf("  %s Skipped: %s (already running)\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
		}

		if hasError {
			failedRigs = append(failedRigs, rigName)
		} else {
			successRigs = append(successRigs, rigName)
		}
		fmt.Println()
	}

	// Summary
	if len(successRigs) > 0 {
		fmt.Printf("%s Started rigs: %s\n", style.Success.Render("✓"), strings.Join(successRigs, ", "))
	}
	if len(failedRigs) > 0 {
		fmt.Printf("%s Failed rigs: %s\n", style.Warning.Render("⚠"), strings.Join(failedRigs, ", "))
		return fmt.Errorf("some rigs failed to start")
	}

	return nil
}

func runRigShutdown(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config and get rig
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", rigName)
	}

	// Check all polecats for uncommitted work (unless nuclear)
	if !rigShutdownNuclear {
		polecatGit := git.NewGit(r.Path)
		polecatMgr := polecat.NewManager(r, polecatGit, nil) // nil tmux: just listing
		polecats, err := polecatMgr.List()
		if err == nil && len(polecats) > 0 {
			var problemPolecats []struct {
				name   string
				status *git.UncommittedWorkStatus
			}

			for _, p := range polecats {
				pGit := git.NewGit(p.ClonePath)
				status, err := pGit.CheckUncommittedWork()
				if err == nil && !status.Clean() {
					problemPolecats = append(problemPolecats, struct {
						name   string
						status *git.UncommittedWorkStatus
					}{p.Name, status})
				}
			}

			if len(problemPolecats) > 0 {
				fmt.Printf("\n%s Cannot shutdown - polecats have uncommitted work:\n\n", style.Warning.Render("⚠"))
				for _, pp := range problemPolecats {
					fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
				}
				fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
				return fmt.Errorf("refusing to shutdown with uncommitted work")
			}
		}
	}

	fmt.Printf("Shutting down rig %s...\n", style.Bold.Render(rigName))

	var errors []string

	// 1. Stop all polecat sessions
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)
	infos, err := polecatMgr.List()
	if err == nil && len(infos) > 0 {
		fmt.Printf("  Stopping %d polecat session(s)...\n", len(infos))
		if err := polecatMgr.StopAll(rigShutdownForce); err != nil {
			errors = append(errors, fmt.Sprintf("polecat sessions: %v", err))
		}
	}

	// 2. Stop the refinery
	refMgr := refinery.NewManager(r)
	if running, _ := refMgr.IsRunning(); running {
		fmt.Printf("  Stopping refinery...\n")
		if err := refMgr.Stop(); err != nil {
			errors = append(errors, fmt.Sprintf("refinery: %v", err))
		}
	}

	// 3. Stop the witness
	witMgr := witness.NewManager(r)
	if running, _ := witMgr.IsRunning(); running {
		fmt.Printf("  Stopping witness...\n")
		if err := witMgr.Stop(); err != nil {
			errors = append(errors, fmt.Sprintf("witness: %v", err))
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\n%s Some agents failed to stop:\n", style.Warning.Render("⚠"))
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("shutdown incomplete")
	}

	fmt.Printf("%s Rig %s shut down successfully\n", style.Success.Render("✓"), rigName)
	return nil
}

func runRigReboot(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	fmt.Printf("Rebooting rig %s...\n\n", style.Bold.Render(rigName))

	// Shutdown first
	if err := runRigShutdown(cmd, args); err != nil {
		// If shutdown fails due to uncommitted work, propagate the error
		return err
	}

	fmt.Println() // Blank line between shutdown and boot

	// Boot
	if err := runRigBoot(cmd, args); err != nil {
		return fmt.Errorf("boot failed: %w", err)
	}

	fmt.Printf("\n%s Rig %s rebooted successfully\n", style.Success.Render("✓"), rigName)
	return nil
}

func runRigStatus(cmd *cobra.Command, args []string) error {
	var rigName string

	if len(args) > 0 {
		rigName = args[0]
	} else {
		// Infer rig from current directory
		roleInfo, err := GetRole()
		if err != nil {
			return fmt.Errorf("detecting rig from current directory: %w", err)
		}
		if roleInfo.Rig == "" {
			return fmt.Errorf("could not detect rig from current directory; please specify rig name")
		}
		rigName = roleInfo.Rig
	}

	// Get rig
	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	t := tmux.NewTmux()

	// Header
	fmt.Printf("%s\n", style.Bold.Render(rigName))

	// Operational state
	opState, opSource := getRigOperationalState(townRoot, rigName)
	if opState == "OPERATIONAL" {
		fmt.Printf("  Status: %s\n", style.Success.Render(opState))
	} else if opState == "PARKED" {
		fmt.Printf("  Status: %s (%s)\n", style.Warning.Render(opState), opSource)
	} else if opState == "DOCKED" {
		fmt.Printf("  Status: %s (%s)\n", style.Dim.Render(opState), opSource)
	}

	fmt.Printf("  Path: %s\n", r.Path)
	if r.Config != nil && r.Config.Prefix != "" {
		fmt.Printf("  Beads prefix: %s-\n", r.Config.Prefix)
	}
	fmt.Println()

	// Witness status
	fmt.Printf("%s\n", style.Bold.Render("Witness"))
	witMgr := witness.NewManager(r)
	witnessRunning, _ := witMgr.IsRunning()
	if witnessRunning {
		fmt.Printf("  %s running\n", style.Success.Render("●"))
	} else {
		fmt.Printf("  %s stopped\n", style.Dim.Render("○"))
	}
	fmt.Println()

	// Refinery status
	fmt.Printf("%s\n", style.Bold.Render("Refinery"))
	refMgr := refinery.NewManager(r)
	refineryRunning, _ := refMgr.IsRunning()
	if refineryRunning {
		fmt.Printf("  %s running\n", style.Success.Render("●"))
		// Show queue size
		queue, err := refMgr.Queue()
		if err == nil && len(queue) > 0 {
			fmt.Printf("  Queue: %d items\n", len(queue))
		}
	} else {
		fmt.Printf("  %s stopped\n", style.Dim.Render("○"))
	}
	fmt.Println()

	// Polecats
	polecatGit := git.NewGit(r.Path)
	polecatMgr := polecat.NewManager(r, polecatGit, t)
	polecats, err := polecatMgr.List()
	fmt.Printf("%s", style.Bold.Render("Polecats"))
	if err != nil || len(polecats) == 0 {
		fmt.Printf(" (none)\n")
	} else {
		fmt.Printf(" (%d)\n", len(polecats))
		for _, p := range polecats {
			sessionName := fmt.Sprintf("gt-%s-%s", rigName, p.Name)
			hasSession, _ := t.HasSession(sessionName)

			sessionIcon := style.Dim.Render("○")
			if hasSession {
				sessionIcon = style.Success.Render("●")
			}

			stateStr := string(p.State)
			if p.Issue != "" {
				stateStr = fmt.Sprintf("%s → %s", p.State, p.Issue)
			}

			fmt.Printf("  %s %s: %s\n", sessionIcon, p.Name, stateStr)
		}
	}
	fmt.Println()

	// Crew
	crewMgr := crew.NewManager(r, git.NewGit(townRoot))
	crewWorkers, err := crewMgr.List()
	fmt.Printf("%s", style.Bold.Render("Crew"))
	if err != nil || len(crewWorkers) == 0 {
		fmt.Printf(" (none)\n")
	} else {
		fmt.Printf(" (%d)\n", len(crewWorkers))
		for _, w := range crewWorkers {
			sessionName := crewSessionName(rigName, w.Name)
			hasSession, _ := t.HasSession(sessionName)

			sessionIcon := style.Dim.Render("○")
			if hasSession {
				sessionIcon = style.Success.Render("●")
			}

			// Get git info
			crewGit := git.NewGit(w.ClonePath)
			branch, _ := crewGit.CurrentBranch()
			gitStatus, _ := crewGit.Status()

			gitInfo := ""
			if gitStatus != nil && !gitStatus.Clean {
				gitInfo = style.Warning.Render(" (dirty)")
			}

			fmt.Printf("  %s %s: %s%s\n", sessionIcon, w.Name, branch, gitInfo)
		}
	}

	return nil
}

func runRigStop(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)

	// Track results
	var succeeded []string
	var failed []string

	// Process each rig
	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failed = append(failed, rigName)
			continue
		}

		// Check all polecats for uncommitted work (unless nuclear)
		if !rigStopNuclear {
			polecatGit := git.NewGit(r.Path)
			polecatMgr := polecat.NewManager(r, polecatGit, nil) // nil tmux: just listing
			polecats, err := polecatMgr.List()
			if err == nil && len(polecats) > 0 {
				var problemPolecats []struct {
					name   string
					status *git.UncommittedWorkStatus
				}

				for _, p := range polecats {
					pGit := git.NewGit(p.ClonePath)
					status, err := pGit.CheckUncommittedWork()
					if err == nil && !status.Clean() {
						problemPolecats = append(problemPolecats, struct {
							name   string
							status *git.UncommittedWorkStatus
						}{p.Name, status})
					}
				}

				if len(problemPolecats) > 0 {
					fmt.Printf("\n%s Cannot stop %s - polecats have uncommitted work:\n", style.Warning.Render("⚠"), rigName)
					for _, pp := range problemPolecats {
						fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
					}
					failed = append(failed, rigName)
					continue
				}
			}
		}

		fmt.Printf("Stopping rig %s...\n", style.Bold.Render(rigName))

		var errors []string

		// 1. Stop all polecat sessions
		t := tmux.NewTmux()
		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err == nil && len(infos) > 0 {
			fmt.Printf("  Stopping %d polecat session(s)...\n", len(infos))
			if err := polecatMgr.StopAll(rigStopForce); err != nil {
				errors = append(errors, fmt.Sprintf("polecat sessions: %v", err))
			}
		}

		// 2. Stop the refinery
		refMgr := refinery.NewManager(r)
		if running, _ := refMgr.IsRunning(); running {
			fmt.Printf("  Stopping refinery...\n")
			if err := refMgr.Stop(); err != nil {
				errors = append(errors, fmt.Sprintf("refinery: %v", err))
			}
		}

		// 3. Stop the witness
		witMgr := witness.NewManager(r)
		if running, _ := witMgr.IsRunning(); running {
			fmt.Printf("  Stopping witness...\n")
			if err := witMgr.Stop(); err != nil {
				errors = append(errors, fmt.Sprintf("witness: %v", err))
			}
		}

		if len(errors) > 0 {
			fmt.Printf("%s Some agents in %s failed to stop:\n", style.Warning.Render("⚠"), rigName)
			for _, e := range errors {
				fmt.Printf("  - %s\n", e)
			}
			failed = append(failed, rigName)
		} else {
			fmt.Printf("%s Rig %s stopped\n", style.Success.Render("✓"), rigName)
			succeeded = append(succeeded, rigName)
		}
	}

	// Summary
	if len(args) > 1 {
		fmt.Println()
		if len(succeeded) > 0 {
			fmt.Printf("%s Stopped: %s\n", style.Success.Render("✓"), strings.Join(succeeded, ", "))
		}
		if len(failed) > 0 {
			fmt.Printf("%s Failed: %s\n", style.Warning.Render("⚠"), strings.Join(failed, ", "))
			fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
			return fmt.Errorf("some rigs failed to stop")
		}
	} else if len(failed) > 0 {
		fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
		return fmt.Errorf("rig failed to stop")
	}

	return nil
}

func runRigRestart(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load rigs config
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	t := tmux.NewTmux()

	// Track results
	var succeeded []string
	var failed []string

	// Process each rig
	for _, rigName := range args {
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			fmt.Printf("%s Rig '%s' not found\n", style.Warning.Render("⚠"), rigName)
			failed = append(failed, rigName)
			continue
		}

		fmt.Printf("Restarting rig %s...\n", style.Bold.Render(rigName))

		// Check all polecats for uncommitted work (unless nuclear)
		if !rigRestartNuclear {
			polecatGit := git.NewGit(r.Path)
			polecatMgr := polecat.NewManager(r, polecatGit, nil) // nil tmux: just listing
			polecats, err := polecatMgr.List()
			if err == nil && len(polecats) > 0 {
				var problemPolecats []struct {
					name   string
					status *git.UncommittedWorkStatus
				}

				for _, p := range polecats {
					pGit := git.NewGit(p.ClonePath)
					status, err := pGit.CheckUncommittedWork()
					if err == nil && !status.Clean() {
						problemPolecats = append(problemPolecats, struct {
							name   string
							status *git.UncommittedWorkStatus
						}{p.Name, status})
					}
				}

				if len(problemPolecats) > 0 {
					fmt.Printf("\n%s Cannot restart %s - polecats have uncommitted work:\n", style.Warning.Render("⚠"), rigName)
					for _, pp := range problemPolecats {
						fmt.Printf("  %s: %s\n", style.Bold.Render(pp.name), pp.status.String())
					}
					failed = append(failed, rigName)
					continue
				}
			}
		}

		var stopErrors []string
		var startErrors []string

		// === STOP PHASE ===
		fmt.Printf("  Stopping...\n")

		// 1. Stop all polecat sessions
		polecatMgr := polecat.NewSessionManager(t, r)
		infos, err := polecatMgr.List()
		if err == nil && len(infos) > 0 {
			fmt.Printf("    Stopping %d polecat session(s)...\n", len(infos))
			if err := polecatMgr.StopAll(rigRestartForce); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("polecat sessions: %v", err))
			}
		}

		// 2. Stop the refinery
		refMgr := refinery.NewManager(r)
		if running, _ := refMgr.IsRunning(); running {
			fmt.Printf("    Stopping refinery...\n")
			if err := refMgr.Stop(); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("refinery: %v", err))
			}
		}

		// 3. Stop the witness
		witMgr := witness.NewManager(r)
		if running, _ := witMgr.IsRunning(); running {
			fmt.Printf("    Stopping witness...\n")
			if err := witMgr.Stop(); err != nil {
				stopErrors = append(stopErrors, fmt.Sprintf("witness: %v", err))
			}
		}

		if len(stopErrors) > 0 {
			fmt.Printf("  %s Stop errors:\n", style.Warning.Render("⚠"))
			for _, e := range stopErrors {
				fmt.Printf("    - %s\n", e)
			}
			failed = append(failed, rigName)
			continue
		}

		// === START PHASE ===
		fmt.Printf("  Starting...\n")

		var started []string
		var skipped []string

		// 1. Start the witness
		witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
		witnessRunning, _ := t.HasSession(witnessSession)
		if witnessRunning {
			skipped = append(skipped, "witness")
		} else {
			fmt.Printf("    Starting witness...\n")
			if err := witMgr.Start(false, "", nil); err != nil {
				if err == witness.ErrAlreadyRunning {
					skipped = append(skipped, "witness")
				} else {
					fmt.Printf("    %s Failed to start witness: %v\n", style.Warning.Render("⚠"), err)
					startErrors = append(startErrors, fmt.Sprintf("witness: %v", err))
				}
			} else {
				started = append(started, "witness")
			}
		}

		// 2. Start the refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
		refineryRunning, _ := t.HasSession(refinerySession)
		if refineryRunning {
			skipped = append(skipped, "refinery")
		} else {
			fmt.Printf("    Starting refinery...\n")
			if err := refMgr.Start(false, ""); err != nil {
				fmt.Printf("    %s Failed to start refinery: %v\n", style.Warning.Render("⚠"), err)
				startErrors = append(startErrors, fmt.Sprintf("refinery: %v", err))
			} else {
				started = append(started, "refinery")
			}
		}

		// Report results for this rig
		if len(started) > 0 {
			fmt.Printf("  %s Started: %s\n", style.Success.Render("✓"), strings.Join(started, ", "))
		}
		if len(skipped) > 0 {
			fmt.Printf("  %s Skipped: %s (already running)\n", style.Dim.Render("•"), strings.Join(skipped, ", "))
		}

		if len(startErrors) > 0 {
			fmt.Printf("  %s Start errors:\n", style.Warning.Render("⚠"))
			for _, e := range startErrors {
				fmt.Printf("    - %s\n", e)
			}
			failed = append(failed, rigName)
		} else {
			fmt.Printf("%s Rig %s restarted\n", style.Success.Render("✓"), rigName)
			succeeded = append(succeeded, rigName)
		}
		fmt.Println()
	}

	// Summary
	if len(args) > 1 {
		if len(succeeded) > 0 {
			fmt.Printf("%s Restarted: %s\n", style.Success.Render("✓"), strings.Join(succeeded, ", "))
		}
		if len(failed) > 0 {
			fmt.Printf("%s Failed: %s\n", style.Warning.Render("⚠"), strings.Join(failed, ", "))
			fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
			return fmt.Errorf("some rigs failed to restart")
		}
	} else if len(failed) > 0 {
		fmt.Printf("\nUse %s to force shutdown (DANGER: will lose work!)\n", style.Bold.Render("--nuclear"))
		return fmt.Errorf("rig failed to restart")
	}

	return nil
}

// getRigOperationalState returns the operational state and source for a rig.
// It checks the wisp layer first (local/ephemeral), then rig bead labels (global).
// Returns state ("OPERATIONAL", "PARKED", or "DOCKED") and source ("local", "global - synced", or "default").
func getRigOperationalState(townRoot, rigName string) (state string, source string) {
	// Check wisp layer first (local/ephemeral overrides)
	wispConfig := wisp.NewConfig(townRoot, rigName)
	if status := wispConfig.GetString("status"); status != "" {
		switch strings.ToLower(status) {
		case "parked":
			return "PARKED", "local"
		case "docked":
			return "DOCKED", "local"
		}
	}

	// Check rig bead labels (global/synced)
	// Rig identity bead ID: <prefix>-rig-<name>
	// Look for status:docked or status:parked labels
	rigPath := filepath.Join(townRoot, rigName)
	rigBeadsDir := beads.ResolveBeadsDir(rigPath)
	bd := beads.NewWithBeadsDir(rigPath, rigBeadsDir)

	// Try to find the rig identity bead
	// Convention: <prefix>-rig-<rigName>
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.Beads != nil {
		rigBeadID := fmt.Sprintf("%s-rig-%s", rigCfg.Beads.Prefix, rigName)
		if issue, err := bd.Show(rigBeadID); err == nil {
			for _, label := range issue.Labels {
				if strings.HasPrefix(label, "status:") {
					statusValue := strings.TrimPrefix(label, "status:")
					switch strings.ToLower(statusValue) {
					case "docked":
						return "DOCKED", "global - synced"
					case "parked":
						return "PARKED", "global - synced"
					}
				}
			}
		}
	}

	// Default: operational
	return "OPERATIONAL", "default"
}
