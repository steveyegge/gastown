package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/doltserver"
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
	Use:     "done",
	GroupID: GroupWork,
	Short:   "Signal work ready for merge queue",
	Long: `Signal that your work is complete and ready for the merge queue.

This is a convenience command for polecats that:
1. Submits the current branch to the merge queue
2. Auto-detects issue ID from branch name
3. Notifies the Witness with the exit outcome
4. Exits the Claude session (polecats don't stay alive after completion)

Exit statuses:
  COMPLETED      - Work done, MR submitted (default)
  ESCALATED      - Hit blocker, needs human intervention
  DEFERRED       - Work paused, issue still open

Examples:
  gt done                              # Submit branch, notify COMPLETED, exit session
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

	// Deferred session kill: ensures selfKillSession runs on ANY exit path for polecats.
	// This is the backstop that prevents zombie sessions when push/MR failures cause
	// early returns before the explicit selfKillSession call.
	//
	// Flags control behavior:
	// - sessionCleanupNeeded: set true after role detection confirms polecat
	// - sessionKilled: set true after explicit selfKillSession succeeds
	//
	// Validation errors (bad flags, wrong role) return BEFORE sessionCleanupNeeded is set,
	// so the defer is a no-op. Operational errors (push fail, MR fail) return AFTER the
	// flag is set, so the defer kills the session.
	//
	// The defer does NOT nuke worktree — only kills session. Worktree cleanup is the
	// Witness's job.
	var sessionCleanupNeeded bool
	var sessionKilled bool
	var deferredTownRoot string
	var deferredRoleInfo RoleInfo
	defer func() {
		if sessionCleanupNeeded && !sessionKilled {
			fmt.Printf("%s Deferred session cleanup (backstop)\n", style.Bold.Render("→"))
			if err := selfKillSession(deferredTownRoot, deferredRoleInfo); err != nil {
				style.PrintWarning("deferred session kill failed: %v", err)
			}
			retErr = NewSilentExit(0)
		}
	}()

	// SIGTERM handler: Claude Code may kill this process via SIGTERM when context
	// is exhausted. Without a handler, the deferred cleanup above doesn't run
	// because the process is killed before defer can execute. This handler gives
	// us a chance to run session cleanup even on external termination.
	// Checkpoints (below) handle the harder case of SIGKILL where no handler runs.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n%s Received SIGTERM — running deferred cleanup\n", style.Bold.Render("→"))
		if sessionCleanupNeeded && !sessionKilled {
			if err := selfKillSession(deferredTownRoot, deferredRoleInfo); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: SIGTERM cleanup failed: %v\n", err)
			}
		}
		os.Exit(1)
	}()
	defer signal.Stop(sigCh)

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
	if rigName == "" {
		// Last resort: try GT_RIG env var
		rigName = os.Getenv("GT_RIG")
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
			// Arm session cleanup before returning so the polecat doesn't get stranded.
			if polecatEnv := os.Getenv("GT_POLECAT"); polecatEnv != "" {
				sessionCleanupNeeded = true
				deferredTownRoot = townRoot
				deferredRoleInfo = RoleInfo{
					Role:    RolePolecat,
					Rig:     rigName,
					Polecat: polecatEnv,
				}
			}
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

		// Enable deferred session kill for polecats.
		// From this point forward, any return/error will trigger the defer
		// to kill the session, preventing zombie sessions.
		if roleInfo.Role == RolePolecat {
			sessionCleanupNeeded = true
			deferredTownRoot = townRoot
			deferredRoleInfo = roleInfo
		}
	}

	// If issue ID not set by flag or branch name, try agent's hook_bead.
	// This handles cases where branch name doesn't contain issue ID
	// (e.g., "polecat/furiosa-mkb0vq9f" doesn't have the actual issue).
	if issueID == "" && agentBeadID != "" {
		bd := beads.New(beads.ResolveBeadsDir(cwd))
		if hookIssue := getIssueFromAgentHook(bd, agentBeadID); hookIssue != "" {
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
		bd := beads.New(beads.ResolveBeadsDir(cwd))
		setDoneIntentLabel(bd, agentBeadID, exitType)
		checkpoints = readDoneCheckpoints(bd, agentBeadID)
		if len(checkpoints) > 0 {
			fmt.Printf("%s Resuming gt done from checkpoint (previous run was interrupted)\n", style.Bold.Render("→"))
		}
	}

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(filepath.Join(townRoot, rigName)); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	// For COMPLETED, we need an issue ID and branch must not be the default branch
	var mrID string
	var pushFailed bool
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

		// Block if there are uncommitted changes (would be lost on completion)
		workStatus, err := g.CheckUncommittedWork()
		if err != nil {
			return fmt.Errorf("checking git status: %w", err)
		}
		if workStatus.HasUncommittedChanges {
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

		// If no commits ahead, work was likely pushed directly to main (or already merged)
		// For polecats, zero commits usually means the polecat hallucinated completion
		// (gastown#1484). Require --cleanup-status=clean as explicit acknowledgment
		// that no code changes were expected (e.g., review or testing tasks).
		if aheadCount == 0 {
			if os.Getenv("GT_POLECAT") != "" && doneCleanupStatus != "clean" {
				return fmt.Errorf("cannot complete: no commits on branch\n" +
					"If this task required no code changes (review, testing, triage),\n" +
					"re-run with: gt done --cleanup-status clean\n" +
					"Otherwise: --status ESCALATED (blocker) or --status DEFERRED (pause)")
			}

			// Non-polecat (crew/mayor) or polecat with --cleanup-status=clean:
			// zero commits is valid — work may have been pushed directly to main,
			// already merged, or task required no code changes.
			fmt.Printf("%s Branch has no commits ahead of %s\n", style.Bold.Render("→"), originDefault)
			fmt.Printf("  Work was likely pushed directly to main or already merged.\n")
			fmt.Printf("  Skipping MR creation - completing without merge request.\n\n")

			// G15 fix: Close the base issue when completing with no MR.
			// Without this, no-op polecats (bug already fixed) leave issues stuck
			// in HOOKED state with assignee pointing to the nuked polecat.
			// Normally the Refinery closes after merge, but with no MR, nothing
			// would ever close the issue.
			if issueID != "" {
				bd := beads.New(beads.ResolveBeadsDir(cwd))
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

			// Skip straight to witness notification (no MR needed)
			goto notifyWitness
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
				directBd := beads.New(beads.ResolveBeadsDir(cwd))
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

		// Write push checkpoint for resume (gt-aufru)
		if agentBeadID != "" {
			cpBd := beads.New(beads.ResolveBeadsDir(cwd))
			writeDoneCheckpoint(cpBd, agentBeadID, CheckpointPushed, branch)
		}

	afterPush:

		if issueID == "" {
			return fmt.Errorf("cannot determine source issue from branch '%s'; use --issue to specify", branch)
		}

		// Initialize beads
		bd := beads.New(beads.ResolveBeadsDir(cwd))

		// Check for no_merge flag - if set, skip merge queue and notify for review
		// Read source issue from main (not polecat branch) — it was created there.
		restore := onMainBranch()
		sourceIssueForNoMerge, err := bd.Show(issueID)
		restore()
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

		// Determine target branch (auto-detect integration branch if applicable)
		// Only if refinery integration branch auto-targeting is enabled
		target := defaultBranch
		refineryEnabled := true
		settingsPath := filepath.Join(townRoot, rigName, "settings", "config.json")
		if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
			refineryEnabled = settings.MergeQueue.IsRefineryIntegrationEnabled()
		}
		if refineryEnabled {
			restore = onMainBranch()
			autoTarget, err := beads.DetectIntegrationBranch(bd, g, issueID)
			restore()
			if err == nil && autoTarget != "" {
				target = autoTarget
			}
		}

		// Get source issue for priority inheritance
		var priority int
		if donePriority >= 0 {
			priority = donePriority
		} else {
			// Read source issue from main where it was created (not polecat branch)
			restore := onMainBranch()
			sourceIssue, err := bd.Show(issueID)
			restore()
			if err != nil {
				priority = 2 // Default
			} else {
				priority = sourceIssue.Priority
			}
		}

		// Check if MR bead already exists for this branch (idempotency)
		// MR beads live on main, not polecat branch.
		restore = onMainBranch()
		existingMR, err := bd.FindMRForBranch(branch)
		restore()
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

			// Create MR bead directly on main (not polecat branch).
			// MR bead is a new unique INSERT — no contention risk —
			// and must be visible to refinery immediately.
			restore := onMainBranch()
			mrIssue, err := bd.Create(beads.CreateOptions{
				Title:       title,
				Type:        "merge-request",
				Priority:    priority,
				Description: description,
				Ephemeral:   true,
			})
			restore()
			if err != nil {
				// Non-fatal: record the error and skip to notifyWitness.
				// Push succeeded so branch is on remote, but MR bead failed.
				errMsg := fmt.Sprintf("MR bead creation failed: %v", err)
				doneErrors = append(doneErrors, errMsg)
				style.PrintWarning("%s\nBranch is pushed but MR bead not created. Witness will be notified.", errMsg)
				goto notifyWitness
			}
			mrID = mrIssue.ID

			// Update agent bead with active_mr reference (for traceability)
			if agentBeadID != "" {
				if err := bd.UpdateAgentActiveMR(agentBeadID, mrID); err != nil {
					style.PrintWarning("could not update agent bead with active_mr: %v", err)
				}
			}

			// Success output
			fmt.Printf("%s Work submitted to merge queue\n", style.Bold.Render("✓"))
			fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))

			// NOTE: Refinery nudge is deferred to AFTER the Dolt branch merge
			// (see post-merge nudge below). Nudging here would race with the
			// merge — refinery wakes up and queries main before the polecat's
			// Dolt branch (containing the MR bead) is merged.
		}

		// Write MR checkpoint for resume (gt-aufru)
		if mrID != "" && agentBeadID != "" {
			cpBd := beads.New(beads.ResolveBeadsDir(cwd))
			writeDoneCheckpoint(cpBd, agentBeadID, CheckpointMRCreated, mrID)
		}

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
	// Branch-per-polecat: merge polecat's Dolt branch to main.
	// This makes all beads changes (MR bead, issue updates) visible on main
	// before the refinery or witness try to read them.
	var mergeFailed bool

	// Resume: skip Dolt merge if already completed (gt-aufru checkpoint)
	if checkpoints[CheckpointDoltMerged] != "" {
		fmt.Printf("%s Dolt branch already merged (resumed from checkpoint)\n", style.Bold.Render("✓"))
		goto afterDoltMerge
	}

	if bdBranch := os.Getenv("BD_BRANCH"); bdBranch != "" {
		fmt.Printf("Merging Dolt branch %s to main...\n", bdBranch)
		if err := doltserver.MergePolecatBranch(townRoot, rigName, bdBranch); err != nil {
			mergeFailed = true
			style.PrintWarning("could not merge Dolt branch: %v (data still on branch %s)", err, bdBranch)
		} else {
			fmt.Printf("%s Dolt branch merged to main\n", style.Bold.Render("✓"))
		}
		// Unset BD_BRANCH so subsequent bd operations (updateAgentStateOnDone)
		// write directly to main instead of the now-deleted branch.
		// Note: this is a lifecycle transition (branch deleted). The systematic fix
		// for BD_BRANCH leaking into read operations is in beads.go (OnMain/StripBdBranch).
		os.Unsetenv("BD_BRANCH")
	}

	// Write Dolt merge checkpoint for resume (gt-aufru)
	if agentBeadID != "" {
		cpBd := beads.New(beads.ResolveBeadsDir(cwd))
		writeDoneCheckpoint(cpBd, agentBeadID, CheckpointDoltMerged, "ok")
	}

afterDoltMerge:
	// Nudge refinery AFTER the Dolt merge so MR bead is visible on main.
	// Skip nudge only if merge was attempted and failed — MR bead is stranded
	// on the polecat branch and refinery won't find it on main.
	// If no branch existed (crew worker), MR bead is already on main.
	if mrID != "" && !mergeFailed {
		nudgeRefinery(rigName, "MERGE_READY received - check inbox for pending work")
	}

	// Notify Witness about completion
	// Use town-level beads for cross-agent mail
	townRouter := mail.NewRouter(townRoot)
	defer townRouter.WaitPendingNotifications()
	witnessAddr := fmt.Sprintf("%s/witness", rigName)

	// Build notification body
	var bodyLines []string
	bodyLines = append(bodyLines, fmt.Sprintf("Exit: %s", exitType))
	if issueID != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("Issue: %s", issueID))
	}
	if mrID != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("MR: %s", mrID))
	}
	bodyLines = append(bodyLines, fmt.Sprintf("Branch: %s", branch))
	// Include convoy ownership info so witness can skip merge flow registration
	if convoyInfo != nil {
		bodyLines = append(bodyLines, fmt.Sprintf("ConvoyID: %s", convoyInfo.ID))
		if convoyInfo.Owned {
			bodyLines = append(bodyLines, "ConvoyOwned: true")
		}
		if convoyInfo.MergeStrategy != "" {
			bodyLines = append(bodyLines, fmt.Sprintf("MergeStrategy: %s", convoyInfo.MergeStrategy))
		}
	}
	if len(doneErrors) > 0 {
		bodyLines = append(bodyLines, fmt.Sprintf("Errors: %s", strings.Join(doneErrors, "; ")))
	}

	doneNotification := &mail.Message{
		To:      witnessAddr,
		From:    sender,
		Subject: fmt.Sprintf("POLECAT_DONE %s", polecatName),
		Body:    strings.Join(bodyLines, "\n"),
	}

	fmt.Printf("\nNotifying Witness...\n")
	if err := townRouter.Send(doneNotification); err != nil {
		style.PrintWarning("could not notify witness: %v", err)
	} else {
		fmt.Printf("%s Witness notified of %s\n", style.Bold.Render("✓"), exitType)
	}

	// Notify witness of work completion (witness is the polecat's direct supervisor).
	// Previously this went to the dispatcher (often mayor), flooding mayor's inbox
	// with routine operational mail. The witness handles polecat lifecycle.
	if issueID != "" {
		workDoneNotification := &mail.Message{
			To:      witnessAddr,
			From:    sender,
			Subject: fmt.Sprintf("WORK_DONE: %s", issueID),
			Body:    strings.Join(bodyLines, "\n"),
		}
		if err := townRouter.Send(workDoneNotification); err != nil {
			style.PrintWarning("could not notify witness of work done: %v", err)
		} else {
			fmt.Printf("%s Witness notified of WORK_DONE for %s\n", style.Bold.Render("✓"), issueID)
		}
	}

	// Write witness notification checkpoint for resume (gt-aufru)
	if agentBeadID != "" {
		cpBd := beads.New(beads.ResolveBeadsDir(cwd))
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

	// Self-cleaning: Nuke our own sandbox and session (if we're a polecat)
	// This is the self-cleaning model - polecats clean up after themselves
	// "done means gone" - both worktree and session are terminated
	selfCleanAttempted := false
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil && roleInfo.Role == RolePolecat {
		selfCleanAttempted = true

		// Step 1: Nuke the worktree (only for COMPLETED with successful push)
		// If push failed, preserve the worktree so Witness/Refinery can still
		// access the branch for recovery. selfNukePolecat also checks
		// branch-on-remote, so this is defense-in-depth.
		if exitType == ExitCompleted && !pushFailed {
			if err := selfNukePolecat(roleInfo, townRoot); err != nil {
				// Non-fatal: Witness will clean up if we fail
				style.PrintWarning("worktree nuke failed: %v (Witness will clean up)", err)
			} else {
				fmt.Printf("%s Worktree nuked\n", style.Bold.Render("✓"))
				// Restore a valid cwd after worktree deletion. Without this,
				// subsequent exec.Command calls (tmux, kill) fail with
				// "ENOENT: posix_spawn '/bin/sh'" because the child process
				// inherits the deleted cwd.
				_ = os.Chdir("/")
			}
		}

		// Step 2: Kill our own session (this terminates Claude and the shell)
		// This is the last thing we do - the process will be killed when tmux session dies
		// All exit types kill the session - "done means gone"
		fmt.Printf("%s Terminating session (done means gone)\n", style.Bold.Render("→"))
		if err := selfKillSession(townRoot, roleInfo); err != nil {
			// If session kill fails, fall through to normal exit
			style.PrintWarning("session kill failed: %v", err)
		} else {
			sessionKilled = true // Prevent deferred kill from double-killing
		}
		// If selfKillSession succeeds, we won't reach here (process killed by tmux)
	}

	// Fallback exit for non-polecats or if self-clean failed
	fmt.Println()
	fmt.Printf("%s Session exiting\n", style.Bold.Render("→"))
	if !selfCleanAttempted {
		fmt.Printf("  Witness will handle cleanup.\n")
	}
	fmt.Printf("  Goodbye!\n")
	return NewSilentExit(0)
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
	CheckpointDoltMerged      DoneCheckpoint = "dolt-merged"
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

// updateAgentStateOnDone clears the agent's hook and reports cleanup status.
// Per gt-zecmc: observable states ("done", "idle") removed - use tmux to discover.
// Non-observable states ("stuck", "awaiting-gate") are still set since they represent
// intentional agent decisions that can't be observed from tmux.
//
// Also self-reports cleanup_status for ZFC compliance (#10).
//
// BUG FIX (hq-3xaxy): This function must be resilient to working directory deletion.
// If the polecat's worktree is deleted before gt done finishes, we use env vars as fallback.
// All errors are warnings, not failures - gt done must complete even if bead ops fail.
func updateAgentStateOnDone(cwd, townRoot, exitType, _ string) { // issueID unused but kept for future audit logging
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

	// Use rig path for slot commands - bd slot doesn't route from town root
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

	// BUG FIX (gt-vwjz6): Close hooked beads before clearing the hook.
	// Previously, the agent's hook_bead slot was cleared but the hooked bead itself
	// stayed status=hooked forever. Now we close the hooked bead before clearing.
	//
	// BUG FIX (hq-i26n2): Check if agent bead exists before clearing hook.
	// Old polecats may not have identity beads, so ClearHookBead would fail.
	// gt done must be resilient - missing agent bead is not an error.
	//
	// BUG FIX (hq-3xaxy): All bead operations are non-fatal. If the agent bead
	// is deleted by another process (e.g., Witness cleanup), we just warn.
	agentBead, err := bd.Show(agentBeadID)
	if err != nil {
		// Agent bead doesn't exist - nothing to clear, that's fine
		// This happens for polecats created before identity beads existed,
		// or if the agent bead was deleted by another process
		style.PrintWarning("agent bead %s not found, skipping state update: %v", agentBeadID, err)
		return
	}

	if agentBead.HookBead != "" {
		hookedBeadID := agentBead.HookBead
		// Only close if the hooked bead exists and is still in "hooked" status
		if hookedBead, err := bd.Show(hookedBeadID); err == nil && hookedBead.Status == beads.StatusHooked {
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

			if err := bd.Close(hookedBeadID); err != nil {
				// Non-fatal: warn but continue
				fmt.Fprintf(os.Stderr, "Warning: couldn't close hooked bead %s: %v\n", hookedBeadID, err)
			}
		}
	}

	// Clear the hook (work is done) - gt-zecmc
	// BUG FIX (hq-3xaxy): This is non-fatal - if hook clearing fails, warn and continue.
	// The Witness will clean up any orphaned state.
	if err := bd.ClearHookBead(agentBeadID); err != nil {
		// Non-fatal: warn but don't fail gt done
		fmt.Fprintf(os.Stderr, "Warning: couldn't clear agent %s hook: %v\n", agentBeadID, err)
	}

	// Only set non-observable states - "stuck" and "awaiting-gate" are intentional
	// agent decisions that can't be discovered from tmux. Skip "done" and "idle"
	// since those are observable (no session = done, session + no hook = idle).
	switch exitType {
	case ExitEscalated:
		// "stuck" = agent is requesting help - not observable from tmux
		if _, err := bd.Run("agent", "state", agentBeadID, "stuck"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s to stuck: %v\n", agentBeadID, err)
		}
	// ExitCompleted and ExitDeferred don't set state - observable from tmux
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

// getIssueFromAgentHook retrieves the issue ID from an agent's hook_bead field.
// This is the authoritative source for what work a polecat is doing, since branch
// names may not contain the issue ID (e.g., "polecat/furiosa-mkb0vq9f").
// Returns empty string if agent doesn't exist or has no hook.
func getIssueFromAgentHook(bd *beads.Beads, agentBeadID string) string {
	if agentBeadID == "" {
		return ""
	}
	agentBead, err := bd.Show(agentBeadID)
	if err != nil {
		return ""
	}
	return agentBead.HookBead
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

// selfNukePolecat deletes this polecat's worktree (self-cleaning model).
// Called by polecats when they complete work via `gt done`.
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

// onMainBranch temporarily clears BD_BRANCH so beads operations target
// Dolt's main branch instead of the polecat's working branch.
// Returns a restore function that must be called after the operation.
//
// Use this for operations that need to read/write beads visible to all agents
// (e.g., source issue lookup, MR bead creation) rather than the polecat's
// isolated Dolt branch.
func onMainBranch() func() {
	saved := os.Getenv("BD_BRANCH")
	os.Unsetenv("BD_BRANCH")
	return func() {
		if saved != "" {
			os.Setenv("BD_BRANCH", saved)
		}
	}
}

// selfKillSession terminates the polecat's own tmux session after logging the event.
// This completes the self-cleaning model: "done means gone" - both worktree and session.
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
		_ = logger.Log(townlog.EventKill, agentID, "self-clean: done means gone")
	}

	// Log to events (JSON audit log with structured payload)
	_ = events.LogFeed(events.TypeSessionDeath, agentID,
		events.SessionDeathPayload(sessionName, agentID, "self-clean: done means gone", "gt done"))

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
