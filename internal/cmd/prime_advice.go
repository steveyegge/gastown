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
// Advice is matched using label-based targeting:
// 1. Agent-specific: label "agent:<id>" matches agent ID
// 2. Role-specific: label "role:<type>" matches role type
// 3. Rig-specific: label "rig:<name>" matches rig name
// 4. Global: label "global" or no targeting labels
func outputAdviceContext(ctx RoleInfo) {
	// Build agent identity for matching
	agentID := buildAgentID(ctx)
	roleType := string(ctx.Role)
	rigName := ctx.Rig

	// Query all advice beads
	adviceBeads, err := queryAdviceBeads()
	if err != nil {
		// Silently skip if bd isn't available or query fails
		explain(false, fmt.Sprintf("Advice query failed: %v", err))
		return
	}

	if len(adviceBeads) == 0 {
		return
	}

	// Filter advice that applies to this agent
	applicable := filterApplicableAdvice(adviceBeads, agentID, roleType, rigName)
	explain(len(applicable) > 0, fmt.Sprintf("Advice: %d of %d beads apply to %s", len(applicable), len(adviceBeads), agentID))
	if len(applicable) == 0 {
		return
	}

	// Output advice section
	fmt.Println()
	fmt.Println("## 📝 Agent Advice")
	fmt.Println()
	for _, advice := range applicable {
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

// queryAdviceBeads fetches all advice beads from the beads database.
func queryAdviceBeads() ([]AdviceBead, error) {
	cmd := exec.Command("bd", "list", "-t", "advice", "--json", "--limit", "100")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list advice: %w", err)
	}

	// Handle empty result
	if len(output) == 0 || strings.TrimSpace(string(output)) == "[]" {
		return nil, nil
	}

	var beads []AdviceBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing advice beads: %w", err)
	}

	return beads, nil
}

// filterApplicableAdvice returns advice beads that apply to this agent.
// Advice matches based on label targeting - see matchesAdvice for details.
func filterApplicableAdvice(beads []AdviceBead, agentID, roleType, rigName string) []AdviceBead {
	var result []AdviceBead

	for _, bead := range beads {
		if matchesAdvice(bead, agentID, roleType, rigName) {
			result = append(result, bead)
		}
	}

	return result
}

// matchesAdvice checks if an advice bead applies to the given agent context.
// Advice uses label-based targeting:
//   - agent:<id> matches specific agent
//   - role:<type> matches role type (polecat, crew, witness, etc.)
//   - rig:<name> matches all agents in a rig
//   - global or no targeting labels = applies to everyone
func matchesAdvice(bead AdviceBead, agentID, roleType, rigName string) bool {
	// Check for targeting labels
	hasAgentLabel := false
	hasRoleLabel := false
	hasRigLabel := false
	hasGlobalLabel := false

	for _, label := range bead.Labels {
		switch {
		case strings.HasPrefix(label, "agent:"):
			hasAgentLabel = true
			targetAgent := strings.TrimPrefix(label, "agent:")
			if targetAgent == agentID {
				return true
			}
		case strings.HasPrefix(label, "role:"):
			hasRoleLabel = true
			targetRole := strings.TrimPrefix(label, "role:")
			if targetRole == roleType {
				return true
			}
		case strings.HasPrefix(label, "rig:"):
			hasRigLabel = true
			targetRig := strings.TrimPrefix(label, "rig:")
			if targetRig == rigName {
				return true
			}
		case label == "global":
			hasGlobalLabel = true
		}
	}

	// If any targeting labels were present but didn't match, exclude
	if hasAgentLabel || hasRoleLabel || hasRigLabel {
		return false
	}

	// Global label or no targeting labels = applies to everyone
	return hasGlobalLabel || len(bead.Labels) == 0
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
