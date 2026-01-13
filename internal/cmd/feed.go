package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/tui/feed"
	"github.com/steveyegge/gastown/internal/ui"
	"github.com/steveyegge/gastown/internal/workspace"
	"golang.org/x/term"
)

var (
	feedFollow   bool
	feedLimit    int
	feedSince    string
	feedMol      string
	feedType     string
	feedRig      string
	feedNoFollow bool
	feedWindow   bool
	feedPlain    bool
)

func init() {
	rootCmd.AddCommand(feedCmd)

	feedCmd.Flags().BoolVarP(&feedFollow, "follow", "f", false, "Stream events in real-time (default when no other flags)")
	feedCmd.Flags().BoolVar(&feedNoFollow, "no-follow", false, "Show events once and exit")
	feedCmd.Flags().IntVarP(&feedLimit, "limit", "n", 100, "Maximum number of events to show")
	feedCmd.Flags().StringVar(&feedSince, "since", "", "Show events since duration (e.g., 5m, 1h, 30s)")
	feedCmd.Flags().StringVar(&feedMol, "mol", "", "Filter by molecule/issue ID prefix")
	feedCmd.Flags().StringVar(&feedType, "type", "", "Filter by event type (create, update, delete, comment)")
	feedCmd.Flags().StringVar(&feedRig, "rig", "", "Run from specific rig's beads directory")
	feedCmd.Flags().BoolVarP(&feedWindow, "window", "w", false, "Open in dedicated tmux window (creates 'feed' window)")
	feedCmd.Flags().BoolVar(&feedPlain, "plain", false, "Use plain text output (bd activity) instead of TUI")
}

var feedCmd = &cobra.Command{
	Use:     "feed",
	GroupID: GroupDiag,
	Short:   "Show real-time activity feed from beads and gt events",
	Long: `Display a real-time feed of issue changes and agent activity.

By default, launches an interactive TUI dashboard with:
  - Agent tree (top): Shows all agents organized by role with latest activity
  - Convoy panel (middle): Shows in-progress and recently landed convoys
  - Event stream (bottom): Chronological feed you can scroll through
  - Vim-style navigation: j/k to scroll, tab to switch panels, 1/2/3 for panels, q to quit

The feed combines multiple event sources:
  - Beads activity: Issue creates, updates, completions (from bd activity)
  - GT events: Agent activity like patrol, sling, handoff (from .events.jsonl)
  - Convoy status: In-progress and recently-landed convoys (refreshes every 10s)

Use --plain for simple text output (wraps bd activity only).

Tmux Integration:
  Use --window to open the feed in a dedicated tmux window named 'feed'.
  This creates a persistent window you can cycle to with C-b n/p.

Event symbols:
  +  created/bonded    - New issue or molecule created
  →  in_progress       - Work started on an issue
  ✓  completed         - Issue closed or step completed
  ✗  failed            - Step or issue failed
  ⊘  deleted           - Issue removed
  🦉  patrol_started   - Witness began patrol cycle
  ⚡  polecat_nudged   - Worker was nudged
  🎯  sling            - Work was slung to worker
  🤝  handoff          - Session handed off

MQ (Merge Queue) event symbols:
  ⚙  merge_started   - Refinery began processing an MR
  ✓  merged          - MR successfully merged (green)
  ✗  merge_failed    - Merge failed (conflict, tests, etc.) (red)
  ⊘  merge_skipped   - MR skipped (already merged, etc.)

Examples:
  gt feed                       # Launch TUI dashboard
  gt feed --plain               # Plain text output (bd activity)
  gt feed --window              # Open in dedicated tmux window
  gt feed --since 1h            # Events from last hour
  gt feed --rig greenplace         # Use gastown rig's beads`,
	RunE: runFeed,
}

func runFeed(cmd *cobra.Command, args []string) error {
	// Detect Claude Code environment and suggest non-interactive mode
	if ui.IsAgentMode() && !feedPlain && !feedNoFollow {
		return fmt.Errorf(`gt feed launches an interactive TUI which doesn't work in Claude Code

Use this instead:
  gt feed --plain --no-follow    # Show recent events as plain text
  gt feed --plain --no-follow --since 5m   # Events from last 5 minutes
  gt feed --plain --no-follow --limit 20   # Last 20 events`)
	}

	// Must be in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace (run from ~/gt or a rig directory)")
	}

	// Determine working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// If --rig specified, find that rig's beads directory
	if feedRig != "" {
		// Try common beads locations for the rig
		candidates := []string{
			fmt.Sprintf("%s/%s/mayor/rig", townRoot, feedRig),
			fmt.Sprintf("%s/%s", townRoot, feedRig),
		}

		found := false
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate + "/.beads"); err == nil {
				workDir = candidate
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("rig '%s' not found or has no .beads directory", feedRig)
		}
	}

	// Build bd activity command (without argv[0] for buildFeedCommand)
	bdArgs := buildFeedArgs()

	// Handle --window mode: open in dedicated tmux window
	if feedWindow {
		return runFeedInWindow(workDir, bdArgs)
	}

	// Use TUI by default if running in a terminal and not --plain
	useTUI := !feedPlain && term.IsTerminal(int(os.Stdout.Fd()))

	if useTUI {
		return runFeedTUI(workDir)
	}

	// Plain mode: exec bd activity directly
	return runFeedDirect(workDir, bdArgs)
}

