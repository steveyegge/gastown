package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/telemetry"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

var doneCmd = &cobra.Command{
	Use:         "done",
	GroupID:     GroupWork,
	Annotations: map[string]string{AnnotationPolecatSafe: "true"},
	Short:       "Signal work ready for merge queue",
	Long: `Signal that your work is complete and ready for the merge queue.

This is a convenience command for polecats that:
1. Submits the current branch to the merge queue
2. Auto-detects issue ID from branch name
3. Notifies the Witness with the exit outcome
4. Syncs worktree to main and transitions polecat to IDLE
   (sandbox preserved, session stays alive for reuse)

Exit statuses:
  COMPLETED      - Work done, MR submitted (default)
  ESCALATED      - Hit blocker, needs human intervention
  DEFERRED       - Work paused, issue still open

Examples:
  gt done                              # Submit branch, notify COMPLETED, transition to IDLE
  gt done --pre-verified               # Submit with pre-verification fast-path
  gt done --issue gt-abc               # Explicit issue ID
  gt done --status ESCALATED           # Signal blocker, skip MR
  gt done --status DEFERRED            # Pause work, skip MR`,
	RunE:         runDone,
	SilenceUsage: true, // Don't print usage on operational errors (confuses agents)
}

var (
	doneIssue         string
	donePriority      int
	doneStatus        string
	doneCleanupStatus string
	doneResume        bool
	donePreVerified   bool
)

// Valid exit types for gt done
const (
	ExitCompleted = "COMPLETED"
	ExitEscalated = "ESCALATED"
	ExitDeferred  = "DEFERRED"
)

func init() {
	doneCmd.Flags().StringVar(&doneIssue, "issue", "", "Source issue ID (default: parse from branch name)")
	doneCmd.Flags().IntVarP(&donePriority, "priority", "p", -1, "Override priority (0-4, default: inherit from issue)")
	doneCmd.Flags().StringVar(&doneStatus, "status", ExitCompleted, "Exit status: COMPLETED, ESCALATED, or DEFERRED")
	doneCmd.Flags().StringVar(&doneCleanupStatus, "cleanup-status", "", "Git cleanup status: clean, uncommitted, unpushed, stash, unknown (ZFC: agent-observed)")
	doneCmd.Flags().BoolVar(&doneResume, "resume", false, "Resume from last checkpoint (auto-detected, for Witness recovery)")
	doneCmd.Flags().BoolVar(&donePreVerified, "pre-verified", false, "Mark MR as pre-verified (polecat ran gates after rebasing onto target)")

	rootCmd.AddCommand(doneCmd)
}

