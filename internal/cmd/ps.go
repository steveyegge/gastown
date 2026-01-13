package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var psCmd = &cobra.Command{
	Use:     "ps",
	GroupID: GroupDiag,
	Short:   "List tmux sessions and processes",
	Long: `List all tmux sessions with Gas Town process information.

Shows:
- Session names
- Running status (alive/dead)
- Process names and PIDs
- Work-on-hook status for polecats
- Attached/detached state`,
	RunE: runPS,
}

var (
	psJSON    bool
	psVerbose bool
)

func init() {
	psCmd.Flags().BoolVar(&psJSON, "json", false, "Output in JSON format")
	psCmd.Flags().BoolVarP(&psVerbose, "verbose", "v", false, "Show detailed information")

	rootCmd.AddCommand(psCmd)
}

// SessionProcess represents a tmux session with process information
type SessionProcess struct {
	Name         string `json:"name"`
	Alive        bool   `json:"alive"`
	Command      string `json:"command"`
	PID          string `json:"pid"`
	WorkDir      string `json:"work_dir"`
	Attached     bool   `json:"attached"`
	HookBead     string `json:"hook_bead,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	Role         string `json:"role,omitempty"`
	IsGasTown    bool   `json:"is_gas_town"`
}

func runPS(cmd *cobra.Command, args []string) error {
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	tmuxClient := tmux.NewTmux()

	// Get all tmux sessions
	sessions, err := tmuxClient.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		if psJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No tmux sessions running")
		}
		return nil
	}

	// Get agent data for work-on-hook information
	agents := loadAgentData()

	// Collect session information
	var sessionProcesses []SessionProcess

	for _, session := range sessions {
		if session == "" {
			continue
		}

		sp := SessionProcess{
			Name:      session,
			IsGasTown: isGasTownSession(session),
		}

		// Get process information
		if cmd, err := tmuxClient.GetPaneCommand(session); err == nil {
			sp.Command = cmd
			sp.Alive = !isShellCommand(cmd)
		}

		// Get PID
		if pid, err := tmuxClient.GetPanePID(session); err == nil {
			sp.PID = pid
		}

		// Get work directory
		if workDir, err := tmuxClient.GetPaneWorkDir(session); err == nil {
			sp.WorkDir = workDir
		}

		// Get attached status
		if info, err := tmuxClient.GetSessionInfo(session); err == nil {
			sp.Attached = info.Attached
		}

		// Get agent information if this is a Gas Town session
		if sp.IsGasTown {
			sp.Role = extractRoleFromSession(session)
			sp.AgentID = extractAgentIDFromSession(session)

			// Look up work-on-hook
			if agentInfo, ok := agents[sp.AgentID]; ok {
				sp.HookBead = agentInfo.HookBead
			}
		}

		sessionProcesses = append(sessionProcesses, sp)
	}

	// Output results
	if psJSON {
		return outputPSJSON(sessionProcesses)
	}

	return outputPSTable(sessionProcesses)
}

func outputPSJSON(sessions []SessionProcess) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(sessions)
}

func outputPSTable(sessions []SessionProcess) error {
	// Print header
	fmt.Printf("%s %-30s %-10s %-15s %-10s %s\n",
		"",
		"SESSION",
		"STATUS",
		"COMMAND",
		"PID",
		"HOOK")

	// Print sessions
	for _, sp := range sessions {
		statusIcon := style.Dim.Render("○")
		statusText := style.Dim.Render("dead")

		if sp.Alive {
			statusIcon = style.Bold.Render("●")
			statusText = style.Bold.Render("alive")
		}

		// Highlight Gas Town sessions
		sessionName := sp.Name
		if sp.IsGasTown {
			sessionName = style.Bold.Render(sp.Name)
		}

		hookInfo := ""
		if sp.HookBead != "" {
			hookInfo = style.Dim.Render("⚑ " + sp.HookBead)
		}

		if psVerbose {
			fmt.Printf("%s %-30s %-10s %-15s %-10s %s\n",
				statusIcon,
				sessionName,
				statusText,
				sp.Command,
				sp.PID,
				hookInfo)

			if sp.WorkDir != "" {
				fmt.Printf("  %s %s\n", style.Dim.Render("├─"), style.Dim.Render("Dir: "+sp.WorkDir))
			}
			if sp.AgentID != "" {
				fmt.Printf("  %s %s\n", style.Dim.Render("├─"), style.Dim.Render("Agent: "+sp.AgentID))
			}
			if sp.Role != "" {
				fmt.Printf("  %s %s\n", style.Dim.Render("└─"), style.Dim.Render("Role: "+sp.Role))
			}
		} else {
			fmt.Printf("%s %-30s %-10s %-15s %-10s %s\n",
				statusIcon,
				sessionName,
				statusText,
				sp.Command,
				sp.PID,
				hookInfo)
		}
	}

	return nil
}

// AgentInfo contains agent bead information
type AgentInfo struct {
	ID       string `json:"id"`
	HookBead string `json:"hook_bead"`
}

func loadAgentData() map[string]AgentInfo {
	agents := make(map[string]AgentInfo)

	cmd := exec.Command("bd", "list", "--type=agent", "--json")
	output, err := cmd.Output()
	if err != nil {
		return agents
	}

	var agentList []AgentInfo
	if err := json.Unmarshal(output, &agentList); err != nil {
		return agents
	}

	for _, agent := range agentList {
		agents[agent.ID] = agent
	}

	return agents
}

func isGasTownSession(name string) bool {
	return strings.HasPrefix(name, "gt-") || strings.HasPrefix(name, "hq-")
}

func extractRoleFromSession(session string) string {
	// Session name format: gt-<rig>-<role>-<name> or gt-<role> (for town-level)
	parts := strings.Split(session, "-")
	if len(parts) < 2 {
		return ""
	}

	// Check for town-level agents (gt-mayor, gt-deacon)
	if len(parts) == 2 {
		return parts[1]
	}

	// Rig-level agents (gt-<rig>-<role>-<name>)
	if len(parts) >= 3 {
		return parts[2]
	}

	return ""
}

func extractAgentIDFromSession(session string) string {
	// Session format: gt-<rig>-<name> → agent ID: gt-<rig>-polecat-<name>
	// Town-level: gt-mayor → mayor
	parts := strings.Split(session, "-")

	if len(parts) == 2 {
		// Town-level agent (gt-mayor, gt-deacon)
		return parts[1]
	}

	// For polecats: gt-gastown-Toast → gt-gastown-polecat-Toast
	if len(parts) == 3 {
		return fmt.Sprintf("%s-%s-polecat-%s", parts[0], parts[1], parts[2])
	}

	return session
}
