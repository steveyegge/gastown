package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// AgentType represents the type of Gas Town agent.
type AgentType int

const (
	AgentMayor AgentType = iota
	AgentDeacon
	AgentWitness
	AgentRefinery
	AgentCrew
	AgentPolecat
	AgentPersonal // Non-GT session (user's terminal session)
)

// AgentSession represents a categorized tmux session.
type AgentSession struct {
	Name      string
	Type      AgentType
	Rig       string // For rig-specific agents
	AgentName string // e.g., crew name, polecat name
	Socket    string // tmux socket name this session lives on
}

// AgentTypeColors maps agent types to tmux color codes.
var AgentTypeColors = map[AgentType]string{
	AgentMayor:    "#[fg=red,bold]",
	AgentDeacon:   "#[fg=yellow,bold]",
	AgentWitness:  "#[fg=cyan]",
	AgentRefinery: "#[fg=blue]",
	AgentCrew:     "#[fg=green]",
	AgentPolecat:  "#[fg=white,dim]",
	AgentPersonal: "#[fg=magenta]",
}

// rigTypeOrder defines the display order of rig-level agent types.
var rigTypeOrder = map[AgentType]int{
	AgentRefinery: 0,
	AgentWitness:  1,
	AgentCrew:     2,
	AgentPolecat:  3,
}

// AgentTypeIcons maps agent types to display icons.
// Uses centralized emojis from constants package.
var AgentTypeIcons = map[AgentType]string{
	AgentMayor:    constants.EmojiMayor,
	AgentDeacon:   constants.EmojiDeacon,
	AgentWitness:  constants.EmojiWitness,
	AgentRefinery: constants.EmojiRefinery,
	AgentCrew:     constants.EmojiCrew,
	AgentPolecat:  constants.EmojiPolecat,
}

var agentsCmd = &cobra.Command{
	Use:     "agents",
	Aliases: []string{"ag"},
	GroupID: GroupAgents,
	Short:   "List Gas Town agent sessions",
	Long: `List Gas Town agent sessions to stdout.

Shows Mayor, Deacon, Witnesses, Refineries, and Crew workers.
Polecats are hidden (use 'gt polecat list' to see them).

Use 'gt agents menu' for an interactive tmux popup menu.`,
	RunE: runAgentsList,
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent sessions (no popup)",
	Long:  `List all agent sessions to stdout without the popup menu.`,
	RunE:  runAgentsList,
}

var agentsMenuCmd = &cobra.Command{
	Use:   "menu",
	Short: "Interactive popup menu for session switching",
	Long:  `Display a tmux popup menu of Gas Town agent sessions for quick switching.`,
	RunE:  runAgents,
}

var agentsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for identity collisions and stale locks",
	Long: `Check for identity collisions and stale locks.

This command helps detect situations where multiple Claude processes
think they own the same worker identity.

Output shows:
  - Active tmux sessions with gt- prefix
  - Identity locks in worker directories
  - Collisions (multiple agents claiming same identity)
  - Stale locks (dead PIDs)`,
	RunE: runAgentsCheck,
}

var agentsFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix identity collisions and clean up stale locks",
	Long: `Clean up identity collisions and stale locks.

This command:
  1. Removes stale locks (where the PID is dead)
  2. Reports collisions that need manual intervention

For collisions with live processes, you must manually:
  - Kill the duplicate session, OR
  - Decide which agent should own the identity`,
	RunE: runAgentsFix,
}

var (
	agentsAllFlag   bool
	agentsCheckJSON bool
)

func init() {
	agentsCmd.PersistentFlags().BoolVarP(&agentsAllFlag, "all", "a", false, "Include polecats in the menu")
	agentsCheckCmd.Flags().BoolVar(&agentsCheckJSON, "json", false, "Output as JSON")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsMenuCmd)
	agentsCmd.AddCommand(agentsCheckCmd)
	agentsCmd.AddCommand(agentsFixCmd)
	rootCmd.AddCommand(agentsCmd)
}

// categorizeSession determines the agent type from a session name.
func categorizeSession(name string) *AgentSession {
	sess := &AgentSession{Name: name}

	identity, err := session.ParseSessionName(name)
	if err != nil {
		return nil
	}

	sess.Rig = identity.Rig
	sess.AgentName = identity.Name

	switch identity.Role {
	case session.RoleMayor:
		sess.Type = AgentMayor
	case session.RoleDeacon:
		sess.Type = AgentDeacon
	case session.RoleWitness:
		sess.Type = AgentWitness
	case session.RoleRefinery:
		sess.Type = AgentRefinery
	case session.RoleCrew:
		sess.Type = AgentCrew
	case session.RolePolecat:
		sess.Type = AgentPolecat
	case session.RoleOverseer:
		return nil // overseer is the human operator, not a display agent
	default:
		return nil
	}

	return sess
}