func runDone(cmd *cobra.Command, args []string) (retErr error) {
	defer func() { telemetry.RecordDone(context.Background(), strings.ToUpper(doneStatus), retErr) }()
	// Guard: Only polecats should call gt done
	// Crew, deacons, witnesses etc. don't use gt done - they persist across tasks.
	// Polecat sessions end with gt done — the session is cleaned up, but the
	// polecat's persistent identity (agent bead, CV chain) survives across assignments.
	actor := os.Getenv("BD_ACTOR")
	if actor != "" && !isPolecatActor(actor) {
		return fmt.Errorf("gt done is for polecats only (you are %s)\nPolecat sessions end with gt done — the session is cleaned up, but identity persists.\nOther roles persist across tasks and don't use gt done.", actor)
	}

	// Validate exit status
	exitType := strings.ToUpper(doneStatus)
	if exitType != ExitCompleted && exitType != ExitEscalated && exitType != ExitDeferred {
		return fmt.Errorf("invalid exit status '%s': must be COMPLETED, ESCALATED, or DEFERRED", doneStatus)
	}

	// Persistent polecat model (gt-hdf8): sessions stay alive after gt done.
	// No deferred session kill — the polecat transitions to IDLE with sandbox
	// preserved. The Witness handles any cleanup if the polecat gets stuck.

	// Find workspace with fallback for deleted worktrees (hq-3xaxy)
	// If the polecat's worktree was deleted by Witness before gt done finishes,
	// getcwd will fail. We fall back to GT_TOWN_ROOT env var in that case.
	townRoot, cwd, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Track if cwd is available - affects which operations we can do
	cwdAvailable := cwd != ""
	if !cwdAvailable {
		style.PrintWarning("working directory deleted (worktree nuked?), using fallback paths")
		// Try to get cwd from GT_POLECAT_PATH env var (set by session manager)
		if polecatPath := os.Getenv("GT_POLECAT_PATH"); polecatPath != "" {
			cwd = polecatPath // May still be gone, but we have a path to use
		}
	}

	// Find current rig - use cwd (which has fallback for deleted worktrees)
	// instead of findCurrentRig which calls os.Getwd() and fails on deleted cwd
	var rigName string
	if cwd != "" {
		relPath, err := filepath.Rel(townRoot, cwd)
		if err == nil {
			parts := strings.Split(relPath, string(filepath.Separator))
			if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
				rigName = parts[0]
			}
		}
	}
	// Prefer GT_RIG over cwd-derived rig name when available.
	// When Claude Code resets shell cwd (e.g., to mayor/rig), the cwd-derived
	// rig name is wrong (e.g., "mayor" instead of "vets"). GT_RIG is set
	// reliably for polecats via session env injection.
	if envRig := os.Getenv("GT_RIG"); envRig != "" {
		rigName = envRig
	}
	if rigName == "" {
		return fmt.Errorf("cannot determine current rig (working directory may be deleted)")
	}

	// When gt is invoked via shell alias (cd ~/gt && gt), or when Claude Code
	// resets the shell CWD to mayor/rig, cwd is NOT the polecat's worktree.
	// Detect and reconstruct actual path.
	//
	// This triggers when cwd is:
	// - The town root itself (cd ~/gt && gt)
	// - The mayor rig path (Claude Code Bash tool CWD reset)
	// - Any non-polecat path within the rig
	cwdIsPolecatWorktree := strings.Contains(cwd, "/polecats/")
	if cwdAvailable && !cwdIsPolecatWorktree {
		if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" && rigName != "" {
			polecatClone := filepath.Join(townRoot, rigName, "polecats", polecatName, rigName)
			if _, err := os.Stat(polecatClone); err == nil {
				cwd = polecatClone
			} else {
				polecatClone = filepath.Join(townRoot, rigName, "polecats", polecatName)
				if _, err := os.Stat(filepath.Join(polecatClone, ".git")); err == nil {
					cwd = polecatClone
				}
			}
		} else if crewName := os.Getenv("GT_CREW"); crewName != "" && rigName != "" {
			crewClone := filepath.Join(townRoot, rigName, "crew", crewName)
			if _, err := os.Stat(crewClone); err == nil {
				cwd = crewClone
			}
		}
	}

	// Normalize polecat CWD: polecats may run gt done from a subdirectory (e.g.,
	// beads-ide/ inside the repo). beads.ResolveBeadsDir only looks at cwd/.beads,
	// not parent dirs, so we must normalize to the git repo root before use.
	// Walk up from cwd until we find .git, stopping if we leave the polecats area.
	if cwdAvailable && cwdIsPolecatWorktree {
		candidate := cwd
		for {
			if _, statErr := os.Stat(filepath.Join(candidate, ".git")); statErr == nil {
				cwd = candidate
				break
			}
			parent := filepath.Dir(candidate)
			if parent == candidate || !strings.Contains(parent, "/polecats/") {
				break // hit filesystem root or left polecats area
			}
			candidate = parent
		}
	}

	// Initialize git - use cwd if available, otherwise use rig's mayor clone
	var g *git.Git
	if cwdAvailable {
		g = git.NewGit(cwd)
	} else {
		// Fallback: use the rig's mayor clone for git operations
		mayorClone := filepath.Join(townRoot, rigName, "mayor", "rig")
		g = git.NewGit(mayorClone)
	}

	// Get current branch - try env var first if cwd is gone
	var branch string
	if !cwdAvailable {
		// Try to get branch from GT_BRANCH env var (set by session manager)
		branch = os.Getenv("GT_BRANCH")
	}
	// CRITICAL FIX: Only call g.CurrentBranch() if we're using the cwd-based git.
	// When cwdAvailable is false, we fall back to the mayor clone for git operations,
	// but the mayor clone is on main/master - NOT the polecat branch. Calling
	// g.CurrentBranch() in that case would incorrectly return main/master.
	if branch == "" {
		if !cwdAvailable {
			// We don't have GT_BRANCH and we're using mayor clone - can't determine branch.
			// Session stays alive (persistent polecat model) — Witness handles recovery.
			return fmt.Errorf("cannot determine branch: GT_BRANCH not set and working directory unavailable")
		}
		var err error
		branch, err = g.CurrentBranch()
		if err != nil {
			// Last resort: try to extract from polecat name (polecat/<name>-<suffix>)
			if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
				branch = fmt.Sprintf("polecat/%s", polecatName)
				style.PrintWarning("could not get branch from git, using fallback: %s", branch)
			} else {
				return fmt.Errorf("getting current branch: %w", err)
			}
		}
	}

	// Auto-detect cleanup status if not explicitly provided
	// This prevents premature polecat cleanup by ensuring witness knows git state
	if doneCleanupStatus == "" {
		if !cwdAvailable {
			// Can't detect git state without working directory, default to unknown
			doneCleanupStatus = "unknown"
			style.PrintWarning("cannot detect cleanup status - working directory deleted")
		} else {
			workStatus, err := g.CheckUncommittedWork()
			if err != nil {
				style.PrintWarning("could not auto-detect cleanup status: %v", err)
			} else {
				switch {
				case workStatus.HasUncommittedChanges:
					doneCleanupStatus = "uncommitted"
				case workStatus.StashCount > 0:
					doneCleanupStatus = "stash"
				default:
					// CheckUncommittedWork.UnpushedCommits doesn't work for branches
					// without upstream tracking (common for polecats). Use the more
					// robust BranchPushedToRemote which compares against origin/main.
					pushed, unpushedCount, err := g.BranchPushedToRemote(branch, "origin")
					if err != nil {
						style.PrintWarning("could not check if branch is pushed: %v", err)
						doneCleanupStatus = "unpushed" // err on side of caution
					} else if !pushed || unpushedCount > 0 {
						doneCleanupStatus = "unpushed"
					} else {
						doneCleanupStatus = "clean"
					}
				}
			}
		}
	}

	// Parse branch info
	info := parseBranchName(branch)

	// Override with explicit flags
	issueID := doneIssue
	if issueID == "" {
		issueID = info.Issue
	}
	worker := info.Worker

	// Determine polecat name from sender detection
	sender := detectSender()
	polecatName := ""
	if parts := strings.Split(sender, "/"); len(parts) >= 2 {
		polecatName = parts[len(parts)-1]
	}

	// Get agent bead ID for cross-referencing
	var agentBeadID string
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil {
		ctx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		agentBeadID = getAgentBeadID(ctx)

		// Persistent polecat model (gt-hdf8): no deferred session kill.
		// Sessions stay alive after gt done — polecat transitions to IDLE.
	}

	// If issue ID not set by flag or branch name, query for hooked beads
	// assigned to this agent. This replaces reading agent_bead.hook_bead
	// (hq-l6mm5: direct bead tracking instead of agent bead slot).
	if issueID == "" && sender != "" {
		bd := beads.New(cwd)
		if hookIssue := findHookedBeadForAgent(bd, sender); hookIssue != "" {
			issueID = hookIssue
		}
	}

	// Write done-intent label EARLY, before push/MR operations.
	// If gt done crashes after this point, the Witness can detect the intent
	// and auto-nuke the zombie polecat.
	//
	// Also read existing checkpoints for resume capability (gt-aufru).
	// If gt done was interrupted (SIGTERM, context exhaustion, SIGKILL),
	// checkpoints indicate which stages completed. On re-invocation, we
	// skip those stages to avoid repeating work or hitting errors.
	checkpoints := map[DoneCheckpoint]string{}
	if agentBeadID != "" {
		bd := beads.New(cwd)
		setDoneIntentLabel(bd, agentBeadID, exitType)
		checkpoints = readDoneCheckpoints(bd, agentBeadID)
		if len(checkpoints) > 0 {
			fmt.Printf("%s Resuming gt done from checkpoint (previous run was interrupted)\n", style.Bold.Render("→"))
		}
	}

	// Write heartbeat state="exiting" (gt-3vr5: heartbeat v2).
	// Tells the witness we're in the gt done flow — trust the agent until
	// heartbeat goes stale. No timer-based inference needed.
	// Parallel to done-intent label for backwards compat during migration.
	if sessionName := os.Getenv("GT_SESSION"); sessionName != "" && townRoot != "" {
		polecat.TouchSessionHeartbeatWithState(townRoot, sessionName, polecat.HeartbeatExiting, "gt done", issueID)
	}

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(filepath.Join(townRoot, rigName)); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	// For COMPLETED, we need an issue ID and branch must not be the default branch
	var mrID string
	var pushFailed bool
	var mrFailed bool
	var doneErrors []string
	var convoyInfo *ConvoyInfo // Populated if issue is tracked by a convoy
	if exitType == ExitCompleted {
		if branch == defaultBranch || branch == "master" {
			return fmt.Errorf("cannot submit %s/master branch to merge queue", defaultBranch)
		}

		// CRITICAL: Verify work exists before completing (hq-xthqf)
		// Polecats calling gt done without commits results in lost work.
		// We MUST check for:
		// 1. Working directory availability (can't verify git state without it)
		// 2. Uncommitted changes (work that would be lost)
		// 3. Unique commits compared to origin (ensures branch was pushed with actual work)

		// Block if working directory not available - can't verify git state
		if !cwdAvailable {
			return fmt.Errorf("cannot complete: working directory not available (worktree deleted?)\nUse --status DEFERRED to exit without completing")
		}

		// Block if there are uncommitted changes (would be lost on completion).
		// Runtime artifacts (.claude/, .beads/, .runtime/, __pycache__/) are
		// excluded — these are toolchain-managed and normally gitignored.
		// Without this filter, gt done fails on virtually every polecat because
		// Cursor creates .claude/ at runtime in every workspace.
		workStatus, err := g.CheckUncommittedWork()
		if err != nil {
			return fmt.Errorf("checking git status: %w", err)
		}
		if workStatus.HasUncommittedChanges && !workStatus.CleanExcludingRuntime() {
			return fmt.Errorf("cannot complete: uncommitted changes would be lost\nCommit your changes first, or use --status DEFERRED to exit without completing\nUncommitted: %s", workStatus.String())
		}

		// Check if branch has commits ahead of origin/default
		// If not, work may have been pushed directly to main - that's fine, just skip MR
		originDefault := "origin/" + defaultBranch
		aheadCount, err := g.CommitsAhead(originDefault, "HEAD")
		if err != nil {
			// Fallback to local branch comparison if origin not available
			aheadCount, err = g.CommitsAhead(defaultBranch, branch)
			if err != nil {
				// Can't determine - assume work exists and continue
				style.PrintWarning("could not check commits ahead of %s: %v", defaultBranch, err)
				aheadCount = 1
			}
		}

		// Check no_merge flag on the hooked bead. When set, this is a non-code
		// task (email, research, API calls) where zero commits is expected.
		// Must be checked before the zero-commit guard below (GH#2496).
		isNoMergeTask := false
		if issueID != "" {
			noMergeBd := beads.New(cwd)
			if noMergeIssue, showErr := noMergeBd.Show(issueID); showErr == nil {
				if af := beads.ParseAttachmentFields(noMergeIssue); af != nil && af.NoMerge {
					isNoMergeTask = true
				}
			}
		}

		// If no commits ahead, work was likely pushed directly to main (or already merged)
		// For polecats, zero commits usually means the polecat sleepwalked through
		// implementation without writing code (gastown#1484, beads#emma).
		// The --cleanup-status=clean escape is preserved for legitimate report-only
		// tasks (audits, reviews) that the formula explicitly directs to use it.
		// no_merge tasks (GH#2496) also bypass: non-code work has no commits by design.
		// IMPORTANT: The error message must NOT mention --cleanup-status=clean.
		// LLM agents read error messages and self-bypass (the original bug).
		if aheadCount == 0 {
			if os.Getenv("GT_POLECAT") != "" && doneCleanupStatus != "clean" && !isNoMergeTask {
				return fmt.Errorf("cannot complete: no commits on branch ahead of %s\n"+
					"Polecats must have at least 1 commit to submit.\n"+
					"If the bug was already fixed upstream: gt done --status DEFERRED\n"+
					"If you're blocked: gt done --status ESCALATED",
					originDefault)
			}

			// Non-polecat (crew/mayor), polecat with --cleanup-status=clean
			// (report-only tasks like audits/reviews), or no_merge polecat
			// (non-code tasks like email/research per GH#2496):
			// zero commits is valid.
			fmt.Printf("%s Branch has no commits ahead of %s\n", style.Bold.Render("→"), originDefault)
			fmt.Printf("  Work was likely pushed directly to main or already merged.\n")
			fmt.Printf("  Skipping MR creation - completing without merge request.\n\n")

			// G15 fix: Close the base issue when completing with no MR.
			// Without this, no-op polecats (bug already fixed) leave issues stuck
			// in HOOKED state with assignee pointing to the nuked polecat.
			// Normally the Refinery closes after merge, but with no MR, nothing
			// would ever close the issue.
			if issueID != "" {
				bd := beads.New(cwd)

				// Acceptance criteria gate: check for unchecked criteria before closing.
				// If criteria exist and are unchecked, warn and skip close — the bead stays
				// open for witness/mayor to handle.
				skipClose := false
				if issue, err := bd.Show(issueID); err == nil {
					if unchecked := beads.HasUncheckedCriteria(issue); unchecked > 0 {
						style.PrintWarning("issue %s has %d unchecked acceptance criteria — skipping close", issueID, unchecked)
						fmt.Printf("  The bead will remain open for witness/mayor review.\n")
						skipClose = true
					}
				}

				if !skipClose {
					closeReason := "Completed with no code changes (already fixed or pushed directly to main)"
					// G15 fix: Force-close bypasses molecule dependency checks.
					// The polecat is about to be nuked — open wisps should not block closure.
					// Retry with backoff handles transient dolt lock contention (A2).
					var closeErr error
					for attempt := 1; attempt <= 3; attempt++ {
						closeErr = bd.ForceCloseWithReason(closeReason, issueID)
						if closeErr == nil {
							fmt.Printf("%s Issue %s closed (no MR needed)\n", style.Bold.Render("✓"), issueID)
							break
						}
						if attempt < 3 {
							style.PrintWarning("close attempt %d/3 failed: %v (retrying in %ds)", attempt, closeErr, attempt*2)
							time.Sleep(time.Duration(attempt*2) * time.Second)
						}
					}
					if closeErr != nil {
						style.PrintWarning("could not close issue %s after 3 attempts: %v (issue may be left HOOKED)", issueID, closeErr)
					}
				}
			}

			// Skip straight to witness notification (no MR needed)
			goto notifyWitness
		}

		// Branch contamination preflight: check if branch is significantly behind
		// origin/main, which indicates the branch may contain stale merge-base
		// artifacts that will pollute the PR diff. (GH#2220)
		contam, err := g.CheckBranchContamination(originDefault)
		if err == nil && contam.Behind > 0 {
			const warnThreshold = 50
			const blockThreshold = 200
			if contam.Behind >= blockThreshold {
				return fmt.Errorf("branch contamination: %d commits behind %s (threshold: %d)\n"+
					"The branch is severely stale and will include unrelated changes in the PR.\n"+
					"Fix: git fetch origin && git rebase origin/%s",
					contam.Behind, originDefault, blockThreshold, defaultBranch)
			} else if contam.Behind >= warnThreshold {
				style.PrintWarning("branch is %d commits behind %s — consider rebasing to avoid PR contamination", contam.Behind, originDefault)
			}
		}

		// Determine merge strategy from convoy (gt-myofa.3)
		// Convoys can override the default MR-based workflow:
		//   direct: push commits straight to target branch, bypass refinery
		//   mr:     default — create merge-request bead, refinery merges
		//   local:  keep on feature branch, no push, no MR (for human review/upstream PRs)
		//
		// Primary: read convoy info from the issue's attachment fields (gt-7b6wf fix).
		// gt sling stores convoy_id and merge_strategy on the issue when dispatching,
		// which avoids unreliable cross-rig dep resolution at gt done time.
		// Fallback: dep-based lookup via getConvoyInfoForIssue (for issues dispatched
		// before this fix, or where attachment fields weren't set).
		convoyInfo = getConvoyInfoFromIssue(issueID, cwd)
		if convoyInfo == nil {
			convoyInfo = getConvoyInfoForIssue(issueID)
		}

		// Handle "local" strategy: skip push and MR entirely
		if convoyInfo != nil && convoyInfo.MergeStrategy == "local" {
			fmt.Printf("%s Local merge strategy: skipping push and merge queue\n", style.Bold.Render("→"))
			fmt.Printf("  Branch: %s\n", branch)
			if issueID != "" {
				fmt.Printf("  Issue: %s\n", issueID)
			}
			fmt.Println()
			fmt.Printf("%s\n", style.Dim.Render("Work stays on local feature branch."))
			goto notifyWitness
		}

		// Handle "direct" strategy: push to target branch, skip MR
		if convoyInfo != nil && convoyInfo.MergeStrategy == "direct" {
			fmt.Printf("%s Direct merge strategy: pushing to %s\n", style.Bold.Render("→"), defaultBranch)
			directRefspec := branch + ":" + defaultBranch
			directPushErr := g.Push("origin", directRefspec, false)
			if directPushErr != nil {
				pushFailed = true
				errMsg := fmt.Sprintf("direct push to %s failed: %v", defaultBranch, directPushErr)
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s", errMsg)
				goto notifyWitness
			}
			fmt.Printf("%s Branch pushed directly to %s\n", style.Bold.Render("✓"), defaultBranch)

			// Close the base issue — no MR/refinery will close it
			if issueID != "" {
				directBd := beads.New(cwd)
				closeReason := fmt.Sprintf("Direct merge to %s (convoy strategy)", defaultBranch)
				var closeErr error
				for attempt := 1; attempt <= 3; attempt++ {
					closeErr = directBd.ForceCloseWithReason(closeReason, issueID)
					if closeErr == nil {
						fmt.Printf("%s Issue %s closed (direct merge)\n", style.Bold.Render("✓"), issueID)
						break
					}
					if attempt < 3 {
						style.PrintWarning("close attempt %d/3 failed: %v (retrying in %ds)", attempt, closeErr, attempt*2)
						time.Sleep(time.Duration(attempt*2) * time.Second)
					}
				}
				if closeErr != nil {
					style.PrintWarning("could not close issue %s after 3 attempts: %v", issueID, closeErr)
				}
			}

			goto notifyWitness
		}

		// Default: "mr" strategy (or no convoy) — push branch, create MR bead

		// Pre-declare push variables for checkpoint goto (gt-aufru)
		var refspec string
		var pushErr error

		// Resume: skip push if already completed in a previous run (gt-aufru)
		if checkpoints[CheckpointPushed] != "" {
			fmt.Printf("%s Branch already pushed (resumed from checkpoint)\n", style.Bold.Render("✓"))
			goto afterPush
		}

		// CRITICAL: Push branch BEFORE creating MR bead (hq-6dk53, hq-a4ksk)
		// The MR bead triggers Refinery to process this branch. If the branch
		// isn't pushed yet, Refinery finds nothing to merge. The worktree gets
		// nuked at the end of gt done, so the commits are lost forever.
		//
		// Use explicit refspec (branch:branch) to create the remote branch.
		// Without refspec, git push follows the tracking config — polecat branches
		// track origin/main, so a bare push sends commits to main directly,
		// bypassing the MR/refinery flow (G20 root cause).
		fmt.Printf("Pushing branch to remote...\n")
		refspec = branch + ":" + branch
		pushErr = g.Push("origin", refspec, false)
		if pushErr != nil {
			// Primary push failed — try fallback from the bare repo (GH #1348).
			// When polecat sessions are reused or worktrees are stale, the worktree's
			// git context may be broken. But the branch always exists in the bare repo
			// (.repo.git) because worktree commits share the same object database.
			style.PrintWarning("primary push failed: %v — trying bare repo fallback...", pushErr)
			rigPath := filepath.Join(townRoot, rigName)
			bareRepoPath := filepath.Join(rigPath, ".repo.git")
			if _, statErr := os.Stat(bareRepoPath); statErr == nil {
				bareGit := git.NewGitWithDir(bareRepoPath, "")
				pushErr = bareGit.Push("origin", refspec, false)
				if pushErr != nil {
					style.PrintWarning("bare repo push also failed: %v", pushErr)
				} else {
					fmt.Printf("%s Branch pushed via bare repo fallback\n", style.Bold.Render("✓"))
				}
			} else {
				// No bare repo — try mayor/rig as last resort
				mayorPath := filepath.Join(rigPath, "mayor", "rig")
				if _, statErr := os.Stat(mayorPath); statErr == nil {
					mayorGit := git.NewGit(mayorPath)
					pushErr = mayorGit.Push("origin", refspec, false)
					if pushErr != nil {
						style.PrintWarning("mayor/rig push also failed: %v", pushErr)
					} else {
						fmt.Printf("%s Branch pushed via mayor/rig fallback\n", style.Bold.Render("✓"))
					}
				}
			}
		}

		if pushErr != nil {
			// All push attempts failed
			pushFailed = true
			errMsg := fmt.Sprintf("push failed for branch '%s': %v", branch, pushErr)
			doneErrors = append(doneErrors, errMsg)
			style.PrintWarning("%s\nCommits exist locally but failed to push. Witness will be notified.", errMsg)
			goto notifyWitness
		}

		// Verify the branch actually exists on remote (GH #1348).
		// Push can return exit 0 without actually pushing (e.g., stale refs,
		// worktree/bare-repo state mismatch). Verify before creating MR bead.
		if exists, verifyErr := g.RemoteBranchExists("origin", branch); verifyErr != nil {
			style.PrintWarning("could not verify push: %v (proceeding optimistically)", verifyErr)
		} else if !exists {
			// Push "succeeded" but branch not on remote — try bare repo verification
			// (worktree git may not see the pushed ref)
			rigPath := filepath.Join(townRoot, rigName)
			bareRepoPath := filepath.Join(rigPath, ".repo.git")
			if _, statErr := os.Stat(bareRepoPath); statErr == nil {
				bareGit := git.NewGitWithDir(bareRepoPath, "")
				exists, verifyErr = bareGit.RemoteBranchExists("origin", branch)
			}
			if verifyErr != nil || !exists {
				pushFailed = true
				errMsg := fmt.Sprintf("push appeared to succeed but branch '%s' not found on remote", branch)
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s\nThis may indicate a stale git context. Witness will be notified.", errMsg)
				goto notifyWitness
			}
		}
		fmt.Printf("%s Branch pushed to origin\n", style.Bold.Render("✓"))

		// Fix cleanup_status after successful push (gt-wcr).
		// Status was detected before push, so "unpushed" is now stale.
		if doneCleanupStatus == "unpushed" {
			doneCleanupStatus = "clean"
		}

		// Write push checkpoint for resume (gt-aufru)
		if agentBeadID != "" {
			cpBd := beads.New(cwd)
			writeDoneCheckpoint(cpBd, agentBeadID, CheckpointPushed, branch)
		}

	afterPush:

		if issueID == "" {
			return fmt.Errorf("cannot determine source issue from branch '%s'; use --issue to specify", branch)
		}

		// Initialize beads — warn if resolved to a local .beads/ (no redirect).
		// Without a redirect, MR beads are invisible to the Refinery.
		resolvedBeads := beads.ResolveBeadsDir(cwd)
		if beads.IsLocalBeadsDir(cwd, resolvedBeads) {
			fmt.Fprintf(os.Stderr, "WARNING: beads resolved to local dir %s (no shared-beads redirect)\n", resolvedBeads)
			fmt.Fprintf(os.Stderr, "  MR beads written here will be invisible to the Refinery — run 'gt polecat repair' to fix\n")
		}
		bd := beads.NewWithBeadsDir(cwd, resolvedBeads)

		// Check for no_merge flag - if set, skip merge queue and notify for review
		sourceIssueForNoMerge, err := bd.Show(issueID)
		if err == nil {
			attachmentFields := beads.ParseAttachmentFields(sourceIssueForNoMerge)
			if attachmentFields != nil && attachmentFields.NoMerge {
				fmt.Printf("%s No-merge mode: skipping merge queue\n", style.Bold.Render("→"))
				fmt.Printf("  Branch: %s\n", branch)
				fmt.Printf("  Issue: %s\n", issueID)
				fmt.Println()
				fmt.Printf("%s\n", style.Dim.Render("Work stays on feature branch for human review."))

				// Mail dispatcher with READY_FOR_REVIEW
				if dispatcher := attachmentFields.DispatchedBy; dispatcher != "" {
					townRouter := mail.NewRouter(townRoot)
					defer townRouter.WaitPendingNotifications()
					reviewMsg := &mail.Message{
						To:      dispatcher,
						From:    detectSender(),
						Subject: fmt.Sprintf("READY_FOR_REVIEW: %s", issueID),
						Body:    fmt.Sprintf("Branch: %s\nIssue: %s\nReady for review.", branch, issueID),
					}
					if err := townRouter.Send(reviewMsg); err != nil {
						style.PrintWarning("could not notify dispatcher: %v", err)
					} else {
						fmt.Printf("%s Dispatcher notified: READY_FOR_REVIEW\n", style.Bold.Render("✓"))
					}
				}

				// Skip MR creation, go to witness notification
				goto notifyWitness
			}
		}

		// Fallback: check if issue belongs to a direct-merge convoy that the
		// primary check (line ~483) missed — e.g., issues dispatched before the
		// attachment-field fix, or where dep-based lookup failed at that point.
		// At this stage the branch was pushed to origin/<branch> (feature branch),
		// NOT to main. So we must push to main now before skipping MR creation.
		convoyInfo = getConvoyInfoFromIssue(issueID, cwd)
		if convoyInfo == nil {
			convoyInfo = getConvoyInfoForIssue(issueID)
		}
		if convoyInfo != nil && convoyInfo.MergeStrategy == "direct" {
			fmt.Printf("%s Late-detected direct merge strategy: pushing to %s\n", style.Bold.Render("→"), defaultBranch)
			fmt.Printf("  Convoy: %s\n", convoyInfo.ID)

			// Push branch directly to main (the earlier push went to origin/<branch>)
			directRefspec := branch + ":" + defaultBranch
			directPushErr := g.Push("origin", directRefspec, false)
			if directPushErr != nil {
				// Direct push failed — fall through to normal MR creation
				style.PrintWarning("late direct push to %s failed: %v — falling through to MR", defaultBranch, directPushErr)
			} else {
				fmt.Printf("%s Branch pushed directly to %s\n", style.Bold.Render("✓"), defaultBranch)

				// Close the issue directly — refinery won't process it.
				if issueID != "" {
					var closeErr error
					for attempt := 1; attempt <= 3; attempt++ {
						closeErr = bd.ForceCloseWithReason(
							fmt.Sprintf("Direct merge to %s (convoy strategy, late detection)", defaultBranch), issueID)
						if closeErr == nil {
							fmt.Printf("%s Issue %s closed (direct merge)\n", style.Bold.Render("✓"), issueID)
							break
						}
						if attempt < 3 {
							style.PrintWarning("close attempt %d/3 failed: %v (retrying in %ds)", attempt, closeErr, attempt*2)
							time.Sleep(time.Duration(attempt*2) * time.Second)
						}
					}
					if closeErr != nil {
						style.PrintWarning("could not close issue %s after 3 attempts: %v", issueID, closeErr)
					}
				}

				goto notifyWitness
			}
		}

		// Determine target branch for the MR.
		// Priority: explicit --base-branch > integration branch auto-detect > rig default.
		target := defaultBranch

		// Check for explicit --base-branch override (stored in formula vars at sling time).
		// When gt sling is called with --base-branch, the value is persisted in the bead's
		// formula_vars field. If it differs from the rig's default branch, use it as the
		// MR target so the refinery merges into the correct branch (GH#2357).
		if sourceIssueForNoMerge != nil {
			if af := beads.ParseAttachmentFields(sourceIssueForNoMerge); af != nil {
				if bb := extractFormulaVar(af.FormulaVars, "base_branch"); bb != "" && bb != defaultBranch {
					target = bb
					fmt.Printf("  Target branch override: %s (from --base-branch)\n", target)
				}
			}
		}

		// Auto-detect integration branch from epic hierarchy (if enabled).
		// Only overrides if no explicit --base-branch was set (target == defaultBranch).
		if target == defaultBranch {
			refineryEnabled := true
			settingsPath := filepath.Join(townRoot, rigName, "settings", "config.json")
			if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
				refineryEnabled = settings.MergeQueue.IsRefineryIntegrationEnabled()
			}
			if refineryEnabled {
				autoTarget, err := beads.DetectIntegrationBranch(bd, g, issueID)
				if err == nil && autoTarget != "" {
					target = autoTarget
				}
			}
		}

		// Get source issue for priority inheritance
		var priority int
		if donePriority >= 0 {
			priority = donePriority
		} else {
			sourceIssue, err := bd.Show(issueID)
			if err != nil {
				priority = 2 // Default
			} else {
				priority = sourceIssue.Priority
			}
		}

		// Pre-declare for checkpoint goto (gt-aufru)
		var existingMR *beads.Issue

		// Resume: skip MR creation if already completed in a previous run (gt-aufru).
		// Mirrors the push checkpoint pattern above. Without this, every retry
		// re-attempts bd.Create which hits unique constraints or creates duplicates.
		if checkpoints[CheckpointMRCreated] != "" {
			mrID = checkpoints[CheckpointMRCreated]
			fmt.Printf("%s MR already created (resumed from checkpoint: %s)\n", style.Bold.Render("✓"), mrID)
			goto afterMR
		}

		// Check if MR bead already exists for this branch (idempotency)
		existingMR, err = bd.FindMRForBranch(branch)
		if err != nil {
			style.PrintWarning("could not check for existing MR: %v", err)
			// Continue with creation attempt - Create will fail if duplicate
		}

		if existingMR != nil {
			// MR already exists - use it instead of creating a new one
			mrID = existingMR.ID
			fmt.Printf("%s MR already exists (idempotent)\n", style.Bold.Render("✓"))
			fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))
		} else {
			// Build MR bead title and description
			title := fmt.Sprintf("Merge: %s", issueID)
			description := fmt.Sprintf("branch: %s\ntarget: %s\nsource_issue: %s\nrig: %s",
				branch, target, issueID, rigName)
			if worker != "" {
				description += fmt.Sprintf("\nworker: %s", worker)
			}
			if agentBeadID != "" {
				description += fmt.Sprintf("\nagent_bead: %s", agentBeadID)
			}

			// Add conflict resolution tracking fields (initialized, updated by Refinery)
			description += "\nretry_count: 0"
			description += "\nlast_conflict_sha: null"
			description += "\nconflict_task_id: null"

			// Phase 3: Add pre-verification metadata if polecat ran gates after rebasing.
			// The refinery uses these fields to fast-path merge without re-running gates.
			if donePreVerified {
				description += "\npre_verified: true"
				description += fmt.Sprintf("\npre_verified_at: %s", time.Now().UTC().Format(time.RFC3339))
				// Capture current origin/target HEAD as the verified base.
				// The polecat rebased onto this SHA before running gates.
				if verifiedBase, baseErr := g.Rev("origin/" + target); baseErr == nil {
					description += fmt.Sprintf("\npre_verified_base: %s", verifiedBase)
				} else {
					style.PrintWarning("could not resolve origin/%s for pre-verified base: %v (pre-verification data incomplete)", target, baseErr)
				}
			}

			mrIssue, err := bd.Create(beads.CreateOptions{
				Title:       title,
				Labels:      []string{"gt:merge-request"},
				Priority:    priority,
				Description: description,
				Ephemeral:   true,
			})
			if err != nil {
				// Non-fatal: record the error and skip to notifyWitness.
				// Push succeeded so branch is on remote, but MR bead failed.
				// Set mrFailed so the witness knows not to send MERGE_READY.
				mrFailed = true
				errMsg := fmt.Sprintf("MR bead creation failed: %v", err)
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s\nBranch is pushed but MR bead not created. Witness will be notified.", errMsg)
				goto notifyWitness
			}
			mrID = mrIssue.ID

			// Guard against empty ID from bd create (observed in ephemeral/wisp mode).
			// Fail fast with a clear message rather than passing "" to bd.Show.
			if mrID == "" {
				mrFailed = true
				errMsg := "MR bead creation returned empty ID"
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s\nBranch is pushed but MR bead has no ID. Witness will be notified.", errMsg)
				goto notifyWitness
			}

			// GH#1945: Verify MR bead is readable before considering it confirmed.
			// bd.Create() succeeds when the bead is written locally, but if the write
			// didn't persist (Dolt failure, corrupt state), we'd nuke the worktree
			// with no MR in the queue — losing the polecat's work permanently.
			if verifiedMR, verifyErr := bd.Show(mrID); verifyErr != nil || verifiedMR == nil {
				mrFailed = true
				errMsg := fmt.Sprintf("MR bead created but verification read-back failed (id=%s): %v", mrID, verifyErr)
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s\nBranch is pushed but MR bead not confirmed. Preserving worktree.", errMsg)
				goto notifyWitness
			}

			// Update agent bead with active_mr reference (for traceability)
			if agentBeadID != "" {
				if err := bd.UpdateAgentActiveMR(agentBeadID, mrID); err != nil {
					style.PrintWarning("could not update agent bead with active_mr: %v", err)
				}
			}

			// GH#2599: Back-link source issue to MR bead for discoverability.
			if issueID != "" {
				comment := fmt.Sprintf("MR created: %s", mrID)
				if _, err := bd.Run("comments", "add", issueID, comment); err != nil {
					style.PrintWarning("could not back-link source issue %s to MR %s: %v", issueID, mrID, err)
				}
			}

			// Success output
			fmt.Printf("%s Work submitted to merge queue (verified)\n", style.Bold.Render("✓"))
			fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))

			// NOTE: Refinery nudge is deferred to AFTER the Dolt branch merge
			// (see post-merge nudge below). Nudging here would race with the
			// merge — refinery wakes up and queries main before the polecat's
			// Dolt branch (containing the MR bead) is merged.
		}

		// Write MR checkpoint for resume (gt-aufru)
		if mrID != "" && agentBeadID != "" {
			cpBd := beads.New(cwd)
			writeDoneCheckpoint(cpBd, agentBeadID, CheckpointMRCreated, mrID)
		}

	afterMR:
		fmt.Printf("  Source: %s\n", branch)
		fmt.Printf("  Target: %s\n", target)
		fmt.Printf("  Issue: %s\n", issueID)
		if worker != "" {
			fmt.Printf("  Worker: %s\n", worker)
		}
		fmt.Printf("  Priority: P%d\n", priority)
		fmt.Println()
		fmt.Printf("%s\n", style.Dim.Render("The Refinery will process your merge request."))
	} else {
		// For ESCALATED or DEFERRED, just print status
		fmt.Printf("%s Signaling %s\n", style.Bold.Render("→"), exitType)
		if issueID != "" {
			fmt.Printf("  Issue: %s\n", issueID)
		}
		fmt.Printf("  Branch: %s\n", branch)
	}

