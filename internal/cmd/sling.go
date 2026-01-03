package cmd

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var slingCmd = &cobra.Command{
	Use:     "sling <bead-or-formula> [target]",
	GroupID: GroupWork,
	Short:   "Assign work to an agent (THE unified work dispatch command)",
	Long: `Sling work onto an agent's hook and start working immediately.

This is THE command for assigning work in Gas Town. It handles:
  - Existing agents (mayor, crew, witness, refinery)
  - Auto-spawning polecats when target is a rig
  - Dispatching to dogs (Deacon's helper workers)
  - Formula instantiation and wisp creation
  - No-tmux mode for manual agent operation
  - Auto-convoy creation for dashboard visibility

Auto-Convoy:
  When slinging a single issue (not a formula), sling automatically creates
  a convoy to track the work unless --no-convoy is specified. This ensures
  all work appears in 'gt convoy list', even "swarm of one" assignments.

  gt sling gt-abc gastown              # Creates "Work: <issue-title>" convoy
  gt sling gt-abc gastown --no-convoy  # Skip auto-convoy creation

Target Resolution:
  gt sling gt-abc                       # Self (current agent)
  gt sling gt-abc crew                  # Crew worker in current rig
  gt sling gp-abc greenplace               # Auto-spawn polecat in rig
  gt sling gt-abc greenplace/Toast         # Specific polecat
  gt sling gt-abc mayor                 # Mayor
  gt sling gt-abc deacon/dogs           # Auto-dispatch to idle dog
  gt sling gt-abc deacon/dogs/alpha     # Specific dog

Spawning Options (when target is a rig):
  gt sling gp-abc greenplace --molecule mol-review  # Use specific workflow
  gt sling gp-abc greenplace --create               # Create polecat if missing
  gt sling gp-abc greenplace --naked                # No-tmux (manual start)
  gt sling gp-abc greenplace --force                # Ignore unread mail
  gt sling gp-abc greenplace --account work         # Use specific Claude account

Natural Language Args:
  gt sling gt-abc --args "patch release"
  gt sling code-review --args "focus on security"

The --args string is stored in the bead and shown via gt prime. Since the
executor is an LLM, it interprets these instructions naturally.

Formula Slinging:
  gt sling mol-release mayor/           # Cook + wisp + attach + nudge
  gt sling towers-of-hanoi --var disks=3

Formula-on-Bead (--on flag):
  gt sling mol-review --on gt-abc       # Apply formula to existing work
  gt sling shiny --on gt-abc crew       # Apply formula, sling to crew

Quality Levels (shorthand for polecat workflows):
  gt sling gp-abc greenplace --quality=basic   # mol-polecat-basic (trivial fixes)
  gt sling gp-abc greenplace --quality=shiny   # mol-polecat-shiny (standard)
  gt sling gp-abc greenplace --quality=chrome  # mol-polecat-chrome (max rigor)

Compare:
  gt hook <bead>      # Just attach (no action)
  gt sling <bead>     # Attach + start now (keep context)
  gt handoff <bead>   # Attach + restart (fresh context)

The propulsion principle: if it's on your hook, YOU RUN IT.

Batch Slinging:
  gt sling gt-abc gt-def gt-ghi gastown   # Sling multiple beads to a rig

  When multiple beads are provided with a rig target, each bead gets its own
  polecat. This parallelizes work dispatch without running gt sling N times.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSling,
}

var (
	slingSubject  string
	slingMessage  string
	slingDryRun   bool
	slingOnTarget string   // --on flag: target bead when slinging a formula
	slingVars     []string // --var flag: formula variables (key=value)
	slingArgs     string   // --args flag: natural language instructions for executor

	// Flags migrated for polecat spawning (used by sling for work assignment
	slingNaked    bool   // --naked: no-tmux mode (skip session creation)
	slingCreate   bool   // --create: create polecat if it doesn't exist
	slingMolecule string // --molecule: workflow to instantiate on the bead
	slingForce    bool   // --force: force spawn even if polecat has unread mail
	slingAccount  string // --account: Claude Code account handle to use
	slingQuality  string // --quality: shorthand for polecat workflow (basic|shiny|chrome)
	slingNoConvoy bool   // --no-convoy: skip auto-convoy creation
)

func init() {
	slingCmd.Flags().StringVarP(&slingSubject, "subject", "s", "", "Context subject for the work")
	slingCmd.Flags().StringVarP(&slingMessage, "message", "m", "", "Context message for the work")
	slingCmd.Flags().BoolVarP(&slingDryRun, "dry-run", "n", false, "Show what would be done")
	slingCmd.Flags().StringVar(&slingOnTarget, "on", "", "Apply formula to existing bead (implies wisp scaffolding)")
	slingCmd.Flags().StringArrayVar(&slingVars, "var", nil, "Formula variable (key=value), can be repeated")
	slingCmd.Flags().StringVarP(&slingArgs, "args", "a", "", "Natural language instructions for the executor (e.g., 'patch release')")

	// Flags for polecat spawning (when target is a rig)
	slingCmd.Flags().BoolVar(&slingNaked, "naked", false, "No-tmux mode: assign work but skip session creation (manual start)")
	slingCmd.Flags().BoolVar(&slingCreate, "create", false, "Create polecat if it doesn't exist")
	slingCmd.Flags().StringVar(&slingMolecule, "molecule", "", "Molecule workflow to instantiate on the bead")
	slingCmd.Flags().BoolVar(&slingForce, "force", false, "Force spawn even if polecat has unread mail")
	slingCmd.Flags().StringVar(&slingAccount, "account", "", "Claude Code account handle to use")
	slingCmd.Flags().StringVarP(&slingQuality, "quality", "q", "", "Polecat workflow quality level (basic|shiny|chrome)")
	slingCmd.Flags().BoolVar(&slingNoConvoy, "no-convoy", false, "Skip auto-convoy creation for single-issue sling")

	rootCmd.AddCommand(slingCmd)
}

func runSling(cmd *cobra.Command, args []string) error {
	// Polecats cannot sling - check early before writing anything
	if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
		return fmt.Errorf("polecats cannot sling (use gt done for handoff)")
	}

	// Get town root early - needed for BEADS_DIR when running bd commands
	// This ensures hq-* beads are accessible even when running from polecat worktree
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// --var is only for standalone formula mode, not formula-on-bead mode
	if slingOnTarget != "" && len(slingVars) > 0 {
		return fmt.Errorf("--var cannot be used with --on (formula-on-bead mode doesn't support variables)")
	}

	// Batch mode detection: multiple beads with rig target
	// Pattern: gt sling gt-abc gt-def gt-ghi gastown
	// When len(args) > 2 and last arg is a rig, sling each bead to its own polecat
	if len(args) > 2 {
		lastArg := args[len(args)-1]
		if rigName, isRig := IsRigName(lastArg); isRig {
			return runBatchSling(args[:len(args)-1], rigName, townBeadsDir)
		}
	}

	// --quality is shorthand for formula-on-bead with polecat workflow
	// Convert: gt sling gp-abc greenplace --quality=shiny
	// To:      gt sling mol-polecat-shiny --on gt-abc gastown
	if slingQuality != "" {
		qualityFormula, err := qualityToFormula(slingQuality)
		if err != nil {
			return err
		}
		// The first arg should be the bead, and we wrap it with the formula
		if slingOnTarget != "" {
			return fmt.Errorf("--quality cannot be used with --on (both specify formula)")
		}
		slingOnTarget = args[0]  // The bead becomes --on target
		args[0] = qualityFormula // The formula becomes first arg
	}

	// Determine mode based on flags and argument types
	var beadID string
	var formulaName string

	if slingOnTarget != "" {
		// Formula-on-bead mode: gt sling <formula> --on <bead>
		formulaName = args[0]
		beadID = slingOnTarget
		// Verify both exist
		if err := verifyBeadExists(beadID); err != nil {
			return err
		}
		if err := verifyFormulaExists(formulaName); err != nil {
			return err
		}
	} else {
		// Could be bead mode or standalone formula mode
		firstArg := args[0]

		// Try as bead first
		if err := verifyBeadExists(firstArg); err == nil {
			// It's a bead
			beadID = firstArg
		} else {
			// Not a bead - try as standalone formula
			if err := verifyFormulaExists(firstArg); err == nil {
				// Standalone formula mode: gt sling <formula> [target]
				return runSlingFormula(args)
			}
			// Neither bead nor formula
			return fmt.Errorf("'%s' is not a valid bead or formula", firstArg)
		}
	}

	// Determine target agent (self or specified)
	var targetAgent string
	var targetPane string
	var hookWorkDir string // Working directory for running bd hook commands

	if len(args) > 1 {
		target := args[1]

		// Resolve "." to current agent identity (like git's "." meaning current directory)
		if target == "." {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return fmt.Errorf("resolving self for '.' target: %w", err)
			}
		} else if dogName, isDog := IsDogTarget(target); isDog {
			if slingDryRun {
				if dogName == "" {
					fmt.Printf("Would dispatch to idle dog in kennel\n")
				} else {
					fmt.Printf("Would dispatch to dog '%s'\n", dogName)
				}
				targetAgent = fmt.Sprintf("deacon/dogs/%s", dogName)
				if dogName == "" {
					targetAgent = "deacon/dogs/<idle>"
				}
				targetPane = "<dog-pane>"
			} else {
				// Dispatch to dog
				dispatchInfo, dispatchErr := DispatchToDog(dogName, slingCreate)
				if dispatchErr != nil {
					return fmt.Errorf("dispatching to dog: %w", dispatchErr)
				}
				targetAgent = dispatchInfo.AgentID
				targetPane = dispatchInfo.Pane
				fmt.Printf("Dispatched to dog %s\n", dispatchInfo.DogName)
			}
		} else if rigName, isRig := IsRigName(target); isRig {
			// Check if target is a rig name (auto-spawn polecat)
			if slingDryRun {
				// Dry run - just indicate what would happen
				fmt.Printf("Would spawn fresh polecat in rig '%s'\n", rigName)
				if slingNaked {
					fmt.Printf("  --naked: would skip tmux session\n")
				}
				targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
				targetPane = "<new-pane>"
			} else {
				// Spawn a fresh polecat in the rig
				fmt.Printf("Target is rig '%s', spawning fresh polecat...\n", rigName)
				spawnOpts := SlingSpawnOptions{
					Force:    slingForce,
					Naked:    slingNaked,
					Account:  slingAccount,
					Create:   slingCreate,
					HookBead: beadID, // Set atomically at spawn time
				}
				spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
				if spawnErr != nil {
					return fmt.Errorf("spawning polecat: %w", spawnErr)
				}
				targetAgent = spawnInfo.AgentID()
				targetPane = spawnInfo.Pane
				hookWorkDir = spawnInfo.ClonePath // Run bd commands from polecat's worktree

				// Wake witness and refinery to monitor the new polecat
				wakeRigAgents(rigName)
			}
		} else {
			// Slinging to an existing agent
			// Skip pane lookup if --naked (agent may be terminated)
			var targetWorkDir string
			targetAgent, targetPane, targetWorkDir, err = resolveTargetAgent(target, slingNaked)
			if err != nil {
				return fmt.Errorf("resolving target: %w", err)
			}
			// Use target's working directory for bd commands (needed for redirect-based routing)
			if targetWorkDir != "" {
				hookWorkDir = targetWorkDir
			}
		}
	} else {
		// Slinging to self
		var selfWorkDir string
		targetAgent, targetPane, selfWorkDir, err = resolveSelfTarget()
		if err != nil {
			return err
		}
		// Use self's working directory for bd commands
		if selfWorkDir != "" {
			hookWorkDir = selfWorkDir
		}
	}

	// Display what we're doing
	if formulaName != "" {
		fmt.Printf("%s Slinging formula %s on %s to %s...\n", style.Bold.Render("🎯"), formulaName, beadID, targetAgent)
	} else {
		fmt.Printf("%s Slinging %s to %s...\n", style.Bold.Render("🎯"), beadID, targetAgent)
	}

	// Check if bead is already pinned (guard against accidental re-sling)
	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("checking bead status: %w", err)
	}
	if info.Status == "pinned" && !slingForce {
		assignee := info.Assignee
		if assignee == "" {
			assignee = "(unknown)"
		}
		return fmt.Errorf("bead %s is already pinned to %s\nUse --force to re-sling", beadID, assignee)
	}

	// Auto-convoy: check if issue is already tracked by a convoy
	// If not, create one for dashboard visibility (unless --no-convoy is set)
	if !slingNoConvoy && formulaName == "" {
		existingConvoy := isTrackedByConvoy(beadID)
		if existingConvoy == "" {
			if slingDryRun {
				fmt.Printf("Would create convoy 'Work: %s'\n", info.Title)
				fmt.Printf("Would add tracking relation to %s\n", beadID)
			} else {
				convoyID, err := createAutoConvoy(beadID, info.Title)
				if err != nil {
					// Log warning but don't fail - convoy is optional
					fmt.Printf("%s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					fmt.Printf("%s Created convoy 🚚 %s\n", style.Bold.Render("→"), convoyID)
					fmt.Printf("  Tracking: %s\n", beadID)
				}
			}
		} else {
			fmt.Printf("%s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
		}
	}

	if slingDryRun {
		if formulaName != "" {
			fmt.Printf("Would instantiate formula %s:\n", formulaName)
			fmt.Printf("  1. bd cook %s\n", formulaName)
			fmt.Printf("  2. bd mol wisp %s --var feature=\"%s\"\n", formulaName, info.Title)
			fmt.Printf("  3. bd mol bond <wisp-root> %s\n", beadID)
			fmt.Printf("  4. bd update <compound-root> --status=hooked --assignee=%s\n", targetAgent)
		} else {
			fmt.Printf("Would run: bd update %s --status=hooked --assignee=%s\n", beadID, targetAgent)
		}
		if slingSubject != "" {
			fmt.Printf("  subject (in nudge): %s\n", slingSubject)
		}
		if slingMessage != "" {
			fmt.Printf("  context: %s\n", slingMessage)
		}
		if slingArgs != "" {
			fmt.Printf("  args (in nudge): %s\n", slingArgs)
		}
		fmt.Printf("Would inject start prompt to pane: %s\n", targetPane)
		return nil
	}

	// Formula-on-bead mode: instantiate formula and bond to original bead
	if formulaName != "" {
		fmt.Printf("  Instantiating formula %s...\n", formulaName)

		// Step 1: Cook the formula (ensures proto exists)
		cookCmd := exec.Command("bd", "cook", formulaName)
		cookCmd.Stderr = os.Stderr
		if err := cookCmd.Run(); err != nil {
			return fmt.Errorf("cooking formula %s: %w", formulaName, err)
		}

		// Step 2: Create wisp with feature variable from bead title
		featureVar := fmt.Sprintf("feature=%s", info.Title)
		wispArgs := []string{"mol", "wisp", formulaName, "--var", featureVar, "--json"}
		wispCmd := exec.Command("bd", wispArgs...)
		wispCmd.Stderr = os.Stderr
		wispOut, err := wispCmd.Output()
		if err != nil {
			return fmt.Errorf("creating wisp for formula %s: %w", formulaName, err)
		}

		// Parse wisp output to get the root ID
		var wispResult struct {
			RootID string `json:"root_id"`
		}
		if err := json.Unmarshal(wispOut, &wispResult); err != nil {
			return fmt.Errorf("parsing wisp output: %w", err)
		}
		wispRootID := wispResult.RootID
		fmt.Printf("%s Formula wisp created: %s\n", style.Bold.Render("✓"), wispRootID)

		// Step 3: Bond wisp to original bead (creates compound)
		bondArgs := []string{"mol", "bond", wispRootID, beadID, "--json"}
		bondCmd := exec.Command("bd", bondArgs...)
		bondCmd.Stderr = os.Stderr
		bondOut, err := bondCmd.Output()
		if err != nil {
			return fmt.Errorf("bonding formula to bead: %w", err)
		}

		// Parse bond output - the wisp root becomes the compound root
		// After bonding, we hook the wisp root (which now contains the original bead)
		var bondResult struct {
			RootID string `json:"root_id"`
		}
		if err := json.Unmarshal(bondOut, &bondResult); err != nil {
			// Fallback: use wisp root as the compound root
			fmt.Printf("%s Could not parse bond output, using wisp root\n", style.Dim.Render("Warning:"))
		} else if bondResult.RootID != "" {
			wispRootID = bondResult.RootID
		}

		fmt.Printf("%s Formula bonded to %s\n", style.Bold.Render("✓"), beadID)

		// Update beadID to hook the compound root instead of bare bead
		beadID = wispRootID
	}

	// Hook the bead using bd update
	// Set BEADS_DIR to town-level beads so hq-* beads are accessible
	// even when running from polecat worktree (which only sees gt-* via redirect)
	hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
	hookCmd.Env = append(os.Environ(), "BEADS_DIR="+townBeadsDir)
	if hookWorkDir != "" {
		hookCmd.Dir = hookWorkDir
	} else {
		hookCmd.Dir = townRoot
	}
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking bead: %w", err)
	}

	fmt.Printf("%s Work attached to hook (status=hooked)\n", style.Bold.Render("✓"))

	// Log sling event to activity feed
	actor := detectActor()
	_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadID, targetAgent))

	// Update agent bead's hook_bead field (ZFC: agents track their current work)
	updateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)

	// Store args in bead description (no-tmux mode: beads as data plane)
	if slingArgs != "" {
		if err := storeArgsInBead(beadID, slingArgs); err != nil {
			// Warn but don't fail - args will still be in the nudge prompt
			fmt.Printf("%s Could not store args in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
	}

	// Try to inject the "start now" prompt (graceful if no tmux)
	if targetPane == "" {
		fmt.Printf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
	} else if err := injectStartPrompt(targetPane, beadID, slingSubject, slingArgs); err != nil {
		// Graceful fallback for no-tmux mode
		fmt.Printf("%s Could not nudge (no tmux?): %v\n", style.Dim.Render("○"), err)
		fmt.Printf("  Agent will discover work via gt prime / bd show\n")
	} else {
		fmt.Printf("%s Start prompt sent\n", style.Bold.Render("▶"))
	}

	return nil
}

// storeArgsInBead stores args in the bead's description using attached_args field.
// This enables no-tmux mode where agents discover args via gt prime / bd show.
func storeArgsInBead(beadID, args string) error {
	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	// Parse the bead
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("parsing bead: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("bead not found")
	}
	issue := &issues[0]

	// Get or create attachment fields
	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	// Set the args
	fields.AttachedArgs = args

	// Update the description
	newDesc := beads.SetAttachmentFields(issue, fields)

	// Update the bead
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// injectStartPrompt sends a prompt to the target pane to start working.
// Uses the reliable nudge pattern: literal mode + 500ms debounce + separate Enter.
func injectStartPrompt(pane, beadID, subject, args string) error {
	if pane == "" {
		return fmt.Errorf("no target pane")
	}

	// Build the prompt to inject
	var prompt string
	if args != "" {
		// Args provided - include them prominently in the prompt
		if subject != "" {
			prompt = fmt.Sprintf("Work slung: %s (%s). Args: %s. Start working now - use these args to guide your execution.", beadID, subject, args)
		} else {
			prompt = fmt.Sprintf("Work slung: %s. Args: %s. Start working now - use these args to guide your execution.", beadID, args)
		}
	} else if subject != "" {
		prompt = fmt.Sprintf("Work slung: %s (%s). Start working on it now - no questions, just begin.", beadID, subject)
	} else {
		prompt = fmt.Sprintf("Work slung: %s. Start working on it now - run `gt hook` to see the hook, then begin.", beadID)
	}

	// Use the reliable nudge pattern (same as gt nudge / tmux.NudgeSession)
	t := tmux.NewTmux()
	return t.NudgePane(pane, prompt)
}

// resolveTargetAgent converts a target spec to agent ID, pane, and hook root.
// If skipPane is true, skip tmux pane lookup (for --naked mode).
func resolveTargetAgent(target string, skipPane bool) (agentID string, pane string, hookRoot string, err error) {
	// First resolve to session name
	sessionName, err := resolveRoleToSession(target)
	if err != nil {
		return "", "", "", err
	}

	// Convert session name to agent ID format (this doesn't require tmux)
	agentID = sessionToAgentID(sessionName)

	// Skip pane lookup if requested (--naked mode)
	if skipPane {
		return agentID, "", "", nil
	}

	// Get the pane for that session
	pane, err = getSessionPane(sessionName)
	if err != nil {
		return "", "", "", fmt.Errorf("getting pane for %s: %w", sessionName, err)
	}

	// Get the target's working directory for hook storage
	t := tmux.NewTmux()
	hookRoot, err = t.GetPaneWorkDir(sessionName)
	if err != nil {
		return "", "", "", fmt.Errorf("getting working dir for %s: %w", sessionName, err)
	}

	return agentID, pane, hookRoot, nil
}

// sessionToAgentID converts a session name to agent ID format.
// Uses session.ParseSessionName for consistent parsing across the codebase.
func sessionToAgentID(sessionName string) string {
	identity, err := session.ParseSessionName(sessionName)
	if err != nil {
		// Fallback for unparseable sessions
		return sessionName
	}
	return identity.Address()
}

// verifyBeadExists checks that the bead exists using bd show.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "show", beadID, "--json")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bead '%s' not found (bd show failed)", beadID)
	}
	return nil
}

// beadInfo holds status and assignee for a bead.
type beadInfo struct {
	Title    string `json:"title"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
}