// getAgentSessions returns all categorized Gas Town sessions from the town socket.
func getAgentSessions(includePolecats bool) ([]*AgentSession, error) {
	t := tmux.NewTmux()
	sessions, err := t.ListSessions()
	if err != nil {
		return nil, err
	}
	return filterAndSortSessions(sessions, includePolecats), nil
}

// socketGroup holds sessions for a single tmux socket.
type socketGroup struct {
	Socket   string
	Sessions []*AgentSession
}

// getAllSocketSessions lists sessions from all known tmux sockets, categorized
// and grouped. The town socket's GT agent sessions come first, followed by
// personal sessions from other sockets (e.g., default).
func getAllSocketSessions(includePolecats bool) []socketGroup {
	townSocket := tmux.GetDefaultSocket()
	var groups []socketGroup

	// Town socket: GT agent sessions
	townTmux := tmux.NewTmux() // uses default (town) socket
	if sessions, err := townTmux.ListSessions(); err == nil && len(sessions) > 0 {
		agents := filterAndSortSessions(sessions, includePolecats)
		for _, a := range agents {
			a.Socket = townSocket
		}
		if len(agents) > 0 {
			groups = append(groups, socketGroup{Socket: townSocket, Sessions: agents})
		}
	}

	// Other sockets: list personal sessions.
	// Check the "default" socket if it differs from the town socket.
	if townSocket != "" && townSocket != "default" {
		defTmux := tmux.NewTmuxWithSocket("default")
		if sessions, err := defTmux.ListSessions(); err == nil && len(sessions) > 0 {
			var personal []*AgentSession
			for _, name := range sessions {
				personal = append(personal, &AgentSession{
					Name:   name,
					Type:   AgentPersonal,
					Socket: "default",
				})
			}
			if len(personal) > 0 {
				groups = append(groups, socketGroup{Socket: "default", Sessions: personal})
			}
		}
	}

	return groups
}

// filterAndSortSessions filters raw session names into categorized, sorted agents.
func filterAndSortSessions(sessionNames []string, includePolecats bool) []*AgentSession {
	var agents []*AgentSession
	for _, name := range sessionNames {
		agent := categorizeSession(name)
		if agent == nil {
			continue
		}
		if agent.Type == AgentPolecat && !includePolecats {
			continue
		}
		// Skip boot sessions (utility session, not a user-facing agent)
		if agent.Name == session.BootSessionName() {
			continue
		}
		agents = append(agents, agent)
	}

	// Sort: mayor, deacon first, then by rig, then by type
	sort.Slice(agents, func(i, j int) bool {
		a, b := agents[i], agents[j]

		// Town-level agents first
		if a.Type == AgentMayor {
			return true
		}
		if b.Type == AgentMayor {
			return false
		}
		if a.Type == AgentDeacon {
			return true
		}
		if b.Type == AgentDeacon {
			return false
		}

		// Then by rig name
		if a.Rig != b.Rig {
			return a.Rig < b.Rig
		}

		// Within rig: refinery, witness, crew, polecat
		if rigTypeOrder[a.Type] != rigTypeOrder[b.Type] {
			return rigTypeOrder[a.Type] < rigTypeOrder[b.Type]
		}

		// Same type: alphabetical by agent name
		return a.AgentName < b.AgentName
	})

	return agents
}

// displayLabel returns the menu display label for an agent.
func (a *AgentSession) displayLabel() string {
	color := AgentTypeColors[a.Type]
	icon := AgentTypeIcons[a.Type]

	switch a.Type {
	case AgentMayor:
		return fmt.Sprintf("%s%s Mayor#[default]", color, icon)
	case AgentDeacon:
		return fmt.Sprintf("%s%s Deacon#[default]", color, icon)
	case AgentWitness:
		return fmt.Sprintf("%s%s %s/witness#[default]", color, icon, a.Rig)
	case AgentRefinery:
		return fmt.Sprintf("%s%s %s/refinery#[default]", color, icon, a.Rig)
	case AgentCrew:
		return fmt.Sprintf("%s%s %s/crew/%s#[default]", color, icon, a.Rig, a.AgentName)
	case AgentPolecat:
		return fmt.Sprintf("%s%s %s/%s#[default]", color, icon, a.Rig, a.AgentName)
	case AgentPersonal:
		return fmt.Sprintf("%s%s#[default]", color, a.Name)
	}
	return a.Name
}