notifyWitness:
	// Nudge refinery — MR bead is already on main (transaction-based shared main).
	if mrID != "" {
		nudgeRefinery(rigName, "MERGE_READY received - check inbox for pending work")
	}

	// Write completion metadata to agent bead for audit trail.
	// Self-managed completion (gt-1qlg): metadata is retained for anomaly
	// detection and crash recovery by witness patrol, but the witness no
	// longer processes routine completions from these fields.
	fmt.Printf("\nNotifying Witness...\n")
	if agentBeadID != "" {
		completionBd := beads.New(cwd)
		meta := &beads.CompletionMetadata{
			ExitType:       exitType,
			MRID:           mrID,
			Branch:         branch,
			HookBead:       issueID,
			MRFailed:       mrFailed,
			CompletionTime: time.Now().UTC().Format(time.RFC3339),
		}
		if err := completionBd.UpdateAgentCompletion(agentBeadID, meta); err != nil {
			style.PrintWarning("could not write completion metadata to agent bead: %v", err)
		}
	}

	// Nudge witness via tmux (observability, not critical path).
	// Self-managed completion (gt-1qlg): witness no longer processes routine completions.
	// The nudge is kept for observability — witness logs the event but doesn't
	// need to act on it. Nudges are free (no Dolt commit).
	nudgeWitness(rigName, fmt.Sprintf("POLECAT_DONE %s exit=%s", polecatName, exitType))
	fmt.Printf("%s Witness notified of %s (via nudge)\n", style.Bold.Render("✓"), exitType)

	// Write witness notification checkpoint for resume (gt-aufru)
	if agentBeadID != "" {
		cpBd := beads.New(cwd)
		writeDoneCheckpoint(cpBd, agentBeadID, CheckpointWitnessNotified, "ok")
	}

	// Log done event (townlog and activity feed)
	if err := LogDone(townRoot, sender, issueID); err != nil {
		style.PrintWarning("could not log done event: %v", err)
	}
	if err := events.LogFeed(events.TypeDone, sender, events.DonePayload(issueID, branch)); err != nil {
		style.PrintWarning("could not log feed event: %v", err)
	}

	// Update agent bead state (ZFC: self-report completion)
	updateAgentStateOnDone(cwd, townRoot, exitType, issueID)

	// Persistent polecat model (gt-hdf8): polecats transition to IDLE after completion.
	// Session stays alive, sandbox preserved, worktree synced to main for reuse.
	// "done means idle" - not "done means dead".
	isPolecat := false
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil && roleInfo.Role == RolePolecat {
		isPolecat = true

		fmt.Printf("%s Sandbox preserved for reuse (persistent polecat)\n", style.Bold.Render("✓"))

		if pushFailed || mrFailed {
			fmt.Printf("%s Work needs recovery (push or MR failed) — session preserved\n", style.Bold.Render("⚠"))
		}

		// Sync worktree to main so the polecat is ready for new assignments.
		// Phase 3 of persistent-polecat-pool: DONE→IDLE syncs to main and deletes old branch.
		// Non-fatal: if sync fails, the polecat is still IDLE and the Witness
		// or next gt sling can handle the branch state.
		if cwdAvailable && !pushFailed {
			// Remember the old branch so we can delete it after switching
			oldBranch := branch

			fmt.Printf("%s Syncing worktree to %s...\n", style.Bold.Render("→"), defaultBranch)
			if err := g.Checkout(defaultBranch); err != nil {
				style.PrintWarning("could not checkout %s: %v (worktree stays on feature branch)", defaultBranch, err)
			} else if err := g.Pull("origin", defaultBranch); err != nil {
				style.PrintWarning("could not pull %s: %v (worktree on %s but may be stale)", defaultBranch, defaultBranch, err)
			} else {
				fmt.Printf("%s Worktree synced to %s\n", style.Bold.Render("✓"), defaultBranch)
			}

			// Delete the old polecat branch (non-fatal: cleanup only).
			// This prevents stale branch accumulation from persistent polecats.
			if oldBranch != "" && oldBranch != defaultBranch && oldBranch != "master" {
				if err := g.DeleteBranch(oldBranch, true); err != nil {
					style.PrintWarning("could not delete old branch %s: %v", oldBranch, err)
				} else {
					fmt.Printf("%s Deleted old branch %s\n", style.Bold.Render("✓"), oldBranch)
				}
			}
		}

		fmt.Printf("%s Polecat transitioned to IDLE — ready for new work\n", style.Bold.Render("✓"))
	}

	fmt.Println()
	if !isPolecat {
		fmt.Printf("%s Session exiting\n", style.Bold.Render("→"))
		fmt.Printf("  Witness will handle cleanup.\n")
	}
	return nil
}