// buildFeedArgs builds the bd activity arguments based on flags.
func buildFeedArgs() []string {
	var args []string

	// Default to follow mode unless --no-follow set
	shouldFollow := !feedNoFollow
	if feedFollow {
		shouldFollow = true
	}

	if shouldFollow {
		args = append(args, "--follow")
	}

	if feedLimit != 100 {
		args = append(args, "--limit", fmt.Sprintf("%d", feedLimit))
	}

	if feedSince != "" {
		args = append(args, "--since", feedSince)
	}

	if feedMol != "" {
		args = append(args, "--mol", feedMol)
	}

	if feedType != "" {
		args = append(args, "--type", feedType)
	}

	return args
}

// runFeedDirect runs bd activity in the current terminal.
func runFeedDirect(workDir string, bdArgs []string) error {
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		return fmt.Errorf("bd not found in PATH: %w", err)
	}

	// Prepend argv[0] for exec
	fullArgs := append([]string{"bd", "activity"}, bdArgs...)

	// Change to the target directory before exec
	if err := os.Chdir(workDir); err != nil {
		return fmt.Errorf("changing to directory %s: %w", workDir, err)
	}

	return syscall.Exec(bdPath, fullArgs, os.Environ())
}

// runFeedTUI runs the interactive TUI feed.
func runFeedTUI(workDir string) error {
	// Must be in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	var sources []feed.EventSource

	// Create event source from bd activity
	bdSource, err := feed.NewBdActivitySource(workDir)
	if err != nil {
		return fmt.Errorf("creating bd activity source: %w", err)
	}
	sources = append(sources, bdSource)

	// Create MQ event source (optional - don't fail if not available)
	mqSource, err := feed.NewMQEventSourceFromWorkDir(workDir)
	if err == nil {
		sources = append(sources, mqSource)
	}

	// Create GT events source (optional - don't fail if not available)
	gtSource, err := feed.NewGtEventsSource(townRoot)
	if err == nil {
		sources = append(sources, gtSource)
	}

	// Combine all sources
	multiSource := feed.NewMultiSource(sources...)
	defer func() { _ = multiSource.Close() }()

	// Create model and connect event source
	m := feed.NewModel()
	m.SetEventChannel(multiSource.Events())
	m.SetTownRoot(townRoot)

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}

// runFeedInWindow opens the feed in a dedicated tmux window.
func runFeedInWindow(workDir string, bdArgs []string) error {
	// Check if we're in tmux
	if !tmux.IsInsideTmux() {
		return fmt.Errorf("--window requires running inside tmux")
	}

	// Get current session from TMUX env var
	// Format: /tmp/tmux-501/default,12345,0 -> we need the session name
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return fmt.Errorf("TMUX environment variable not set")
	}

	t := tmux.NewTmux()

	// Get current session name
	sessionName, err := getCurrentTmuxSession()
	if err != nil {
		return fmt.Errorf("getting current session: %w", err)
	}

	// Build the command to run in the window
	// Always use follow mode in window (it's meant to be persistent)
	feedCmd := fmt.Sprintf("cd %s && bd activity --follow", workDir)
	if len(bdArgs) > 0 {
		// Filter out --follow if present (we add it unconditionally)
		var filteredArgs []string
		for _, arg := range bdArgs {
			if arg != "--follow" {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		if len(filteredArgs) > 0 {
			feedCmd = fmt.Sprintf("cd %s && bd activity --follow %s", workDir, strings.Join(filteredArgs, " "))
		}
	}

	// Check if 'feed' window already exists
	windowTarget := sessionName + ":feed"
	exists, err := windowExists(t, sessionName, "feed")
	if err != nil {
		return fmt.Errorf("checking for feed window: %w", err)
	}

	if exists {
		// Window exists - just switch to it
		fmt.Printf("Switching to existing feed window...\n")
		return selectWindow(t, windowTarget)
	}

	// Create new window named 'feed' with the bd activity command
	fmt.Printf("Creating feed window in session %s...\n", sessionName)
	if err := createWindow(t, sessionName, "feed", workDir, feedCmd); err != nil {
		return fmt.Errorf("creating feed window: %w", err)
	}

	// Switch to the new window
	return selectWindow(t, windowTarget)
}

// windowExists checks if a window with the given name exists in the session.
// Note: getCurrentTmuxSession is defined in handoff.go
func windowExists(_ *tmux.Tmux, session, windowName string) (bool, error) { // t unused: direct exec for simplicity
	cmd := exec.Command("tmux", "list-windows", "-t", session, "-F", "#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == windowName {
			return true, nil
		}
	}
	return false, nil
}

// createWindow creates a new tmux window with the given name and command.
func createWindow(_ *tmux.Tmux, session, windowName, workDir, command string) error { // t unused: direct exec for simplicity
	args := []string{"new-window", "-t", session, "-n", windowName, "-c", workDir, command}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

// selectWindow switches to the specified window.
func selectWindow(_ *tmux.Tmux, target string) error { // t unused: direct exec for simplicity
	cmd := exec.Command("tmux", "select-window", "-t", target)
	return cmd.Run()
}
