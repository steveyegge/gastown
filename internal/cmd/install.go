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

The HQ (headquarters) is the top-level directory where Gas Town is installed -
the root of your workspace where all rigs and agents live. It contains:
  - CLAUDE.md            Mayor role context (Mayor runs from HQ root)
  - mayor/               Mayor config, state, and rig registry
  - .beads/              Town-level beads DB (hq-* prefix for mayor mail)

If path is omitted, uses the current directory.

See docs/hq.md for advanced HQ configurations including beads
redirects, multi-system setups, and HQ templates.

Examples:
  gt install ~/gt                              # Create HQ at ~/gt
  gt install . --name my-workspace             # Initialize current dir
  gt install ~/gt --no-beads                   # Skip .beads/ initialization
  gt install ~/gt --git                        # Also init git with .gitignore
  gt install ~/gt --github=user/repo           # Create private GitHub repo (default)
  gt install ~/gt --github=user/repo --public  # Create public GitHub repo
  gt install ~/gt --shell                      # Install shell integration (sets GT_TOWN_ROOT/GT_RIG)`,
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
		// If only --wrappers is requested in existing town, just install wrappers and exit
		if installWrappers {
			if err := wrappers.Install(); err != nil {
				return fmt.Errorf("installing wrapper scripts: %w", err)
			}
			fmt.Printf("✓ Installed gt-codex and gt-opencode to %s\n", wrappers.BinDir())
			return nil
		}
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
			fmt.Printf("   ✓ Initialized .beads/ (town-level beads with hq- prefix)\n")

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

func createMayorCLAUDEmd(mayorDir, _ string) error {
	// Create a minimal bootstrap pointer instead of full context.
	// Full context is injected ephemerally by `gt prime` at session start.
	// This keeps the on-disk file small (<30 lines) per priming architecture.
	bootstrap := `# Mayor Context

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check mail: ` + "`gt mail inbox`" + `
- Check rigs: ` + "`gt rig list`" + `
- Start patrol: ` + "`gt patrol start`" + `
`
	claudePath := filepath.Join(mayorDir, "CLAUDE.md")
	return os.WriteFile(claudePath, []byte(bootstrap), 0644)
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
func initTownBeads(townPath string) error {
	// Run: bd init --prefix hq
	cmd := exec.Command("bd", "init", "--prefix", "hq")
	cmd.Dir = townPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if beads is already initialized
		if strings.Contains(string(output), "already initialized") {
			// Already initialized - still need to ensure fingerprint exists
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

	// Ensure issues.jsonl exists BEFORE creating routes.jsonl.
	// bd init creates beads.db but not issues.jsonl in SQLite mode.
	// If routes.jsonl is created first, bd's auto-export will write issues to routes.jsonl,
	// corrupting it. Creating an empty issues.jsonl prevents this.
	issuesJSONL := filepath.Join(townPath, ".beads", "issues.jsonl")
	if _, err := os.Stat(issuesJSONL); os.IsNotExist(err) {
		if err := os.WriteFile(issuesJSONL, []byte{}, 0644); err != nil {
			fmt.Printf("   %s Could not create issues.jsonl: %v\n", style.Dim.Render("⚠"), err)
		}
	}

	// Ensure routes.jsonl has an explicit town-level mapping for hq-* beads.
	// This keeps hq-* operations stable even when invoked from rig worktrees.
	if err := beads.AppendRoute(townPath, beads.Route{Prefix: "hq-", Path: "."}); err != nil {
		// Non-fatal: routing still works in many contexts, but explicit mapping is preferred.
		fmt.Printf("   %s Could not update routes.jsonl: %v\n", style.Dim.Render("⚠"), err)
	}

	return nil
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

	// Role beads (global templates) - use shared definitions from beads package
	for _, role := range beads.AllRoleBeadDefs() {
		// Check if already exists
		if _, err := bd.Show(role.ID); err == nil {
			continue // Already exists
		}

		// Create role bead using the beads API
		// CreateWithID with Type: "role" automatically adds gt:role label
		_, err := bd.CreateWithID(role.ID, beads.CreateOptions{
			Title:       role.Title,
			Type:        "role",
			Description: role.Desc,
			Priority:    -1, // No priority
		})
		if err != nil {
			// Log but continue - role beads are optional
			fmt.Printf("   %s Could not create role bead %s: %v\n",
				style.Dim.Render("⚠"), role.ID, err)
			continue
		}
		fmt.Printf("   ✓ Created role bead: %s\n", role.ID)
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
