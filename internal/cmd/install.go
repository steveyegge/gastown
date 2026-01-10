package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/shell"
	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
	"github.com/steveyegge/gastown/internal/wrappers"
)

var (
	installForce      bool
	installName       string
	installOwner      string
	installPublicName string
	installNoBeads    bool
	installGit        bool
	installGitHub     string
	installPublic     bool
	installShell      bool
	installWrappers   bool
)

var installCmd = &cobra.Command{
	Use:     "install [path]",
	GroupID: GroupWorkspace,
	Short:   "Create a new Gas Town HQ (workspace)",
	Long: `Create a new Gas Town HQ at the specified path.

QUICK START
───────────
  1. Create HQ:        gt install ~/gt --shell
  2. Add a project:    gt rig add myproject https://github.com/user/repo
  3. Join the crew:    gt crew add yourname --rig myproject
  4. Start working:    cd ~/gt/myproject/crew/yourname && gt attach

ARCHITECTURE
────────────
Gas Town is a multi-agent workspace manager. After setup, your HQ looks like:

  ~/gt/                          ← HQ (town root)
  ├── CLAUDE.md                  ← Mayor's role context
  ├── mayor/                     ← Town-level coordination
  │   └── rigs.json              ← Registry of all projects
  ├── .beads/                    ← Town-level issue tracking (hq-* prefix)
  │
  └── myproject/                 ← A "rig" (project container)
      ├── config.json            ← Rig configuration
      ├── .beads/                ← Rig-level issues (project prefix)
      ├── .repo.git/             ← Shared bare repo
      │
      ├── mayor/rig/             ← Mayor's working clone
      ├── refinery/rig/          ← Refinery reviews PRs here
      ├── witness/               ← Witness monitors this rig
      ├── polecats/              ← AI workers (spawned on demand)
      │   └── polecat-1/rig/     ← Each polecat has its own clone
      └── crew/                  ← Human workspaces
          └── yourname/          ← Your personal workspace

AGENTS
──────
  Mayor      Coordinates work across all rigs, delegates to Witness
  Witness    Monitors a rig, triages issues, spawns polecats
  Refinery   Reviews PRs, runs CI, merges approved changes
  Polecats   AI workers that implement features/fixes (ephemeral)
  Crew       Human developers with persistent workspaces

Each agent runs in its own tmux session with isolated git worktree.

WHAT THIS COMMAND CREATES
─────────────────────────
  - CLAUDE.md            Mayor role context
  - mayor/               Mayor config, state, and rig registry
  - .beads/              Town-level beads DB (hq-* prefix)

NEXT STEPS AFTER INSTALL
────────────────────────
  gt rig add <name> <url>     Add a project to manage
  gt crew add <name>          Create your workspace in a rig
  gt start                    Start the Mayor daemon
  gt status                   See what's running

Examples:
  gt install ~/gt                              # Create HQ at ~/gt
  gt install ~/gt --shell                      # Recommended: also add shell integration
  gt install . --name my-workspace             # Initialize current dir
  gt install ~/gt --git                        # Also init git with .gitignore
  gt install ~/gt --github=user/repo           # Create private GitHub repo`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

func init() {
	installCmd.Flags().BoolVarP(&installForce, "force", "f", false, "Overwrite existing HQ")
	installCmd.Flags().StringVarP(&installName, "name", "n", "", "Town name (defaults to directory name)")
	installCmd.Flags().StringVar(&installOwner, "owner", "", "Owner email for entity identity (defaults to git config user.email)")
	installCmd.Flags().StringVar(&installPublicName, "public-name", "", "Public display name (defaults to town name)")
	installCmd.Flags().BoolVar(&installNoBeads, "no-beads", false, "Skip town beads initialization")
	installCmd.Flags().BoolVar(&installGit, "git", false, "Initialize git with .gitignore")
	installCmd.Flags().StringVar(&installGitHub, "github", "", "Create GitHub repo (format: owner/repo, private by default)")
	installCmd.Flags().BoolVar(&installPublic, "public", false, "Make GitHub repo public (use with --github)")
	installCmd.Flags().BoolVar(&installShell, "shell", false, "Install shell integration (sets GT_TOWN_ROOT/GT_RIG env vars)")
	installCmd.Flags().BoolVar(&installWrappers, "wrappers", false, "Install gt-codex/gt-opencode wrapper scripts to ~/bin/")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	// Determine target path
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	// Expand ~ and resolve to absolute path
	if targetPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		targetPath = filepath.Join(home, targetPath[1:])
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Determine town name
	townName := installName
	if townName == "" {
		townName = filepath.Base(absPath)
	}

	// Check if already a workspace
	if isWS, _ := workspace.IsWorkspace(absPath); isWS && !installForce {
		return fmt.Errorf("directory is already a Gas Town HQ (use --force to reinitialize)")
	}

	// Check if inside an existing workspace
	if existingRoot, _ := workspace.Find(absPath); existingRoot != "" && existingRoot != absPath {
		style.PrintWarning("Creating HQ inside existing workspace at %s", existingRoot)
	}

	// Ensure beads (bd) is available before proceeding
	if !installNoBeads {
		if err := deps.EnsureBeads(true); err != nil {
			return fmt.Errorf("beads dependency check failed: %w", err)
		}
	}

	fmt.Printf("%s Creating Gas Town HQ at %s\n\n",
		style.Bold.Render("🏭"), style.Dim.Render(absPath))

	// Create directory structure
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Create mayor directory (holds config, state, and mail)
	mayorDir := filepath.Join(absPath, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		return fmt.Errorf("creating mayor directory: %w", err)
	}
	fmt.Printf("   ✓ Created mayor/\n")

	// Determine owner (defaults to git user.email)
	owner := installOwner
	if owner == "" {
		out, err := exec.Command("git", "config", "user.email").Output()
		if err == nil {
			owner = strings.TrimSpace(string(out))
		}
	}

	// Determine public name (defaults to town name)
	publicName := installPublicName
	if publicName == "" {
		publicName = townName
	}

	// Create town.json in mayor/
	townConfig := &config.TownConfig{
		Type:       "town",
		Version:    config.CurrentTownVersion,
		Name:       townName,
		Owner:      owner,
		PublicName: publicName,
		CreatedAt:  time.Now(),
	}
	townPath := filepath.Join(mayorDir, "town.json")
	if err := config.SaveTownConfig(townPath, townConfig); err != nil {
		return fmt.Errorf("writing town.json: %w", err)
	}
	fmt.Printf("   ✓ Created mayor/town.json\n")

	// Create rigs.json in mayor/
	rigsConfig := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs:    make(map[string]config.RigEntry),
	}
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		return fmt.Errorf("writing rigs.json: %w", err)
	}
	fmt.Printf("   ✓ Created mayor/rigs.json\n")

	// Create Mayor CLAUDE.md at mayor/ (Mayor's canonical home)
	// IMPORTANT: CLAUDE.md must be in ~/gt/mayor/, NOT ~/gt/
	// CLAUDE.md at town root would be inherited by ALL agents via directory traversal,
	// causing crew/polecat/etc to receive Mayor-specific instructions.
	if err := createMayorCLAUDEmd(mayorDir, absPath); err != nil {
		fmt.Printf("   %s Could not create CLAUDE.md: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Created mayor/CLAUDE.md\n")
	}

	// Create mayor settings (mayor runs from ~/gt/mayor/)
	// IMPORTANT: Settings must be in ~/gt/mayor/.claude/, NOT ~/gt/.claude/
	// Settings at town root would be found by ALL agents via directory traversal,
	// causing crew/polecat/etc to cd to town root before running commands.
	// mayorDir already defined above
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		fmt.Printf("   %s Could not create mayor directory: %v\n", style.Dim.Render("⚠"), err)
	} else if err := claude.EnsureSettingsForRole(mayorDir, "mayor"); err != nil {
		fmt.Printf("   %s Could not create mayor settings: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Created mayor/.claude/settings.json\n")
	}

	// Create deacon directory and settings (deacon runs from ~/gt/deacon/)
	deaconDir := filepath.Join(absPath, "deacon")
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		fmt.Printf("   %s Could not create deacon directory: %v\n", style.Dim.Render("⚠"), err)
	} else if err := claude.EnsureSettingsForRole(deaconDir, "deacon"); err != nil {
		fmt.Printf("   %s Could not create deacon settings: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Created deacon/.claude/settings.json\n")
	}

	// Initialize git BEFORE beads so that bd can compute repository fingerprint.
	// The fingerprint is required for the daemon to start properly.
	if installGit || installGitHub != "" {
		fmt.Println()
		if err := InitGitForHarness(absPath, installGitHub, !installPublic); err != nil {
			return fmt.Errorf("git initialization failed: %w", err)
		}
	}

	// Initialize town-level beads database (optional)
	// Town beads (hq- prefix) stores mayor mail, cross-rig coordination, and handoffs.
	// Rig beads are separate and have their own prefixes.
	if !installNoBeads {
		if err := initTownBeads(absPath); err != nil {
			fmt.Printf("   %s Could not initialize town beads: %v\n", style.Dim.Render("⚠"), err)
		} else {
			fmt.Printf("   ✓ Initialized .beads/ (town-level beads)\n")

			// Provision embedded formulas to .beads/formulas/
			if count, err := formula.ProvisionFormulas(absPath); err != nil {
				// Non-fatal: formulas are optional, just convenience
				fmt.Printf("   %s Could not provision formulas: %v\n", style.Dim.Render("⚠"), err)
			} else if count > 0 {
				fmt.Printf("   ✓ Provisioned %d formulas\n", count)
			}
		}

		// Create town-level agent beads (Mayor, Deacon) and role beads.
		// These use hq- prefix and are stored in town beads for cross-rig coordination.
		if err := initTownAgentBeads(absPath); err != nil {
			fmt.Printf("   %s Could not create town-level agent beads: %v\n", style.Dim.Render("⚠"), err)
		}
	}

	// Detect and save overseer identity
	overseer, err := config.DetectOverseer(absPath)
	if err != nil {
		fmt.Printf("   %s Could not detect overseer identity: %v\n", style.Dim.Render("⚠"), err)
	} else {
		overseerPath := config.OverseerConfigPath(absPath)
		if err := config.SaveOverseerConfig(overseerPath, overseer); err != nil {
			fmt.Printf("   %s Could not save overseer config: %v\n", style.Dim.Render("⚠"), err)
		} else {
			fmt.Printf("   ✓ Detected overseer: %s (via %s)\n", overseer.FormatOverseerIdentity(), overseer.Source)
		}
	}

	// Provision town-level slash commands (.claude/commands/)
	// All agents inherit these via Claude's directory traversal - no per-workspace copies needed.
	if err := templates.ProvisionCommands(absPath); err != nil {
		fmt.Printf("   %s Could not provision slash commands: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Created .claude/commands/ (slash commands for all agents)\n")
	}

	if installShell {
		fmt.Println()
		if err := shell.Install(); err != nil {
			fmt.Printf("   %s Could not install shell integration: %v\n", style.Dim.Render("⚠"), err)
		} else {
			fmt.Printf("   ✓ Installed shell integration (%s)\n", shell.RCFilePath(shell.DetectShell()))
		}
		if err := state.Enable(Version); err != nil {
			fmt.Printf("   %s Could not enable Gas Town: %v\n", style.Dim.Render("⚠"), err)
		} else {
			fmt.Printf("   ✓ Enabled Gas Town globally\n")
		}
	}

	if installWrappers {
		fmt.Println()
		if err := wrappers.Install(); err != nil {
			fmt.Printf("   %s Could not install wrapper scripts: %v\n", style.Dim.Render("⚠"), err)
		} else {
			fmt.Printf("   ✓ Installed gt-codex and gt-opencode to %s\n", wrappers.BinDir())
		}
	}

	fmt.Printf("\n%s HQ created successfully!\n", style.Bold.Render("✓"))
	fmt.Println()
	fmt.Println("Next steps:")
	step := 1
	if !installGit && installGitHub == "" {
		fmt.Printf("  %d. Initialize git: %s\n", step, style.Dim.Render("gt git-init"))
		step++
	}
	fmt.Printf("  %d. Add a rig: %s\n", step, style.Dim.Render("gt rig add <name> <git-url>"))
	step++
	fmt.Printf("  %d. (Optional) Configure agents: %s\n", step, style.Dim.Render("gt config agent list"))
	step++
	fmt.Printf("  %d. Enter the Mayor's office: %s\n", step, style.Dim.Render("gt mayor attach"))

	return nil
}

