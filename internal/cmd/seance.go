package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	seanceRole   string
	seanceRig    string
	seanceRecent int
	seanceTalk   string
	seancePrompt string
	seanceJSON   bool
)

var seanceCmd = &cobra.Command{
	Use:     "seance",
	GroupID: GroupDiag,
	Short:   "Talk to your predecessor sessions",
	Long: `Seance lets you literally talk to predecessor sessions.

"Where did you put the stuff you left for me?" - The #1 handoff question.

Instead of parsing logs, seance spawns a Claude subprocess that resumes
a predecessor session with full context. You can ask questions directly:
  - "Why did you make this decision?"
  - "Where were you stuck?"
  - "What did you try that didn't work?"

DISCOVERY:
  gt seance                     # List recent sessions from events
  gt seance --role crew         # Filter by role type
  gt seance --rig gastown       # Filter by rig
  gt seance --recent 10         # Last N sessions

THE SEANCE (talk to predecessor):
  gt seance --talk <session-id>              # Interactive conversation
  gt seance --talk <id> -p "Where is X?"     # One-shot question

The --talk flag spawns: claude --fork-session --resume <id>
This loads the predecessor's full context without modifying their session.

Sessions are discovered from:
  1. Events emitted by SessionStart hooks (~/gt/.events.jsonl)
  2. The [GAS TOWN] beacon makes sessions searchable in /resume`,
	RunE: runSeance,
}

func init() {
	seanceCmd.Flags().StringVar(&seanceRole, "role", "", "Filter by role (crew, polecat, witness, etc.)")
	seanceCmd.Flags().StringVar(&seanceRig, "rig", "", "Filter by rig name")
	seanceCmd.Flags().IntVarP(&seanceRecent, "recent", "n", 20, "Number of recent sessions to show")
	seanceCmd.Flags().StringVarP(&seanceTalk, "talk", "t", "", "Session ID to commune with")
	seanceCmd.Flags().StringVarP(&seancePrompt, "prompt", "p", "", "One-shot prompt (with --talk)")
	seanceCmd.Flags().BoolVar(&seanceJSON, "json", false, "Output as JSON")

	rootCmd.AddCommand(seanceCmd)
}

// sessionEvent represents a session_start event from our event stream.
type sessionEvent struct {
	Timestamp string                 `json:"ts"`
	Type      string                 `json:"type"`
	Actor     string                 `json:"actor"`
	Payload   map[string]interface{} `json:"payload"`
}

func runSeance(cmd *cobra.Command, args []string) error {
	// If --talk is provided, spawn a seance
	if seanceTalk != "" {
		return runSeanceTalk(seanceTalk, seancePrompt)
	}

	// Otherwise, list discoverable sessions
	return runSeanceList()
}

