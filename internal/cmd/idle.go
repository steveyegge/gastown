// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

const (
	idleDir = "/tmp/gt-idle"
)

var (
	idleJSON bool
)

func init() {
	rootCmd.AddCommand(idleCmd)
	idleCmd.AddCommand(idleMarkCmd)
	idleCmd.AddCommand(idleClearCmd)
	idleCmd.AddCommand(idleMenuCmd)
	idleCmd.AddCommand(idleSetupCmd)

	idleCmd.Flags().BoolVar(&idleJSON, "json", false, "Output as JSON")
}

var idleCmd = &cobra.Command{
	Use:     "idle",
	GroupID: GroupDiag,
	Short:   "Manage idle worker status for session cycling",
	Long: `Track which Gas Town workers are idle (waiting for input) to enable
efficient cycling through sessions that need attention.

Workers mark themselves idle via Stop hooks and clear the status via
UserPromptSubmit hooks. Use 'gt idle menu' to interactively pick an
idle worker to switch to.

Subcommands:
  (none)    Show list of idle workers
  mark      Mark current session as idle (for Stop hooks)
  clear     Clear idle status (for UserPromptSubmit hooks)
  menu      Interactive picker for idle workers
  setup     Show Claude Code hook configuration

Examples:
  gt idle                  # List idle workers
  gt idle menu             # Interactive picker (bind to tmux key)
  gt idle mark             # Mark current session idle (use in Stop hook)
  gt idle clear            # Clear idle status (use in UserPromptSubmit hook)
  gt idle setup            # Show hook configuration`,
	Args: cobra.NoArgs,
	RunE: runIdleList,
}

var idleMarkCmd = &cobra.Command{
	Use:   "mark",
	Short: "Mark current session as idle",
	Long: `Mark the current tmux session as idle (waiting for input).

This command is designed to be called from a Claude Code Stop hook.
It creates a marker file that gt idle menu uses to find idle workers.

The marker file is created at /tmp/gt-idle/<session-name> and contains
a timestamp of when the session became idle.`,
	Args: cobra.NoArgs,
	RunE: runIdleMark,
}

var idleClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear idle status for current session",
	Long: `Clear the idle marker for the current tmux session.

This command is designed to be called from a Claude Code UserPromptSubmit hook.
It removes the marker file, indicating the worker is now busy.`,
	Args: cobra.NoArgs,
	RunE: runIdleClear,
}

var idleMenuCmd = &cobra.Command{
	Use:   "menu",
	Short: "Interactive picker for idle workers",
	Long: `Show a tmux popup menu to select an idle worker session.

This command is designed to be bound to a tmux key for quick access.
It shows all currently idle Gas Town workers and lets you switch to one.

Recommended tmux binding (add to ~/.tmux.conf):
  bind-key i run-shell "gt idle menu"

Then press prefix + i to open the idle worker picker.`,
	Args: cobra.NoArgs,
	RunE: runIdleMenu,
}

var idleSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Show Claude Code hook configuration",
	Long: `Show the Claude Code hook configuration needed for idle tracking.

This outputs the JSON hooks configuration that should be added to
your .claude/settings.json file to enable automatic idle tracking.`,
	Args: cobra.NoArgs,
	RunE: runIdleSetup,
}

// idleSession represents an idle worker session
type idleSession struct {
	Name      string
	IdleSince time.Time
	IsCurrent bool
}

// getIdleSessions returns all currently idle sessions
func getIdleSessions() ([]idleSession, error) {
	// Ensure idle dir exists
	if err := os.MkdirAll(idleDir, 0755); err != nil {
		return nil, fmt.Errorf("creating idle dir: %w", err)
	}

	entries, err := os.ReadDir(idleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading idle dir: %w", err)
	}

	// Get current session name (ignore error - may not be in tmux)
	currentSessionName, _ := getCurrentTmuxSession()

	var result []idleSession
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Only include gt- sessions (Gas Town workers)
		if !strings.HasPrefix(name, "gt-") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		result = append(result, idleSession{
			Name:      name,
			IdleSince: info.ModTime(),
			IsCurrent: name == currentSessionName,
		})
	}

	// Sort by idle time (oldest first - they've been waiting longest)
	sort.Slice(result, func(i, j int) bool {
		return result[i].IdleSince.Before(result[j].IdleSince)
	})

	return result, nil
}