// setDoneIntentLabel writes a done-intent:<type>:<unix-ts> label on the agent bead
// EARLY in gt done, before push/MR. This allows the Witness to detect polecats that
// crashed mid-gt-done: if the session is dead but done-intent exists, the polecat was
// trying to exit and should be auto-nuked.
//
// Follows the existing idle:N / backoff-until:TIMESTAMP label pattern.
// Non-fatal: if this fails, gt done continues without the safety net.
func setDoneIntentLabel(bd *beads.Beads, agentBeadID, exitType string) {
	if agentBeadID == "" {
		return
	}
	label := fmt.Sprintf("done-intent:%s:%d", exitType, time.Now().Unix())
	if err := bd.Update(agentBeadID, beads.UpdateOptions{
		AddLabels: []string{label},
	}); err != nil {
		// Non-fatal: warn but continue
		fmt.Fprintf(os.Stderr, "Warning: couldn't set done-intent label on %s: %v\n", agentBeadID, err)
	}
}

// clearDoneIntentLabel removes any done-intent:* label from the agent bead.
// Called at the end of updateAgentStateOnDone on clean exit.
// Uses read-modify-write pattern (same as clearAgentBackoffUntil).
func clearDoneIntentLabel(bd *beads.Beads, agentBeadID string) {
	if agentBeadID == "" {
		return
	}
	issue, err := bd.Show(agentBeadID)
	if err != nil {
		return // Agent bead gone, nothing to clear
	}

	var toRemove []string
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "done-intent:") {
			toRemove = append(toRemove, label)
		}
	}
	if len(toRemove) == 0 {
		return // No done-intent label to clear
	}

	if err := bd.Update(agentBeadID, beads.UpdateOptions{
		RemoveLabels: toRemove,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't clear done-intent label on %s: %v\n", agentBeadID, err)
	}
}

