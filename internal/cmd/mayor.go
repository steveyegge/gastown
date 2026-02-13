package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/terminal"
	"github.com/steveyegge/gastown/internal/workspace"
)

var mayorCmd = &cobra.Command{
	Use:     "mayor",
	Aliases: []string{"may"},
	GroupID: GroupAgents,
	Short:   "Manage the Mayor (Chief of Staff for cross-rig coordination)",
	RunE:    requireSubcommand,
	Long: `Manage the Mayor - the Overseer's Chief of Staff.

The Mayor is the global coordinator for Gas Town:
  - Receives escalations from Witnesses and Deacon
  - Coordinates work across multiple rigs
  - Handles human communication when needed
  - Routes strategic decisions and cross-project issues

The Mayor is the primary interface between the human Overseer and the
automated agents. When in doubt, escalate to the Mayor.

Role shortcuts: "mayor" in mail/nudge addresses resolves to this agent.`,
}

var mayorAgentOverride string
var mayorTarget string
var mayorBrowser bool

var mayorStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Mayor session",
	Long: `Start the Mayor tmux session.

Creates a new detached tmux session for the Mayor and launches Claude.
The session runs in the workspace root directory.`,
	RunE: runMayorStart,
}

var mayorStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Mayor session",
	Long: `Stop the Mayor tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runMayorStop,
}

var mayorAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Mayor session",
	Long: `Attach to the running Mayor tmux session.

Attaches the current terminal to the Mayor's tmux session.
Detach with Ctrl-B D.`,
	RunE: runMayorAttach,
}

var mayorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Mayor session status",
	Long:  `Check if the Mayor tmux session is currently running.`,
	RunE:  runMayorStatus,
}

var mayorRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Mayor session",
	Long: `Restart the Mayor tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runMayorRestart,
}

func init() {
	mayorCmd.AddCommand(mayorStartCmd)
	mayorCmd.AddCommand(mayorStopCmd)
	mayorCmd.AddCommand(mayorAttachCmd)
	mayorCmd.AddCommand(mayorStatusCmd)
	mayorCmd.AddCommand(mayorRestartCmd)

	mayorStartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorStartCmd.Flags().StringVar(&mayorTarget, "target", "", "Execution target: 'k8s' to run in Kubernetes (default: local tmux)")
	mayorAttachCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")
	mayorAttachCmd.Flags().BoolVarP(&mayorBrowser, "browser", "b", false, "Open web terminal in browser instead of attaching")
	mayorRestartCmd.Flags().StringVar(&mayorAgentOverride, "agent", "", "Agent alias to run the Mayor with (overrides town default)")

	rootCmd.AddCommand(mayorCmd)
}

// getMayorManager returns a mayor manager for the current workspace.
func getMayorManager() (*mayor.Manager, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	return mayor.NewManager(townRoot), nil
}

// getMayorSessionName returns the Mayor session name.
func getMayorSessionName() string {
	return mayor.SessionName()
}

func runMayorStart(cmd *cobra.Command, args []string) error {
	if mayorTarget == "k8s" {
		return runMayorStartK8s()
	}

	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	fmt.Println("Starting Mayor session...")
	if err := mgr.Start(mayorAgentOverride); err != nil {
		if err == mayor.ErrAlreadyRunning {
			return fmt.Errorf("Mayor session already running. Attach with: gt mayor attach")
		}
		return err
	}

	fmt.Printf("%s Mayor session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("gt mayor attach"))

	return nil
}

// runMayorStartK8s creates an agent bead for the mayor that the K8s controller
// will detect and translate into a pod creation. The controller watches for
// agent beads with agent_state=spawning and execution_target:k8s label.
func runMayorStartK8s() error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	fmt.Println("Dispatching Mayor to Kubernetes...")

	// Create or reopen the mayor agent bead with spawning state.
	// Town-level beads live in the town root .beads directory.
	beadsClient := beads.New(townRoot)
	agentBeadID := beads.MayorBeadIDTown() // "hq-mayor"
	_, err = beadsClient.CreateOrReopenAgentBead(agentBeadID, agentBeadID, &beads.AgentFields{
		RoleType:   "mayor",
		AgentState: "spawning",
	})
	if err != nil {
		return fmt.Errorf("creating mayor agent bead: %w", err)
	}

	// Label so the controller knows this is a K8s agent.
	if err := beadsClient.AddLabel(agentBeadID, "execution_target:k8s"); err != nil {
		fmt.Printf("Warning: could not add execution_target label: %v\n", err)
	}

	// Emit spawn event with payload fields the controller's watcher can parse.
	// The watcher's extractAgentInfo reads payload["rig"], payload["role"],
	// payload["agent"] when the actor field doesn't have 3 parts.
	_ = events.LogFeed(events.TypeSpawn, "mayor", map[string]interface{}{
		"rig":   "town",
		"role":  "mayor",
		"agent": "hq",
	})

	fmt.Printf("%s Mayor dispatched to K8s (agent_state=spawning, bead=%s)\n",
		style.Bold.Render("✓"), agentBeadID)
	fmt.Printf("  The controller will create a mayor pod when it detects this bead.\n")
	fmt.Printf("  Attach with: %s\n", style.Dim.Render("gt mayor attach"))

	return nil
}