// getBeadInfo returns status and assignee for a bead.
func getBeadInfo(beadID string) (*beadInfo, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	// bd show --json returns an array (issue + dependents), take first element
	var infos []beadInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return nil, fmt.Errorf("parsing bead info: %w", err)
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	return &infos[0], nil
}

// detectCloneRoot finds the root of the current git clone.
func detectCloneRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveSelfTarget determines agent identity, pane, and hook root for slinging to self.
func resolveSelfTarget() (agentID string, pane string, hookRoot string, err error) {
	roleInfo, err := GetRole()
	if err != nil {
		return "", "", "", fmt.Errorf("detecting role: %w", err)
	}

	// Build agent identity from role
	// Town-level agents use trailing slash to match addressToIdentity() normalization
	switch roleInfo.Role {
	case RoleMayor:
		agentID = "mayor/"
	case RoleDeacon:
		agentID = "deacon/"
	case RoleWitness:
		agentID = fmt.Sprintf("%s/witness", roleInfo.Rig)
	case RoleRefinery:
		agentID = fmt.Sprintf("%s/refinery", roleInfo.Rig)
	case RolePolecat:
		agentID = fmt.Sprintf("%s/polecats/%s", roleInfo.Rig, roleInfo.Polecat)
	case RoleCrew:
		agentID = fmt.Sprintf("%s/crew/%s", roleInfo.Rig, roleInfo.Polecat)
	default:
		return "", "", "", fmt.Errorf("cannot determine agent identity (role: %s)", roleInfo.Role)
	}

	pane = os.Getenv("TMUX_PANE")
	hookRoot = roleInfo.Home
	if hookRoot == "" {
		// Fallback to git root if home not determined
		hookRoot, err = detectCloneRoot()
		if err != nil {
			return "", "", "", fmt.Errorf("detecting clone root: %w", err)
		}
	}

	return agentID, pane, hookRoot, nil
}

