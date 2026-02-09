package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	hooksScanJSON     bool
	hooksScanVerbose  bool
	hooksScanProvider string
)

var hooksScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan workspace for existing hooks",
	Long: `Scan for hooks configuration files and display hooks by type.

Supports both Claude Code (.claude/settings.json) and OpenCode (.opencode/plugins/gastown.js).

Hook types (Claude):
  SessionStart     - Runs when Claude session starts
  PreCompact       - Runs before context compaction
  UserPromptSubmit - Runs before user prompt is submitted
  PreToolUse       - Runs before tool execution
  PostToolUse      - Runs after tool execution
  Stop             - Runs when Claude session stops

For OpenCode, shows plugin configuration and transform hooks.

Examples:
  gt hooks scan                   # List all hooks in workspace
  gt hooks scan --provider claude # Scan only Claude hooks
  gt hooks scan --provider opencode # Scan only OpenCode plugins
  gt hooks scan --verbose         # Show hook commands
  gt hooks scan --json            # Output as JSON`,
	RunE: runHooksScan,
}

func init() {
	hooksCmd.AddCommand(hooksScanCmd)
	hooksScanCmd.Flags().BoolVar(&hooksScanJSON, "json", false, "Output as JSON")
	hooksScanCmd.Flags().BoolVarP(&hooksScanVerbose, "verbose", "v", false, "Show hook commands")
	hooksScanCmd.Flags().StringVar(&hooksScanProvider, "provider", "", "Provider to scan (claude, opencode, or empty for both)")
}

// HookInfo contains information about a discovered hook.
type HookInfo struct {
	Type     string   `json:"type"`     // Hook type (SessionStart, etc.)
	Location string   `json:"location"` // Path to the settings file
	Agent    string   `json:"agent"`    // Agent that owns this hook (e.g., "polecat/nux")
	Matcher  string   `json:"matcher"`  // Pattern matcher (empty = all)
	Commands []string `json:"commands"` // Hook commands
	Status   string   `json:"status"`   // "active" or "disabled"
	Provider string   `json:"provider"` // "claude" or "opencode"
}

// HooksOutput is the JSON output structure.
type HooksOutput struct {
	TownRoot string     `json:"town_root"`
	Hooks    []HookInfo `json:"hooks"`
	Count    int        `json:"count"`
}

func runHooksScan(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Validate provider
	provider := strings.ToLower(hooksScanProvider)
	if provider != "" && provider != "claude" && provider != "opencode" {
		return fmt.Errorf("invalid provider %q: must be 'claude', 'opencode', or empty", hooksScanProvider)
	}

	var allHooks []HookInfo

	// Scan Claude hooks if provider is empty or "claude"
	if provider == "" || provider == "claude" {
		claudeHooks, err := discoverClaudeHooks(townRoot)
		if err != nil {
			return fmt.Errorf("discovering Claude hooks: %w", err)
		}
		allHooks = append(allHooks, claudeHooks...)
	}

	// Scan OpenCode plugins if provider is empty or "opencode"
	if provider == "" || provider == "opencode" {
		opencodeHooks, err := discoverOpenCodePlugins(townRoot)
		if err != nil {
			return fmt.Errorf("discovering OpenCode plugins: %w", err)
		}
		allHooks = append(allHooks, opencodeHooks...)
	}

	if hooksScanJSON {
		return outputHooksJSON(townRoot, allHooks)
	}

	return outputHooksHuman(townRoot, allHooks)
}

// discoverClaudeHooks finds all Claude Code hooks in the workspace.
func discoverClaudeHooks(townRoot string) ([]HookInfo, error) {
	targets, err := hooks.DiscoverTargets(townRoot)
	if err != nil {
		return nil, err
	}

	var infos []HookInfo

	for _, target := range targets {
		settings, err := hooks.LoadSettings(target.Path)
		if err != nil {
			continue // Skip files that can't be parsed
		}

		found := extractClaudeHookInfos(settings, target.Path, target.DisplayKey())
		infos = append(infos, found...)
	}

	return infos, nil
}

// extractClaudeHookInfos extracts HookInfo entries from a loaded settings file.
func extractClaudeHookInfos(settings *hooks.SettingsJSON, path, agent string) []HookInfo {
	var infos []HookInfo

	for _, eventType := range hooks.EventTypes {
		entries := settings.Hooks.GetEntries(eventType)
		for _, entry := range entries {
			var commands []string
			for _, h := range entry.Hooks {
				if h.Command != "" {
					commands = append(commands, h.Command)
				}
			}

			if len(commands) > 0 {
				infos = append(infos, HookInfo{
					Type:     eventType,
					Location: path,
					Agent:    agent,
					Matcher:  entry.Matcher,
					Commands: commands,
					Status:   "active",
					Provider: "claude",
				})
			}
		}
	}

	return infos
}

