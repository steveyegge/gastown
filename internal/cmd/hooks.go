package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/hooks"
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

var hooksReportErrorCmd = &cobra.Command{
	Use:   "report-error",
	Short: "Report a hook error (for use in hook commands)",
	Long: `Report a hook error with deduplication.

This command is designed to be used in hook commands instead of "|| true"
to capture errors while still allowing the hook to succeed.

Errors are deduplicated within a 60-second window to prevent spam.
Use "gt hooks errors" to view recent errors.

Examples:
  # In a hook command (replaces || true)
  some-command || gt hooks report-error --type SessionStart --command "some-command" --exit-code $?

  # Report with stderr capture
  output=$(some-command 2>&1) || gt hooks report-error --type SessionStart --command "some-command" --exit-code $? --stderr "$output"`,
	RunE: runHooksReportError,
}

var hooksErrorsCmd = &cobra.Command{
	Use:   "errors",
	Short: "List recent hook errors",
	Long: `List recent hook errors logged by hooks.

Shows errors that were reported via "gt hooks report-error".
Errors are deduplicated, so the count shows how many times
the same error occurred within the deduplication window.

Examples:
  gt hooks errors           # Show last 20 errors
  gt hooks errors --limit 50 # Show last 50 errors
  gt hooks errors --json    # JSON output
  gt hooks errors --clear   # Clear all errors`,
	RunE: runHooksErrors,
}

var (
	// report-error flags
	reportHookType  string
	reportCommand   string
	reportExitCode  int
	reportStderr    string
	reportRole      string

	// errors flags
	errorsLimit int
	errorsJSON  bool
	errorsClear bool
)

func init() {
	rootCmd.AddCommand(hooksCmd)
	hooksCmd.Flags().BoolVar(&hooksJSON, "json", false, "Output as JSON")
	hooksCmd.Flags().BoolVarP(&hooksVerbose, "verbose", "v", false, "Show hook commands")

	// report-error subcommand
	hooksReportErrorCmd.Flags().StringVar(&reportHookType, "type", "", "Hook type (SessionStart, UserPromptSubmit, etc.)")
	hooksReportErrorCmd.Flags().StringVar(&reportCommand, "command", "", "The command that failed")
	hooksReportErrorCmd.Flags().IntVar(&reportExitCode, "exit-code", 1, "Exit code of the failed command")
	hooksReportErrorCmd.Flags().StringVar(&reportStderr, "stderr", "", "Standard error output")
	hooksReportErrorCmd.Flags().StringVar(&reportRole, "role", "", "Gas Town role (auto-detected if not specified)")
	_ = hooksReportErrorCmd.MarkFlagRequired("type")
	_ = hooksReportErrorCmd.MarkFlagRequired("command")
	hooksCmd.AddCommand(hooksReportErrorCmd)

	// errors subcommand
	hooksErrorsCmd.Flags().IntVar(&errorsLimit, "limit", 20, "Maximum number of errors to show")
	hooksErrorsCmd.Flags().BoolVar(&errorsJSON, "json", false, "Output as JSON")
	hooksErrorsCmd.Flags().BoolVar(&errorsClear, "clear", false, "Clear all logged errors")
	hooksCmd.AddCommand(hooksErrorsCmd)
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

		// Polecats-level hooks (inherited by all polecats)
		polecatsDir := filepath.Join(rigPath, "polecats")
		locations = append(locations, struct {
			path  string
			agent string
		}{filepath.Join(polecatsDir, ".claude", "settings.json"), fmt.Sprintf("%s/polecats", rigName)})

		// Individual polecat hooks
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

		// Crew-level hooks (inherited by all crew members)
		crewDir := filepath.Join(rigPath, "crew")
		locations = append(locations, struct {
			path  string
			agent string
		}{filepath.Join(crewDir, ".claude", "settings.json"), fmt.Sprintf("%s/crew", rigName)})

		// Individual crew member hooks
		if crew, err := os.ReadDir(crewDir); err == nil {
			for _, c := range crew {
				if c.IsDir() && !strings.HasPrefix(c.Name(), ".") {
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

func runHooksReportError(cmd *cobra.Command, args []string) error {
	// Auto-detect role if not specified
	role := reportRole
	if role == "" {
		role = detectSender()
	}
	if role == "" {
		role = "unknown"
	}

	logged, err := hooks.ReportHookError(reportHookType, reportCommand, reportExitCode, reportStderr, role)
	if err != nil {
		// Don't fail the hook, just log to stderr
		fmt.Fprintf(os.Stderr, "Warning: failed to log hook error: %v\n", err)
		return nil
	}

	if logged {
		fmt.Fprintf(os.Stderr, "Hook error logged: %s [%s] exit %d\n", reportHookType, reportCommand, reportExitCode)
	}
	// Else: deduplicated, no output

	return nil
}

func runHooksErrors(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	log := hooks.NewErrorLog(townRoot)

	if errorsClear {
		if err := log.ClearErrors(); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clearing errors: %w", err)
		}
		fmt.Println("Hook errors cleared")
		return nil
	}

	errors, err := log.GetRecentErrors(errorsLimit)
	if err != nil {
		return fmt.Errorf("getting errors: %w", err)
	}

	if errorsJSON {
		data, _ := json.MarshalIndent(errors, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(errors) == 0 {
		fmt.Println(style.Dim.Render("No hook errors logged"))
		return nil
	}

	fmt.Printf("\n%s Recent Hook Errors\n\n", style.Bold.Render("⚠️"))

	for _, e := range errors {
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		age := formatHookErrorAge(ts)

		countStr := ""
		if e.Count > 1 {
			countStr = fmt.Sprintf(" (x%d)", e.Count)
		}

		fmt.Printf("  %s %s [exit %d]%s\n", style.Bold.Render(e.HookType), age, e.ExitCode, countStr)
		fmt.Printf("    %s %s\n", style.Dim.Render("Command:"), truncateCommand(e.Command, 60))
		fmt.Printf("    %s %s\n", style.Dim.Render("Role:"), e.Role)
		if e.Stderr != "" {
			fmt.Printf("    %s %s\n", style.Dim.Render("Stderr:"), truncateCommand(e.Stderr, 60))
		}
		fmt.Println()
	}

	fmt.Printf("%s %d error(s) shown\n", style.Dim.Render("Total:"), len(errors))
	fmt.Printf("%s gt hooks errors --clear\n", style.Dim.Render("Clear with:"))

	return nil
}

func formatHookErrorAge(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncateCommand(s string, maxLen int) string {
	// Remove newlines
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