// verifyFormulaExists checks that the formula exists using bd formula show.
// Formulas are TOML files (.formula.toml).
func verifyFormulaExists(formulaName string) error {
	// Try bd formula show (handles all formula file formats)
	cmd := exec.Command("bd", "formula", "show", formulaName)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Try with mol- prefix
	cmd = exec.Command("bd", "formula", "show", "mol-"+formulaName)
	if err := cmd.Run(); err == nil {
		return nil
	}

	return fmt.Errorf("formula '%s' not found (check 'bd formula list')", formulaName)
}

// runSlingFormula handles standalone formula slinging.
// Flow: cook → wisp → attach to hook → nudge
func runSlingFormula(args []string) error {
	formulaName := args[0]

	// Get town root early - needed for BEADS_DIR when running bd commands
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Determine target (self or specified)
	var target string
	if len(args) > 1 {
		target = args[1]
	}

	// Resolve target agent and pane
	var targetAgent string
	var targetPane string

	if target != "" {
		// Resolve "." to current agent identity (like git's "." meaning current directory)
		if target == "." {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return fmt.Errorf("resolving self for '.' target: %w", err)
			}
		} else if dogName, isDog := IsDogTarget(target); isDog {
			if slingDryRun {
				if dogName == "" {
					fmt.Printf("Would dispatch to idle dog in kennel\n")
				} else {
					fmt.Printf("Would dispatch to dog '%s'\n", dogName)
				}
				targetAgent = fmt.Sprintf("deacon/dogs/%s", dogName)
				if dogName == "" {
					targetAgent = "deacon/dogs/<idle>"
				}
				targetPane = "<dog-pane>"
			} else {
				// Dispatch to dog
				dispatchInfo, dispatchErr := DispatchToDog(dogName, slingCreate)
				if dispatchErr != nil {
					return fmt.Errorf("dispatching to dog: %w", dispatchErr)
				}
				targetAgent = dispatchInfo.AgentID
				targetPane = dispatchInfo.Pane
				fmt.Printf("Dispatched to dog %s\n", dispatchInfo.DogName)
			}
		} else if rigName, isRig := IsRigName(target); isRig {
			// Check if target is a rig name (auto-spawn polecat)
			if slingDryRun {
				// Dry run - just indicate what would happen
				fmt.Printf("Would spawn fresh polecat in rig '%s'\n", rigName)
				if slingNaked {
					fmt.Printf("  --naked: would skip tmux session\n")
				}
				targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
				targetPane = "<new-pane>"
			} else {
				// Spawn a fresh polecat in the rig
				fmt.Printf("Target is rig '%s', spawning fresh polecat...\n", rigName)
				spawnOpts := SlingSpawnOptions{
					Force:   slingForce,
					Naked:   slingNaked,
					Account: slingAccount,
					Create:  slingCreate,
				}
				spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
				if spawnErr != nil {
					return fmt.Errorf("spawning polecat: %w", spawnErr)
				}
				targetAgent = spawnInfo.AgentID()
				targetPane = spawnInfo.Pane

				// Wake witness and refinery to monitor the new polecat
				wakeRigAgents(rigName)
			}
		} else {
			// Slinging to an existing agent
			// Skip pane lookup if --naked (agent may be terminated)
			var targetWorkDir string
			targetAgent, targetPane, targetWorkDir, err = resolveTargetAgent(target, slingNaked)
			if err != nil {
				return fmt.Errorf("resolving target: %w", err)
			}
			// Use target's working directory for bd commands (needed for redirect-based routing)
			_ = targetWorkDir // Formula sling doesn't need hookWorkDir
		}
	} else {
		// Slinging to self
		var selfWorkDir string
		targetAgent, targetPane, selfWorkDir, err = resolveSelfTarget()
		if err != nil {
			return err
		}
		_ = selfWorkDir // Formula sling doesn't need hookWorkDir
	}

	fmt.Printf("%s Slinging formula %s to %s...\n", style.Bold.Render("🎯"), formulaName, targetAgent)

	if slingDryRun {
		fmt.Printf("Would cook formula: %s\n", formulaName)
		fmt.Printf("Would create wisp and pin to: %s\n", targetAgent)
		for _, v := range slingVars {
			fmt.Printf("  --var %s\n", v)
		}
		fmt.Printf("Would nudge pane: %s\n", targetPane)
		return nil
	}

	// Step 1: Cook the formula (ensures proto exists)
	fmt.Printf("  Cooking formula...\n")
	cookArgs := []string{"cook", formulaName}
	cookCmd := exec.Command("bd", cookArgs...)
	cookCmd.Stderr = os.Stderr
	if err := cookCmd.Run(); err != nil {
		return fmt.Errorf("cooking formula: %w", err)
	}

	// Step 2: Create wisp instance (ephemeral)
	fmt.Printf("  Creating wisp...\n")
	wispArgs := []string{"mol", "wisp", formulaName}
	for _, v := range slingVars {
		wispArgs = append(wispArgs, "--var", v)
	}
	wispArgs = append(wispArgs, "--json")

	wispCmd := exec.Command("bd", wispArgs...)
	wispCmd.Stderr = os.Stderr // Show wisp errors to user
	wispOut, err := wispCmd.Output()
	if err != nil {
		return fmt.Errorf("creating wisp: %w", err)
	}

	// Parse wisp output to get the root ID
	var wispResult struct {
		RootID string `json:"root_id"`
	}
	if err := json.Unmarshal(wispOut, &wispResult); err != nil {
		// Fallback: use formula name as identifier, but warn user
		fmt.Printf("%s Could not parse wisp output, using formula name as ID\n", style.Dim.Render("Warning:"))
		wispResult.RootID = formulaName
	}

	fmt.Printf("%s Wisp created: %s\n", style.Bold.Render("✓"), wispResult.RootID)

	// Step 3: Hook the wisp bead using bd update (discovery-based approach)
	// Set BEADS_DIR to town-level beads so hq-* beads are accessible
	hookCmd := exec.Command("bd", "update", wispResult.RootID, "--status=hooked", "--assignee="+targetAgent)
	hookCmd.Env = append(os.Environ(), "BEADS_DIR="+townBeadsDir)
	hookCmd.Dir = townRoot
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking wisp bead: %w", err)
	}
	fmt.Printf("%s Attached to hook (status=hooked)\n", style.Bold.Render("✓"))

	// Log sling event to activity feed (formula slinging)
	actor := detectActor()
	payload := events.SlingPayload(wispResult.RootID, targetAgent)
	payload["formula"] = formulaName
	_ = events.LogFeed(events.TypeSling, actor, payload)

	// Update agent bead's hook_bead field (ZFC: agents track their current work)
	// Note: formula slinging uses town root as workDir (no polecat-specific path)
	updateAgentHookBead(targetAgent, wispResult.RootID, "", townBeadsDir)

	// Store args in wisp bead if provided (no-tmux mode: beads as data plane)
	if slingArgs != "" {
		if err := storeArgsInBead(wispResult.RootID, slingArgs); err != nil {
			fmt.Printf("%s Could not store args in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
	}

	// Step 4: Nudge to start (graceful if no tmux)
	if targetPane == "" {
		fmt.Printf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
		return nil
	}

	var prompt string
	if slingArgs != "" {
		prompt = fmt.Sprintf("Formula %s slung. Args: %s. Run `gt hook` to see your hook, then execute using these args.", formulaName, slingArgs)
	} else {
		prompt = fmt.Sprintf("Formula %s slung. Run `gt hook` to see your hook, then execute the steps.", formulaName)
	}
	t := tmux.NewTmux()
	if err := t.NudgePane(targetPane, prompt); err != nil {
		// Graceful fallback for no-tmux mode
		fmt.Printf("%s Could not nudge (no tmux?): %v\n", style.Dim.Render("○"), err)
		fmt.Printf("  Agent will discover work via gt prime / bd show\n")
	} else {
		fmt.Printf("%s Nudged to start\n", style.Bold.Render("▶"))
	}

	return nil
}

// updateAgentHookBead updates the agent bead's state when work is slung.
// This enables the witness to see that each agent is working.
//
// We run from the polecat's workDir (which redirects to the rig's beads database)
// WITHOUT setting BEADS_DIR, so the redirect mechanism works for gt-* agent beads.
//
// Note: We only update the agent_state field, not hook_bead. The hook_bead field
// requires cross-database access (agent in rig db, hook bead in town db), but
// bd slot set has a bug where it doesn't support this. See BD_BUG_AGENT_STATE_ROUTING.md.
// The work is still correctly attached via `bd update <bead> --assignee=<agent>`.
func updateAgentHookBead(agentID, _, workDir, townBeadsDir string) { // beadID unused due to BD_BUG_AGENT_STATE_ROUTING
	_ = townBeadsDir // Not used - BEADS_DIR breaks redirect mechanism

	// Convert agent ID to agent bead ID
	// Format examples (canonical: prefix-rig-role-name):
	//   greenplace/crew/max -> gt-greenplace-crew-max
	//   greenplace/polecats/Toast -> gt-greenplace-polecat-Toast
	//   mayor -> gt-mayor
	//   greenplace/witness -> gt-greenplace-witness
	agentBeadID := agentIDToBeadID(agentID)
	if agentBeadID == "" {
		return
	}

	// Determine the directory to run bd commands from:
	// - If workDir is provided (polecat's clone path), use it for redirect-based routing
	// - Otherwise fall back to town root
	bdWorkDir := workDir
	if bdWorkDir == "" {
		townRoot, err := workspace.FindFromCwd()
		if err != nil {
			// Not in a Gas Town workspace - can't update agent bead
			fmt.Fprintf(os.Stderr, "Warning: couldn't find town root to update agent hook: %v\n", err)
			return
		}
		bdWorkDir = townRoot
	}

	// Run from workDir WITHOUT BEADS_DIR to enable redirect-based routing.
	// Only update agent_state (not hook_bead) due to bd cross-database bug.
	bd := beads.New(bdWorkDir)
	if err := bd.UpdateAgentState(agentBeadID, "running", nil); err != nil {
		// Log warning instead of silent ignore - helps debug cross-beads issues
		fmt.Fprintf(os.Stderr, "Warning: couldn't update agent %s state: %v\n", agentBeadID, err)
		return
	}
}

// wakeRigAgents wakes the witness and refinery for a rig after polecat dispatch.
// This ensures the patrol agents are ready to monitor and merge.
func wakeRigAgents(rigName string) {
	// Boot the rig (idempotent - no-op if already running)
	bootCmd := exec.Command("gt", "rig", "boot", rigName)
	_ = bootCmd.Run() // Ignore errors - rig might already be running

	// Nudge witness and refinery to clear any backoff
	t := tmux.NewTmux()
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
	refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)

	// Silent nudges - sessions might not exist yet
	_ = t.NudgeSession(witnessSession, "Polecat dispatched - check for work")
	_ = t.NudgeSession(refinerySession, "Polecat dispatched - check for merge requests")
}