// buildMenuAction returns a tmux command string for the display-menu action
// that handles both same-socket and cross-socket session switching.
//
// targetSocket is the socket the session lives on. When set, the action:
//  1. Tries switch-client first (instant, no flicker — works on same socket)
//  2. Falls back to detach + reattach via -L <socket> (cross-socket)
//
// When targetSocket is empty, uses plain switch-client (single-server).
func buildMenuAction(targetSocket, session string) string {
	if targetSocket == "" {
		return fmt.Sprintf("switch-client -t '%s'", session)
	}
	// Try switch-client (same socket, instant). If it fails (cross-socket),
	// detach and reattach to the target socket's session.
	return fmt.Sprintf(
		"run-shell 'tmux -L %s switch-client -t \"%s\" 2>/dev/null || tmux detach-client -E \"tmux -L %s attach -t %s\"'",
		targetSocket, session, targetSocket, session,
	)
}

// shortcutKey returns a keyboard shortcut for the menu item.
func shortcutKey(index int) string {
	if index < 9 {
		return fmt.Sprintf("%d", index+1)
	}
	if index < 35 {
		// a-z after 1-9
		return string(rune('a' + index - 9))
	}
	return ""
}

func runAgents(cmd *cobra.Command, args []string) error {
	groups := getAllSocketSessions(agentsAllFlag)

	// Count total sessions across all groups
	total := 0
	for _, g := range groups {
		total += len(g.Sessions)
	}

	if total == 0 {
		fmt.Println("No agent sessions running.")
		fmt.Println("\nStart agents with:")
		fmt.Println("  gt mayor start")
		fmt.Println("  gt deacon start")
		return nil
	}

	// Build display-menu arguments
	menuArgs := []string{
		"display-menu",
		"-T", "#[fg=cyan,bold]⚙️  Gas Town Sessions",
		"-x", "C", // Center horizontally
		"-y", "C", // Center vertically
	}

	multiSocket := len(groups) > 1
	keyIndex := 0

	for gi, group := range groups {
		// Socket header when there are multiple sockets
		if multiSocket {
			if gi > 0 {
				menuArgs = append(menuArgs, "") // separator
			}
			menuArgs = append(menuArgs,
				fmt.Sprintf("#[fg=white,bold]── %s ──", group.Socket), "", "")
		}

		var currentRig string
		for _, agent := range group.Sessions {
			// Rig sub-headers for GT agent sessions
			if agent.Type != AgentPersonal && agent.Rig != "" && agent.Rig != currentRig {
				if currentRig != "" || (keyIndex > 0 && !multiSocket) {
					menuArgs = append(menuArgs, "")
				}
				menuArgs = append(menuArgs,
					fmt.Sprintf("#[fg=white,dim]── %s ──", agent.Rig), "", "")
				currentRig = agent.Rig
			}

			key := shortcutKey(keyIndex)
			label := agent.displayLabel()
			action := buildMenuAction(agent.Socket, agent.Name)

			menuArgs = append(menuArgs, label, key, action)
			keyIndex++
		}
	}

	// Execute tmux display-menu
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	execCmd := exec.Command(tmuxPath, menuArgs...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	return execCmd.Run()
}

func runAgentsList(cmd *cobra.Command, args []string) error {
	agents, err := getAgentSessions(agentsAllFlag)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agent sessions running.")
		return nil
	}

	var currentRig string
	for _, agent := range agents {
		// Print rig header
		if agent.Rig != "" && agent.Rig != currentRig {
			if currentRig != "" {
				fmt.Println()
			}
			fmt.Printf("── %s ──\n", agent.Rig)
			currentRig = agent.Rig
		}

		icon := AgentTypeIcons[agent.Type]
		switch agent.Type {
		case AgentMayor:
			fmt.Printf("  %s Mayor\n", icon)
		case AgentDeacon:
			fmt.Printf("  %s Deacon\n", icon)
		case AgentWitness:
			fmt.Printf("  %s witness\n", icon)
		case AgentRefinery:
			fmt.Printf("  %s refinery\n", icon)
		case AgentCrew:
			fmt.Printf("  %s crew/%s\n", icon, agent.AgentName)
		case AgentPolecat:
			fmt.Printf("  %s %s\n", icon, agent.AgentName)
		}
	}

	return nil
}

// CollisionReport holds the results of a collision check.
type CollisionReport struct {
	TotalSessions int                       `json:"total_sessions"`
	TotalLocks    int                       `json:"total_locks"`
	Collisions    int                       `json:"collisions"`
	StaleLocks    int                       `json:"stale_locks"`
	Issues        []CollisionIssue          `json:"issues,omitempty"`
	Locks         map[string]*lock.LockInfo `json:"locks,omitempty"`
}

