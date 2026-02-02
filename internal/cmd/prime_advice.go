package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// AdviceBead represents an advice issue from beads.
type AdviceBead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels,omitempty"`
}

// outputAdviceContext queries and outputs advice applicable to this agent.
// Delegates all subscription matching to beads via `bd advice list --for=<agent-id>`.
// The beads CLI implements the subscription model where agents auto-subscribe to:
//   - global
//   - agent:<their-id>
//   - rig:<their-rig>
//   - role:<their-role>
//
// Beads handles all filtering including rig-scoping (ensures rig:X advice only
// shows to agents in rig X). See docs/design/advice-subscription-model-v2.md.
func outputAdviceContext(ctx RoleInfo) {
	// Build agent identity for subscription matching
	agentID := buildAgentID(ctx)
	if agentID == "" {
		explain(false, "Advice: could not build agent ID")
		return
	}

	// Query advice using beads subscription model
	adviceBeads, err := queryAdviceForAgent(agentID)
	if err != nil {
		// Silently skip if bd isn't available or query fails
		explain(false, fmt.Sprintf("Advice query failed: %v", err))
		return
	}

	if len(adviceBeads) == 0 {
		return
	}

	explain(true, fmt.Sprintf("Advice: %d beads matched subscriptions for %s", len(adviceBeads), agentID))

	// Output advice section
	fmt.Println()
	fmt.Println("## 📝 Agent Advice")
	fmt.Println()
	for _, advice := range adviceBeads {
		// Show scope indicator
		scope := getAdviceScope(advice)
		fmt.Printf("**[%s]** %s\n", scope, advice.Title)
		if advice.Description != "" {
			// Indent description for readability
			lines := strings.Split(advice.Description, "\n")
			for _, line := range lines {
				fmt.Printf("  %s\n", line)
			}
		}
		fmt.Println()
	}
}

// buildAgentID constructs the full agent identifier from role context.
// Format: <rig>/<role-type>/<name> e.g., "gastown/polecats/alpha" or "gastown/crew/decision_notify"
// Town-level roles (Mayor, Deacon) return simple identifiers without rig prefix.
func buildAgentID(ctx RoleInfo) string {
	switch ctx.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RolePolecat:
		if ctx.Rig != "" && ctx.Polecat != "" {
			return fmt.Sprintf("%s/polecats/%s", ctx.Rig, ctx.Polecat)
		}
	case RoleCrew:
		// Note: Crew name is also stored in ctx.Polecat field
		if ctx.Rig != "" && ctx.Polecat != "" {
			return fmt.Sprintf("%s/crew/%s", ctx.Rig, ctx.Polecat)
		}
	case RoleWitness:
		if ctx.Rig != "" {
			return fmt.Sprintf("%s/witness", ctx.Rig)
		}
	case RoleRefinery:
		if ctx.Rig != "" {
			return fmt.Sprintf("%s/refinery", ctx.Rig)
		}
	}

	return ""
}

// queryAdviceForAgent fetches advice beads matching the agent's subscriptions.
// Uses `bd advice list --for=<agent-id> --json` which:
//   - Auto-subscribes to: global, agent:<id>, rig:<rig>, role:<role>
//   - Handles rig-scoping (rig:X advice only matches agents subscribed to rig:X)
//   - Returns labels in JSON output (fixed in beads commit 794e5326)
//
// Beads handles all filtering internally, so we trust the returned results.
func queryAdviceForAgent(agentID string) ([]AdviceBead, error) {
	cmd := exec.Command("bd", "advice", "list", "--for="+agentID, "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd advice list --for=%s: %w", agentID, err)
	}

	// Handle empty result
	if len(output) == 0 || strings.TrimSpace(string(output)) == "[]" {
		return nil, nil
	}

	var beads []AdviceBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing advice: %w", err)
	}

	return beads, nil
}

// getAdviceScope returns a human-readable scope indicator for the advice.
func getAdviceScope(bead AdviceBead) string {
	for _, label := range bead.Labels {
		switch {
		case strings.HasPrefix(label, "agent:"):
			return "Agent"
		case strings.HasPrefix(label, "role:"):
			role := strings.TrimPrefix(label, "role:")
			// Capitalize first letter
			if len(role) > 0 {
				return strings.ToUpper(role[:1]) + role[1:]
			}
			return role
		case strings.HasPrefix(label, "rig:"):
			return strings.TrimPrefix(label, "rig:")
		}
	}
	return "Global"
}