func createMayorCLAUDEmd(mayorDir, townRoot string) error {
	townName, _ := workspace.GetTownName(townRoot)
	return templates.CreateMayorCLAUDEmd(
		mayorDir,
		townRoot,
		townName,
		session.MayorSessionName(),
		session.DeaconSessionName(),
	)
}

func writeJSON(path string, data interface{}) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// initTownBeads initializes town-level beads database using bd init.
// Town beads use the "hq-" prefix for mayor mail and cross-rig coordination.
// If the directory already has beads with a different prefix, we use that
// prefix instead to allow coexistence with existing beads users.
func initTownBeads(townPath string) error {
	// Check if beads already exists with a different prefix
	existingPrefix := detectExistingBeadsPrefix(townPath)
	usePrefix := "hq"

	if existingPrefix != "" && existingPrefix != "hq" {
		// Existing beads found - use that prefix for compatibility
		fmt.Printf("   ℹ Found existing beads with prefix '%s' - using for compatibility\n", existingPrefix)
		usePrefix = existingPrefix
	}

	// Run: bd init --prefix <prefix>
	cmd := exec.Command("bd", "init", "--prefix", usePrefix)
	cmd.Dir = townPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if beads is already initialized
		if strings.Contains(string(output), "already initialized") {
			// Already initialized - still need to ensure fingerprint exists
		} else if strings.Contains(string(output), "database uses") {
			// Prefix mismatch - extract actual prefix and retry
			// Error format: "database uses 'X' but you specified 'Y'"
			if actual := extractPrefixFromError(string(output)); actual != "" {
				fmt.Printf("   ℹ Detected existing beads prefix '%s' - adapting\n", actual)
				usePrefix = actual
				// Don't retry init - just proceed with existing database
			}
		} else {
			return fmt.Errorf("bd init failed: %s", strings.TrimSpace(string(output)))
		}
	}

	// Configure custom types for Gas Town (agent, role, rig, convoy, slot).
	// These were extracted from beads core in v0.46.0 and now require explicit config.
	configCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	configCmd.Dir = townPath
	if configOutput, configErr := configCmd.CombinedOutput(); configErr != nil {
		// Non-fatal: older beads versions don't need this, newer ones do
		fmt.Printf("   %s Could not set custom types: %s\n", style.Dim.Render("⚠"), strings.TrimSpace(string(configOutput)))
	}

	// Ensure database has repository fingerprint (GH #25).
	// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
	// Without fingerprint, the bd daemon fails to start silently.
	if err := ensureRepoFingerprint(townPath); err != nil {
		// Non-fatal: fingerprint is optional for functionality, just daemon optimization
		fmt.Printf("   %s Could not verify repo fingerprint: %v\n", style.Dim.Render("⚠"), err)
	}

	// Ensure routes.jsonl has an explicit town-level mapping for the prefix.
	// This keeps operations stable even when invoked from rig worktrees.
	if err := beads.AppendRoute(townPath, beads.Route{Prefix: usePrefix + "-", Path: "."}); err != nil {
		// Non-fatal: routing still works in many contexts, but explicit mapping is preferred.
		fmt.Printf("   %s Could not update routes.jsonl: %v\n", style.Dim.Render("⚠"), err)
	}

	return nil
}

