package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
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
  gt sling gp-abc greenplace --create               # Create polecat if missing
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

Compare:
  gt hook <bead>      # Just attach (no action)
  gt sling <bead>     # Attach + start now (keep context)
  gt handoff <bead>   # Attach + restart (fresh context)

The propulsion principle: if it's on your hook, YOU RUN IT.

Batch Slinging:
  gt sling gt-abc gt-def gt-ghi gastown   # Sling multiple beads to a rig

  When multiple beads are provided with a rig target, each bead gets its own
  polecat. This parallelizes work dispatch without running gt sling N times.

JSON Output (--json):
  For automation, use --json to get structured output:
  gt sling gt-abc gastown --json

  Output includes: action, bead_id, target, convoy_id, nudge_sent, and more.
  Actions: "slung" (normal), "spawned" (new polecat), "dry_run" (--dry-run).`,
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
	slingJSON     bool     // --json flag: output structured JSON

	// Flags migrated for polecat spawning (used by sling for work assignment)
	slingCreate   bool   // --create: create polecat if it doesn't exist
	slingForce    bool   // --force: force spawn even if polecat has unread mail
	slingAccount  string // --account: Claude Code account handle to use
	slingAgent    string // --agent: override runtime agent for this sling/spawn
	slingNoConvoy bool   // --no-convoy: skip auto-convoy creation
)

// Sling action constants for JSON output.
const (
	slingActionSlung   = "slung"   // Successfully slung work to target
	slingActionDryRun  = "dry_run" // Dry run - no actual changes made
	slingActionSpawned = "spawned" // Spawned polecat and slung work
)

// slingResult represents the outcome of a sling operation for JSON output.
type slingResult struct {
	Action         string `json:"action"`                     // One of: slung, dry_run, spawned
	BeadID         string `json:"bead_id"`                    // The bead that was slung (final ID after bonding)
	OriginalBeadID string `json:"original_bead_id,omitempty"` // Original bead ID (before formula bonding)
	Target         string `json:"target"`                     // Target agent
	Formula        string `json:"formula,omitempty"`          // Formula name if formula-on-bead mode
	WispID         string `json:"wisp_id,omitempty"`          // Created wisp ID if formula mode
	ConvoyID       string `json:"convoy_id,omitempty"`        // Auto-created convoy ID
	Pane           string `json:"pane,omitempty"`             // Target pane
	SpawnedPolecat bool   `json:"spawned_polecat,omitempty"`  // Whether a polecat was spawned
	PolecatName    string `json:"polecat_name,omitempty"`     // Name of spawned polecat
	NudgeSent      bool   `json:"nudge_sent"`                 // Whether start prompt was sent
}

// outputSlingResult outputs the sling result as JSON.
func outputSlingResult(result slingResult) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(result)
}

// slingPrintf prints formatted output only if not in JSON mode.
func slingPrintf(format string, args ...interface{}) {
	if !slingJSON {
		fmt.Printf(format, args...)
	}
}

func init() {
	slingCmd.Flags().StringVarP(&slingSubject, "subject", "s", "", "Context subject for the work")
	slingCmd.Flags().StringVarP(&slingMessage, "message", "m", "", "Context message for the work")
	slingCmd.Flags().BoolVarP(&slingDryRun, "dry-run", "n", false, "Show what would be done")
	slingCmd.Flags().StringVar(&slingOnTarget, "on", "", "Apply formula to existing bead (implies wisp scaffolding)")
	slingCmd.Flags().StringArrayVar(&slingVars, "var", nil, "Formula variable (key=value), can be repeated")
	slingCmd.Flags().StringVarP(&slingArgs, "args", "a", "", "Natural language instructions for the executor (e.g., 'patch release')")

	// Flags for polecat spawning (when target is a rig)
	slingCmd.Flags().BoolVar(&slingCreate, "create", false, "Create polecat if it doesn't exist")
	slingCmd.Flags().BoolVar(&slingForce, "force", false, "Force spawn even if polecat has unread mail")
	slingCmd.Flags().StringVar(&slingAccount, "account", "", "Claude Code account handle to use")
	slingCmd.Flags().StringVar(&slingAgent, "agent", "", "Override agent/runtime for this sling (e.g., claude, gemini, codex, or custom alias)")
	slingCmd.Flags().BoolVar(&slingNoConvoy, "no-convoy", false, "Skip auto-convoy creation for single-issue sling")
	slingCmd.Flags().BoolVar(&slingJSON, "json", false, "Output structured JSON for automation")

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
			// It's a verified bead
			beadID = firstArg
		} else {
			// Not a verified bead - try as standalone formula
			if err := verifyFormulaExists(firstArg); err == nil {
				// Standalone formula mode: gt sling <formula> [target]
				return runSlingFormula(args)
			}
			// Not a formula either - check if it looks like a bead ID (routing issue workaround).
			// Accept it and let the actual bd update fail later if the bead doesn't exist.
			// This fixes: gt sling bd-ka761 beads/crew/dave failing with 'not a valid bead or formula'
			if looksLikeBeadID(firstArg) {
				beadID = firstArg
			} else {
				// Neither bead nor formula
				return fmt.Errorf("'%s' is not a valid bead or formula", firstArg)
			}
		}
	}

	// Initialize result for JSON output
	result := slingResult{
		Action: slingActionSlung,
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
					slingPrintf("Would dispatch to idle dog in kennel\n")
				} else {
					slingPrintf("Would dispatch to dog '%s'\n", dogName)
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
				slingPrintf("Dispatched to dog %s\n", dispatchInfo.DogName)
			}
		} else if rigName, isRig := IsRigName(target); isRig {
			// Check if target is a rig name (auto-spawn polecat)
			if slingDryRun {
				// Dry run - just indicate what would happen
				slingPrintf("Would spawn fresh polecat in rig '%s'\n", rigName)
				targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
				targetPane = "<new-pane>"
				result.SpawnedPolecat = true
			} else {
				// Spawn a fresh polecat in the rig
				slingPrintf("Target is rig '%s', spawning fresh polecat...\n", rigName)
				spawnOpts := SlingSpawnOptions{
					Force:    slingForce,
					Account:  slingAccount,
					Create:   slingCreate,
					HookBead: beadID, // Set atomically at spawn time
					Agent:    slingAgent,
				}
				spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
				if spawnErr != nil {
					return fmt.Errorf("spawning polecat: %w", spawnErr)
				}
				targetAgent = spawnInfo.AgentID()
				targetPane = spawnInfo.Pane
				hookWorkDir = spawnInfo.ClonePath // Run bd commands from polecat's worktree
				result.SpawnedPolecat = true
				result.PolecatName = spawnInfo.PolecatName
				result.Action = slingActionSpawned

				// Wake witness and refinery to monitor the new polecat
				wakeRigAgents(rigName)
			}
		} else {
			// Slinging to an existing agent
			var targetWorkDir string
			targetAgent, targetPane, targetWorkDir, err = resolveTargetAgent(target)
			if err != nil {
				// Check if this is a dead polecat (no active session)
				// If so, spawn a fresh polecat instead of failing
				if isPolecatTarget(target) {
					// Extract rig name from polecat target (format: rig/polecats/name)
					parts := strings.Split(target, "/")
					if len(parts) >= 3 && parts[1] == "polecats" {
						rigName := parts[0]
						slingPrintf("Target polecat has no active session, spawning fresh polecat in rig '%s'...\n", rigName)
						spawnOpts := SlingSpawnOptions{
							Force:    slingForce,
							Account:  slingAccount,
							Create:   slingCreate,
							HookBead: beadID,
							Agent:    slingAgent,
						}
						spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
						if spawnErr != nil {
							return fmt.Errorf("spawning polecat to replace dead polecat: %w", spawnErr)
						}
						targetAgent = spawnInfo.AgentID()
						targetPane = spawnInfo.Pane
						hookWorkDir = spawnInfo.ClonePath
						result.SpawnedPolecat = true
						result.PolecatName = spawnInfo.PolecatName
						result.Action = slingActionSpawned

						// Wake witness and refinery to monitor the new polecat
						wakeRigAgents(rigName)
					} else {
						return fmt.Errorf("resolving target: %w", err)
					}
				} else {
					return fmt.Errorf("resolving target: %w", err)
				}
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

	// Set result fields (BeadID may be updated after formula bonding)
	result.BeadID = beadID
	result.Target = targetAgent
	result.Formula = formulaName
	result.Pane = targetPane
	if formulaName != "" {
		result.OriginalBeadID = beadID // Track original before formula changes it
	}

	// Display what we're doing
	if formulaName != "" {
		slingPrintf("%s Slinging formula %s on %s to %s...\n", style.Bold.Render("🎯"), formulaName, beadID, targetAgent)
	} else {
		slingPrintf("%s Slinging %s to %s...\n", style.Bold.Render("🎯"), beadID, targetAgent)
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
				slingPrintf("Would create convoy 'Work: %s'\n", info.Title)
				slingPrintf("Would add tracking relation to %s\n", beadID)
			} else {
				convoyID, err := createAutoConvoy(beadID, info.Title)
				if err != nil {
					// Log warning but don't fail - convoy is optional
					slingPrintf("%s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					result.ConvoyID = convoyID
					slingPrintf("%s Created convoy 🚚 %s\n", style.Bold.Render("→"), convoyID)
					slingPrintf("  Tracking: %s\n", beadID)
				}
			}
		} else {
			result.ConvoyID = existingConvoy
			slingPrintf("%s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
		}
	}

	if slingDryRun {
		if formulaName != "" {
			slingPrintf("Would instantiate formula %s:\n", formulaName)
			slingPrintf("  1. bd cook %s\n", formulaName)
			slingPrintf("  2. bd mol wisp %s --var feature=\"%s\" --var issue=\"%s\"\n", formulaName, info.Title, beadID)
			slingPrintf("  3. bd mol bond <wisp-root> %s\n", beadID)
			slingPrintf("  4. bd update <compound-root> --status=hooked --assignee=%s\n", targetAgent)
		} else {
			slingPrintf("Would run: bd update %s --status=hooked --assignee=%s\n", beadID, targetAgent)
		}
		if slingSubject != "" {
			slingPrintf("  subject (in nudge): %s\n", slingSubject)
		}
		if slingMessage != "" {
			slingPrintf("  context: %s\n", slingMessage)
		}
		if slingArgs != "" {
			slingPrintf("  args (in nudge): %s\n", slingArgs)
		}
		slingPrintf("Would inject start prompt to pane: %s\n", targetPane)
		if slingJSON {
			result.Action = slingActionDryRun
			return outputSlingResult(result)
		}
		return nil
	}

	// Formula-on-bead mode: instantiate formula and bond to original bead
	if formulaName != "" {
		slingPrintf("  Instantiating formula %s...\n", formulaName)

		// Route bd mutations (wisp/bond) to the correct beads context for the target bead.
		// Some bd mol commands don't support prefix routing, so we must run them from the
		// rig directory that owns the bead's database.
		formulaWorkDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)

		// Step 1: Cook the formula (ensures proto exists)
		// Cook runs from rig directory to access the correct formula database
		cookCmd := exec.Command("bd", "--no-daemon", "cook", formulaName)
		cookCmd.Dir = formulaWorkDir
		cookCmd.Stderr = os.Stderr
		if err := cookCmd.Run(); err != nil {
			return fmt.Errorf("cooking formula %s: %w", formulaName, err)
		}

		// Step 2: Create wisp with feature and issue variables from bead
		// Run from rig directory so wisp is created in correct database
		featureVar := fmt.Sprintf("feature=%s", info.Title)
		issueVar := fmt.Sprintf("issue=%s", beadID)
		wispArgs := []string{"--no-daemon", "mol", "wisp", formulaName, "--var", featureVar, "--var", issueVar, "--json"}
		wispCmd := exec.Command("bd", wispArgs...)
		wispCmd.Dir = formulaWorkDir
		wispCmd.Env = append(os.Environ(), "GT_ROOT="+townRoot)
		wispCmd.Stderr = os.Stderr
		wispOut, err := wispCmd.Output()
		if err != nil {
			return fmt.Errorf("creating wisp for formula %s: %w", formulaName, err)
		}

		// Parse wisp output to get the root ID
		wispRootID, err := parseWispIDFromJSON(wispOut)
		if err != nil {
			return fmt.Errorf("parsing wisp output: %w", err)
		}
		result.WispID = wispRootID
		slingPrintf("%s Formula wisp created: %s\n", style.Bold.Render("✓"), wispRootID)

		// Step 3: Bond wisp to original bead (creates compound)
		// Use --no-daemon for mol bond (requires direct database access)
		bondArgs := []string{"--no-daemon", "mol", "bond", wispRootID, beadID, "--json"}
		bondCmd := exec.Command("bd", bondArgs...)
		bondCmd.Dir = formulaWorkDir
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
			slingPrintf("%s Could not parse bond output, using wisp root\n", style.Dim.Render("Warning:"))
		} else if bondResult.RootID != "" {
			wispRootID = bondResult.RootID
		}

		slingPrintf("%s Formula bonded to %s\n", style.Bold.Render("✓"), beadID)

		// Update beadID to hook the compound root instead of bare bead
		beadID = wispRootID
		result.BeadID = wispRootID // Update result to reflect final bead ID
	}

	// Hook the bead using bd update.
	// See: https://github.com/steveyegge/gastown/issues/148
	hookCmd := exec.Command("bd", "--no-daemon", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
	hookCmd.Dir = beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking bead: %w", err)
	}

	slingPrintf("%s Work attached to hook (status=hooked)\n", style.Bold.Render("✓"))

	// Log sling event to activity feed
	actor := detectActor()
	_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadID, targetAgent))

	// Update agent bead's hook_bead field (ZFC: agents track their current work)
	updateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)

	// Auto-attach mol-polecat-work to polecat agent beads
	// This ensures polecats have the standard work molecule attached for guidance
	if strings.Contains(targetAgent, "/polecats/") {
		if err := attachPolecatWorkMolecule(targetAgent, hookWorkDir, townRoot); err != nil {
			// Warn but don't fail - polecat will still work without molecule
			slingPrintf("%s Could not attach work molecule: %v\n", style.Dim.Render("Warning:"), err)
		}
	}

	// Store dispatcher in bead description (enables completion notification to dispatcher)
	if err := storeDispatcherInBead(beadID, actor); err != nil {
		// Warn but don't fail - polecat will still complete work
		slingPrintf("%s Could not store dispatcher in bead: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Store args in bead description (no-tmux mode: beads as data plane)
	if slingArgs != "" {
		if err := storeArgsInBead(beadID, slingArgs); err != nil {
			// Warn but don't fail - args will still be in the nudge prompt
			slingPrintf("%s Could not store args in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			slingPrintf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
	}

	// Try to inject the "start now" prompt (graceful if no tmux)
	if targetPane == "" {
		slingPrintf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
	} else {
		// Ensure agent is ready before nudging (prevents race condition where
		// message arrives before Claude has fully started - see issue #115)
		sessionName := getSessionFromPane(targetPane)
		if sessionName != "" {
			if err := ensureAgentReady(sessionName); err != nil {
				// Non-fatal: warn and continue, agent will discover work via gt prime
				slingPrintf("%s Could not verify agent ready: %v\n", style.Dim.Render("○"), err)
			}
		}

		if err := injectStartPrompt(targetPane, beadID, slingSubject, slingArgs); err != nil {
			// Graceful fallback for no-tmux mode
			slingPrintf("%s Could not nudge (no tmux?): %v\n", style.Dim.Render("○"), err)
			slingPrintf("  Agent will discover work via gt prime / bd show\n")
		} else {
			result.NudgeSent = true
			slingPrintf("%s Start prompt sent\n", style.Bold.Render("▶"))
		}
	}

	// Output JSON if requested
	if slingJSON {
		return outputSlingResult(result)
	}

	return nil
}