// DoneCheckpoint represents a checkpoint stage in the gt done flow (gt-aufru).
// Checkpoints are stored as labels on the agent bead, enabling resume after
// process interruption (context exhaustion, SIGTERM, etc.).
type DoneCheckpoint string

const (
	CheckpointPushed          DoneCheckpoint = "pushed"
	CheckpointMRCreated       DoneCheckpoint = "mr-created"
	CheckpointWitnessNotified DoneCheckpoint = "witness-notified"
)

// writeDoneCheckpoint writes a checkpoint label on the agent bead.
// Format: done-cp:<stage>:<value>:<unix-ts>
// Non-fatal: if this fails, gt done continues without the checkpoint.
func writeDoneCheckpoint(bd *beads.Beads, agentBeadID string, cp DoneCheckpoint, value string) {
	if agentBeadID == "" {
		return
	}
	label := fmt.Sprintf("done-cp:%s:%s:%d", cp, value, time.Now().Unix())
	if err := bd.Update(agentBeadID, beads.UpdateOptions{
		AddLabels: []string{label},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't write checkpoint %s on %s: %v\n", cp, agentBeadID, err)
	}
}

// readDoneCheckpoints reads all done-cp:* labels from the agent bead.
// Returns a map of checkpoint stage -> value. Empty map if none found.
func readDoneCheckpoints(bd *beads.Beads, agentBeadID string) map[DoneCheckpoint]string {
	checkpoints := make(map[DoneCheckpoint]string)
	if agentBeadID == "" {
		return checkpoints
	}
	issue, err := bd.Show(agentBeadID)
	if err != nil {
		return checkpoints
	}
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "done-cp:") {
			// Format: done-cp:<stage>:<value>:<ts>
			parts := strings.SplitN(label, ":", 4)
			if len(parts) >= 3 {
				stage := DoneCheckpoint(parts[1])
				value := parts[2]
				checkpoints[stage] = value
			}
		}
	}
	return checkpoints
}