// detectExistingBeadsPrefix checks if a directory has an existing beads database
// and returns its prefix. Returns empty string if no beads or can't determine prefix.
func detectExistingBeadsPrefix(path string) string {
	configPath := filepath.Join(path, ".beads", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// Parse YAML for issue-prefix or prefix field
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "issue-prefix:") {
			prefix := strings.TrimPrefix(line, "issue-prefix:")
			prefix = strings.TrimSpace(prefix)
			prefix = strings.Trim(prefix, "\"'")
			if prefix != "" {
				return prefix
			}
		}
		if strings.HasPrefix(line, "prefix:") {
			prefix := strings.TrimPrefix(line, "prefix:")
			prefix = strings.TrimSpace(prefix)
			prefix = strings.Trim(prefix, "\"'")
			if prefix != "" {
				return prefix
			}
		}
	}
	return ""
}

// extractPrefixFromError extracts the actual prefix from a beads error message.
// Error format: "database uses 'X' but you specified 'Y'"
func extractPrefixFromError(errMsg string) string {
	// Look for pattern: database uses 'X'
	if idx := strings.Index(errMsg, "database uses '"); idx != -1 {
		start := idx + len("database uses '")
		end := strings.Index(errMsg[start:], "'")
		if end != -1 {
			return errMsg[start : start+end]
		}
	}
	return ""
}

