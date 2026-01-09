package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	hooksJSON    bool
	hooksVerbose bool
)

var hooksCmd = &cobra.Command{
	Use:     "hooks",
	GroupID: GroupConfig,
	Short:   "List all Claude Code hooks in the workspace",
	Long: `List all Claude Code hooks configured in the workspace.

Scans for .claude/settings.json files and displays hooks by type.

Hook types:
  SessionStart     - Runs when Claude session starts
  PreCompact       - Runs before context compaction
  UserPromptSubmit - Runs before user prompt is submitted
  PreToolUse       - Runs before tool execution
  PostToolUse      - Runs after tool execution
  Stop             - Runs when Claude session stops

Examples:
  gt hooks              # List all hooks in workspace
  gt hooks --verbose    # Show hook commands
  gt hooks --json       # Output as JSON`,
	RunE: runHooks,
}

func init() {
	rootCmd.AddCommand(hooksCmd)
	hooksCmd.Flags().BoolVar(&hooksJSON, "json", false, "Output as JSON")
	hooksCmd.Flags().BoolVarP(&hooksVerbose, "verbose", "v", false, "Show hook commands")
}

// ClaudeSettings represents the Claude Code settings.json structure.
type ClaudeSettings struct {
	EnabledPlugins map[string]bool                  `json:"enabledPlugins,omitempty"`
	Hooks          map[string][]ClaudeHookMatcher   `json:"hooks,omitempty"`
}

// ClaudeHookMatcher represents a hook matcher entry.
type ClaudeHookMatcher struct {
	Matcher string       `json:"matcher"`
	Hooks   []ClaudeHook `json:"hooks"`
}

// ClaudeHook represents an individual hook.
type ClaudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
}

// HookInfo contains information about a discovered hook.
type HookInfo struct {
	Type     string   `json:"type"`     // Hook type (SessionStart, etc.)
	Location string   `json:"location"` // Path to the settings file
	Agent    string   `json:"agent"`    // Agent that owns this hook (e.g., "polecat/nux")
	Matcher  string   `json:"matcher"`  // Pattern matcher (empty = all)
	Commands []string `json:"commands"` // Hook commands
	Status   string   `json:"status"`   // "active" or "disabled"
}

// HooksOutput is the JSON output structure.
type HooksOutput struct {
	TownRoot string     `json:"town_root"`
	Hooks    []HookInfo `json:"hooks"`
	Count    int        `json:"count"`
}

func runHooks(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find all .claude/settings.json files
	hooks, err := discoverHooks(townRoot)
	if err != nil {
		return fmt.Errorf("discovering hooks: %w", err)
	}

	if hooksJSON {
		return outputHooksJSON(townRoot, hooks)
	}

	return outputHooksHuman(townRoot, hooks)
}

