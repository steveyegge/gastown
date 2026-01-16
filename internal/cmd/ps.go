package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
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
	Name      string `json:"name"`
	Alive     bool   `json:"alive"`
	Command   string `json:"command"`
	PID       string `json:"pid"`
	WorkDir   string `json:"work_dir"`
	Attached  bool   `json:"attached"`
	HookBead  string `json:"hook_bead,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Role      string `json:"role,omitempty"`
	IsGasTown bool   `json:"is_gas_town"`
}

func runPS(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
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
	agents := loadAgentData(townRoot)

	// Collect session information
	var sessionProcesses []SessionProcess

	for _, sessionName := range sessions {
		if sessionName == "" {
			continue
		}

		sp := SessionProcess{
			Name:      sessionName,
			IsGasTown: isGasTownSession(sessionName),
		}

		// Get process information
		if cmd, err := tmuxClient.GetPaneCommand(sessionName); err == nil {
			sp.Command = cmd
			sp.Alive = !isShellCommand(cmd)
		}

		// Get PID
		if pid, err := tmuxClient.GetPanePID(sessionName); err == nil {
			sp.PID = pid
		}

		// Get work directory
		if workDir, err := tmuxClient.GetPaneWorkDir(sessionName); err == nil {
			sp.WorkDir = workDir
		}

		// Get attached status
		if info, err := tmuxClient.GetSessionInfo(sessionName); err == nil {
			sp.Attached = info.Attached
		}

		// Get agent information if this is a Gas Town session
		if sp.IsGasTown {
			identity, err := session.ParseSessionName(sessionName)
			if err == nil {
				sp.Role = string(identity.Role)
				sp.AgentID = agentIDFromIdentity(townRoot, identity)
			}

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

func loadAgentData(townRoot string) map[string]AgentInfo {
	agents := make(map[string]AgentInfo)

	cmd := exec.Command("bd", "list", "--type=agent", "--json")
	if townRoot != "" {
		cmd.Dir = townRoot
	}
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

func agentIDFromIdentity(townRoot string, identity *session.AgentIdentity) string {
	if identity == nil {
		return ""
	}
	switch identity.Role {
	case session.RoleMayor:
		return beads.MayorBeadIDTown()
	case session.RoleDeacon:
		return beads.DeaconBeadIDTown()
	case session.RoleWitness:
		prefix := config.GetRigPrefix(townRoot, identity.Rig)
		return beads.WitnessBeadIDWithPrefix(prefix, identity.Rig)
	case session.RoleRefinery:
		prefix := config.GetRigPrefix(townRoot, identity.Rig)
		return beads.RefineryBeadIDWithPrefix(prefix, identity.Rig)
	case session.RoleCrew:
		prefix := config.GetRigPrefix(townRoot, identity.Rig)
		return beads.CrewBeadIDWithPrefix(prefix, identity.Rig, identity.Name)
	case session.RolePolecat:
		prefix := config.GetRigPrefix(townRoot, identity.Rig)
		return beads.PolecatBeadIDWithPrefix(prefix, identity.Rig, identity.Name)
	default:
		return ""
	}
}
