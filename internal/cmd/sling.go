package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
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

Merge Strategy (--merge):
  Controls how completed work lands. Stored on the auto-convoy.
  gt sling gt-abc gastown --merge=direct  # Push branch directly to main
  gt sling gt-abc gastown --merge=mr      # Merge queue (default)
  gt sling gt-abc gastown --merge=local   # Keep on feature branch

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

Stdin Mode (for shell-quoting-safe multi-line content):
  echo "review for security issues" | gt sling gt-abc gastown --stdin
  gt sling gt-abc gastown --stdin <<'EOF'
  Focus on:
  1. SQL injection in query builders
  2. XSS in template rendering
  EOF

  # With --args on CLI, stdin goes to --message:
  echo "Extra context here" | gt sling gt-abc gastown --args "patch release" --stdin

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
  gt sling gt-abc gt-def gastown --max-concurrent 3  # Limit concurrent spawns

  When multiple beads are provided with a rig target, each bead gets its own
  polecat. This parallelizes work dispatch without running gt sling N times.
  Use --max-concurrent to throttle spawn rate and prevent Dolt server overload.`,
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
	slingStdin       bool     // --stdin: read --message and/or --args from stdin
	slingHookRawBead bool     // --hook-raw-bead: hook raw bead without default formula (expert mode)

	// Flags migrated for polecat spawning (used by sling for work assignment)
	slingCreate        bool   // --create: create polecat if it doesn't exist
	slingForce         bool   // --force: force spawn even if polecat has unread mail
	slingAccount       string // --account: Claude Code account handle to use
	slingAgent         string // --agent: override runtime agent for this sling/spawn
	slingNoConvoy      bool   // --no-convoy: skip auto-convoy creation
	slingOwned         bool   // --owned: mark auto-convoy as caller-managed lifecycle
	slingNoMerge       bool   // --no-merge: skip merge queue on completion (for upstream PRs/human review)
	slingMerge         string // --merge: merge strategy for convoy (direct/mr/local)
	slingNoBoot        bool   // --no-boot: skip wakeRigAgents (avoid witness/refinery boot and lock contention)
	slingMaxConcurrent int    // --max-concurrent: limit concurrent spawns in batch mode
	slingBaseBranch    string // --base-branch: override base branch for polecat worktree
)

func init() {
	slingCmd.Flags().StringVarP(&slingSubject, "subject", "s", "", "Context subject for the work")
	slingCmd.Flags().StringVarP(&slingMessage, "message", "m", "", "Context message for the work")
	slingCmd.Flags().BoolVarP(&slingDryRun, "dry-run", "n", false, "Show what would be done")
	slingCmd.Flags().StringVar(&slingOnTarget, "on", "", "Apply formula to existing bead (implies wisp scaffolding)")
	slingCmd.Flags().StringArrayVar(&slingVars, "var", nil, "Formula variable (key=value), can be repeated")
	slingCmd.Flags().StringVarP(&slingArgs, "args", "a", "", "Natural language instructions for the executor (e.g., 'patch release')")
	slingCmd.Flags().BoolVar(&slingStdin, "stdin", false, "Read --message and/or --args from stdin (avoids shell quoting issues)")

	// Flags for polecat spawning (when target is a rig)
	slingCmd.Flags().BoolVar(&slingCreate, "create", false, "Create polecat if it doesn't exist")
	slingCmd.Flags().BoolVar(&slingForce, "force", false, "Force spawn even if polecat has unread mail")
	slingCmd.Flags().StringVar(&slingAccount, "account", "", "Claude Code account handle to use")
	slingCmd.Flags().StringVar(&slingAgent, "agent", "", "Override agent/runtime for this sling (e.g., claude, gemini, codex, or custom alias)")
	slingCmd.Flags().BoolVar(&slingNoConvoy, "no-convoy", false, "Skip auto-convoy creation for single-issue sling")
	slingCmd.Flags().BoolVar(&slingOwned, "owned", false, "Mark auto-convoy as caller-managed lifecycle (no automatic witness/refinery registration)")
	slingCmd.Flags().BoolVar(&slingHookRawBead, "hook-raw-bead", false, "Hook raw bead without default formula (expert mode)")
	slingCmd.Flags().BoolVar(&slingNoMerge, "no-merge", false, "Skip merge queue on completion (keep work on feature branch for review)")
	slingCmd.Flags().StringVar(&slingMerge, "merge", "", "Merge strategy: direct (push to main), mr (merge queue, default), local (keep on branch)")
	slingCmd.Flags().BoolVar(&slingNoBoot, "no-boot", false, "Skip rig boot after polecat spawn (avoids witness/refinery lock contention)")
	slingCmd.Flags().IntVar(&slingMaxConcurrent, "max-concurrent", 0, "Limit concurrent polecat spawns in batch mode (0 = no limit)")
	slingCmd.Flags().StringVar(&slingBaseBranch, "base-branch", "", "Override base branch for polecat worktree (e.g., 'develop', 'release/v2')")

	rootCmd.AddCommand(slingCmd)
}

func runSling(cmd *cobra.Command, args []string) error {
	// Polecats cannot sling - check early before writing anything.
	// Check GT_ROLE first: coordinators (mayor, witness, etc.) may have a stale
	// GT_POLECAT in their environment from spawning polecats. Only block if the
	// parsed role is actually polecat (handles compound forms like
	// "gastown/polecats/Toast"). If GT_ROLE is unset, fall back to GT_POLECAT.
	if role := os.Getenv("GT_ROLE"); role != "" {
		parsedRole, _, _ := parseRoleString(role)
		if parsedRole == RolePolecat {
			return fmt.Errorf("polecats cannot sling (use gt done for handoff)")
		}
	} else if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
		return fmt.Errorf("polecats cannot sling (use gt done for handoff)")
	}

	// Validate --merge flag if provided
	if slingMerge != "" {
		switch slingMerge {
		case "direct", "mr", "local":
			// Valid
		default:
			return fmt.Errorf("invalid --merge value %q: must be direct, mr, or local", slingMerge)
		}
	}

	// Disable Dolt auto-commit for all bd commands run during sling (gt-u6n6a).
	// Under concurrent load (batch slinging), auto-commits from individual bd writes
	// cause manifest contention and 'database is read only' errors. The Dolt server
	// handles commits — individual auto-commits are unnecessary.
	prevAutoCommit := os.Getenv("BD_DOLT_AUTO_COMMIT")
	os.Setenv("BD_DOLT_AUTO_COMMIT", "off")
	defer func() {
		if prevAutoCommit == "" {
			os.Unsetenv("BD_DOLT_AUTO_COMMIT")
		} else {
			os.Setenv("BD_DOLT_AUTO_COMMIT", prevAutoCommit)
		}
	}()

	// Handle --stdin: read message/args from stdin (avoids shell quoting issues)
	if slingStdin {
		if slingMessage != "" && slingArgs != "" {
			return fmt.Errorf("cannot use --stdin when both --message and --args are already provided")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		stdinContent := strings.TrimRight(string(data), "\n")
		if slingArgs == "" {
			// Default: stdin populates --args (the primary instruction channel)
			slingArgs = stdinContent
		} else {
			// --args already set on CLI, stdin goes to --message
			slingMessage = stdinContent
		}
	}

	// Get town root early - needed for BEADS_DIR when running bd commands
	// This ensures hq-* beads are accessible even when running from polecat worktree
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Normalize target arguments: trim trailing slashes from target to handle tab-completion
	// artifacts like "gt sling sl-123 slingshot/" → "gt sling sl-123 slingshot"
	// This makes sling more forgiving without breaking existing functionality.
	// Note: Internal agent IDs like "mayor/" are outputs, not user inputs.
	for i := range args {
		args[i] = strings.TrimRight(args[i], "/")
	}

	// Validate target format early, before any dispatch path (bead, formula, batch)
	// can trigger resolveTarget side-effects like polecat spawning.
	if len(args) > 1 {
		if err := ValidateTarget(args[len(args)-1]); err != nil {
			return err
		}
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

	// Check if bead is already assigned (guard against accidental re-sling).
	// This must happen before resolveTarget(), since rig targets can spawn/hook a new polecat as a side-effect.
	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("checking bead status: %w", err)
	}
	originalStatus := info.Status
	originalAssignee := info.Assignee
	force := slingForce // local copy to avoid mutating package-level flag
	if (info.Status == "pinned" || info.Status == "hooked") && !force {
		// Auto-force when hooked agent's session is confirmed dead (gt-pqf9x).
		// This eliminates the #1 friction in convoy feeding: stale hooks from
		// dead polecats blocking re-sling without --force.
		// IMPORTANT: Stale-hook check must run BEFORE idempotency check so that
		// a dead polecat with a matching target triggers re-sling, not a no-op.
		if info.Status == "hooked" && info.Assignee != "" && isHookedAgentDeadFn(info.Assignee) {
			fmt.Printf("%s Hooked agent %s has no active session, auto-forcing re-sling...\n",
				style.Warning.Render("⚠"), info.Assignee)
			force = true
		} else {
			// Agent is alive (or bead is pinned) — check idempotency before erroring.
			target := ""
			if len(args) > 1 {
				// Batch mode (len(args) > 2) exits earlier at line 231, so
				// args[len(args)-1] is always the target here.
				target = args[len(args)-1]
			}
			// Only resolve self-agent when needed (empty/dot target = self-sling).
			// For explicit targets, idempotency works regardless of cwd/env.
			selfAgent := ""
			skipIdempotency := false
			if target == "" || target == "." {
				sa, _, _, err := resolveSelfTarget()
				if err != nil {
					// Can't determine self — skip idempotency for self-target,
					// fall through to the existing error path.
					skipIdempotency = true
				} else {
					selfAgent = sa
				}
			}
			if !skipIdempotency && matchesSlingTarget(target, info.Assignee, selfAgent) {
				if formulaName == "" {
					// Plain sling to same target: no-op.
					fmt.Printf("%s Bead %s is already %s to %s, no-op\n",
						style.Dim.Render("○"), beadID, info.Status, info.Assignee)
					return nil
				}
				// Formula-on-bead with matching target: fall through so
				// formula instantiation (cook/wisp/bond) runs. The bead
				// stays hooked/pinned to the same agent — only the formula
				// work is new. We don't set force=true to avoid triggering
				// the unhook/reassign path at the force-handler below.
			} else {
				assignee := info.Assignee
				if assignee == "" {
					assignee = "(unknown)"
				}
				return fmt.Errorf("bead %s is already %s to %s\nUse --force to re-sling", beadID, info.Status, assignee)
			}
		}
	}

	// Preflight: check existing molecules BEFORE spawning polecat.
	// When formulaName is already known (explicit formula via --on flag), we can
	// validate early to avoid spawning a polecat that will be immediately orphaned
	// if the check fails. The auto-apply path (formulaName set after resolveTarget)
	// handles its own check with rollback protection below.
	//
	// Trade-off: with --force, this burns molecules before resolveTarget(). If
	// target resolution then fails, molecule state is irreversibly destroyed.
	// This is accepted --force semantics — the alternative (defer burn until after
	// spawn) re-introduces the orphaned polecat problem this PR fixes, since a
	// post-spawn burn failure would leave both a spawned polecat AND stale molecules.
	preflightFormulaName := formulaName
	if formulaName != "" {
		if err := ensureNoExistingMolecules(info, beadID, townRoot, force, slingDryRun); err != nil {
			return err
		}
	}

	// Resolve target agent using shared dispatch logic.
	// Note: args[1] == args[len(args)-1] here because batch mode (len(args) > 2
	// with rig last arg) exits at line 234. The only remaining case is len(args) <= 2.
	var target string
	if len(args) > 1 {
		target = args[1]
	}
	resolved, err := resolveTarget(target, ResolveTargetOptions{
		DryRun:     slingDryRun,
		Force:      force,
		Create:     slingCreate,
		Account:    slingAccount,
		Agent:      slingAgent,
		NoBoot:     slingNoBoot,
		HookBead:   beadID,
		BeadID:     beadID,
		TownRoot:   townRoot,
		BaseBranch: slingBaseBranch,
	})
	if err != nil {
		return err
	}
	targetAgent := resolved.Agent
	targetPane := resolved.Pane
	hookWorkDir := resolved.WorkDir
	hookSetAtomically := resolved.HookSetAtomically
	delayedDogInfo := resolved.DelayedDogInfo
	newPolecatInfo := resolved.NewPolecatInfo
	isSelfSling := resolved.IsSelfSling

	// Inject base_branch var for formula instantiation (non-main only; formula default handles main)
	if newPolecatInfo != nil && newPolecatInfo.BaseBranch != "" && newPolecatInfo.BaseBranch != "main" {
		slingVars = append(slingVars, fmt.Sprintf("base_branch=%s", newPolecatInfo.BaseBranch))
	}

	// Cross-rig guard: prevent slinging beads to polecats in the wrong rig (gt-myecw).
	// Polecats work in their rig's worktree and cannot fix code owned by another rig.
	// Skip for self-sling (user knows what they're doing) and --force overrides.
	if strings.Contains(targetAgent, "/polecats/") && !force && !isSelfSling {
		if err := checkCrossRigGuard(beadID, targetAgent, townRoot); err != nil {
			return err
		}
	}

	// Display what we're doing
	if formulaName != "" {
		fmt.Printf("%s Slinging formula %s on %s to %s...\n", style.Bold.Render("🎯"), formulaName, beadID, targetAgent)
	} else {
		fmt.Printf("%s Slinging %s to %s...\n", style.Bold.Render("🎯"), beadID, targetAgent)
	}

	// Handle --force when bead is already hooked: send shutdown to old polecat and unhook
	if info.Status == "hooked" && force && info.Assignee != "" {
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
				defer router.WaitPendingNotifications()
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

	// Auto-convoy: check if issue is already tracked by a convoy
	// If not, create one for dashboard visibility (unless --no-convoy is set)
	if !slingNoConvoy && formulaName == "" {
		existingConvoy := isTrackedByConvoy(beadID)
		if existingConvoy == "" {
			if slingDryRun {
				fmt.Printf("Would create convoy 'Work: %s'\n", info.Title)
				fmt.Printf("Would add tracking relation to %s\n", beadID)
				if slingMerge != "" {
					fmt.Printf("Would set convoy merge strategy: %s\n", slingMerge)
				}
			} else {
				convoyID, err := createAutoConvoy(beadID, info.Title, slingOwned, slingMerge)
				if err != nil {
					// Log warning but don't fail - convoy is optional
					fmt.Printf("%s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					fmt.Printf("%s Created convoy 🚚 %s\n", style.Bold.Render("→"), convoyID)
					fmt.Printf("  Tracking: %s\n", beadID)
					if slingOwned {
						fmt.Printf("  Lifecycle: caller-managed (owned)\n")
					}
					if slingMerge != "" {
						fmt.Printf("  Merge:    %s\n", slingMerge)
					}
				}
			}
		} else {
			fmt.Printf("%s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
		}
	}

	// Issue #288: Auto-apply mol-polecat-work when slinging bare bead to polecat.
	// This ensures polecats get structured work guidance through formula-on-bead.
	// Use --hook-raw-bead to bypass for expert/debugging scenarios.
	if formulaName == "" && !slingHookRawBead && strings.Contains(targetAgent, "/polecats/") {
		formulaName = "mol-polecat-work"
		fmt.Printf("  Auto-applying %s for polecat work...\n", formulaName)
	}

	// Guard: ensure only one molecule is attached to a work bead.
	// Explicit formulas (--on flag) are checked in the preflight block above,
	// before any polecat is spawned. This check handles the auto-apply path
	// (formulaName was empty at preflight but set to mol-polecat-work after
	// resolveTarget). Failures here include rollback since polecat is already spawned.
	if formulaName != "" && preflightFormulaName == "" {
		if err := ensureNoExistingMolecules(info, beadID, townRoot, force, slingDryRun); err != nil {
			if newPolecatInfo != nil {
				rollbackSlingArtifactsFn(newPolecatInfo, beadID, hookWorkDir)
				if force && originalStatus == "pinned" {
					restorePinnedBead(townRoot, beadID, originalAssignee)
				}
			}
			return err
		}
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

		// Auto-inject rig command vars as defaults (user --var flags override)
		if parts := strings.SplitN(targetAgent, "/", 2); len(parts) >= 1 && parts[0] != "" {
			rigCmdVars := loadRigCommandVars(townRoot, parts[0])
			slingVars = append(rigCmdVars, slingVars...)
		}

		result, err := InstantiateFormulaOnBead(formulaName, beadID, info.Title, hookWorkDir, townRoot, false, slingVars)
		if err != nil {
			// If we spawned a fresh polecat (rig target), rollback the partial artifacts.
			// Otherwise, a wisp creation failure (e.g., missing required vars) leaves an orphaned polecat.
			if newPolecatInfo != nil {
				fmt.Printf("%s Formula instantiation failed, rolling back spawned polecat %s...\n",
					style.Warning.Render("⚠"), newPolecatInfo.PolecatName)
				rollbackSlingArtifactsFn(newPolecatInfo, beadID, hookWorkDir)
				// Under --force, if this bead was previously pinned, rollback's unhook would otherwise
				// clear the pinned state. Restore pinned state so we don't lose the original hook.
				if force && originalStatus == "pinned" {
					restorePinnedBead(townRoot, beadID, originalAssignee)
				}
			}
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

	// Hook the bead with retry and verification.
	// See: https://github.com/steveyegge/gastown/issues/148
	hookDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
	if err := hookBeadWithRetry(beadID, targetAgent, hookDir); err != nil {
		if newPolecatInfo != nil {
			rollbackSlingArtifactsFn(newPolecatInfo, beadID, hookWorkDir)
			if force && originalStatus == "pinned" {
				restorePinnedBead(townRoot, beadID, originalAssignee)
			}
		}
		return err
	}

	fmt.Printf("%s Work attached to hook (status=hooked)\n", style.Bold.Render("✓"))

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

	// Store all attachment fields in a single read-modify-write cycle.
	// This eliminates the race condition where sequential independent updates
	// (dispatcher, args, no_merge, attached_molecule) could overwrite each other.
	fieldUpdates := beadFieldUpdates{
		Dispatcher:       actor,
		Args:             slingArgs,
		AttachedMolecule: attachedMoleculeID,
		NoMerge:          slingNoMerge,
	}
	if err := storeFieldsInBead(beadID, fieldUpdates); err != nil {
		// Warn but don't fail - polecat will still complete work
		fmt.Printf("%s Could not store fields in bead: %v\n", style.Dim.Render("Warning:"), err)
	} else {
		if slingArgs != "" {
			fmt.Printf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
		if slingNoMerge {
			fmt.Printf("%s No-merge mode enabled (work stays on feature branch)\n", style.Bold.Render("✓"))
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

	// Create Dolt branch AFTER all sling writes (hook, formula, fields) are complete.
	// CommitWorkingSet flushes working set to HEAD, then CreatePolecatBranch forks
	// from HEAD — ensuring the polecat's branch includes all writes.
	if newPolecatInfo != nil && newPolecatInfo.DoltBranch != "" {
		if err := newPolecatInfo.CreateDoltBranch(); err != nil {
			rollbackSlingArtifactsFn(newPolecatInfo, beadID, hookWorkDir)
			return fmt.Errorf("creating Dolt branch: %w", err)
		}
	}

	// Start polecat session now that attached_molecule is set.
	// This ensures polecat sees the molecule when gt prime runs on session start.
	freshlySpawned := newPolecatInfo != nil
	if freshlySpawned {
		pane, err := newPolecatInfo.StartSession()
		if err != nil {
			// Rollback: session failed, clean up zombie artifacts (worktree, hooked bead).
			// Without rollback, next sling attempt fails with "bead already hooked" (gt-jn40ft).
			fmt.Printf("%s Session failed, rolling back spawned polecat %s...\n", style.Warning.Render("⚠"), newPolecatInfo.PolecatName)
			rollbackSlingArtifactsFn(newPolecatInfo, beadID, hookWorkDir)
			return fmt.Errorf("starting polecat session: %w", err)
		}
		targetPane = pane
	}

	// Try to inject the "start now" prompt (graceful if no tmux)
	// Skip for freshly spawned polecats - SessionManager.Start() already sent StartupNudge.
	// Skip for self-sling - agent is currently processing the sling command and will see
	// the hooked work on next turn. Nudging would inject text while agent is busy.
	if freshlySpawned {
		// Fresh polecat already got StartupNudge from SessionManager.Start()
	} else if isSelfSling {
		// Self-sling: agent already knows about the work (just slung it)
		fmt.Printf("%s Self-sling: work hooked, will process on next turn\n", style.Dim.Render("○"))
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

// checkCrossRigGuard validates that a bead's prefix matches the target rig.
// Polecats work in their rig's worktree and cannot fix code owned by another rig.
// Returns an error if the bead belongs to a different rig than the target polecat.
// Town-root beads (hq-*) are rejected — tasks must be created in the target rig.
func checkCrossRigGuard(beadID, targetAgent, townRoot string) error {
	beadPrefix := beads.ExtractPrefix(beadID)
	if beadPrefix == "" {
		return nil // Can't determine prefix, skip check
	}

	// Extract target rig from agent path (e.g., "gastown/polecats/Toast" → "gastown")
	targetRig := strings.SplitN(targetAgent, "/", 2)[0]
	if targetRig == "" {
		return nil
	}

	beadRig := beads.GetRigNameForPrefix(townRoot, beadPrefix)

	if beadRig != targetRig {
		if beadRig == "" {
			return fmt.Errorf("bead %s (prefix %q) is not in rig %q — it belongs to town root\n"+
				"Create the task from the rig directory: cd %s && bd create --title=...\n"+
				"Use --force to override", beadID, strings.TrimSuffix(beadPrefix, "-"), targetRig, targetRig)
		}
		return fmt.Errorf("cross-rig mismatch: bead %s (prefix %q) belongs to rig %q, but target is rig %q\n"+
			"Create the task from the target rig: cd %s && bd create --title=...\n"+
			"Use --force to override", beadID, strings.TrimSuffix(beadPrefix, "-"), beadRig, targetRig, targetRig)
	}

	return nil
}

// rollbackSlingArtifactsFn is a seam for tests. Production uses rollbackSlingArtifacts.
var rollbackSlingArtifactsFn = rollbackSlingArtifacts

func restorePinnedBead(townRoot, beadID, assignee string) {
	if townRoot == "" || beadID == "" {
		return
	}
	dir := beads.ResolveHookDir(townRoot, beadID, "")
	cmd := exec.Command("bd", "update", beadID, "--status=pinned", "--assignee="+assignee)
	if dir != "" {
		cmd.Dir = dir
	}
	if err := cmd.Run(); err != nil {
		fmt.Printf("  %s Could not restore pinned state for bead %s: %v\n", style.Dim.Render("Warning:"), beadID, err)
	} else {
		fmt.Printf("  %s Restored pinned state for bead %s\n", style.Dim.Render("○"), beadID)
	}
}

// rollbackSlingArtifacts cleans up artifacts left by a partial sling when session start fails.
// This prevents zombie polecats that block subsequent sling attempts with "bead already hooked".
// Cleanup is best-effort: each step logs warnings but continues to clean as much as possible.
func rollbackSlingArtifacts(spawnInfo *SpawnedPolecatInfo, beadID, hookWorkDir string) {
	townRoot, err := workspace.FindFromCwdOrError()

	// 1. Unhook the bead (set status back to open so it can be re-slung).
	// Some failure modes happen before any bead is hooked (e.g., wisp creation fails).
	if beadID != "" {
		if err != nil {
			fmt.Printf("  %s Could not find workspace to unhook bead %s: %v\n", style.Dim.Render("Warning:"), beadID, err)
		} else {
			unhookDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
			unhookCmd := exec.Command("bd", "update", beadID, "--status=open", "--assignee=")
			unhookCmd.Dir = unhookDir
			if err := unhookCmd.Run(); err != nil {
				fmt.Printf("  %s Could not unhook bead %s: %v\n", style.Dim.Render("Warning:"), beadID, err)
			} else {
				fmt.Printf("  %s Unhooked bead %s\n", style.Dim.Render("○"), beadID)
			}
		}
	}

	// 2. Clean up Dolt branch if it was created
	if err == nil && spawnInfo.DoltBranch != "" && townRoot != "" {
		doltserver.DeletePolecatBranch(townRoot, spawnInfo.RigName, spawnInfo.DoltBranch)
	}

	// 3. Clean up the spawned polecat (worktree, agent bead, etc.)
	cleanupSpawnedPolecat(spawnInfo, spawnInfo.RigName)
}
