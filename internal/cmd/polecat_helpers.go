package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

// polecatTarget represents a polecat to operate on.
type polecatTarget struct {
	rigName     string
	polecatName string
	mgr         *polecat.Manager
	r           *rig.Rig
}

// resolvePolecatTargets builds a list of polecats from command args.
// If useAll is true, the first arg is treated as a rig name and all polecats in it are returned.
// Otherwise, args are parsed as rig/polecat addresses.
func resolvePolecatTargets(args []string, useAll bool) ([]polecatTarget, error) {
	var targets []polecatTarget

	if useAll {
		// --all flag: first arg is just the rig name
		rigName := args[0]
		// Check if it looks like rig/polecat format
		if _, _, err := parseAddress(rigName); err == nil {
			return nil, fmt.Errorf("with --all, provide just the rig name (e.g., 'gt polecat <cmd> %s --all')", strings.Split(rigName, "/")[0])
		}

		mgr, r, err := getPolecatManager(rigName)
		if err != nil {
			return nil, err
		}

		polecats, err := mgr.List()
		if err != nil {
			return nil, fmt.Errorf("listing polecats: %w", err)
		}

		for _, p := range polecats {
			targets = append(targets, polecatTarget{
				rigName:     rigName,
				polecatName: p.Name,
				mgr:         mgr,
				r:           r,
			})
		}
	} else {
		// Multiple rig/polecat arguments - require explicit rig/polecat format
		for _, arg := range args {
			// Validate format: must contain "/" to avoid misinterpreting rig names as polecat names
			if !strings.Contains(arg, "/") {
				return nil, fmt.Errorf("invalid address '%s': must be in 'rig/polecat' format (e.g., 'gastown/Toast')", arg)
			}

			rigName, polecatName, err := parseAddress(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid address '%s': %w", arg, err)
			}

			mgr, r, err := getPolecatManager(rigName)
			if err != nil {
				return nil, err
			}

			targets = append(targets, polecatTarget{
				rigName:     rigName,
				polecatName: polecatName,
				mgr:         mgr,
				r:           r,
			})
		}
	}

	return targets, nil
}

// SafetyCheckResult holds the result of safety checks for a polecat.
type SafetyCheckResult struct {
	Polecat       string
	Blocked       bool
	Reasons       []string
	CleanupStatus polecat.CleanupStatus
	HookBead      string
	HookStale     bool // true if hooked bead is closed
	OpenMR        string
	GitState      *GitState
}

// checkPolecatSafety performs safety checks before destructive operations.
// Returns nil if the polecat is safe to operate on, or a SafetyCheckResult with reasons if blocked.
func checkPolecatSafety(target polecatTarget) *SafetyCheckResult {
	result := &SafetyCheckResult{
		Polecat: fmt.Sprintf("%s/%s", target.rigName, target.polecatName),
	}

	// Get polecat info for branch name
	polecatInfo, infoErr := target.mgr.Get(target.polecatName)

	// Check 1: Unpushed commits via cleanup_status or git state
	bd := beads.New(target.r.Path)
	agentBeadID := polecatBeadIDForRig(target.r, target.rigName, target.polecatName)
	agentIssue, fields, err := bd.GetAgentBead(agentBeadID)

	if err != nil || fields == nil {
		// No agent bead - fall back to git check
		if infoErr == nil && polecatInfo != nil {
			gitState, gitErr := getGitState(polecatInfo.ClonePath)
			result.GitState = gitState
			if gitErr != nil {
				result.Reasons = append(result.Reasons, "cannot check git state")
			} else if !gitState.Clean {
				if gitState.UnpushedCommits > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d unpushed commit(s)", gitState.UnpushedCommits))
				} else if len(gitState.UncommittedFiles) > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d uncommitted file(s)", len(gitState.UncommittedFiles)))
				} else if gitState.StashCount > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d stash(es)", gitState.StashCount))
				}
			}
		}
	} else {
		// Check cleanup_status from agent bead
		result.CleanupStatus = polecat.CleanupStatus(fields.CleanupStatus)
		switch result.CleanupStatus {
		case polecat.CleanupClean:
			// OK
		case polecat.CleanupUnpushed:
			result.Reasons = append(result.Reasons, "has unpushed commits")
		case polecat.CleanupUncommitted:
			result.Reasons = append(result.Reasons, "has uncommitted changes")
		case polecat.CleanupStash:
			result.Reasons = append(result.Reasons, "has stashed changes")
		case polecat.CleanupUnknown, "":
			result.Reasons = append(result.Reasons, "cleanup status unknown")
		default:
			result.Reasons = append(result.Reasons, fmt.Sprintf("cleanup status: %s", result.CleanupStatus))
		}

		// Check 3: Work on hook
		hookBead := agentIssue.HookBead
		if hookBead == "" {
			hookBead = fields.HookBead
		}
		if hookBead != "" {
			result.HookBead = hookBead
			// Check if hooked bead is still active (not closed)
			hookedIssue, err := bd.Show(hookBead)
			if err == nil && hookedIssue != nil {
				if hookedIssue.Status != "closed" {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has work on hook (%s)", hookBead))
				} else {
					result.HookStale = true
				}
			} else {
				result.Reasons = append(result.Reasons, fmt.Sprintf("has work on hook (%s, unverified)", hookBead))
			}
		}
	}

	// Check 2: Open MR beads for this branch
	if infoErr == nil && polecatInfo != nil && polecatInfo.Branch != "" {
		mr, mrErr := bd.FindMRForBranch(polecatInfo.Branch)
		if mrErr == nil && mr != nil {
			result.OpenMR = mr.ID
			result.Reasons = append(result.Reasons, fmt.Sprintf("has open MR (%s)", mr.ID))
		}
	}

	result.Blocked = len(result.Reasons) > 0
	return result
}

