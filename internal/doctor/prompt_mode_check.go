package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// PromptModeCheck verifies that agent configurations have the correct
// prompt_mode for their command type:
//   - pir/pi agents need prompt_mode="none" (they exit with positional args;
//     gt delivers the beacon via NudgeSession/tmux send-keys)
//   - claude/omp/opencode agents need prompt_mode="arg" (positional arg
//     starts an interactive session)
type PromptModeCheck struct {
	BaseCheck
}

// NewPromptModeCheck creates a new prompt mode check.
func NewPromptModeCheck() *PromptModeCheck {
	return &PromptModeCheck{
		BaseCheck: BaseCheck{
			CheckName:        "prompt-mode",
			CheckDescription: "Check agent prompt_mode matches command type",
			CheckCategory:    CategoryConfig,
		},
	}
}

// needsNudgeMode returns true for TUI agents that exit when given a
// positional arg. These need prompt_mode="none" so gt uses NudgeSession.
func needsNudgeMode(command string) bool {
	switch command {
	case "pir", "pi":
		return true
	default:
		return false
	}
}

// Run checks all agent definitions and role assignments for prompt_mode issues.
func (c *PromptModeCheck) Run(ctx *CheckContext) *CheckResult {
	townSettings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(ctx.TownRoot))
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("Could not load town settings: %v", err),
		}
	}

	var problems []string
	var checked int

	// Check all agent definitions in config.json
	for name, rc := range townSettings.Agents {
		if rc == nil {
			continue
		}
		checked++
		if problem := checkPromptMode(rc.Command, rc.PromptMode); problem != "" {
			problems = append(problems, fmt.Sprintf("agent %q: command=%s %s", name, rc.Command, problem))
		}
	}

	// Check role_agents -> resolved agent for each role
	roles := []string{"boot", "deacon", "mayor", "dog", "witness", "refinery", "polecat", "crew"}
	for _, role := range roles {
		agentName := ""
		if townSettings.RoleAgents != nil {
			agentName = townSettings.RoleAgents[role]
		}
		if agentName == "" {
			agentName = townSettings.DefaultAgent
		}
		if agentName == "" {
			continue
		}

		rc := townSettings.Agents[agentName]
		if rc == nil {
			if preset := config.GetAgentPresetByName(agentName); preset != nil {
				presetRC := config.RuntimeConfigFromPreset(config.AgentPreset(agentName))
				if problem := checkPromptMode(presetRC.Command, presetRC.PromptMode); problem != "" {
					problems = append(problems, fmt.Sprintf(
						"role %q -> preset %q: command=%s %s", role, agentName, presetRC.Command, problem))
				}
			}
			continue
		}

		checked++
		if problem := checkPromptMode(rc.Command, rc.PromptMode); problem != "" {
			problems = append(problems, fmt.Sprintf(
				"role %q -> agent %q: command=%s %s", role, agentName, rc.Command, problem))
		}
	}

	if len(problems) > 0 {
		return &CheckResult{
			Name:   c.Name(),
			Status: StatusError,
			Message: fmt.Sprintf(
				"%d prompt_mode issue(s):\n  %s\n\nFix: pir/pi agents need \"none\" (NudgeSession); claude/omp need \"arg\"",
				len(problems), strings.Join(problems, "\n  ")),
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("All %d agent configs have correct prompt_mode", checked),
	}
}

// checkPromptMode returns an error message if prompt_mode is wrong for the command.
func checkPromptMode(command, promptMode string) string {
	mode := promptMode
	if mode == "" {
		mode = "(unset)"
	}

	if needsNudgeMode(command) {
		if promptMode == "arg" {
			return fmt.Sprintf("prompt_mode=%s — pir exits with positional args (needs \"none\")", mode)
		}
	} else if command != "" {
		if promptMode == "none" || promptMode == "" {
			return fmt.Sprintf("prompt_mode=%s — beacon may not be delivered (needs \"arg\")", mode)
		}
	}
	return ""
}