// discoverOpenCodePlugins finds all OpenCode plugins in the workspace.
func discoverOpenCodePlugins(townRoot string) ([]HookInfo, error) {
	var infos []HookInfo

	// Scan for .opencode directories
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	// Check town-level agents
	for _, agent := range []string{"mayor", "deacon"} {
		pluginPath := filepath.Join(townRoot, agent, ".opencode", "plugins", "gastown.js")
		if info, err := os.Stat(pluginPath); err == nil && !info.IsDir() {
			infos = append(infos, HookInfo{
				Type:     "Plugin",
				Location: pluginPath,
				Agent:    agent,
				Matcher:  "",
				Commands: []string{"gastown.js OpenCode plugin"},
				Status:   "active",
				Provider: "opencode",
			})
		}
	}

	// Scan rigs
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "mayor" || entry.Name() == "deacon" ||
			entry.Name() == ".beads" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip directories that aren't rigs
		if !isRigForScan(rigPath) {
			continue
		}

		// Check rig-level
		rigPluginPath := filepath.Join(rigPath, ".opencode", "plugins", "gastown.js")
		if info, err := os.Stat(rigPluginPath); err == nil && !info.IsDir() {
			infos = append(infos, HookInfo{
				Type:     "Plugin",
				Location: rigPluginPath,
				Agent:    rigName + "/rig",
				Matcher:  "",
				Commands: []string{"gastown.js OpenCode plugin"},
				Status:   "active",
				Provider: "opencode",
			})
		}

		// Check crew members
		crewDir := filepath.Join(rigPath, "crew")
		if crewEntries, err := os.ReadDir(crewDir); err == nil {
			for _, crew := range crewEntries {
				if crew.IsDir() && !strings.HasPrefix(crew.Name(), ".") {
					crewPluginPath := filepath.Join(crewDir, crew.Name(), ".opencode", "plugins", "gastown.js")
					if info, err := os.Stat(crewPluginPath); err == nil && !info.IsDir() {
						infos = append(infos, HookInfo{
							Type:     "Plugin",
							Location: crewPluginPath,
							Agent:    rigName + "/crew/" + crew.Name(),
							Matcher:  "",
							Commands: []string{"gastown.js OpenCode plugin"},
							Status:   "active",
							Provider: "opencode",
						})
					}
				}
			}
		}

		// Check polecats
		polecatsDir := filepath.Join(rigPath, "polecats")
		if polecatEntries, err := os.ReadDir(polecatsDir); err == nil {
			for _, polecat := range polecatEntries {
				if polecat.IsDir() && !strings.HasPrefix(polecat.Name(), ".") {
					polecatPluginPath := filepath.Join(polecatsDir, polecat.Name(), ".opencode", "plugins", "gastown.js")
					if info, err := os.Stat(polecatPluginPath); err == nil && !info.IsDir() {
						infos = append(infos, HookInfo{
							Type:     "Plugin",
							Location: polecatPluginPath,
							Agent:    rigName + "/polecat/" + polecat.Name(),
							Matcher:  "",
							Commands: []string{"gastown.js OpenCode plugin"},
							Status:   "active",
							Provider: "opencode",
						})
					}
				}
			}
		}

		// Check witness
		witnessDir := filepath.Join(rigPath, "witness")
		witnessPluginPath := filepath.Join(witnessDir, ".opencode", "plugins", "gastown.js")
		if info, err := os.Stat(witnessPluginPath); err == nil && !info.IsDir() {
			infos = append(infos, HookInfo{
				Type:     "Plugin",
				Location: witnessPluginPath,
				Agent:    rigName + "/witness",
				Matcher:  "",
				Commands: []string{"gastown.js OpenCode plugin"},
				Status:   "active",
				Provider: "opencode",
			})
		}

		// Check refinery
		refineryDir := filepath.Join(rigPath, "refinery")
		refineryPluginPath := filepath.Join(refineryDir, ".opencode", "plugins", "gastown.js")
		if info, err := os.Stat(refineryPluginPath); err == nil && !info.IsDir() {
			infos = append(infos, HookInfo{
				Type:     "Plugin",
				Location: refineryPluginPath,
				Agent:    rigName + "/refinery",
				Matcher:  "",
				Commands: []string{"gastown.js OpenCode plugin"},
				Status:   "active",
				Provider: "opencode",
			})
		}
	}

	return infos, nil
}