// CollisionIssue describes a single collision or lock issue.
type CollisionIssue struct {
	Type      string `json:"type"` // "stale", "collision", "orphaned"
	WorkerDir string `json:"worker_dir"`
	Message   string `json:"message"`
	PID       int    `json:"pid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

func runAgentsCheck(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	report, err := buildCollisionReport(townRoot)
	if err != nil {
		return err
	}

	if agentsCheckJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// Text output
	if len(report.Issues) == 0 {
		fmt.Printf("%s All agents healthy\n", style.Bold.Render("✓"))
		fmt.Printf("  Sessions: %d, Locks: %d\n", report.TotalSessions, report.TotalLocks)
		return nil
	}

	fmt.Printf("%s\n\n", style.Bold.Render("⚠️  Issues Detected"))
	fmt.Printf("Collisions: %d, Stale locks: %d\n\n", report.Collisions, report.StaleLocks)

	for _, issue := range report.Issues {
		fmt.Printf("%s %s\n", style.Bold.Render("!"), issue.Message)
		fmt.Printf("  Dir: %s\n", issue.WorkerDir)
		if issue.PID > 0 {
			fmt.Printf("  PID: %d\n", issue.PID)
		}
		fmt.Println()
	}

	fmt.Printf("Run %s to fix stale locks\n", style.Dim.Render("gt agents fix"))

	return nil
}

func runAgentsFix(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Clean stale locks
	cleaned, err := lock.CleanStaleLocks(townRoot)
	if err != nil {
		return fmt.Errorf("cleaning stale locks: %w", err)
	}

	if cleaned > 0 {
		fmt.Printf("%s Cleaned %d stale lock(s)\n", style.Bold.Render("✓"), cleaned)
	} else {
		fmt.Printf("%s No stale locks found\n", style.Dim.Render("○"))
	}

	// Check for remaining issues
	report, err := buildCollisionReport(townRoot)
	if err != nil {
		return err
	}

	if report.Collisions > 0 {
		fmt.Println()
		fmt.Printf("%s %d collision(s) require manual intervention:\n\n",
			style.Bold.Render("⚠"), report.Collisions)

		for _, issue := range report.Issues {
			if issue.Type == "collision" {
				fmt.Printf("  %s %s\n", style.Bold.Render("!"), issue.Message)
			}
		}

		fmt.Println()
		fmt.Printf("To fix, close duplicate sessions or remove lock files manually.\n")
	}

	return nil
}

func buildCollisionReport(townRoot string) (*CollisionReport, error) {
	report := &CollisionReport{
		Locks: make(map[string]*lock.LockInfo),
	}

	// Get all tmux sessions
	t := tmux.NewTmux()
	sessions, err := t.ListSessions()
	if err != nil {
		sessions = []string{} // Continue even if tmux not running
	}

	// Filter to Gas Town sessions
	var gtSessions []string
	for _, s := range sessions {
		if session.IsKnownSession(s) {
			gtSessions = append(gtSessions, s)
		}
	}
	report.TotalSessions = len(gtSessions)

	// Find all locks
	locks, err := lock.FindAllLocks(townRoot)
	if err != nil {
		return nil, fmt.Errorf("finding locks: %w", err)
	}
	report.TotalLocks = len(locks)
	report.Locks = locks

	// Check each lock for issues
	for workerDir, lockInfo := range locks {
		if lockInfo.IsStale() {
			report.StaleLocks++
			report.Issues = append(report.Issues, CollisionIssue{
				Type:      "stale",
				WorkerDir: workerDir,
				Message:   fmt.Sprintf("Stale lock (dead PID %d)", lockInfo.PID),
				PID:       lockInfo.PID,
				SessionID: lockInfo.SessionID,
			})
			continue
		}

		// Check if the locked session exists in tmux
		expectedSession := guessSessionFromWorkerDir(workerDir, townRoot)
		if expectedSession != "" {
			found := false
			for _, s := range gtSessions {
				if s == expectedSession {
					found = true
					break
				}
			}
			if !found {
				// Lock exists but session doesn't - potential orphan or collision
				report.Collisions++
				report.Issues = append(report.Issues, CollisionIssue{
					Type:      "orphaned",
					WorkerDir: workerDir,
					Message:   fmt.Sprintf("Lock exists (PID %d) but no tmux session '%s'", lockInfo.PID, expectedSession),
					PID:       lockInfo.PID,
					SessionID: lockInfo.SessionID,
				})
			}
		}
	}

	return report, nil
}

func guessSessionFromWorkerDir(workerDir, townRoot string) string {
	relPath, err := filepath.Rel(townRoot, workerDir)
	if err != nil {
		return ""
	}

	parts := strings.Split(filepath.ToSlash(relPath), "/")
	if len(parts) < 3 {
		return ""
	}

	rig := parts[0]
	workerType := parts[1]
	workerName := parts[2]

	switch workerType {
	case "crew":
		return session.CrewSessionName(session.PrefixFor(rig), workerName)
	case "polecats":
		return session.PolecatSessionName(session.PrefixFor(rig), workerName)
	case "witness":
		return session.WitnessSessionName(session.PrefixFor(rig))
	case "refinery":
		return session.RefinerySessionName(session.PrefixFor(rig))
	}

	return ""
}