// discoverHooks finds all Claude Code hooks in the workspace.
func discoverHooks(townRoot string) ([]HookInfo, error) {
	var hooks []HookInfo

	// Scan known locations for .claude/settings.json
	// NOTE: Mayor settings are at ~/gt/mayor/.claude/, NOT ~/gt/.claude/
	// Settings at town root would pollute all child workspaces.
	locations := []struct {
		path  string
		agent string
	}{
		{filepath.Join(townRoot, "mayor", ".claude", "settings.json"), "mayor/"},
		{filepath.Join(townRoot, "deacon", ".claude", "settings.json"), "deacon/"},
	}

	// Scan rigs
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "mayor" || entry.Name() == ".beads" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Rig-level hooks
		locations = append(locations, struct {
			path  string
			agent string
		}{filepath.Join(rigPath, ".claude", "settings.json"), fmt.Sprintf("%s/rig", rigName)})

		// Polecats
		polecatsDir := filepath.Join(rigPath, "polecats")
		if polecats, err := os.ReadDir(polecatsDir); err == nil {
			for _, p := range polecats {
				if p.IsDir() && !strings.HasPrefix(p.Name(), ".") {
					locations = append(locations, struct {
						path  string
						agent string
					}{filepath.Join(polecatsDir, p.Name(), ".claude", "settings.json"), fmt.Sprintf("%s/%s", rigName, p.Name())})
				}
			}
		}

		// Crew members
		crewDir := filepath.Join(rigPath, "crew")
		if crew, err := os.ReadDir(crewDir); err == nil {
			for _, c := range crew {
				if c.IsDir() {
					locations = append(locations, struct {
						path  string
						agent string
					}{filepath.Join(crewDir, c.Name(), ".claude", "settings.json"), fmt.Sprintf("%s/crew/%s", rigName, c.Name())})
				}
			}
		}

		// Witness
		witnessPath := filepath.Join(rigPath, "witness", ".claude", "settings.json")
		locations = append(locations, struct {
			path  string
			agent string
		}{witnessPath, fmt.Sprintf("%s/witness", rigName)})

		// Refinery
		refineryPath := filepath.Join(rigPath, "refinery", ".claude", "settings.json")
		locations = append(locations, struct {
			path  string
			agent string
		}{refineryPath, fmt.Sprintf("%s/refinery", rigName)})
	}

	// Process each location
	for _, loc := range locations {
		if _, err := os.Stat(loc.path); os.IsNotExist(err) {
			continue
		}

		found, err := parseHooksFile(loc.path, loc.agent)
		if err != nil {
			// Skip files that can't be parsed
			continue
		}
		hooks = append(hooks, found...)
	}

	return hooks, nil
}

// parseHooksFile parses a .claude/settings.json file and extracts hooks.
func parseHooksFile(path, agent string) ([]HookInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	var hooks []HookInfo

	for hookType, matchers := range settings.Hooks {
		for _, matcher := range matchers {
			var commands []string
			for _, h := range matcher.Hooks {
				if h.Command != "" {
					commands = append(commands, h.Command)
				}
			}

			if len(commands) > 0 {
				hooks = append(hooks, HookInfo{
					Type:     hookType,
					Location: path,
					Agent:    agent,
					Matcher:  matcher.Matcher,
					Commands: commands,
					Status:   "active",
				})
			}
		}
	}

	return hooks, nil
}

func outputHooksJSON(townRoot string, hooks []HookInfo) error {
	output := HooksOutput{
		TownRoot: townRoot,
		Hooks:    hooks,
		Count:    len(hooks),
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func outputHooksHuman(townRoot string, hooks []HookInfo) error {
	if len(hooks) == 0 {
		fmt.Println(style.Dim.Render("No Claude Code hooks found in workspace"))
		return nil
	}

	fmt.Printf("\n%s Claude Code Hooks\n", style.Bold.Render("🪝"))
	fmt.Printf("Town root: %s\n\n", style.Dim.Render(townRoot))

	// Group by hook type
	byType := make(map[string][]HookInfo)
	typeOrder := []string{"SessionStart", "PreCompact", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}

	for _, h := range hooks {
		byType[h.Type] = append(byType[h.Type], h)
	}

	// Add any types not in the predefined order
	for t := range byType {
		found := false
		for _, o := range typeOrder {
			if t == o {
				found = true
				break
			}
		}
		if !found {
			typeOrder = append(typeOrder, t)
		}
	}

	for _, hookType := range typeOrder {
		typeHooks := byType[hookType]
		if len(typeHooks) == 0 {
			continue
		}

		fmt.Printf("%s %s\n", style.Bold.Render("▸"), hookType)

		for _, h := range typeHooks {
			statusIcon := "●"
			if h.Status != "active" {
				statusIcon = "○"
			}

			matcherStr := ""
			if h.Matcher != "" {
				matcherStr = fmt.Sprintf(" [%s]", h.Matcher)
			}

			fmt.Printf("  %s %-25s%s\n", statusIcon, h.Agent, style.Dim.Render(matcherStr))

			if hooksVerbose {
				for _, cmd := range h.Commands {
					fmt.Printf("    %s %s\n", style.Dim.Render("→"), cmd)
				}
			}
		}
		fmt.Println()
	}

	fmt.Printf("%s %d hooks found\n", style.Dim.Render("Total:"), len(hooks))

	return nil
}
