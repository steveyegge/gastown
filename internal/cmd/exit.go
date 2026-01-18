package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/townlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

var exitCmd = &cobra.Command{
	Use:     "exit",
	GroupID: GroupWork,
	Short:   "Exit session for non-code tasks",
	Long: `Exit the session cleanly for tasks that don't produce code changes.

This command is for polecats completing non-code work (QA, research, investigation,
documentation review, etc.) where there's no branch to submit to the merge queue.

For code tasks, use 'gt done' instead which handles MR submission.

What gt exit does:
1. Verifies no uncommitted changes (fails if there are - use gt done instead)
2. Clears the agent's hook if set
3. Closes any hooked beads
4. Notifies the Witness of clean exit
5. Terminates the session

Examples:
  gt exit                    # Exit cleanly after non-code work
  gt exit --force            # Exit even with uncommitted changes (warns but proceeds)
  gt exit --message "Done"   # Include exit message for Witness`,
	RunE: runExit,
}

var (
	exitForce   bool
	exitMessage string
)

func init() {
	exitCmd.Flags().BoolVar(&exitForce, "force", false, "Exit even if there are uncommitted changes (not recommended)")
	exitCmd.Flags().StringVarP(&exitMessage, "message", "m", "", "Optional message to include in exit notification")

	rootCmd.AddCommand(exitCmd)
}

func runExit(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, cwd, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cwdAvailable := cwd != ""
	if !cwdAvailable {
		style.PrintWarning("working directory unavailable, using fallback paths")
		if polecatPath := os.Getenv("GT_POLECAT_PATH"); polecatPath != "" {
			cwd = polecatPath
		}
	}

	// Find current rig
	rigName, _, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Check for uncommitted changes if cwd is available
	if cwdAvailable {
		g := git.NewGit(cwd)
		workStatus, err := g.CheckUncommittedWork()
		if err != nil {
			style.PrintWarning("could not check git status: %v", err)
		} else if workStatus.HasUncommittedChanges {
			if exitForce {
				style.PrintWarning("uncommitted changes exist - proceeding anyway due to --force")
				fmt.Printf("  %s\n", workStatus.String())
			} else {
				return fmt.Errorf("uncommitted changes detected - use 'gt done' for code tasks or 'gt exit --force' to exit anyway\nUncommitted: %s", workStatus.String())
			}
		}
	}

	// Get sender identity
	sender := detectSender()
	polecatName := ""
	if parts := strings.Split(sender, "/"); len(parts) >= 2 {
		polecatName = parts[len(parts)-1]
	}

	// Clear hook and close hooked beads
	clearAgentHook(cwd, townRoot)

	// Notify Witness about exit
	townRouter := mail.NewRouter(townRoot)
	witnessAddr := fmt.Sprintf("%s/witness", rigName)

	var bodyLines []string
	bodyLines = append(bodyLines, "Exit: CLEAN_EXIT")
	if exitMessage != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("Message: %s", exitMessage))
	}

	exitNotification := &mail.Message{
		To:      witnessAddr,
		From:    sender,
		Subject: fmt.Sprintf("POLECAT_EXIT %s", polecatName),
		Body:    strings.Join(bodyLines, "\n"),
	}

	fmt.Printf("Notifying Witness...\n")
	if err := townRouter.Send(exitNotification); err != nil {
		style.PrintWarning("could not notify witness: %v", err)
	} else {
		fmt.Printf("%s Witness notified of clean exit\n", style.Bold.Render("✓"))
	}

	// Log exit event
	_ = events.LogFeed(events.TypeDone, sender, events.DonePayload("", ""))

	// Self-cleaning for polecats
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil && roleInfo.Role == RolePolecat {
		// Log to townlog
		if townRoot != "" {
			logger := townlog.NewLogger(townRoot)
			agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
			_ = logger.Log(townlog.EventKill, agentID, "gt exit: clean termination")
		}

		// Kill session
		fmt.Printf("%s Terminating session\n", style.Bold.Render("→"))
		if err := selfKillExitSession(townRoot, roleInfo); err != nil {
			style.PrintWarning("session kill failed: %v", err)
		}
	}

	fmt.Println()
	fmt.Printf("%s Session exiting\n", style.Bold.Render("→"))
	fmt.Printf("  Goodbye!\n")
	os.Exit(0)

	return nil
}

// clearAgentHook clears the agent's hook and closes any hooked beads.
func clearAgentHook(cwd, townRoot string) {
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return
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
		return
	}

	// Use rig path for beads
	var beadsPath string
	switch ctx.Role {
	case RoleMayor, RoleDeacon:
		beadsPath = townRoot
	default:
		beadsPath = filepath.Join(townRoot, ctx.Rig)
	}
	bd := beads.New(beadsPath)

	// Get agent bead to check for hooked work
	agentBead, err := bd.Show(agentBeadID)
	if err != nil {
		return
	}

	// Close hooked bead if exists
	if agentBead.HookBead != "" {
		hookedBeadID := agentBead.HookBead
		if hookedBead, err := bd.Show(hookedBeadID); err == nil && hookedBead.Status == beads.StatusHooked {
			if err := bd.Close(hookedBeadID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: couldn't close hooked bead %s: %v\n", hookedBeadID, err)
			} else {
				fmt.Printf("%s Closed hooked bead %s\n", style.Bold.Render("✓"), hookedBeadID)
			}
		}
	}

	// Clear the hook
	if err := bd.ClearHookBead(agentBeadID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't clear agent hook: %v\n", err)
	} else {
		fmt.Printf("%s Hook cleared\n", style.Bold.Render("✓"))
	}
}

// selfKillExitSession terminates the polecat's own tmux session.
// Similar to selfKillSession in done.go but with different logging.
func selfKillExitSession(townRoot string, roleInfo RoleInfo) error {
	rigName := os.Getenv("GT_RIG")
	polecatName := os.Getenv("GT_POLECAT")

	if rigName == "" {
		rigName = roleInfo.Rig
	}
	if polecatName == "" {
		polecatName = roleInfo.Polecat
	}

	if rigName == "" || polecatName == "" {
		return fmt.Errorf("cannot determine session: rig=%q, polecat=%q", rigName, polecatName)
	}

	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)

	// Log session death event
	_ = events.LogFeed(events.TypeSessionDeath, agentID,
		events.SessionDeathPayload(sessionName, agentID, "gt exit: clean termination", "gt exit"))

	// Kill tmux session
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName) //nolint:gosec // sessionName from env vars
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("killing session %s: %w", sessionName, err)
	}

	return nil
}
