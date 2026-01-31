package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	fixDryRun     bool
	fixSettingsPath string
)

var hooksFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix hook output issues that cause 'hook error' display",
	Long: `Fix hooks that output plain text instead of JSON.

Claude Code expects hooks to either:
1. Output nothing (silent success)
2. Output valid JSON

When hooks output plain text on exit 0, Claude Code's JSON parser fails
and shows 'hook error' in the UI even though the hooks work correctly.

This command detects and fixes such hooks by appending output redirection
to suppress their text output.

Common offenders:
  - claude-flow hooks (npx @claude-flow/cli@latest hooks ...)
  - Other third-party hooks that log info messages

Examples:
  gt hooks fix                        # Fix hooks in ~/.claude/settings.json
  gt hooks fix --settings /path/to/settings.json
  gt hooks fix --dry-run              # Preview changes without modifying`,
	RunE: runHooksFix,
}

func init() {
	hooksCmd.AddCommand(hooksFixCmd)
	hooksFixCmd.Flags().BoolVar(&fixDryRun, "dry-run", false, "Preview changes without modifying files")
	hooksFixCmd.Flags().StringVar(&fixSettingsPath, "settings", "", "Path to settings.json (default: ~/.claude/settings.json)")
}

// ExtendedClaudeSettings includes all fields we need to preserve
type ExtendedClaudeSettings struct {
	EditorMode     string                           `json:"editorMode,omitempty"`
	EnabledPlugins map[string]bool                  `json:"enabledPlugins,omitempty"`
	Hooks          map[string][]ExtendedHookMatcher `json:"hooks,omitempty"`
	StatusLine     json.RawMessage                  `json:"statusLine,omitempty"`
	Permissions    json.RawMessage                  `json:"permissions,omitempty"`
	Attribution    json.RawMessage                  `json:"attribution,omitempty"`
	ClaudeFlow     json.RawMessage                  `json:"claudeFlow,omitempty"`
	// Catch-all for other fields
	Extra map[string]json.RawMessage `json:"-"`
}

// ExtendedHookMatcher includes all hook matcher fields
type ExtendedHookMatcher struct {
	Matcher string              `json:"matcher"`
	Hooks   []ExtendedClaudeHook `json:"hooks"`
}

// ExtendedClaudeHook includes all hook fields
type ExtendedClaudeHook struct {
	Type            string `json:"type"`
	Command         string `json:"command,omitempty"`
	Timeout         int    `json:"timeout,omitempty"`
	ContinueOnError bool   `json:"continueOnError,omitempty"`
}

// Patterns that indicate hooks with text output that need fixing
var problematicPatterns = []*regexp.Regexp{
	// claude-flow hooks that output [INFO] messages
	regexp.MustCompile(`npx\s+@claude-flow/cli[@\w]*\s+hooks\s+(pre-command|post-command|pre-edit|post-edit|pre-task|post-task)`),
}

// outputRedirection patterns to detect already-fixed hooks
var outputRedirectionPattern = regexp.MustCompile(`>\s*/dev/null\s*2>&1\s*$|2>&1\s*>\s*/dev/null\s*$`)

func runHooksFix(cmd *cobra.Command, args []string) error {
	// Determine settings path
	settingsPath := fixSettingsPath
	if settingsPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		settingsPath = filepath.Join(homeDir, ".claude", "settings.json")
	}

	// Check if file exists
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return fmt.Errorf("settings file not found: %s", settingsPath)
	}

	// Read current settings
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}

	// Parse settings - use map to preserve unknown fields
	var rawSettings map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawSettings); err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	// Parse hooks separately
	var hooks map[string][]ExtendedHookMatcher
	if hooksRaw, ok := rawSettings["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
			return fmt.Errorf("parsing hooks: %w", err)
		}
	}

	if hooks == nil {
		fmt.Println(style.Dim.Render("No hooks found in settings"))
		return nil
	}

	// Scan and fix hooks
	fixes := 0
	skipped := 0

	fmt.Printf("\n%s Scanning hooks in %s\n\n", style.Bold.Render("🔍"), settingsPath)

	for hookType, matchers := range hooks {
		for i, matcher := range matchers {
			for j, hook := range matcher.Hooks {
				if hook.Command == "" {
					continue
				}

				// Check if this hook matches problematic patterns
				needsFix := false
				for _, pattern := range problematicPatterns {
					if pattern.MatchString(hook.Command) {
						needsFix = true
						break
					}
				}

				if !needsFix {
					continue
				}

				// Check if already fixed
				if outputRedirectionPattern.MatchString(hook.Command) {
					skipped++
					if fixDryRun {
						fmt.Printf("  %s %s:%s - already fixed\n",
							style.Dim.Render("○"), hookType, matcher.Matcher)
					}
					continue
				}

				// Apply fix
				fixes++
				originalCmd := hook.Command
				fixedCmd := hook.Command + " > /dev/null 2>&1"

				if fixDryRun {
					fmt.Printf("  %s %s:%s\n", style.Warning.Render("●"), hookType, matcher.Matcher)
					fmt.Printf("    %s %s\n", style.Dim.Render("before:"), truncateCommand(originalCmd, 70))
					fmt.Printf("    %s %s\n", style.Dim.Render("after: "), truncateCommand(fixedCmd, 70))
				} else {
					hooks[hookType][i].Hooks[j].Command = fixedCmd
					fmt.Printf("  %s %s:%s - fixed\n", style.Success.Render("✓"), hookType, matcher.Matcher)
				}
			}
		}
	}

	if fixes == 0 && skipped == 0 {
		fmt.Println(style.Dim.Render("No problematic hooks found"))
		return nil
	}

	fmt.Println()

	if fixDryRun {
		fmt.Printf("%s Would fix %d hook(s), %d already fixed\n",
			style.Dim.Render("Dry run:"), fixes, skipped)
		fmt.Println(style.Dim.Render("Run without --dry-run to apply changes"))
		return nil
	}

	if fixes > 0 {
		// Marshal hooks back
		hooksData, err := json.Marshal(hooks)
		if err != nil {
			return fmt.Errorf("marshaling hooks: %w", err)
		}
		rawSettings["hooks"] = hooksData

		// Write back with formatting
		output, err := json.MarshalIndent(rawSettings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling settings: %w", err)
		}

		if err := os.WriteFile(settingsPath, output, 0600); err != nil {
			return fmt.Errorf("writing settings: %w", err)
		}

		fmt.Printf("%s Fixed %d hook(s), %d already fixed\n",
			style.Success.Render("Done:"), fixes, skipped)
	} else {
		fmt.Printf("%s All %d problematic hook(s) already fixed\n",
			style.Success.Render("Done:"), skipped)
	}

	return nil
}

// HookOutputFixer provides a utility for fixing hook output issues
type HookOutputFixer struct {
	settingsPath string
}

// NewHookOutputFixer creates a new fixer for the given settings path
func NewHookOutputFixer(settingsPath string) *HookOutputFixer {
	return &HookOutputFixer{settingsPath: settingsPath}
}

// NeedsOutputFix checks if a command likely outputs text that needs suppression
func NeedsOutputFix(command string) bool {
	// Already has output redirection
	if outputRedirectionPattern.MatchString(command) {
		return false
	}

	// Check against known problematic patterns
	for _, pattern := range problematicPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}

	return false
}

// FixCommand adds output redirection to suppress text output
func FixCommand(command string) string {
	if !NeedsOutputFix(command) {
		return command
	}
	return strings.TrimSpace(command) + " > /dev/null 2>&1"
}