// detectActor returns the current agent's actor string for event logging.
func detectActor() string {
	roleInfo, err := GetRole()
	if err != nil {
		return "unknown"
	}
	return roleInfo.ActorString()
}

// agentIDToBeadID converts an agent ID to its corresponding agent bead ID.
// Uses canonical naming: prefix-rig-role-name
// This function uses "gt-" prefix by default. For non-gastown rigs, use the
// appropriate *WithPrefix functions that accept the rig's configured prefix.
func agentIDToBeadID(agentID string) string {
	// Handle simple cases (town-level agents)
	if agentID == "mayor" {
		return beads.MayorBeadID()
	}
	if agentID == "deacon" {
		return beads.DeaconBeadID()
	}

	// Parse path-style agent IDs
	parts := strings.Split(agentID, "/")
	if len(parts) < 2 {
		return ""
	}

	rig := parts[0]

	switch {
	case len(parts) == 2 && parts[1] == "witness":
		return beads.WitnessBeadID(rig)
	case len(parts) == 2 && parts[1] == "refinery":
		return beads.RefineryBeadID(rig)
	case len(parts) == 3 && parts[1] == "crew":
		return beads.CrewBeadID(rig, parts[2])
	case len(parts) == 3 && parts[1] == "polecats":
		return beads.PolecatBeadID(rig, parts[2])
	default:
		return ""
	}
}