func runMayorStop(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	fmt.Println("Stopping Mayor session...")
	if err := mgr.Stop(); err != nil {
		if err == mayor.ErrNotRunning {
			return fmt.Errorf("Mayor session is not running")
		}
		return err
	}

	fmt.Printf("%s Mayor session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runMayorAttach(cmd *cobra.Command, args []string) error {
	// When GT_K8S_NAMESPACE is set, try K8s attach first — no local workspace needed.
	// This allows `gt mayor attach` from any directory (e.g., ~/).
	if os.Getenv("GT_K8S_NAMESPACE") != "" {
		if podName, ns := detectMayorK8sPod(""); podName != "" {
			fmt.Printf("%s Attaching to K8s Mayor pod via coop...\n",
				style.Bold.Render("☸"))
			return attachToCoopPodWithBrowser(podName, ns, mayorBrowser)
		}
	}

	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	backend := terminal.NewCoopBackend(terminal.CoopConfig{})
	sessionID := mgr.SessionName()

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// No local session — auto-start
		fmt.Println("Mayor session not running, starting...")
		if err := mgr.Start(mayorAgentOverride); err != nil {
			return err
		}
	} else {
		// Session exists - check if runtime is still running
		if agentRunning, _ := backend.IsAgentRunning(sessionID); !agentRunning {
			// Runtime has exited, restart it
			fmt.Println("Runtime exited, restarting...")

			if err := backend.RespawnPane(sessionID); err != nil {
				return fmt.Errorf("restarting runtime: %w", err)
			}

			fmt.Printf("%s Mayor restarted with context\n", style.Bold.Render("✓"))
		}
	}

	// Use shared attach helper
	return attachToTmuxSession(sessionID)
}

func runMayorStatus(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	info, err := mgr.Status()
	if err != nil {
		if err == mayor.ErrNotRunning {
			// Check for K8s coop pod.
			townRoot, _ := workspace.FindFromCwdOrError()
			if podName, ns := detectMayorK8sPod(townRoot); podName != "" {
				fmt.Printf("%s Mayor is running in %s\n",
					style.Bold.Render("☸"),
					style.Bold.Render("Kubernetes"))
				fmt.Printf("  Pod: %s (namespace: %s, coop)\n", podName, ns)
				fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt mayor attach"))
				return nil
			}

			fmt.Printf("%s Mayor session is %s\n",
				style.Dim.Render("○"),
				"not running")
			fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt mayor start"))
			return nil
		}
		return fmt.Errorf("checking status: %w", err)
	}

	status := "detached"
	if info.Attached {
		status = "attached"
	}
	fmt.Printf("%s Mayor session is %s\n",
		style.Bold.Render("●"),
		style.Bold.Render("running"))
	fmt.Printf("  Status: %s\n", status)
	fmt.Printf("  Created: %s\n", info.Created)
	fmt.Printf("\nAttach with: %s\n", style.Dim.Render("gt mayor attach"))

	return nil
}

func runMayorRestart(cmd *cobra.Command, args []string) error {
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	// Stop if running (ignore not-running error)
	if err := mgr.Stop(); err != nil && err != mayor.ErrNotRunning {
		return fmt.Errorf("stopping session: %w", err)
	}

	// Start fresh
	return runMayorStart(cmd, args)
}

// detectMayorK8sPod checks if the mayor is running as a K8s pod.
// Returns (podName, namespace) if found, or ("", "") if not.
//
// Detection is simple: check if the well-known pod name exists and is Running.
// The pod name follows the controller convention: gt-town-mayor-hq.
func detectMayorK8sPod(_ string) (string, string) {
	podName := "gt-town-mayor-hq"

	ns := os.Getenv("GT_K8S_NAMESPACE")
	if ns == "" {
		return "", ""
	}

	out, err := exec.Command("kubectl", "get", "pod", podName, "-n", ns,
		"-o", "jsonpath={.status.phase}").Output()
	if err != nil {
		return "", ""
	}
	if strings.TrimSpace(string(out)) != "Running" {
		return "", ""
	}

	return podName, ns
}