// isRigForScan checks if a directory looks like a rig.
func isRigForScan(path string) bool {
	for _, sub := range []string{"crew", "witness", "polecats", "refinery"} {
		info, err := os.Stat(filepath.Join(path, sub))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// parseHooksFile is a backward compatibility alias for loading and parsing Claude hooks.
// It loads a settings.json file and extracts hook information.
// Deprecated: Use hooks.LoadSettings and extractClaudeHookInfos instead.
func parseHooksFile(path, agent string) ([]HookInfo, error) {
	settings, err := hooks.LoadSettings(path)
	if err != nil {
		return nil, err
	}
	return extractClaudeHookInfos(settings, path, agent), nil
}

// discoverHooks is a backward compatibility alias for discovering Claude hooks.
// It discovers all Claude hooks in the workspace using hooks.DiscoverTargets.
// Deprecated: Use discoverClaudeHooks instead.
func discoverHooks(townRoot string) ([]HookInfo, error) {
	return discoverClaudeHooks(townRoot)
}

func outputHooksJSON(townRoot string, hookInfos []HookInfo) error {
	output := HooksOutput{
		TownRoot: townRoot,
		Hooks:    hookInfos,
		Count:    len(hookInfos),
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func outputHooksHuman(townRoot string, hookInfos []HookInfo) error {
	if len(hookInfos) == 0 {
		fmt.Println(style.Dim.Render("No hooks found in workspace"))
		return nil
	}

	// Separate by provider
	var claudeHooks, opencodeHooks []HookInfo
	for _, h := range hookInfos {
		if h.Provider == "opencode" {
			opencodeHooks = append(opencodeHooks, h)
		} else {
			claudeHooks = append(claudeHooks, h)
		}
	}

	fmt.Printf("\n%s Hooks in Workspace\n", style.Bold.Render("🪝"))
	fmt.Printf("Town root: %s\n\n", style.Dim.Render(townRoot))

	// Display Claude hooks
	if len(claudeHooks) > 0 {
		fmt.Printf("%s Claude Code Hooks\n", style.Bold.Render("▸"))

		// Group by hook type
		byType := make(map[string][]HookInfo)
		for _, h := range claudeHooks {
			byType[h.Type] = append(byType[h.Type], h)
		}

		// Use canonical event type order, plus any extras
		typeOrder := make([]string, len(hooks.EventTypes))
		copy(typeOrder, hooks.EventTypes)
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

			fmt.Printf("  %s %s\n", style.Bold.Render("•"), hookType)

			for _, h := range typeHooks {
				statusIcon := "●"
				if h.Status != "active" {
					statusIcon = "○"
				}

				matcherStr := ""
				if h.Matcher != "" {
					matcherStr = fmt.Sprintf(" [%s]", h.Matcher)
				}

				fmt.Printf("    %s %-25s%s\n", statusIcon, h.Agent, style.Dim.Render(matcherStr))

				if hooksScanVerbose {
					for _, cmd := range h.Commands {
						fmt.Printf("      %s %s\n", style.Dim.Render("→"), cmd)
					}
				}
			}
		}
		fmt.Println()
	}

	// Display OpenCode plugins
	if len(opencodeHooks) > 0 {
		fmt.Printf("%s OpenCode Plugins\n", style.Bold.Render("▸"))

		for _, h := range opencodeHooks {
			statusIcon := "●"
			if h.Status != "active" {
				statusIcon = "○"
			}

			fmt.Printf("  %s %-25s\n", statusIcon, h.Agent)

			if hooksScanVerbose {
				for _, cmd := range h.Commands {
					fmt.Printf("    %s %s\n", style.Dim.Render("→"), cmd)
				}
				fmt.Printf("    %s %s\n", style.Dim.Render("→"), style.Dim.Render(h.Location))
			}
		}
		fmt.Println()
	}

	// Summary
	claudeCount := len(claudeHooks)
	opencodeCount := len(opencodeHooks)

	if claudeCount > 0 && opencodeCount > 0 {
		fmt.Printf("%s %d hooks found (%d Claude, %d OpenCode)\n",
			style.Dim.Render("Total:"), len(hookInfos), claudeCount, opencodeCount)
	} else {
		fmt.Printf("%s %d hooks found\n", style.Dim.Render("Total:"), len(hookInfos))
	}

	return nil
}
