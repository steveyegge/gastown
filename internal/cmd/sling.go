package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
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

Convoy Batching:
  Use --convoy to add multiple issues to a single convoy instead of creating
  separate convoys for each. This is recommended when slinging related work.

  # Create convoy first, then add issues to it
  gt convoy create "Release v2.0" gt-abc
  gt sling gt-def gastown --convoy hq-cv-xyz
  gt sling gt-ghi gastown --convoy hq-cv-xyz

  Use 'gt workload <agent>' to see all hooked issues for an agent.

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

Ownership and Merge Strategy:
  gt sling gt-abc gastown --owned         # Caller-managed convoy (use gt convoy land)
  gt sling gt-abc gastown --merge=direct  # Push directly to main (no MR)
  gt sling gt-abc gastown --merge=local   # Merge locally, push main
  gt sling gt-abc gastown --owned --merge=direct  # Full caller control`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSling,
}

var (
	slingSubject     string
	slingMessage     string
	slingDryRun      bool
	slingOnTarget    string   // --on flag: target bead when slinging a formula
	slingVars        []string // --var flag: formula variables (key=value)
	slingArgs        string   // --args flag: natural language instructions for executor
	slingHookRawBead bool     // --hook-raw-bead: hook raw bead without default formula (expert mode)

	// Flags migrated for polecat spawning (used by sling for work assignment)
	slingCreate        bool   // --create: create polecat if it doesn't exist
	slingForce         bool   // --force: force spawn even if polecat has unread mail
	slingAccount       string // --account: Claude Code account handle to use
	slingAgent         string // --agent: override runtime agent for this sling/spawn
	slingNoConvoy      bool   // --no-convoy: skip auto-convoy creation
	slingConvoy        string // --convoy: add to existing convoy instead of creating new one
	slingNoMerge       bool   // --no-merge: skip merge queue on completion (for upstream PRs/human review)
	slingMergeStrategy string // --merge: merge strategy (direct/mr/local)
	slingOwned         bool   // --owned: caller-owned convoy (no witness/refinery)
)

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
	slingCmd.Flags().StringVar(&slingConvoy, "convoy", "", "Add to existing convoy instead of creating new one")
	slingCmd.Flags().BoolVar(&slingHookRawBead, "hook-raw-bead", false, "Hook raw bead without default formula (expert mode)")
	slingCmd.Flags().BoolVar(&slingNoMerge, "no-merge", false, "Skip merge queue on completion (keep work on feature branch for review)")
	slingCmd.Flags().StringVar(&slingMergeStrategy, "merge", "", "Merge strategy: direct (push to main), mr (refinery), local (merge locally)")
	slingCmd.Flags().BoolVar(&slingOwned, "owned", false, "Create caller-owned convoy (caller manages lifecycle via gt convoy land)")

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

	// Normalize target arguments: trim trailing slashes from target to handle tab-completion
	// artifacts like "gt sling sl-123 slingshot/" → "gt sling sl-123 slingshot"
	// This makes sling more forgiving without breaking existing functionality.
	// Note: Internal agent IDs like "mayor/" are outputs, not user inputs.
	for i := range args {
		args[i] = strings.TrimRight(args[i], "/")
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
	attachedMoleculeID := ""

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

	// Determine target agent (self or specified)
	var targetAgent string
	var targetPane string
	var hookWorkDir string                  // Working directory for running bd hook commands
	var hookSetAtomically bool              // True if hook was set during polecat spawn (skip redundant update)
	var delayedDogInfo *DogDispatchInfo     // For delayed dog session start after hook is set
	var crewTargetRig, crewTargetName string // Track crew target for mail notification (hq--bug-gt_sling_crew_doesn_t_send_mail)

	// Deferred spawn: don't spawn polecat until AFTER formula instantiation succeeds.
	// This prevents orphan polecats when formula fails (GH #gt-e9o).
	var deferredRigName string
	var deferredSpawnOpts SlingSpawnOptions

	if len(args) > 1 {
		target := args[1]

		// Check if target is a crew member for mail notification (hq--bug-gt_sling_crew_doesn_t_send_mail)
		if rig, name, ok := parseCrewTarget(target); ok {
			crewTargetRig, crewTargetName = rig, name
		}

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
				// Dispatch to dog with delayed session start
				// Session starts after hook is set to avoid race condition
				dispatchOpts := DogDispatchOptions{
					Create:            slingCreate,
					WorkDesc:          beadID,
					DelaySessionStart: true,
				}
				dispatchInfo, dispatchErr := DispatchToDog(dogName, dispatchOpts)
				if dispatchErr != nil {
					return fmt.Errorf("dispatching to dog: %w", dispatchErr)
				}
				targetAgent = dispatchInfo.AgentID
				delayedDogInfo = dispatchInfo // Store for later session start
				fmt.Printf("Dispatched to dog %s (session start delayed)\n", dispatchInfo.DogName)
			}
		} else if rigName, isRig := IsRigName(target); isRig {
			// Check if target is a rig name (auto-spawn polecat)
			if slingDryRun {
				// Dry run - just indicate what would happen
				fmt.Printf("Would spawn fresh polecat in rig '%s'\n", rigName)
				targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
				targetPane = "<new-pane>"
			} else {
				// DEFERRED SPAWN: Don't spawn polecat yet - we need to validate bead
				// and instantiate formula first. This prevents orphan polecats when
				// formula instantiation fails (GH #gt-e9o).
				fmt.Printf("Target is rig '%s', will spawn polecat after validation...\n", rigName)
				deferredRigName = rigName
				deferredSpawnOpts = SlingSpawnOptions{
					Force:   slingForce,
					Account: slingAccount,
					Create:  slingCreate,
					// HookBead: NOT set - we'll hook via bd update after spawn
					Agent: slingAgent,
				}
				// Use placeholder values until spawn
				targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
				targetPane = ""
				// hookWorkDir stays empty - formula instantiation will use townRoot
				// hookSetAtomically = false - hook will be set via bd update
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
						// DEFERRED SPAWN: Don't spawn yet - validate bead and instantiate
						// formula first. This prevents orphan polecats (GH #gt-e9o).
						fmt.Printf("Target polecat has no active session, will spawn fresh polecat after validation...\n")
						deferredRigName = rigName
						deferredSpawnOpts = SlingSpawnOptions{
							Force:   slingForce,
							Account: slingAccount,
							Create:  slingCreate,
							// HookBead: NOT set - we'll hook via bd update after spawn
							Agent: slingAgent,
						}
						// Use placeholder values until spawn
						targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
						targetPane = ""
						// hookWorkDir stays empty - formula instantiation will use townRoot
						// hookSetAtomically = false - hook will be set via bd update
					} else {
						return fmt.Errorf("resolving target: %w", err)
					}
				} else if rigName, crewName, ok := parseCrewTarget(target); ok {
					// FIX (hq-cc7214.25): Auto-start crew session if not running
					fmt.Printf("Target crew %s/%s has no active session, starting...\n", rigName, crewName)

					// Get rig and crew manager
					rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
					rigsConfig, configErr := config.LoadRigsConfig(rigsConfigPath)
					if configErr != nil {
						return fmt.Errorf("loading rigs config: %w", configErr)
					}
					g := git.NewGit(townRoot)
					rigMgr := rig.NewManager(townRoot, rigsConfig, g)
					r, rigErr := rigMgr.GetRig(rigName)
					if rigErr != nil {
						return fmt.Errorf("getting rig %s: %w", rigName, rigErr)
					}

					crewGit := git.NewGit(r.Path)
					crewMgr := crew.NewManager(r, crewGit)

					// Resolve account config
					accountsPath := constants.MayorAccountsPath(townRoot)
					resolvedAcct, accountErr := config.ResolveAccount(accountsPath, slingAccount)
					if accountErr != nil {
						return fmt.Errorf("resolving account: %w", accountErr)
					}

					// Extract account fields (handle nil account)
					var claudeConfigDir, authToken, baseURL string
					if resolvedAcct != nil {
						claudeConfigDir = resolvedAcct.ConfigDir
						authToken = resolvedAcct.AuthToken
						baseURL = resolvedAcct.BaseURL
					}

					// Start the crew session
					startOpts := crew.StartOptions{
						Account:         slingAccount,
						ClaudeConfigDir: claudeConfigDir,
						AgentOverride:   slingAgent,
						AuthToken:       authToken,
						BaseURL:         baseURL,
					}
					if startErr := crewMgr.Start(crewName, startOpts); startErr != nil {
						return fmt.Errorf("starting crew session %s/%s: %w", rigName, crewName, startErr)
					}

					// Retry resolving the target now that session is running
					targetAgent, targetPane, targetWorkDir, err = resolveTargetAgent(target)
					if err != nil {
						return fmt.Errorf("resolving target after starting crew: %w", err)
					}
					if targetWorkDir != "" {
						hookWorkDir = targetWorkDir
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

	// Display what we're doing
	if formulaName != "" {
		fmt.Printf("%s Slinging formula %s on %s to %s...\n", style.Bold.Render("🎯"), formulaName, beadID, targetAgent)
	} else {
		fmt.Printf("%s Slinging %s to %s...\n", style.Bold.Render("🎯"), beadID, targetAgent)
	}

	// Check if bead is already assigned (guard against accidental re-sling)
	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("checking bead status: %w", err)
	}
	// Reject slinging closed beads - prevents wasting polecat sessions (hq-kih0dt)
	if info.Status == "closed" {
		return fmt.Errorf("bead %s is already closed\nCannot sling completed work to agents", beadID)
	}
	if (info.Status == "pinned" || info.Status == "hooked") && !slingForce {
		assignee := info.Assignee
		if assignee == "" {
			assignee = "(unknown)"
		}
		return fmt.Errorf("bead %s is already %s to %s\nUse --force to re-sling", beadID, info.Status, assignee)
	}

	// Handle --force when bead is already hooked: send shutdown to old polecat and unhook
	if info.Status == "hooked" && slingForce && info.Assignee != "" {
		fmt.Printf("%s Bead already hooked to %s, forcing reassignment...\n", style.Warning.Render("⚠"), info.Assignee)

		// Determine requester identity from env vars, fall back to "gt-sling"
		requester := "gt-sling"
		if polecat := os.Getenv("GT_POLECAT"); polecat != "" {
			requester = polecat
		} else if user := os.Getenv("USER"); user != "" {
			requester = user
		}

		// Extract rig name from assignee (e.g., "gastown/polecats/Toast" -> "gastown")
		assigneeParts := strings.Split(info.Assignee, "/")
		if len(assigneeParts) >= 3 && assigneeParts[1] == "polecats" {
			oldRigName := assigneeParts[0]
			oldPolecatName := assigneeParts[2]

			// Send LIFECYCLE:Shutdown to witness - will auto-nuke if clean,
			// otherwise create cleanup wisp for manual intervention
			if townRoot != "" {
				router := mail.NewRouter(townRoot)
				shutdownMsg := &mail.Message{
					From:     "gt-sling",
					To:       fmt.Sprintf("%s/witness", oldRigName),
					Subject:  fmt.Sprintf("LIFECYCLE:Shutdown %s", oldPolecatName),
					Body:     fmt.Sprintf("Reason: work_reassigned\nRequestedBy: %s\nBead: %s\nNewAssignee: %s", requester, beadID, targetAgent),
					Type:     mail.TypeTask,
					Priority: mail.PriorityHigh,
				}
				if err := router.Send(shutdownMsg); err != nil {
					fmt.Printf("%s Could not send shutdown to witness: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					fmt.Printf("%s Sent LIFECYCLE:Shutdown to %s/witness for %s\n", style.Bold.Render("→"), oldRigName, oldPolecatName)
				}
			}
		}

		// Unhook the bead from old owner (set status back to open)
		unhookCmd := exec.Command("bd", "update", beadID, "--status=open", "--assignee=")
		unhookCmd.Dir = beads.ResolveHookDir(townRoot, beadID, "")
		if err := unhookCmd.Run(); err != nil {
			fmt.Printf("%s Could not unhook bead from old owner: %v\n", style.Dim.Render("Warning:"), err)
		}
	}

	// Workload warning: check if target already has many hooked issues
	// Threshold of 3 hooked issues triggers a warning to suggest batching
	const workloadThreshold = 3
	existingWorkload := countHookedBeadsForAgent(townRoot, targetAgent)
	if existingWorkload >= workloadThreshold && slingConvoy == "" {
		fmt.Printf("%s %s already has %d hooked issues\n", style.Warning.Render("⚠"), targetAgent, existingWorkload)
		fmt.Printf("  Consider using --convoy to batch related work:\n")
		fmt.Printf("    gt convoy create \"Batch name\" %s\n", beadID)
		fmt.Printf("    gt sling <next-bead> %s --convoy <convoy-id>\n", targetAgent)
		fmt.Printf("  Or use 'gt workload %s' to see full queue\n\n", targetAgent)
	}

	// Convoy handling: add to existing convoy, create auto-convoy, or skip
	// Priority: --convoy flag > existing convoy > auto-create
	if !slingNoConvoy && formulaName == "" {
		if slingConvoy != "" {
			// User specified convoy to add to
			if slingDryRun {
				fmt.Printf("Would add %s to convoy %s\n", beadID, slingConvoy)
			} else {
				if err := addToConvoy(slingConvoy, beadID); err != nil {
					fmt.Printf("%s Could not add to convoy %s: %v\n", style.Dim.Render("Warning:"), slingConvoy, err)
				} else {
					fmt.Printf("%s Added to convoy 🚚 %s\n", style.Bold.Render("→"), slingConvoy)
				}
			}
		} else {
			// Check if already tracked by a convoy
			existingConvoy := isTrackedByConvoy(beadID)
			if existingConvoy == "" {
				if slingDryRun {
					fmt.Printf("Would create convoy 'Work: %s'\n", info.Title)
					fmt.Printf("Would add tracking relation to %s\n", beadID)
				} else {
					convoyID, err := createAutoConvoy(beadID, info.Title, targetAgent)
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
	}

	// Issue #288: Auto-apply mol-polecat-work when slinging bare bead to polecat.
	// This ensures polecats get structured work guidance through formula-on-bead.
	// Use --hook-raw-bead to bypass for expert/debugging scenarios.
	if formulaName == "" && !slingHookRawBead && strings.Contains(targetAgent, "/polecats/") {
		formulaName = "mol-polecat-work"
		fmt.Printf("  Auto-applying %s for polecat work...\n", formulaName)
	}

	if slingDryRun {
		if formulaName != "" {
			fmt.Printf("Would instantiate formula %s:\n", formulaName)
			fmt.Printf("  1. bd cook %s\n", formulaName)
			fmt.Printf("  2. bd mol wisp %s --var feature=\"%s\" --var issue=\"%s\"\n", formulaName, info.Title, beadID)
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

		result, err := InstantiateFormulaOnBead(formulaName, beadID, info.Title, hookWorkDir, townRoot, false, slingVars)
		if err != nil {
			return fmt.Errorf("instantiating formula %s: %w", formulaName, err)
		}

		fmt.Printf("%s Formula wisp created: %s\n", style.Bold.Render("✓"), result.WispRootID)
		fmt.Printf("%s Formula bonded to %s\n", style.Bold.Render("✓"), beadID)

		// Record attached molecule - will be stored in BASE bead (not wisp).
		// The base bead is hooked, and its attached_molecule points to the wisp.
		// This enables:
		// - gt hook/gt prime: read base bead, follow attached_molecule to show wisp steps
		// - gt done: close attached_molecule (wisp) first, then close base bead
		// - Compound resolution: base bead -> attached_molecule -> wisp
		attachedMoleculeID = result.WispRootID

		// NOTE: We intentionally keep beadID as the ORIGINAL base bead, not the wisp.
		// The base bead is hooked so that:
		// 1. gt done closes both the base bead AND the attached molecule (wisp)
		// 2. The base bead's attached_molecule field points to the wisp for compound resolution
		// Previously, this line incorrectly set beadID = wispRootID, causing:
		// - Wisp hooked instead of base bead
		// - attached_molecule stored as self-reference in wisp (meaningless)
		// - Base bead left orphaned after gt done
	}

	// Execute deferred polecat spawn if needed (for rig targets).
	// This happens AFTER formula instantiation to prevent orphan polecats on failure (GH #gt-e9o).
	//
	// Two paths: OJ dispatch (GT_SLING_OJ=1) or legacy tmux spawn.
	var ojDispatch *OjDispatchInfo // Non-nil when OJ dispatch is used
	if deferredRigName != "" && ojSlingEnabled() {
		// OJ dispatch path: GT allocates name, OJ daemon owns the polecat lifecycle.
		// OJ handles workspace creation, agent spawn, monitoring, crash recovery, cleanup.
		fmt.Printf("  Dispatching to OJ daemon for %s...\n", deferredRigName)

		// Ensure the gt-sling runbook is available for OJ
		if err := ensureOjRunbook(townRoot); err != nil {
			return fmt.Errorf("ensuring OJ runbook: %w", err)
		}

		// Get instructions from bead title for the OJ job
		instructions := getBeadInstructions(beadID)
		if slingArgs != "" {
			instructions = slingArgs
		}
		if instructions == "" {
			instructions = "Execute work on hook"
		}

		// Get base branch from bead labels
		base := GetBeadBase(beadID)

		dispatchInfo, dispatchErr := dispatchToOj(deferredRigName, deferredSpawnOpts, beadID, instructions, base, townRoot)
		if dispatchErr != nil {
			return fmt.Errorf("OJ dispatch: %w", dispatchErr)
		}
		ojDispatch = dispatchInfo
		targetAgent = dispatchInfo.AgentID
		targetPane = ""             // No tmux pane — OJ owns the session
		hookWorkDir = townRoot      // Use town root for bd commands
		hookSetAtomically = false   // OJ runbook handles hook via provision step

		// Store OJ job ID in the bead for daemon health checks
		if dispatchInfo.JobID != "" {
			if err := storeOjJobIDInBead(beadID, dispatchInfo.JobID); err != nil {
				fmt.Printf("%s Could not store OJ job ID: %v\n", style.Dim.Render("Warning:"), err)
			} else {
				fmt.Printf("%s OJ job dispatched: %s\n", style.Bold.Render("✓"), dispatchInfo.JobID)
			}
		}

		// Wake witness and refinery to monitor the new polecat
		wakeRigAgents(deferredRigName)
	} else if deferredRigName != "" {
		// Legacy tmux spawn path
		// Set HookBead atomically at spawn time to prevent race condition (GH #hq-3d01de).
		// Without this, the polecat might start before bd update sets the hook.
		deferredSpawnOpts.HookBead = beadID

		fmt.Printf("  Spawning polecat in %s...\n", deferredRigName)
		spawnInfo, spawnErr := SpawnPolecatForSling(deferredRigName, deferredSpawnOpts)
		if spawnErr != nil {
			return fmt.Errorf("spawning polecat: %w", spawnErr)
		}
		targetAgent = spawnInfo.AgentID()
		targetPane = spawnInfo.Pane
		hookWorkDir = spawnInfo.ClonePath // Run bd commands from polecat's worktree
		hookSetAtomically = true          // Hook was set during spawn - skip redundant updateAgentHookBead

		// Wake witness and refinery to monitor the new polecat
		wakeRigAgents(deferredRigName)
	}

	// Hook the bead using bd update with retry logic.
	// Dolt can fail with concurrency errors (HTTP 400) when multiple agents write simultaneously.
	// We retry with exponential backoff and verify the hook actually stuck.
	// See: https://github.com/steveyegge/gastown/issues/148
	hookDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
	const maxRetries = 3
	skipVerify := os.Getenv("GT_TEST_SKIP_HOOK_VERIFY") != "" // For tests with stub bd
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
		hookCmd.Dir = hookDir
		hookCmd.Stderr = os.Stderr
		if err := hookCmd.Run(); err != nil {
			lastErr = err
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Printf("%s Hook attempt %d failed, retrying in %v...\n", style.Warning.Render("⚠"), attempt, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hooking bead after %d attempts: %w", maxRetries, err)
		}

		// Skip verification in test mode (stubs don't track state)
		if skipVerify {
			break
		}

		// Verify the hook actually stuck (Dolt concurrency can cause silent failures)
		verifyInfo, verifyErr := getBeadInfo(beadID)
		if verifyErr != nil {
			lastErr = fmt.Errorf("verifying hook: %w", verifyErr)
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Printf("%s Hook verification failed, retrying in %v...\n", style.Warning.Render("⚠"), backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("verifying hook after %d attempts: %w", maxRetries, lastErr)
		}

		if verifyInfo.Status != "hooked" || verifyInfo.Assignee != targetAgent {
			lastErr = fmt.Errorf("hook did not stick: status=%s, assignee=%s (expected hooked, %s)",
				verifyInfo.Status, verifyInfo.Assignee, targetAgent)
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Printf("%s %v, retrying in %v...\n", style.Warning.Render("⚠"), lastErr, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hook failed after %d attempts: %w", maxRetries, lastErr)
		}

		// Success!
		break
	}

	fmt.Printf("%s Work attached to hook (status=hooked)\n", style.Bold.Render("✓"))

	// Send mail notification to crew member about new work assignment (hq--bug-gt_sling_crew_doesn_t_send_mail)
	if crewTargetName != "" {
		router := mail.NewRouter(townRoot)
		crewAddress := fmt.Sprintf("%s/crew/%s", crewTargetRig, crewTargetName)
		workMailSubject := fmt.Sprintf("WORK: %s", info.Title)
		workMailBody := fmt.Sprintf("Bead: %s\nAssigned by: %s\n\nWork is on your hook. Run 'gt hook' to view details.",
			beadID, detectActor())
		workMsg := &mail.Message{
			From:       detectActor(),
			To:         crewAddress,
			Subject:    workMailSubject,
			Body:       workMailBody,
			Type:       mail.TypeTask,
			Priority:   mail.PriorityNormal,
			SkipNotify: true, // We'll send a tmux nudge separately
		}
		if err := router.Send(workMsg); err != nil {
			fmt.Printf("%s Could not send mail notification to crew: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Mail notification sent to %s\n", style.Bold.Render("📬"), crewAddress)
		}
	}

	// Log sling event to activity feed
	actor := detectActor()
	_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadID, targetAgent))

	// Update agent bead's hook_bead field (ZFC: agents track their current work)
	// Skip if hook was already set atomically during polecat spawn - avoids "agent bead not found"
	// error when polecat redirect setup fails (GH #gt-mzyk5: agent bead created in rig beads
	// but updateAgentHookBead looks in polecat's local beads if redirect is missing).
	if !hookSetAtomically {
		updateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)
	}

	// Store dispatcher in bead description (enables completion notification to dispatcher)
	if err := storeDispatcherInBead(beadID, actor); err != nil {
		// Warn but don't fail - polecat will still complete work
		fmt.Printf("%s Could not store dispatcher in bead: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Store args in bead description (no-tmux mode: beads as data plane)
	if slingArgs != "" {
		if err := storeArgsInBead(beadID, slingArgs); err != nil {
			// Warn but don't fail - args will still be in the nudge prompt
			fmt.Printf("%s Could not store args in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
	}

	// Store no_merge flag in bead (skips merge queue on completion)
	if slingNoMerge {
		if err := storeNoMergeInBead(beadID, true); err != nil {
			fmt.Printf("%s Could not store no_merge in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s No-merge mode enabled (work stays on feature branch)\n", style.Bold.Render("✓"))
		}
	}

	// Store merge strategy in bead if specified
	if slingMergeStrategy != "" {
		if err := storeMergeStrategyInBead(beadID, slingMergeStrategy); err != nil {
			fmt.Printf("%s Could not store merge strategy in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Merge strategy: %s\n", style.Bold.Render("✓"), slingMergeStrategy)
		}
	}

	// Store convoy_owned flag in bead if specified
	if slingOwned {
		if err := storeConvoyOwnedInBead(beadID, true); err != nil {
			fmt.Printf("%s Could not store convoy_owned in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Convoy owned: caller-managed (use gt convoy land)\n", style.Bold.Render("✓"))
		}
	}

	// Record the attached molecule in the BASE bead's description.
	// This field points to the wisp (compound root) and enables:
	// - gt hook/gt prime: follow attached_molecule to show molecule steps
	// - gt done: close attached_molecule (wisp) before closing hooked bead
	// - Compound resolution: base bead -> attached_molecule -> wisp
	if attachedMoleculeID != "" {
		if err := storeAttachedMoleculeInBead(beadID, attachedMoleculeID); err != nil {
			// Warn but don't fail - polecat can still work through steps
			fmt.Printf("%s Could not store attached_molecule: %v\n", style.Dim.Render("Warning:"), err)
		}
	}

	// Start delayed dog session now that hook is set
	// This ensures dog sees the hook when gt prime runs on session start
	if delayedDogInfo != nil {
		pane, err := delayedDogInfo.StartDelayedSession()
		if err != nil {
			return fmt.Errorf("starting delayed dog session: %w", err)
		}
		targetPane = pane
	}

	// Try to inject the "start now" prompt (graceful if no tmux)
	// Skip for freshly spawned polecats - SessionManager.Start() already sent StartupNudge.
	// Skip for OJ-dispatched polecats - OJ daemon owns the session lifecycle.
	// Note: In deferred spawn mode (GH #gt-e9o), the polecat session was started
	// in the deferred spawn block above after formula instantiation succeeded.
	freshlySpawned := deferredRigName != ""
	if ojDispatch != nil {
		// OJ dispatch: daemon owns session lifecycle, no tmux nudge needed
		fmt.Printf("%s OJ daemon managing polecat %s\n", style.Bold.Render("✓"), ojDispatch.PolecatName)
	} else if freshlySpawned {
		// Fresh polecat already got StartupNudge from SessionManager.Start()
	} else if targetPane == "" {
		fmt.Printf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
	} else {
		// Ensure agent is ready before nudging (prevents race condition where
		// message arrives before Claude has fully started - see issue #115)
		sessionName := getSessionFromPane(targetPane)
		if sessionName != "" {
			if err := ensureAgentReady(sessionName); err != nil {
				// Non-fatal: warn and continue, agent will discover work via gt prime
				fmt.Printf("%s Could not verify agent ready: %v\n", style.Dim.Render("○"), err)
			}
		}

		if err := injectStartPrompt(targetPane, beadID, slingSubject, slingArgs); err != nil {
			// Graceful fallback for no-tmux mode
			fmt.Printf("%s Could not nudge (no tmux?): %v\n", style.Dim.Render("○"), err)
			fmt.Printf("  Agent will discover work via gt prime / bd show\n")
		} else {
			fmt.Printf("%s Start prompt sent\n", style.Bold.Render("▶"))
		}
	}

	return nil
}