// clearDoneCheckpoints removes all done-cp:* labels from the agent bead.
// Called on clean exit to prevent stale checkpoints from interfering with future runs.
func clearDoneCheckpoints(bd *beads.Beads, agentBeadID string) {
	if agentBeadID == "" {
		return
	}
	issue, err := bd.Show(agentBeadID)
	if err != nil {
		return
	}
	var toRemove []string
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "done-cp:") {
			toRemove = append(toRemove, label)
		}
	}
	if len(toRemove) == 0 {
		return
	}
	if err := bd.Update(agentBeadID, beads.UpdateOptions{
		RemoveLabels: toRemove,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't clear done checkpoints on %s: %v\n", agentBeadID, err)
	}
}

// updateAgentStateOnDone closes the hooked work bead and reports cleanup status.
// Uses issueID directly to find the hooked bead instead of reading the agent bead's
// hook_bead slot (hq-l6mm5: direct bead tracking).
//
// Per gt-zecmc: observable states ("done", "idle") removed - use tmux to discover.
// Non-observable states ("stuck", "awaiting-gate") are still set since they represent
// intentional agent decisions that can't be observed from tmux.
//
// Also self-reports cleanup_status for ZFC compliance (#10).
//
// BUG FIX (hq-3xaxy): This function must be resilient to working directory deletion.
// If the polecat's worktree is deleted before gt done finishes, we use env vars as fallback.
// All errors are warnings, not failures - gt done must complete even if bead ops fail.
func updateAgentStateOnDone(cwd, townRoot, exitType, issueID string) {
	// Get role context - try multiple sources for resilience
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		// Fallback: try to construct role info from environment variables
		// This handles the case where cwd is deleted but env vars are set
		envRole := os.Getenv("GT_ROLE")
		envRig := os.Getenv("GT_RIG")
		envPolecat := os.Getenv("GT_POLECAT")

		if envRole == "" || envRig == "" {
			// Can't determine role, skip agent state update
			style.PrintWarning("could not determine role for agent state update (env: GT_ROLE=%q, GT_RIG=%q)", envRole, envRig)
			return
		}

		// Parse role string to get Role type
		parsedRole, _, _ := parseRoleString(envRole)

		roleInfo = RoleInfo{
			Role:     parsedRole,
			Rig:      envRig,
			Polecat:  envPolecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
			Source:   "env-fallback",
		}
	}

	ctx := RoleContext{
		Role:     roleInfo.Role,
		Rig:      roleInfo.Rig,
		Polecat:  roleInfo.Polecat,
		TownRoot: townRoot,
		WorkDir:  cwd,
	}

	agentBeadID := getAgentBeadID(ctx)
	if agentBeadID == "" {
		style.PrintWarning("no agent bead ID found for %s/%s, skipping agent state update", ctx.Rig, ctx.Polecat)
		return
	}

	// Use rig path for bd commands.
	// IMPORTANT: Use the rig's directory (not polecat worktree) so bd commands
	// work even if the polecat worktree is deleted.
	var beadsPath string
	switch ctx.Role {
	case RoleMayor, RoleDeacon:
		beadsPath = townRoot
	default:
		beadsPath = filepath.Join(townRoot, ctx.Rig)
	}
	bd := beads.New(beadsPath)

	// Find the hooked bead to close. Use issueID directly instead of reading
	// agent bead's hook_bead slot (hq-l6mm5: direct bead tracking).
	hookedBeadID := issueID
	if hookedBeadID == "" {
		// Fallback: query for hooked beads assigned to this agent
		agentID := roleInfo.ActorString()
		if found := findHookedBeadForAgent(bd, agentID); found != "" {
			hookedBeadID = found
		}
	}

	if hookedBeadID != "" && exitType != ExitDeferred {
		// BUG FIX (gt-pftz): Close hooked bead unless already terminal (closed/tombstone).
		// Previously checked hookedBead.Status == StatusHooked, but polecats update
		// their work bead to in_progress during work. The exact-match check caused
		// gt done to skip closing the bead, leaving it as unassigned open work after
		// the hook was cleared — triggering infinite dispatch loops.
		//
		// DEFERRED exits preserve the bead: work is paused, not done. The bead
		// stays open/in_progress so it can be resumed on the next session.
		if hookedBead, err := bd.Show(hookedBeadID); err == nil && !beads.IssueStatus(hookedBead.Status).IsTerminal() {
			// Guard: never close a rig identity bead. Polecats dispatched with the
			// rig bead as their hook (via mol-polecat-work) must not close permanent
			// infrastructure. Skip close and fall through to idle state update.
			if beads.HasLabel(hookedBead, "gt:rig") {
				fmt.Fprintf(os.Stderr, "Note: hooked bead %s is a rig identity bead (gt:rig) — skipping close\n", hookedBeadID)
				goto doneStateUpdate
			}

			// BUG FIX: Close attached molecule (wisp) BEFORE closing hooked bead.
			// When using formula-on-bead (gt sling formula --on bead), the base bead
			// has attached_molecule pointing to the wisp. Without this fix, gt done
			// only closed the hooked bead, leaving the wisp orphaned.
			// Order matters: wisp closes -> unblocks base bead -> base bead closes.
			attachment := beads.ParseAttachmentFields(hookedBead)
			if attachment != nil && attachment.AttachedMolecule != "" {
				// Close molecule step descendants before closing the wisp root.
				// bd close doesn't cascade — without this, open/in_progress steps
				// from the molecule stay stuck forever after gt done completes.
				// Order: step children -> wisp root -> base bead.
				if n := closeDescendants(bd, attachment.AttachedMolecule); n > 0 {
					fmt.Fprintf(os.Stderr, "Closed %d molecule step(s) for %s\n", n, attachment.AttachedMolecule)
				}

				// Close the wisp root with --force and audit reason.
				// ForceCloseWithReason handles any status (hooked, open, in_progress)
				// and records the reason + session for attribution.
				// Same pattern as gt mol burn/squash (#1879).
				if closeErr := bd.ForceCloseWithReason("done", attachment.AttachedMolecule); closeErr != nil {
					if !errors.Is(closeErr, beads.ErrNotFound) {
						fmt.Fprintf(os.Stderr, "Warning: couldn't close attached molecule %s: %v\n", attachment.AttachedMolecule, closeErr)
						// Don't try to close hookedBeadID - it may still be blocked
						// The Witness will clean up orphaned state
						return
					}
					// Not found = already burned/deleted by another path, continue
				}
			}

			// Acceptance criteria gate: skip close if criteria are unchecked.
			if unchecked := beads.HasUncheckedCriteria(hookedBead); unchecked > 0 {
				style.PrintWarning("hooked bead %s has %d unchecked acceptance criteria — skipping close", hookedBeadID, unchecked)
				fmt.Fprintf(os.Stderr, "  The bead will remain open for witness/mayor review.\n")
			} else if err := bd.Close(hookedBeadID); err != nil {
				// Non-fatal: warn but continue
				fmt.Fprintf(os.Stderr, "Warning: couldn't close hooked bead %s: %v\n", hookedBeadID, err)
			}
		}
	}

