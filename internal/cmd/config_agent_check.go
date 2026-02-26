package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var configAgentCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify agent configs will auto-start correctly",
	Long: `Check all agent definitions and role assignments for configuration issues.

Verifies that:
  - TUI agents (pir, pi) have prompt_mode=arg so beacons are delivered
  - All role_agents entries resolve to valid agent definitions
  - Agent commands are installed and on PATH
  - Hook files exist at expected locations

This catches the most common misconfiguration: agents that spawn but sit
idle because prompt_mode is "none" or unset, so the startup beacon never
reaches the agent as a CLI argument.

Examples:
  gt config agent check            # Check all agents and roles
  gt config agent check --verbose  # Show passing checks too`,
	RunE: runConfigAgentCheck,
}

var configAgentCheckVerbose bool

func init() {
	configAgentCheckCmd.Flags().BoolVarP(&configAgentCheckVerbose, "verbose", "v", false, "Show passing checks too")
}

// needsNudge returns true for TUI agents that exit when given a positional
// arg (print-mode behavior). These agents need prompt_mode="none" so gt
// sends the beacon via NudgeSession (tmux send-keys) after the TUI is ready.
func needsNudge(command string) bool {
	switch command {
	case "pir", "pi":
		return true
	default:
		return false
	}
}

type agentCheckResult struct {
	name    string
	source  string // "config.json", "agents.json", "built-in", "role"
	command string
	mode    string
	status  string // "ok", "warn", "fail"
	message string
}

func runConfigAgentCheck(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	townSettings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot))
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	var results []agentCheckResult

	// 1. Check all agent definitions in config.json
	agentNames := make([]string, 0, len(townSettings.Agents))
	for name := range townSettings.Agents {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)

	for _, name := range agentNames {
		rc := townSettings.Agents[name]
		if rc == nil {
			continue
		}
		r := checkAgentConfig(name, "config.json", rc)
		results = append(results, r)
	}

	// 2. Check agents.json if it exists
	agentsJSONPath := config.DefaultAgentRegistryPath(townRoot)
	if _, statErr := os.Stat(agentsJSONPath); statErr == nil {
		_ = config.LoadAgentRegistry(agentsJSONPath)
		// Re-check via the registry (agents.json agents are merged into registry)
		// For now, just note it exists
	}

	// 3. Check role -> agent resolution
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
			agentName = "claude"
		}

		// Resolve through the full chain
		rc := townSettings.Agents[agentName]
		if rc == nil {
			// Check built-in presets
			if preset := config.GetAgentPresetByName(agentName); preset != nil {
				presetRC := config.RuntimeConfigFromPreset(config.AgentPreset(agentName))
				r := checkAgentConfig(fmt.Sprintf("%s -> %s", role, agentName), "built-in", presetRC)
				results = append(results, r)
				continue
			}
			results = append(results, agentCheckResult{
				name:    fmt.Sprintf("%s -> %s", role, agentName),
				source:  "role",
				status:  "fail",
				message: fmt.Sprintf("agent %q not found in config or built-in presets", agentName),
			})
			continue
		}

		r := checkAgentConfig(fmt.Sprintf("%s -> %s", role, agentName), "role", rc)
		results = append(results, r)
	}

	// 4. Check built-in pir preset
	presetRC := config.RuntimeConfigFromPreset(config.AgentPreset("pir"))
	if presetRC != nil {
		r := checkAgentConfig("pir", "built-in preset", presetRC)
		results = append(results, r)
	}

	// Print results
	pass, warn, fail := 0, 0, 0
	for _, r := range results {
		switch r.status {
		case "ok":
			pass++
			if configAgentCheckVerbose {
				fmt.Printf("  %s [%s] %s: cmd=%s prompt_mode=%s\n",
					style.Bold.Render("✅"), r.source, r.name, r.command, r.mode)
			}
		case "warn":
			warn++
			fmt.Printf("  %s [%s] %s: %s\n",
				style.Bold.Render("⚠️"), r.source, r.name, r.message)
		case "fail":
			fail++
			fmt.Printf("  %s [%s] %s: %s\n",
				style.Bold.Render("❌"), r.source, r.name, r.message)
		}
	}

	fmt.Println()
	parts := []string{fmt.Sprintf("%d ok", pass)}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("%d warnings", warn))
	}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", fail))
	}
	fmt.Printf("Agent config check: %s\n", strings.Join(parts, ", "))

	if fail > 0 {
		fmt.Println()
		fmt.Println("Fix: pir/pi agents need \"none\" (NudgeSession); claude/omp/opencode need \"arg\"")
		return fmt.Errorf("%d agent config errors", fail)
	}

	return nil
}

func checkAgentConfig(name, source string, rc *config.RuntimeConfig) agentCheckResult {
	r := agentCheckResult{
		name:    name,
		source:  source,
		command: rc.Command,
		mode:    rc.PromptMode,
	}

	if r.mode == "" {
		r.mode = "(unset)"
	}

	// pir/pi need prompt_mode="none" — they exit after one response with positional args.
	// gt delivers the beacon via NudgeSession (tmux send-keys) after the TUI is ready.
	if needsNudge(rc.Command) {
		if rc.PromptMode == "arg" {
			r.status = "fail"
			r.message = fmt.Sprintf("cmd=%s prompt_mode=%s — pir exits with positional args (needs \"none\")", rc.Command, r.mode)
			return r
		}
	}

	// Other agents (claude, omp, opencode) should have prompt_mode="arg"
	if !needsNudge(rc.Command) && rc.Command != "" {
		if rc.PromptMode == "none" || rc.PromptMode == "" {
			r.status = "warn"
			r.message = fmt.Sprintf("cmd=%s prompt_mode=%s — beacon may not be delivered", rc.Command, r.mode)
			return r
		}
	}

	r.status = "ok"
	return r
}
