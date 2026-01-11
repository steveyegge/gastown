package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var dndCmd = &cobra.Command{
	Use:     "dnd [on|off|status]",
	GroupID: GroupComm,
	Short:   "Toggle Do Not Disturb mode for notifications",
	Long: `Control notification level for the current agent.

Do Not Disturb (DND) mode mutes non-critical notifications,
allowing you to focus on work without interruption.

Subcommands:
  on      Enable DND mode (mute notifications)
  off     Disable DND mode (resume normal notifications)
  status  Show current notification level

Without arguments, toggles DND mode.

Related: gt notify - for fine-grained notification level control`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDnd,
}

func init() {
	rootCmd.AddCommand(dndCmd)
}

func runDnd(cmd *cobra.Command, args []string) error {
	// Get current agent bead ID
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return fmt.Errorf("determining role: %w", err)
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
		return fmt.Errorf("could not determine agent bead ID for role %s", roleInfo.Role)
	}

	bd := beads.New(townRoot)

	// Get current level
	currentLevel, err := bd.GetAgentNotificationLevel(agentBeadID)
	if err != nil {
		// Agent bead might not exist yet - default to normal
		currentLevel = beads.NotifyNormal
	}

	// Determine action
	var action string
	if len(args) == 0 {
		// Toggle: if muted -> normal, else -> muted
		if currentLevel == beads.NotifyMuted {
			action = "off"
		} else {
			action = "on"
		}
	} else {
		action = args[0]
	}

	switch action {
	case "on":
		if err := bd.UpdateAgentNotificationLevel(agentBeadID, beads.NotifyMuted); err != nil {
			return fmt.Errorf("enabling DND: %w", err)
		}
		fmt.Printf("%s DND enabled - notifications muted\n", style.SuccessPrefix)
		fmt.Printf("  Run %s to resume notifications\n", style.Bold.Render("gt dnd off"))

	case "off":
		if err := bd.UpdateAgentNotificationLevel(agentBeadID, beads.NotifyNormal); err != nil {
			return fmt.Errorf("disabling DND: %w", err)
		}
		fmt.Printf("%s DND disabled - notifications resumed\n", style.SuccessPrefix)

	case "status":
		levelDisplay := currentLevel
		if levelDisplay == "" {
			levelDisplay = beads.NotifyNormal
		}

		icon := "🔔"
		description := "All important notifications"
		switch levelDisplay {
		case beads.NotifyVerbose:
			icon = "🔊"
			description = "All notifications (verbose)"
		case beads.NotifyMuted:
			icon = "🔕"
			description = "Notifications muted (DND)"
		}

		fmt.Printf("%s Notification level: %s\n", icon, style.Bold.Render(levelDisplay))
		fmt.Printf("  %s\n", style.Dim.Render(description))

	default:
		return fmt.Errorf("unknown action %q: use on, off, or status", action)
	}

	return nil
}