func rigPrefix(r *rig.Rig) string {
	townRoot := filepath.Dir(r.Path)
	return beads.GetPrefixForRig(townRoot, r.Name)
}

func polecatBeadIDForRig(r *rig.Rig, rigName, polecatName string) string {
	return beads.PolecatBeadIDWithPrefix(rigPrefix(r), rigName, polecatName)
}

// displaySafetyCheckBlocked prints blocked polecats and guidance.
func displaySafetyCheckBlocked(blocked []*SafetyCheckResult) {
	fmt.Printf("%s Cannot nuke the following polecats:\n\n", style.Error.Render("Error:"))
	var polecatList []string
	for _, b := range blocked {
		fmt.Printf("  %s:\n", style.Bold.Render(b.Polecat))
		for _, r := range b.Reasons {
			fmt.Printf("    - %s\n", r)
		}
		polecatList = append(polecatList, b.Polecat)
	}
	fmt.Println()
	fmt.Println("Safety checks failed. Resolve issues before nuking, or use --force.")
	fmt.Println("Options:")
	fmt.Printf("  1. Complete work: gt done (from polecat session)\n")
	fmt.Printf("  2. Push changes: git push (from polecat worktree)\n")
	fmt.Printf("  3. Escalate: gt mail send mayor/ -s \"RECOVERY_NEEDED\" -m \"...\"\n")
	fmt.Printf("  4. Force nuke (LOSES WORK): gt polecat nuke --force %s\n", strings.Join(polecatList, " "))
	fmt.Println()
}

// displayDryRunSafetyCheck shows safety check status for dry-run mode.
func displayDryRunSafetyCheck(target polecatTarget) {
	fmt.Printf("\n  Safety checks:\n")
	polecatInfo, infoErr := target.mgr.Get(target.polecatName)
	bd := beads.New(target.r.Path)
	agentBeadID := polecatBeadIDForRig(target.r, target.rigName, target.polecatName)
	agentIssue, fields, err := bd.GetAgentBead(agentBeadID)

	// Check 1: Git state
	if err != nil || fields == nil {
		if infoErr == nil && polecatInfo != nil {
			gitState, gitErr := getGitState(polecatInfo.ClonePath)
			if gitErr != nil {
				fmt.Printf("    - Git state: %s\n", style.Warning.Render("cannot check"))
			} else if gitState.Clean {
				fmt.Printf("    - Git state: %s\n", style.Success.Render("clean"))
			} else {
				fmt.Printf("    - Git state: %s\n", style.Error.Render("dirty"))
			}
		} else {
			fmt.Printf("    - Git state: %s\n", style.Dim.Render("unknown (no polecat info)"))
		}
		fmt.Printf("    - Hook: %s\n", style.Dim.Render("unknown (no agent bead)"))
	} else {
		cleanupStatus := polecat.CleanupStatus(fields.CleanupStatus)
		if cleanupStatus.IsSafe() {
			fmt.Printf("    - Git state: %s\n", style.Success.Render("clean"))
		} else if cleanupStatus.RequiresRecovery() {
			fmt.Printf("    - Git state: %s (%s)\n", style.Error.Render("dirty"), cleanupStatus)
		} else {
			fmt.Printf("    - Git state: %s\n", style.Warning.Render("unknown"))
		}

		hookBead := agentIssue.HookBead
		if hookBead == "" {
			hookBead = fields.HookBead
		}
		if hookBead != "" {
			hookedIssue, err := bd.Show(hookBead)
			if err == nil && hookedIssue != nil && hookedIssue.Status == "closed" {
				fmt.Printf("    - Hook: %s (%s, closed - stale)\n", style.Warning.Render("stale"), hookBead)
			} else {
				fmt.Printf("    - Hook: %s (%s)\n", style.Error.Render("has work"), hookBead)
			}
		} else {
			fmt.Printf("    - Hook: %s\n", style.Success.Render("empty"))
		}
	}

	// Check 2: Open MR
	if infoErr == nil && polecatInfo != nil && polecatInfo.Branch != "" {
		mr, mrErr := bd.FindMRForBranch(polecatInfo.Branch)
		if mrErr == nil && mr != nil {
			fmt.Printf("    - Open MR: %s (%s)\n", style.Error.Render("yes"), mr.ID)
		} else {
			fmt.Printf("    - Open MR: %s\n", style.Success.Render("none"))
		}
	} else {
		fmt.Printf("    - Open MR: %s\n", style.Dim.Render("unknown (no branch info)"))
	}
}