doneStateUpdate:
	// No ClearHookBead call needed — agent bead hook slot is no longer maintained (hq-l6mm5).

	// Purge closed ephemeral beads (wisps) accumulated during this and prior sessions.
	// Without this, closed wisps from mol-polecat-work steps, mol-witness-patrol cycles,
	// etc. accumulate across sessions and pollute bd ready/list output (hq-6161m).
	// Best-effort: failures are non-fatal since the work is already done.
	purgeClosedEphemeralBeads(bd)

	// Self-managed completion (gt-1qlg, polecat-self-managed-completion.md Phase 2):
	// Polecat sets agent_state=idle directly, skipping the intermediate "done" state.
	// The witness is no longer in the critical path for routine completions.
	// Completion metadata (exit_type, MR ID, branch) remains on the agent bead
	// for audit purposes and anomaly detection by witness patrol.
	// Exception: ESCALATED exits use "stuck" — the polecat needs help.
	doneState := "idle"
	if exitType == ExitEscalated {
		doneState = "stuck"
	}
	if _, err := bd.Run("agent", "state", agentBeadID, doneState); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s to %s: %v\n", agentBeadID, doneState, err)
	}

	// ZFC #10: Self-report cleanup status
	// Agent observes git state and passes cleanup status via --cleanup-status flag
	if doneCleanupStatus != "" {
		cleanupStatus := parseCleanupStatus(doneCleanupStatus)
		if cleanupStatus != polecat.CleanupUnknown {
			if err := bd.UpdateAgentCleanupStatus(agentBeadID, string(cleanupStatus)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: couldn't update agent %s cleanup status: %v\n", agentBeadID, err)
				return
			}
		}
	}

	// Clear done-intent label and checkpoints on clean exit — gt done completed
	// successfully. If we don't reach here (crash/stuck), the Witness uses the
	// lingering labels to detect the zombie and resume from checkpoints.
	clearDoneIntentLabel(bd, agentBeadID)
	clearDoneCheckpoints(bd, agentBeadID)
}

