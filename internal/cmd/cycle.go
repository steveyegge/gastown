package cmd

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/factory"
)

func init() {
	rootCmd.AddCommand(cycleCmd)
	cycleCmd.AddCommand(cycleNextCmd)
	cycleCmd.AddCommand(cyclePrevCmd)
}

var cycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "Cycle between sessions in the same group",
	Long: `Cycle between related tmux sessions based on the current session type.

Session groups:
- Town sessions: Mayor ↔ Deacon
- Crew sessions: All crew members in the same rig (e.g., greenplace/crew/max ↔ greenplace/crew/joe)
- Rig infra sessions: Witness ↔ Refinery (per rig)
- Polecat sessions: All polecats in the same rig (e.g., greenplace/Toast ↔ greenplace/Nux)

The appropriate cycling is detected automatically from the agent's environment.`,
}

var cycleNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Switch to next session in group",
	Long: `Switch to the next session in the current group.

This command is typically invoked via the C-b n keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleToSession(1)
	},
}

var cyclePrevCmd = &cobra.Command{
	Use:   "prev",
	Short: "Switch to previous session in group",
	Long: `Switch to the previous session in the current group.

This command is typically invoked via the C-b p keybinding. It automatically
detects whether you're in a town-level session (Mayor/Deacon) or a crew session
and cycles within the appropriate group.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cycleToSession(-1)
	},
}

// cycleToSession dispatches to the appropriate cycling function based on agent role.
// direction: 1 for next, -1 for previous
func cycleToSession(direction int) error {
	// Get current agent identity from environment
	currentID, err := agent.Self()
	if err != nil {
		return nil // Not in a GT session, nothing to do
	}

	agents := factory.Agents()

	// Get all running agents
	allIDs, err := agents.List()
	if err != nil {
		return nil
	}

	role, rig, _ := currentID.Parse()

	// Find siblings based on role
	var siblings []agent.AgentID
	switch role {
	case constants.RoleMayor, constants.RoleDeacon:
		// Town-level: cycle between mayor and deacon
		siblings = filterByRoles(allIDs, constants.RoleMayor, constants.RoleDeacon)
	case constants.RoleCrew:
		// Crew: cycle between all crew in same rig
		siblings = filterByRoleAndRig(allIDs, constants.RoleCrew, rig)
	case constants.RoleWitness, constants.RoleRefinery:
		// Rig infra: cycle between witness and refinery in same rig
		siblings = filterByRolesAndRig(allIDs, rig, constants.RoleWitness, constants.RoleRefinery)
	case constants.RolePolecat:
		// Polecat: cycle between all polecats in same rig
		siblings = filterByRoleAndRig(allIDs, constants.RolePolecat, rig)
	default:
		return nil
	}

	if len(siblings) <= 1 {
		return nil // Nothing to cycle to
	}

	// Sort for consistent ordering
	sortAgentIDs(siblings)

	// Find current position
	currentIdx := -1
	for i, id := range siblings {
		if id == currentID {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return nil // Current agent not in siblings list
	}

	// Calculate target index (with wrapping)
	targetIdx := (currentIdx + direction + len(siblings)) % len(siblings)

	if targetIdx == currentIdx {
		return nil // Only one session
	}

	// Switch to target session
	return agents.Attach(siblings[targetIdx])
}

// filterByRoles returns agents matching any of the given roles.
func filterByRoles(ids []agent.AgentID, roles ...string) []agent.AgentID {
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	var result []agent.AgentID
	for _, id := range ids {
		role, _, _ := id.Parse()
		if roleSet[role] {
			result = append(result, id)
		}
	}
	return result
}

// filterByRoleAndRig returns agents matching role and rig.
func filterByRoleAndRig(ids []agent.AgentID, role, rig string) []agent.AgentID {
	var result []agent.AgentID
	for _, id := range ids {
		r, g, _ := id.Parse()
		if r == role && g == rig {
			result = append(result, id)
		}
	}
	return result
}

// filterByRolesAndRig returns agents matching any of the roles in the given rig.
func filterByRolesAndRig(ids []agent.AgentID, rig string, roles ...string) []agent.AgentID {
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	var result []agent.AgentID
	for _, id := range ids {
		r, g, _ := id.Parse()
		if roleSet[r] && g == rig {
			result = append(result, id)
		}
	}
	return result
}

// sortAgentIDs sorts agent IDs by their string representation.
func sortAgentIDs(ids []agent.AgentID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}
