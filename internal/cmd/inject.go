package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/inject"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	injectDrainJSON    bool
	injectDrainQuiet   bool
	injectDrainSession string
)

var injectCmd = &cobra.Command{
	Use:     "inject",
	GroupID: GroupConfig,
	Short:   "Injection queue management",
	Long: `Manage the Claude Code hook injection queue.

The injection queue solves API 400 concurrency errors that occur when multiple
hooks try to inject content simultaneously. Instead of writing to stdout directly,
hooks queue their content, and 'gt inject drain' outputs everything safely.

ARCHITECTURE:
  UserPromptSubmit hook → gt mail check --inject (queues content)
  PostToolUse hook      → gt inject drain (outputs queued content)

QUEUE LOCATION:
  .runtime/inject-queue/<session-id>.jsonl

This separation ensures that only one hook outputs content at a time,
avoiding the "content already present" API error.`,
	RunE: requireSubcommand,
}

var injectDrainCmd = &cobra.Command{
	Use:   "drain",
	Short: "Output and clear queued injection content",
	Long: `Drain the injection queue, outputting all queued content.

This command should be called from a PostToolUse or Stop hook to safely
output any content that was queued during UserPromptSubmit.

The output format wraps each entry's content as-is (typically system-reminder
tags), separated by blank lines if there are multiple entries.

Exit codes:
  0 - Content was drained (or queue empty with --quiet)
  1 - Queue empty (normal mode)

Examples:
  gt inject drain                    # Output and clear queue
  gt inject drain --quiet            # Silent if empty
  gt inject drain --session abc123   # Explicit session ID`,
	RunE:          runInjectDrain,
	SilenceUsage:  true, // Exit codes signal status, not errors
	SilenceErrors: true, // Suppress "Error: exit 1" message
}

var injectStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show injection queue status",
	Long: `Show the current injection queue status without draining.

Displays the number of queued entries and their types.`,
	RunE: runInjectStatus,
}

var injectClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the injection queue without outputting",
	Long: `Clear all queued content without outputting it.

Use this to reset the queue state, for example during debugging
or when recovering from errors.`,
	RunE: runInjectClear,
}

func init() {
	// Drain flags
	injectDrainCmd.Flags().BoolVar(&injectDrainJSON, "json", false, "Output as JSON")
	injectDrainCmd.Flags().BoolVarP(&injectDrainQuiet, "quiet", "q", false, "Exit 0 even if queue is empty")
	injectDrainCmd.Flags().StringVar(&injectDrainSession, "session", "", "Explicit session ID (default: auto-detect)")

	// Status flags
	injectStatusCmd.Flags().StringVar(&injectDrainSession, "session", "", "Explicit session ID")
	injectStatusCmd.Flags().BoolVar(&injectDrainJSON, "json", false, "Output as JSON")

	// Clear flags
	injectClearCmd.Flags().StringVar(&injectDrainSession, "session", "", "Explicit session ID")

	injectCmd.AddCommand(injectDrainCmd)
	injectCmd.AddCommand(injectStatusCmd)
	injectCmd.AddCommand(injectClearCmd)
	rootCmd.AddCommand(injectCmd)
}

func getInjectQueue() (*inject.Queue, error) {
	// Get session ID
	sessionID := injectDrainSession
	if sessionID == "" {
		sessionID = runtime.SessionIDFromEnv()
	}
	if sessionID == "" {
		return nil, fmt.Errorf("no session ID (set CLAUDE_SESSION_ID or use --session)")
	}

	// Find work directory (prefer workspace root, fall back to cwd)
	workDir, err := workspace.FindFromCwdOrError()
	if err != nil {
		// Try current directory
		workDir, _ = os.Getwd()
	}

	// Check for .runtime directory
	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		// Try parent directories
		for dir := workDir; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
			runtimeDir = filepath.Join(dir, constants.DirRuntime)
			if _, err := os.Stat(runtimeDir); err == nil {
				workDir = dir
				break
			}
		}
	}

	return inject.NewQueue(workDir, sessionID), nil
}

func runInjectDrain(cmd *cobra.Command, args []string) error {
	queue, err := getInjectQueue()
	if err != nil {
		if injectDrainQuiet {
			return nil
		}
		return err
	}

	entries, err := queue.Drain()
	if err != nil {
		if injectDrainQuiet {
			return nil
		}
		return fmt.Errorf("draining queue: %w", err)
	}

	if len(entries) == 0 {
		if injectDrainQuiet {
			return nil
		}
		return NewSilentExit(1)
	}

	// Output each entry's content
	for i, entry := range entries {
		if i > 0 {
			fmt.Println() // Blank line between entries
		}
		fmt.Print(entry.Content)
	}

	return nil
}

func runInjectStatus(cmd *cobra.Command, args []string) error {
	queue, err := getInjectQueue()
	if err != nil {
		return err
	}

	entries, err := queue.Peek()
	if err != nil {
		return fmt.Errorf("checking queue: %w", err)
	}

	if injectDrainJSON {
		// JSON output
		type statusJSON struct {
			Count   int      `json:"count"`
			Types   []string `json:"types"`
			Session string   `json:"session"`
		}
		var types []string
		for _, e := range entries {
			types = append(types, string(e.Type))
		}
		status := statusJSON{
			Count:   len(entries),
			Types:   types,
			Session: injectDrainSession,
		}
		return outputJSON(status)
	}

	if len(entries) == 0 {
		fmt.Println("Injection queue: empty")
		return nil
	}

	fmt.Printf("Injection queue: %d entries\n", len(entries))
	for i, e := range entries {
		fmt.Printf("  %d. [%s] %d bytes\n", i+1, e.Type, len(e.Content))
	}
	return nil
}

func runInjectClear(cmd *cobra.Command, args []string) error {
	queue, err := getInjectQueue()
	if err != nil {
		return err
	}

	if err := queue.Clear(); err != nil {
		return fmt.Errorf("clearing queue: %w", err)
	}

	fmt.Println("Injection queue cleared")
	return nil
}