// findHookedBeadForAgent queries for beads with status=hooked assigned to this agent.
// This is the authoritative source for what work a polecat is doing, since the
// work bead itself tracks status and assignee (hq-l6mm5).
// Returns empty string if no hooked bead is found.
func findHookedBeadForAgent(bd *beads.Beads, agentID string) string {
	if agentID == "" {
		return ""
	}
	hookedBeads, err := bd.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil || len(hookedBeads) == 0 {
		return ""
	}
	return hookedBeads[0].ID
}

// parseCleanupStatus converts a string flag value to a CleanupStatus.
// ZFC: Agent observes git state and passes the appropriate status.
func parseCleanupStatus(s string) polecat.CleanupStatus {
	switch strings.ToLower(s) {
	case "clean":
		return polecat.CleanupClean
	case "uncommitted", "has_uncommitted":
		return polecat.CleanupUncommitted
	case "stash", "has_stash":
		return polecat.CleanupStash
	case "unpushed", "has_unpushed":
		return polecat.CleanupUnpushed
	default:
		return polecat.CleanupUnknown
	}
}

// selfNukePolecat deletes this polecat's worktree.
// DEPRECATED (gt-4ac): No longer called from gt done. Polecats now go idle
// instead of self-nuking. Kept for explicit nuke scenarios.
// This is safe because:
// 1. Work has been pushed to origin (verified below)
// 2. We're about to exit anyway
// 3. Unix allows deleting directories while processes run in them
func selfNukePolecat(roleInfo RoleInfo, _ string) error {
	if roleInfo.Role != RolePolecat || roleInfo.Polecat == "" || roleInfo.Rig == "" {
		return fmt.Errorf("not a polecat: role=%s, polecat=%s, rig=%s", roleInfo.Role, roleInfo.Polecat, roleInfo.Rig)
	}

	// Get polecat manager using existing helper
	mgr, _, err := getPolecatManager(roleInfo.Rig)
	if err != nil {
		return fmt.Errorf("getting polecat manager: %w", err)
	}

	// Verify branch actually exists on a remote before nuking local copy.
	// If push didn't land (no remote, auth failure, etc.), preserve worktree
	// so Witness/Refinery can still access the branch.
	clonePath := mgr.ClonePath(roleInfo.Polecat)
	polecatGit := git.NewGit(clonePath)
	remotes, err := polecatGit.Remotes()
	if err != nil || len(remotes) == 0 {
		return fmt.Errorf("no git remotes configured — preserving worktree to prevent data loss")
	}
	branchName, err := polecatGit.CurrentBranch()
	if err != nil {
		return fmt.Errorf("cannot determine current branch — preserving worktree: %w", err)
	}
	pushed := false
	for _, remote := range remotes {
		exists, err := polecatGit.RemoteBranchExists(remote, branchName)
		if err == nil && exists {
			pushed = true
			break
		}
	}
	if !pushed {
		return fmt.Errorf("branch %s not found on any remote — preserving worktree", branchName)
	}

	// Use nuclear=true since we verified the branch is pushed
	// selfNuke=true because polecat is deleting its own worktree from inside it
	if err := mgr.RemoveWithOptions(roleInfo.Polecat, true, true, true); err != nil {
		return fmt.Errorf("removing worktree: %w", err)
	}

	return nil
}

// isPolecatActor checks if a BD_ACTOR value represents a polecat.
// Polecat actors have format: rigname/polecats/polecatname
// Non-polecat actors have formats like: gastown/crew/name, rigname/witness, etc.
func isPolecatActor(actor string) bool {
	parts := strings.Split(actor, "/")
	return len(parts) >= 2 && parts[1] == "polecats"
}

// selfKillSession terminates the polecat's own tmux session after logging the event.
// DEPRECATED (gt-hdf8): No longer called from gt done. Polecats now transition to
// IDLE with session preserved instead of self-killing. Kept for explicit kill scenarios
// (e.g., Witness-directed termination).
//
// The polecat determines its session from environment variables:
// - GT_RIG: the rig name
// - GT_POLECAT: the polecat name
// Session name format: gt-<rig>-<polecat>
func selfKillSession(townRoot string, roleInfo RoleInfo) error {
	// Get session info from environment (set at session startup)
	rigName := os.Getenv("GT_RIG")
	polecatName := os.Getenv("GT_POLECAT")

	// Fall back to roleInfo if env vars not set (shouldn't happen but be safe)
	if rigName == "" {
		rigName = roleInfo.Rig
	}
	if polecatName == "" {
		polecatName = roleInfo.Polecat
	}

	if rigName == "" || polecatName == "" {
		return fmt.Errorf("cannot determine session: rig=%q, polecat=%q", rigName, polecatName)
	}

	sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)

	// Log to townlog (human-readable audit log)
	if townRoot != "" {
		logger := townlog.NewLogger(townRoot)
		_ = logger.Log(townlog.EventKill, agentID, "self-clean: done means idle")
	}

	// Log to events (JSON audit log with structured payload)
	_ = events.LogFeed(events.TypeSessionDeath, agentID,
		events.SessionDeathPayload(sessionName, agentID, "self-clean: done means idle", "gt done"))

	// Kill our own tmux session with proper process cleanup
	// This will terminate Claude and all child processes, completing the self-cleaning cycle.
	// We use KillSessionWithProcessesExcluding to ensure no orphaned processes are left behind,
	// while excluding our own PID to avoid killing ourselves before cleanup completes.
	// The tmux kill-session at the end will terminate us along with the session.
	t := tmux.NewTmux()
	myPID := strconv.Itoa(os.Getpid())
	if err := t.KillSessionWithProcessesExcluding(sessionName, []string{myPID}); err != nil {
		return fmt.Errorf("killing session %s: %w", sessionName, err)
	}

	return nil
}

// purgeClosedEphemeralBeads removes closed ephemeral beads (wisps) that accumulated
// during this and prior sessions. Polecat/witness sessions create mol-polecat-work
// steps, mol-witness-patrol cycles, etc. as wisps. These get closed during normal
// operation but are never deleted, accumulating hundreds of rows that pollute
// bd ready/list output. (hq-6161m)
//
// Best-effort: errors are logged but don't block gt done completion.
func purgeClosedEphemeralBeads(bd *beads.Beads) {
	out, err := bd.Run("purge", "--force", "--quiet")
	if err != nil {
		// Non-fatal: purge failure shouldn't block session completion
		fmt.Fprintf(os.Stderr, "Warning: wisp purge failed: %v\n", err)
		return
	}
	// bd purge --force --quiet outputs the count of purged beads
	outStr := strings.TrimSpace(string(out))
	if outStr != "" && outStr != "0" {
		fmt.Fprintf(os.Stderr, "Purged closed ephemeral beads: %s\n", outStr)
	}
}