// qualityToFormula converts a quality level to the corresponding polecat workflow formula.
func qualityToFormula(quality string) (string, error) {
	switch strings.ToLower(quality) {
	case "basic", "b":
		return "mol-polecat-basic", nil
	case "shiny", "s":
		return "mol-polecat-shiny", nil
	case "chrome", "c":
		return "mol-polecat-chrome", nil
	default:
		return "", fmt.Errorf("invalid quality level '%s' (use: basic, shiny, or chrome)", quality)
	}
}

// IsDogTarget checks if target is a dog target pattern.
// Returns the dog name (or empty for pool dispatch) and true if it's a dog target.
// Patterns:
//   - "deacon/dogs" -> ("", true) - dispatch to any idle dog
//   - "deacon/dogs/alpha" -> ("alpha", true) - dispatch to specific dog
func IsDogTarget(target string) (dogName string, isDog bool) {
	target = strings.ToLower(target)

	// Check for exact "deacon/dogs" (pool dispatch)
	if target == "deacon/dogs" {
		return "", true
	}

	// Check for "deacon/dogs/<name>" (specific dog)
	if strings.HasPrefix(target, "deacon/dogs/") {
		name := strings.TrimPrefix(target, "deacon/dogs/")
		if name != "" && !strings.Contains(name, "/") {
			return name, true
		}
	}

	return "", false
}