func runSeanceList() error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Read session events from our event stream
	sessions, err := discoverSessions(townRoot)
	if err != nil {
		return fmt.Errorf("discovering sessions: %w", err)
	}

	// Apply filters
	var filtered []sessionEvent
	for _, s := range sessions {
		if seanceRole != "" {
			actor := strings.ToLower(s.Actor)
			if !strings.Contains(actor, strings.ToLower(seanceRole)) {
				continue
			}
		}
		if seanceRig != "" {
			actor := strings.ToLower(s.Actor)
			if !strings.Contains(actor, strings.ToLower(seanceRig)) {
				continue
			}
		}
		filtered = append(filtered, s)
	}

	// Apply limit
	if seanceRecent > 0 && len(filtered) > seanceRecent {
		filtered = filtered[:seanceRecent]
	}

	if seanceJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	if len(filtered) == 0 {
		fmt.Println("No session events found.")
		fmt.Println(style.Dim.Render("Sessions are discovered from ~/gt/.events.jsonl"))
		fmt.Println(style.Dim.Render("Ensure SessionStart hooks emit session_start events"))
		return nil
	}

	// Print header
	fmt.Printf("%s\n\n", style.Bold.Render("Discoverable Sessions"))

	// Column widths
	idWidth := 12
	roleWidth := 26
	timeWidth := 16
	topicWidth := 28

	fmt.Printf("%-*s  %-*s  %-*s  %-*s\n",
		idWidth, "SESSION_ID",
		roleWidth, "ROLE",
		timeWidth, "STARTED",
		topicWidth, "TOPIC")
	fmt.Printf("%s\n", strings.Repeat("─", idWidth+roleWidth+timeWidth+topicWidth+6))

	for _, s := range filtered {
		sessionID := getPayloadString(s.Payload, "session_id")
		if len(sessionID) > idWidth {
			sessionID = sessionID[:idWidth-1] + "…"
		}

		role := s.Actor
		if len(role) > roleWidth {
			role = role[:roleWidth-1] + "…"
		}

		timeStr := formatEventTime(s.Timestamp)

		topic := getPayloadString(s.Payload, "topic")
		if topic == "" {
			topic = "-"
		}
		if len(topic) > topicWidth {
			topic = topic[:topicWidth-1] + "…"
		}

		fmt.Printf("%-*s  %-*s  %-*s  %-*s\n",
			idWidth, sessionID,
			roleWidth, role,
			timeWidth, timeStr,
			topicWidth, topic)
	}

	fmt.Printf("\n%s\n", style.Bold.Render("Talk to a predecessor:"))
	fmt.Printf("  gt seance --talk <session-id>\n")
	fmt.Printf("  gt seance --talk <session-id> -p \"Where did you put X?\"\n")

	return nil
}

func runSeanceTalk(sessionID, prompt string) error {
	// Expand short IDs if needed (user might provide partial)
	// For now, require full ID or let claude --resume handle it

	fmt.Printf("%s Summoning session %s...\n\n", style.Bold.Render("🔮"), sessionID)

	// Find the session in another account and symlink it to the current account
	// This allows Claude to load sessions from any account while keeping
	// the forked session in the current account
	townRoot, _ := workspace.FindFromCwd()
	cleanup, err := symlinkSessionToCurrentAccount(townRoot, sessionID)
	if err != nil {
		// Not fatal - session might already be in current account
		fmt.Printf("%s\n", style.Dim.Render("Note: "+err.Error()))
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Build the command
	args := []string{"--fork-session", "--resume", sessionID}

	if prompt != "" {
		// One-shot mode with --print
		args = append(args, "--print", prompt)

		cmd := exec.Command("claude", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("seance failed: %w", err)
		}
		return nil
	}

	// Interactive mode - just launch claude
	cmd := exec.Command("claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("%s\n", style.Dim.Render("You are now talking to your predecessor. Ask them anything."))
	fmt.Printf("%s\n\n", style.Dim.Render("Exit with /exit or Ctrl+C"))

	if err := cmd.Run(); err != nil {
		// Exit errors are normal when user exits
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 || exitErr.ExitCode() == 130 {
				return nil // Normal exit or Ctrl+C
			}
		}
		return fmt.Errorf("seance ended: %w", err)
	}

	return nil
}

// discoverSessions reads session_start events from our event stream.
func discoverSessions(townRoot string) ([]sessionEvent, error) {
	eventsPath := filepath.Join(townRoot, events.EventsFile)

	file, err := os.Open(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var sessions []sessionEvent
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var event sessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		if event.Type == events.TypeSessionStart {
			sessions = append(sessions, event)
		}
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp > sessions[j].Timestamp
	})

	return sessions, scanner.Err()
}

func getPayloadString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func formatEventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("2006-01-02 15:04")
}

// sessionsIndex represents the structure of sessions-index.json files.
type sessionsIndex struct {
	Version int `json:"version"`
	Entries []struct {
		SessionID string `json:"sessionId"`
	} `json:"entries"`
}