// runIdleTmuxCommand executes a tmux command and returns stdout
func runIdleTmuxCommand(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runIdleList(cmd *cobra.Command, args []string) error {
	sessions, err := getIdleSessions()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No idle workers")
		return nil
	}

	if idleJSON {
		// JSON output
		fmt.Println("[")
		for i, s := range sessions {
			comma := ","
			if i == len(sessions)-1 {
				comma = ""
			}
			fmt.Printf(`  {"name": %q, "idle_since": %q, "idle_duration": %q}%s`+"\n",
				s.Name, s.IdleSince.Format(time.RFC3339), time.Since(s.IdleSince).Round(time.Second), comma)
		}
		fmt.Println("]")
		return nil
	}

	// Human-readable output
	fmt.Printf("%s Idle workers (%d)\n\n", style.Bold.Render("⏸"), len(sessions))
	for _, s := range sessions {
		marker := "  "
		if s.IsCurrent {
			marker = style.Bold.Render("● ")
		}
		duration := time.Since(s.IdleSince).Round(time.Second)
		fmt.Printf("%s%s %s\n", marker, s.Name, style.Dim.Render(fmt.Sprintf("(%s)", duration)))
	}

	fmt.Printf("\n%s\n", style.Dim.Render("Use 'gt idle menu' or bind to tmux key: bind-key i run-shell \"gt idle menu\""))
	return nil
}

func runIdleMark(cmd *cobra.Command, args []string) error {
	sessionName, err := getCurrentTmuxSession()
	if err != nil || sessionName == "" {
		// Not in tmux, silently succeed (might be testing)
		return nil
	}

	// Only track gt- sessions
	if !strings.HasPrefix(sessionName, "gt-") {
		return nil
	}

	// Ensure idle dir exists
	if err := os.MkdirAll(idleDir, 0755); err != nil {
		return fmt.Errorf("creating idle dir: %w", err)
	}

	// Create/update marker file
	markerPath := filepath.Join(idleDir, sessionName)
	f, err := os.Create(markerPath)
	if err != nil {
		return fmt.Errorf("creating marker: %w", err)
	}
	defer f.Close()

	// Write timestamp
	fmt.Fprintf(f, "%d\n", time.Now().Unix())
	return nil
}

func runIdleClear(cmd *cobra.Command, args []string) error {
	sessionName, err := getCurrentTmuxSession()
	if err != nil || sessionName == "" {
		return nil
	}

	markerPath := filepath.Join(idleDir, sessionName)
	err = os.Remove(markerPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing marker: %w", err)
	}
	return nil
}

func runIdleMenu(cmd *cobra.Command, args []string) error {
	sessions, err := getIdleSessions()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		// Show message via tmux display-message
		_, _ = runIdleTmuxCommand("display-message", "No idle workers")
		return nil
	}

	currentSession, _ := getCurrentTmuxSession()

	// Build tmux display-menu arguments
	menuArgs := []string{
		"display-menu",
		"-T", fmt.Sprintf("#[bold]Idle Workers (%d)", len(sessions)),
		"-x", "C",
		"-y", "C",
	}

	for _, s := range sessions {
		label := s.Name
		if s.Name == currentSession {
			label = fmt.Sprintf("● %s (here)", s.Name)
		} else {
			duration := time.Since(s.IdleSince).Round(time.Second)
			label = fmt.Sprintf("  %s (%s)", s.Name, duration)
		}

		// Menu item: label, shortcut (empty), command
		menuArgs = append(menuArgs, label, "", fmt.Sprintf("switch-client -t %s", s.Name))
	}

	// Add help footer
	menuArgs = append(menuArgs, "", "", "") // separator
	menuArgs = append(menuArgs, "#[dim]↑↓ navigate  Enter select  q quit", "", "")

	_, err = runIdleTmuxCommand(menuArgs...)
	return err
}

func runIdleSetup(cmd *cobra.Command, args []string) error {
	fmt.Println(`Add these hooks to your .claude/settings.json:

{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt idle mark"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt idle clear"
          }
        ]
      }
    ]
  }
}

Then add this to your ~/.tmux.conf:

  # Idle worker picker (Gas Town)
  bind-key i run-shell "gt idle menu"

Now:
  - Workers auto-mark idle when Claude finishes responding
  - Workers auto-clear when you submit a new prompt
  - Press prefix + i to see and switch to idle workers`)

	return nil
}