// DogDispatchInfo contains information about a dog dispatch.
type DogDispatchInfo struct {
	DogName string // Name of the dog
	AgentID string // Agent ID format (deacon/dogs/<name>)
	Pane    string // Tmux pane (empty if no session)
	Spawned bool   // True if dog was spawned (new)
}

// DispatchToDog finds or spawns a dog for work dispatch.
// If dogName is empty, finds an idle dog from the pool.
// If create is true and no dogs exist, creates one.
func DispatchToDog(dogName string, create bool) (*DogDispatchInfo, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("finding town root: %w", err)
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading rigs config: %w", err)
	}

	mgr := dog.NewManager(townRoot, rigsConfig)

	var targetDog *dog.Dog
	var spawned bool

	if dogName != "" {
		// Specific dog requested
		targetDog, err = mgr.Get(dogName)
		if err != nil {
			if create {
				// Create the dog if it doesn't exist
				targetDog, err = mgr.Add(dogName)
				if err != nil {
					return nil, fmt.Errorf("creating dog %s: %w", dogName, err)
				}
				fmt.Printf("✓ Created dog %s\n", dogName)
				spawned = true
			} else {
				return nil, fmt.Errorf("dog %s not found (use --create to add)", dogName)
			}
		}
	} else {
		// Pool dispatch - find an idle dog
		targetDog, err = mgr.GetIdleDog()
		if err != nil {
			return nil, fmt.Errorf("finding idle dog: %w", err)
		}

		if targetDog == nil {
			if create {
				// No idle dogs - create one
				newName := generateDogName(mgr)
				targetDog, err = mgr.Add(newName)
				if err != nil {
					return nil, fmt.Errorf("creating dog %s: %w", newName, err)
				}
				fmt.Printf("✓ Created dog %s (pool was empty)\n", newName)
				spawned = true
			} else {
				return nil, fmt.Errorf("no idle dogs available (use --create to add)")
			}
		}
	}

	// Mark dog as working
	if err := mgr.SetState(targetDog.Name, dog.StateWorking); err != nil {
		return nil, fmt.Errorf("setting dog state: %w", err)
	}

	// Build agent ID
	agentID := fmt.Sprintf("deacon/dogs/%s", targetDog.Name)

	// Try to find tmux session for the dog (dogs may run in tmux like polecats)
	// Dogs use the pattern gt-{town}-deacon-{name}
	townName, _ := workspace.GetTownName(townRoot)
	sessionName := fmt.Sprintf("gt-%s-deacon-%s", townName, targetDog.Name)
	t := tmux.NewTmux()
	var pane string
	if has, _ := t.HasSession(sessionName); has {
		// Get the pane from the session
		pane, _ = getSessionPane(sessionName)
	}

	return &DogDispatchInfo{
		DogName: targetDog.Name,
		AgentID: agentID,
		Pane:    pane,
		Spawned: spawned,
	}, nil
}