// sessionLocation contains the location info for a session.
type sessionLocation struct {
	configDir  string // The account's config directory
	projectDir string // The project directory name (e.g., "-Users-jv-gt-gastown-crew-propane")
}

// findSessionLocation searches all account config directories for a session.
// Returns the config directory and project directory that contain the session.
func findSessionLocation(townRoot, sessionID string) *sessionLocation {
	if townRoot == "" {
		return nil
	}

	// Load accounts config
	accountsPath := constants.MayorAccountsPath(townRoot)
	cfg, err := config.LoadAccountsConfig(accountsPath)
	if err != nil {
		return nil
	}

	// Search each account's config directory
	for _, acct := range cfg.Accounts {
		if acct.ConfigDir == "" {
			continue
		}

		// Expand ~ in path
		configDir := acct.ConfigDir
		if strings.HasPrefix(configDir, "~/") {
			home, _ := os.UserHomeDir()
			configDir = filepath.Join(home, configDir[2:])
		}

		// Search all sessions-index.json files in this account
		projectsDir := filepath.Join(configDir, "projects")
		if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
			continue
		}

		// Walk through project directories
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			indexPath := filepath.Join(projectsDir, entry.Name(), "sessions-index.json")
			if _, err := os.Stat(indexPath); os.IsNotExist(err) {
				continue
			}

			// Read and parse the sessions index
			data, err := os.ReadFile(indexPath)
			if err != nil {
				continue
			}

			var index sessionsIndex
			if err := json.Unmarshal(data, &index); err != nil {
				continue
			}

			// Check if this index contains our session
			for _, e := range index.Entries {
				if e.SessionID == sessionID {
					return &sessionLocation{
						configDir:  configDir,
						projectDir: entry.Name(),
					}
				}
			}
		}
	}

	return nil
}

// symlinkSessionToCurrentAccount finds a session in any account and symlinks
// the project directory to the current account so Claude can access it.
// Returns a cleanup function to remove the symlink after use.
func symlinkSessionToCurrentAccount(townRoot, sessionID string) (cleanup func(), err error) {
	// Find where the session lives
	loc := findSessionLocation(townRoot, sessionID)
	if loc == nil {
		return nil, fmt.Errorf("session not found in any account")
	}

	// Get current account's config directory (resolve ~/.claude symlink)
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	claudeDir := filepath.Join(home, ".claude")
	currentConfigDir, err := filepath.EvalSymlinks(claudeDir)
	if err != nil {
		// ~/.claude might not be a symlink, use it directly
		currentConfigDir = claudeDir
	}

	// If session is already in current account, nothing to do
	if loc.configDir == currentConfigDir {
		return nil, nil
	}

	// Source: the project directory in the other account
	sourceProjectDir := filepath.Join(loc.configDir, "projects", loc.projectDir)

	// Target: symlink in current account's projects directory
	currentProjectsDir := filepath.Join(currentConfigDir, "projects")
	if err := os.MkdirAll(currentProjectsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating projects directory: %w", err)
	}

	targetSymlink := filepath.Join(currentProjectsDir, loc.projectDir)

	// Check if target already exists
	if info, err := os.Lstat(targetSymlink); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// Already a symlink - check if it points to the right place
			existing, _ := os.Readlink(targetSymlink)
			if existing == sourceProjectDir {
				// Already symlinked correctly, no cleanup needed
				return nil, nil
			}
			// Different symlink, remove it
			_ = os.Remove(targetSymlink)
		} else {
			// Real directory exists - don't overwrite it
			return nil, fmt.Errorf("project directory already exists in current account")
		}
	}

	// Create the symlink
	if err := os.Symlink(sourceProjectDir, targetSymlink); err != nil {
		return nil, fmt.Errorf("creating symlink: %w", err)
	}

	// Return cleanup function to remove the symlink
	cleanup = func() {
		_ = os.Remove(targetSymlink)
	}

	return cleanup, nil
}