// ensureRepoFingerprint runs bd migrate --update-repo-id to ensure the database
// has a repository fingerprint. Legacy databases (pre-0.17.5) lack this, which
// prevents the daemon from starting properly.
func ensureRepoFingerprint(beadsPath string) error {
	cmd := exec.Command("bd", "migrate", "--update-repo-id")
	cmd.Dir = beadsPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd migrate --update-repo-id: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// ensureCustomTypes registers Gas Town custom issue types with beads.
// Beads core only supports built-in types (bug, feature, task, etc.).
// Gas Town needs custom types: agent, role, rig, convoy, slot.
// This is idempotent - safe to call multiple times.
func ensureCustomTypes(beadsPath string) error {
	cmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	cmd.Dir = beadsPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd config set types.custom: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// initTownAgentBeads creates town-level agent and role beads using hq- prefix.
// This creates:
//   - hq-mayor, hq-deacon (agent beads for town-level agents)
//   - hq-mayor-role, hq-deacon-role, hq-witness-role, hq-refinery-role,
//     hq-polecat-role, hq-crew-role (role definition beads)
//
// These beads are stored in town beads (~/gt/.beads/) and are shared across all rigs.
// Rig-level agent beads (witness, refinery) are created by gt rig add in rig beads.
//
// ERROR HANDLING ASYMMETRY:
// Agent beads (Mayor, Deacon) use hard fail - installation aborts if creation fails.
// Role beads use soft fail - logs warning and continues if creation fails.
//
// Rationale: Agent beads are identity beads that track agent state, hooks, and
// form the foundation of the CV/reputation ledger. Without them, agents cannot
// be properly tracked or coordinated. Role beads are documentation templates
// that define role characteristics but are not required for agent operation -
// agents can function without their role bead existing.
func initTownAgentBeads(townPath string) error {
	bd := beads.New(townPath)

	// bd init doesn't enable "custom" issue types by default, but Gas Town uses
	// agent/role beads during install and runtime. Ensure these types are enabled
	// before attempting to create any town-level system beads.
	if err := ensureBeadsCustomTypes(townPath, []string{"agent", "role", "rig", "convoy", "slot"}); err != nil {
		return err
	}

	// Role beads (global templates)
	roleDefs := []struct {
		id    string
		title string
		desc  string
	}{
		{
			id:    beads.MayorRoleBeadIDTown(),
			title: "Mayor Role",
			desc:  "Role definition for Mayor agents. Global coordinator for cross-rig work.",
		},
		{
			id:    beads.DeaconRoleBeadIDTown(),
			title: "Deacon Role",
			desc:  "Role definition for Deacon agents. Daemon beacon for heartbeats and monitoring.",
		},
		{
			id:    beads.DogRoleBeadIDTown(),
			title: "Dog Role",
			desc:  "Role definition for Dog agents. Town-level workers for cross-rig tasks.",
		},
		{
			id:    beads.WitnessRoleBeadIDTown(),
			title: "Witness Role",
			desc:  "Role definition for Witness agents. Per-rig worker monitor with progressive nudging.",
		},
		{
			id:    beads.RefineryRoleBeadIDTown(),
			title: "Refinery Role",
			desc:  "Role definition for Refinery agents. Merge queue processor with verification gates.",
		},
		{
			id:    beads.PolecatRoleBeadIDTown(),
			title: "Polecat Role",
			desc:  "Role definition for Polecat agents. Ephemeral workers for batch work dispatch.",
		},
		{
			id:    beads.CrewRoleBeadIDTown(),
			title: "Crew Role",
			desc:  "Role definition for Crew agents. Persistent user-managed workspaces.",
		},
	}

	for _, role := range roleDefs {
		// Check if already exists
		if _, err := bd.Show(role.id); err == nil {
			continue // Already exists
		}

		// Create role bead using bd create --type=role
		cmd := exec.Command("bd", "create",
			"--type=role",
			"--id="+role.id,
			"--title="+role.title,
			"--description="+role.desc,
		)
		cmd.Dir = townPath
		if output, err := cmd.CombinedOutput(); err != nil {
			// Log but continue - role beads are optional
			fmt.Printf("   %s Could not create role bead %s: %s\n",
				style.Dim.Render("⚠"), role.id, strings.TrimSpace(string(output)))
			continue
		}
		fmt.Printf("   ✓ Created role bead: %s\n", role.id)
	}

	// Town-level agent beads
	agentDefs := []struct {
		id       string
		roleType string
		title    string
	}{
		{
			id:       beads.MayorBeadIDTown(),
			roleType: "mayor",
			title:    "Mayor - global coordinator, handles cross-rig communication and escalations.",
		},
		{
			id:       beads.DeaconBeadIDTown(),
			roleType: "deacon",
			title:    "Deacon (daemon beacon) - receives mechanical heartbeats, runs town plugins and monitoring.",
		},
	}

	existingAgents, err := bd.List(beads.ListOptions{
		Status:   "all",
		Type:     "agent",
		Priority: -1,
	})
	if err != nil {
		return fmt.Errorf("listing existing agent beads: %w", err)
	}
	existingAgentIDs := make(map[string]struct{}, len(existingAgents))
	for _, issue := range existingAgents {
		existingAgentIDs[issue.ID] = struct{}{}
	}

	for _, agent := range agentDefs {
		if _, ok := existingAgentIDs[agent.id]; ok {
			continue
		}

		fields := &beads.AgentFields{
			RoleType:   agent.roleType,
			Rig:        "", // Town-level agents have no rig
			AgentState: "idle",
			HookBead:   "",
			RoleBead:   beads.RoleBeadIDTown(agent.roleType),
		}

		if _, err := bd.CreateAgentBead(agent.id, agent.title, fields); err != nil {
			return fmt.Errorf("creating %s: %w", agent.id, err)
		}
		fmt.Printf("   ✓ Created agent bead: %s\n", agent.id)
	}

	return nil
}

func ensureBeadsCustomTypes(workDir string, types []string) error {
	if len(types) == 0 {
		return nil
	}

	cmd := exec.Command("bd", "config", "set", "types.custom", strings.Join(types, ","))
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd config set types.custom failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
