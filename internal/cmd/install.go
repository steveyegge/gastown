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
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
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
  gt install ~/gt --github=user/repo --public  # Create public GitHub repo`,
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

	// Create mayor state.json
	mayorState := &config.AgentState{
		Role:       "mayor",
		LastActive: time.Now(),
	}
	statePath := filepath.Join(mayorDir, "state.json")
	if err := config.SaveAgentState(statePath, mayorState); err != nil {
		return fmt.Errorf("writing mayor state: %w", err)
	}
	fmt.Printf("   ✓ Created mayor/state.json\n")

	// Create Mayor CLAUDE.md at HQ root (Mayor runs from there)
	if err := createMayorCLAUDEmd(absPath, absPath); err != nil {
		fmt.Printf("   %s Could not create CLAUDE.md: %v\n", style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Created CLAUDE.md\n")
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

	// Initialize git if requested (--git or --github implies --git)
	if installGit || installGitHub != "" {
		fmt.Println()
		if err := InitGitForHarness(absPath, installGitHub, !installPublic); err != nil {
			return fmt.Errorf("git initialization failed: %w", err)
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
	fmt.Printf("  %d. Enter the Mayor's office: %s\n", step, style.Dim.Render("gt mayor attach"))

	return nil
}

func createMayorCLAUDEmd(hqRoot, townRoot string) error {
	tmpl, err := templates.New()
	if err != nil {
		return err
	}

	// Get town name for session names
	townName, _ := workspace.GetTownName(townRoot)

	data := templates.RoleData{
		Role:          "mayor",
		TownRoot:      townRoot,
		TownName:      townName,
		WorkDir:       hqRoot,
		MayorSession:  session.MayorSessionName(),
		DeaconSession: session.DeaconSessionName(),
	}

	content, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		return err
	}

	claudePath := filepath.Join(hqRoot, "CLAUDE.md")
	return os.WriteFile(claudePath, []byte(content), 0644)
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

	// Ensure database has repository fingerprint (GH #25).
	// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
	// Without fingerprint, the bd daemon fails to start silently.
	if err := ensureRepoFingerprint(townPath); err != nil {
		// Non-fatal: fingerprint is optional for functionality, just daemon optimization
		fmt.Printf("   %s Could not verify repo fingerprint: %v\n", style.Dim.Render("⚠"), err)
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

// initTownAgentBeads creates town-level agent and role beads using hq- prefix.
// This creates:
// - hq-mayor, hq-deacon (agent beads for town-level agents)
// - hq-mayor-role, hq-deacon-role, hq-witness-role, hq-refinery-role,
//   hq-polecat-role, hq-crew-role (role definition beads)
//
// These beads are stored in town beads (~/gt/.beads/) and are shared across all rigs.
// Rig-level agent beads (witness, refinery) are created by gt rig add in rig beads.
func initTownAgentBeads(townPath string) error {
	bd := beads.New(townPath)

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

	for _, agent := range agentDefs {
		// Check if already exists
		if _, err := bd.Show(agent.id); err == nil {
			continue // Already exists
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

	return nil
}