// generateDogName creates a unique dog name for pool expansion.
func generateDogName(mgr *dog.Manager) string {
	// Use Greek alphabet for dog names
	names := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}

	dogs, _ := mgr.List()
	existing := make(map[string]bool)
	for _, d := range dogs {
		existing[d.Name] = true
	}

	for _, name := range names {
		if !existing[name] {
			return name
		}
	}

	// Fallback: numbered dogs
	for i := 1; i <= 100; i++ {
		name := fmt.Sprintf("dog%d", i)
		if !existing[name] {
			return name
		}
	}

	return fmt.Sprintf("dog%d", len(dogs)+1)
}

// slingGenerateShortID generates a short random ID (5 lowercase chars).
func slingGenerateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

// isTrackedByConvoy checks if an issue is already being tracked by a convoy.
// Returns the convoy ID if tracked, empty string otherwise.
func isTrackedByConvoy(beadID string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}

	// Query town beads for any convoy that tracks this issue
	// Convoys use "tracks" dependency type: convoy -> tracked issue
	townBeads := filepath.Join(townRoot, ".beads")
	dbPath := filepath.Join(townBeads, "beads.db")

	// Query dependencies where this bead is being tracked
	// Also check for external reference format: external:rig:issue-id
	query := fmt.Sprintf(`
		SELECT d.issue_id
		FROM dependencies d
		JOIN issues i ON d.issue_id = i.id
		WHERE d.type = 'tracks'
		AND i.issue_type = 'convoy'
		AND (d.depends_on_id = '%s' OR d.depends_on_id LIKE '%%:%s')
		LIMIT 1
	`, beadID, beadID)

	queryCmd := exec.Command("sqlite3", dbPath, query)
	out, err := queryCmd.Output()
	if err != nil {
		return ""
	}

	convoyID := strings.TrimSpace(string(out))
	return convoyID
}

// createAutoConvoy creates an auto-convoy for a single issue and tracks it.
// Returns the created convoy ID.
func createAutoConvoy(beadID, beadTitle string) (string, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", fmt.Errorf("finding town root: %w", err)
	}

	townBeads := filepath.Join(townRoot, ".beads")

	// Generate convoy ID with cv- prefix
	convoyID := fmt.Sprintf("hq-cv-%s", slingGenerateShortID())

	// Create convoy with title "Work: <issue-title>"
	convoyTitle := fmt.Sprintf("Work: %s", beadTitle)
	description := fmt.Sprintf("Auto-created convoy tracking %s", beadID)

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}

	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeads
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating convoy: %w", err)
	}

	// Add tracking relation: convoy tracks the issue
	depArgs := []string{"dep", "add", convoyID, beadID, "--type=tracks"}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = townBeads
	depCmd.Stderr = os.Stderr

	if err := depCmd.Run(); err != nil {
		// Convoy was created but tracking failed - log warning but continue
		fmt.Printf("%s Could not add tracking relation: %v\n", style.Dim.Render("Warning:"), err)
	}

	return convoyID, nil
}

// runBatchSling handles slinging multiple beads to a rig.
// Each bead gets its own freshly spawned polecat.
func runBatchSling(beadIDs []string, rigName string, townBeadsDir string) error {
	// Validate all beads exist before spawning any polecats
	for _, beadID := range beadIDs {
		if err := verifyBeadExists(beadID); err != nil {
			return fmt.Errorf("bead '%s' not found", beadID)
		}
	}

	if slingDryRun {
		fmt.Printf("%s Batch slinging %d beads to rig '%s':\n", style.Bold.Render("🎯"), len(beadIDs), rigName)
		for _, beadID := range beadIDs {
			fmt.Printf("  Would spawn polecat for: %s\n", beadID)
		}
		if slingNaked {
			fmt.Printf("  --naked: would skip tmux sessions\n")
		}
		return nil
	}

	fmt.Printf("%s Batch slinging %d beads to rig '%s'...\n", style.Bold.Render("🎯"), len(beadIDs), rigName)

	// Track results for summary
	type slingResult struct {
		beadID   string
		polecat  string
		success  bool
		errMsg   string
	}
	results := make([]slingResult, 0, len(beadIDs))

	// Spawn a polecat for each bead and sling it
	for i, beadID := range beadIDs {
		fmt.Printf("\n[%d/%d] Slinging %s...\n", i+1, len(beadIDs), beadID)

		// Check bead status
		info, err := getBeadInfo(beadID)
		if err != nil {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: err.Error()})
			fmt.Printf("  %s Could not get bead info: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		if info.Status == "pinned" && !slingForce {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: "already pinned"})
			fmt.Printf("  %s Already pinned (use --force to re-sling)\n", style.Dim.Render("✗"))
			continue
		}

		// Spawn a fresh polecat
		spawnOpts := SlingSpawnOptions{
			Force:    slingForce,
			Naked:    slingNaked,
			Account:  slingAccount,
			Create:   slingCreate,
			HookBead: beadID, // Set atomically at spawn time
		}
		spawnInfo, err := SpawnPolecatForSling(rigName, spawnOpts)
		if err != nil {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: err.Error()})
			fmt.Printf("  %s Failed to spawn polecat: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		targetAgent := spawnInfo.AgentID()
		hookWorkDir := spawnInfo.ClonePath

		// Auto-convoy: check if issue is already tracked
		if !slingNoConvoy {
			existingConvoy := isTrackedByConvoy(beadID)
			if existingConvoy == "" {
				convoyID, err := createAutoConvoy(beadID, info.Title)
				if err != nil {
					fmt.Printf("  %s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					fmt.Printf("  %s Created convoy 🚚 %s\n", style.Bold.Render("→"), convoyID)
				}
			} else {
				fmt.Printf("  %s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
			}
		}

		// Hook the bead
		hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
		hookCmd.Env = append(os.Environ(), "BEADS_DIR="+townBeadsDir)
		if hookWorkDir != "" {
			hookCmd.Dir = hookWorkDir
		}
		hookCmd.Stderr = os.Stderr
		if err := hookCmd.Run(); err != nil {
			results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: false, errMsg: "hook failed"})
			fmt.Printf("  %s Failed to hook bead: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		fmt.Printf("  %s Work attached to %s\n", style.Bold.Render("✓"), spawnInfo.PolecatName)

		// Log sling event
		actor := detectActor()
		_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadID, targetAgent))

		// Update agent bead state
		updateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)

		// Store args if provided
		if slingArgs != "" {
			if err := storeArgsInBead(beadID, slingArgs); err != nil {
				fmt.Printf("  %s Could not store args: %v\n", style.Dim.Render("Warning:"), err)
			}
		}

		// Nudge the polecat
		if spawnInfo.Pane != "" {
			if err := injectStartPrompt(spawnInfo.Pane, beadID, slingSubject, slingArgs); err != nil {
				fmt.Printf("  %s Could not nudge (agent will discover via gt prime)\n", style.Dim.Render("○"))
			} else {
				fmt.Printf("  %s Start prompt sent\n", style.Bold.Render("▶"))
			}
		}

		results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: true})
	}

	// Wake witness and refinery once at the end
	wakeRigAgents(rigName)

	// Print summary
	successCount := 0
	for _, r := range results {
		if r.success {
			successCount++
		}
	}

	fmt.Printf("\n%s Batch sling complete: %d/%d succeeded\n", style.Bold.Render("📊"), successCount, len(beadIDs))
	if successCount < len(beadIDs) {
		for _, r := range results {
			if !r.success {
				fmt.Printf("  %s %s: %s\n", style.Dim.Render("✗"), r.beadID, r.errMsg)
			}
		}
	}

	return nil
}
