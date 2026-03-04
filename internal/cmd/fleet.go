package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/fleet"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var fleetCmd = &cobra.Command{
	Use:     "fleet",
	GroupID: GroupWorkspace,
	Short:   "Manage the machine fleet for remote polecat dispatch",
	RunE:    requireSubcommand,
	Long: `Manage the fleet of machines available for polecat dispatch.

Gas Town can dispatch polecats to remote machines over SSH,
keeping the laptop as command-center only. Satellites run gt
locally and connect back to the primary's Dolt server.

Configuration: mayor/fleet.json`,
}

// --- fleet status ---

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet machines and their status",
	RunE:  runFleetStatus,
}

func runFleetStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	fc, err := fleet.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading fleet config: %w", err)
	}

	fmt.Printf("Fleet: %d machines (policy: %s)\n", len(fc.Machines), fc.DispatchPolicy)
	if fc.DoltHost != "" {
		fmt.Printf("Dolt: %s:%d\n", fc.DoltHost, fc.DoltPort)
	}
	fmt.Println()

	statuses := fleet.PingAll(fc)
	for _, s := range statuses {
		icon := style.Bold.Render("●")
		if !s.Reachable {
			icon = style.Dim.Render("○")
		}
		enabledStr := ""
		if !s.Enabled {
			enabledStr = " (disabled)"
		}
		latencyStr := ""
		if s.Reachable {
			latencyStr = fmt.Sprintf(" %dms", s.Latency.Milliseconds())
		}
		errStr := ""
		if s.Error != "" {
			errStr = fmt.Sprintf(" error=%s", s.Error)
		}

		fmt.Printf("  %s %-20s %-16s roles=%-20s%s%s%s\n",
			icon, s.Name, s.Host,
			strings.Join(s.Roles, ","),
			enabledStr, latencyStr, errStr)

		if s.Reachable && len(s.Sessions) > 0 {
			for _, sess := range s.Sessions {
				fmt.Printf("      tmux: %s\n", sess)
			}
		}
	}
	return nil
}

// --- fleet ping ---

var fleetPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Check SSH connectivity to all fleet machines",
	RunE:  runFleetPing,
}

func runFleetPing(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	fc, err := fleet.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading fleet config: %w", err)
	}

	statuses := fleet.PingAll(fc)
	allOK := true
	for _, s := range statuses {
		if s.Reachable {
			fmt.Printf("  %s %s (%s) — %dms\n", style.Bold.Render("✓"), s.Name, s.Host, s.Latency.Milliseconds())
		} else {
			fmt.Printf("  %s %s (%s) — %s\n", style.Warning.Render("✗"), s.Name, s.Host, s.Error)
			allOK = false
		}
	}
	if !allOK {
		return fmt.Errorf("some machines unreachable")
	}
	return nil
}

// --- fleet spawn-local ---
// Satellite-side command: spawn a polecat locally and output JSON.
// Called by the primary via SSH, never by humans directly.

var (
	fleetSpawnDoltHost  string
	fleetSpawnDoltPort  int
	fleetSpawnBead      string
	fleetSpawnAccount   string
	fleetSpawnAgent     string
	fleetSpawnBaseBranch string
	fleetSpawnForce     bool
	fleetSpawnJSON      bool
)

var fleetSpawnLocalCmd = &cobra.Command{
	Use:    "spawn-local <rig>",
	Short:  "Spawn a polecat locally (satellite-side, called via SSH)",
	Args:   cobra.ExactArgs(1),
	Hidden: true, // Internal command, not for human use
	RunE:   runFleetSpawnLocal,
}

func runFleetSpawnLocal(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Override Dolt connection if provided (primary passes these via SSH)
	if fleetSpawnDoltHost != "" {
		os.Setenv("GT_DOLT_HOST", fleetSpawnDoltHost)
	}
	if fleetSpawnDoltPort > 0 {
		os.Setenv("GT_DOLT_PORT", fmt.Sprintf("%d", fleetSpawnDoltPort))
	}

	// Spawn polecat using the standard local flow
	spawnOpts := SlingSpawnOptions{
		Force:      fleetSpawnForce,
		Account:    fleetSpawnAccount,
		HookBead:   fleetSpawnBead,
		Agent:      fleetSpawnAgent,
		BaseBranch: fleetSpawnBaseBranch,
		Create:     true,
	}

	spawnInfo, err := SpawnPolecatForSling(rigName, spawnOpts)
	if err != nil {
		if fleetSpawnJSON {
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Fprintln(os.Stderr, string(errJSON))
		}
		return fmt.Errorf("spawn failed: %w", err)
	}

	if fleetSpawnJSON {
		result := fleet.SpawnResult{
			RigName:     spawnInfo.RigName,
			PolecatName: spawnInfo.PolecatName,
			SessionName: spawnInfo.SessionName,
			ClonePath:   spawnInfo.ClonePath,
			BaseBranch:  spawnInfo.BaseBranch,
			Branch:      spawnInfo.Branch,
		}
		data, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("encoding result: %w", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("Spawned %s/%s (session: %s)\n", rigName, spawnInfo.PolecatName, spawnInfo.SessionName)
	}

	return nil
}

// --- fleet attach ---

var fleetAttachCmd = &cobra.Command{
	Use:   "attach <machine> <session>",
	Short: "Attach to a remote tmux session via SSH",
	Args:  cobra.ExactArgs(2),
	RunE:  runFleetAttach,
}

func runFleetAttach(cmd *cobra.Command, args []string) error {
	machineName := args[0]
	sessionName := args[1]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	fc, err := fleet.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading fleet config: %w", err)
	}

	machine, ok := fc.Machines[machineName]
	if !ok {
		return fmt.Errorf("machine %q not found in fleet", machineName)
	}

	sshCmd := fleet.RunSSHInteractive(machine.SSHTarget(), fmt.Sprintf("tmux attach-session -t %s", sessionName))
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	return sshCmd.Run()
}

// --- fleet sessions ---

var fleetSessionsCmd = &cobra.Command{
	Use:   "sessions <machine>",
	Short: "List tmux sessions on a remote machine",
	Args:  cobra.ExactArgs(1),
	RunE:  runFleetSessions,
}

func runFleetSessions(cmd *cobra.Command, args []string) error {
	machineName := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	fc, err := fleet.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading fleet config: %w", err)
	}

	machine, ok := fc.Machines[machineName]
	if !ok {
		return fmt.Errorf("machine %q not found in fleet", machineName)
	}

	sessions, err := fleet.ListSessions(machine)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Printf("No gt sessions on %s\n", machineName)
		return nil
	}

	fmt.Printf("Sessions on %s:\n", machineName)
	for _, s := range sessions {
		fmt.Printf("  %s\n", s)
	}
	return nil
}

// --- fleet enable/disable ---

var fleetEnableCmd = &cobra.Command{
	Use:   "enable <machine>",
	Short: "Enable a machine for dispatch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setMachineEnabled(args[0], true)
	},
}

var fleetDisableCmd = &cobra.Command{
	Use:   "disable <machine>",
	Short: "Disable a machine (stop new dispatches)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setMachineEnabled(args[0], false)
	},
}

func setMachineEnabled(machineName string, enabled bool) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	fleetPath := constants.MayorFleetPath(townRoot)
	fc, err := config.LoadFleetConfig(fleetPath)
	if err != nil {
		return fmt.Errorf("loading fleet config: %w", err)
	}

	machine, ok := fc.Machines[machineName]
	if !ok {
		return fmt.Errorf("machine %q not found in fleet", machineName)
	}

	machine.Enabled = enabled
	if err := config.SaveFleetConfig(fleetPath, fc); err != nil {
		return fmt.Errorf("saving fleet config: %w", err)
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	fmt.Printf("Machine %s %s\n", machineName, action)
	return nil
}

// --- fleet session-start ---
// Satellite-side command: start a polecat tmux session locally.
// Called by the primary via SSH after the bead has been hooked.

var (
	fleetSessionStartDoltHost string
	fleetSessionStartDoltPort int
	fleetSessionStartAccount  string
	fleetSessionStartAgent    string
)

var fleetSessionStartCmd = &cobra.Command{
	Use:    "session-start <rig/polecat>",
	Short:  "Start a polecat session locally (satellite-side, called via SSH)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE:   runFleetSessionStart,
}

func runFleetSessionStart(cmd *cobra.Command, args []string) error {
	// Parse rig/polecat from argument
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected <rig>/<polecat>, got %q", args[0])
	}
	rigName := parts[0]
	polecatName := parts[1]

	// Override Dolt connection if provided
	if fleetSessionStartDoltHost != "" {
		os.Setenv("GT_DOLT_HOST", fleetSessionStartDoltHost)
	}
	if fleetSessionStartDoltPort > 0 {
		os.Setenv("GT_DOLT_PORT", fmt.Sprintf("%d", fleetSessionStartDoltPort))
	}

	// Build a SpawnedPolecatInfo and call StartSession
	info := &SpawnedPolecatInfo{
		RigName:     rigName,
		PolecatName: polecatName,
		account:     fleetSessionStartAccount,
		agent:       fleetSessionStartAgent,
	}

	// We need to resolve the session name from the rig
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}
	info.SessionName = fmt.Sprintf("gt-%s-p-%s", rigName, polecatName)

	// Resolve clone path from polecat manager
	rigsConfigPath := fmt.Sprintf("%s/mayor/rigs.json", townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return fmt.Errorf("loading rigs config: %w", err)
	}
	rigEntry, ok := rigsConfig.Rigs[rigName]
	if !ok {
		return fmt.Errorf("rig %q not found", rigName)
	}
	rigPath := rigEntry.LocalRepo
	if rigPath == "" {
		rigPath = fmt.Sprintf("%s/%s", townRoot, rigName)
	}
	info.ClonePath = fmt.Sprintf("%s/polecats/%s", rigPath, polecatName)

	pane, err := info.StartSession()
	if err != nil {
		return fmt.Errorf("starting session: %w", err)
	}

	fmt.Printf("Session started: %s (pane: %s)\n", info.SessionName, pane)
	return nil
}

// --- init ---

func init() {
	// spawn-local flags
	fleetSpawnLocalCmd.Flags().StringVar(&fleetSpawnDoltHost, "dolt-host", "", "Dolt server host (Tailscale IP)")
	fleetSpawnLocalCmd.Flags().IntVar(&fleetSpawnDoltPort, "dolt-port", 0, "Dolt server port")
	fleetSpawnLocalCmd.Flags().StringVar(&fleetSpawnBead, "bead", "", "Bead ID to hook at spawn time")
	fleetSpawnLocalCmd.Flags().StringVar(&fleetSpawnAccount, "account", "", "Claude account handle")
	fleetSpawnLocalCmd.Flags().StringVar(&fleetSpawnAgent, "agent", "", "Agent override")
	fleetSpawnLocalCmd.Flags().StringVar(&fleetSpawnBaseBranch, "base-branch", "", "Base branch override")
	fleetSpawnLocalCmd.Flags().BoolVar(&fleetSpawnForce, "force", false, "Force spawn")
	fleetSpawnLocalCmd.Flags().BoolVar(&fleetSpawnJSON, "json", false, "Output JSON result")

	// session-start flags
	fleetSessionStartCmd.Flags().StringVar(&fleetSessionStartDoltHost, "dolt-host", "", "Dolt server host")
	fleetSessionStartCmd.Flags().IntVar(&fleetSessionStartDoltPort, "dolt-port", 0, "Dolt server port")
	fleetSessionStartCmd.Flags().StringVar(&fleetSessionStartAccount, "account", "", "Claude account handle")
	fleetSessionStartCmd.Flags().StringVar(&fleetSessionStartAgent, "agent", "", "Agent override")

	fleetCmd.AddCommand(fleetStatusCmd)
	fleetCmd.AddCommand(fleetPingCmd)
	fleetCmd.AddCommand(fleetSpawnLocalCmd)
	fleetCmd.AddCommand(fleetSessionStartCmd)
	fleetCmd.AddCommand(fleetAttachCmd)
	fleetCmd.AddCommand(fleetSessionsCmd)
	fleetCmd.AddCommand(fleetEnableCmd)
	fleetCmd.AddCommand(fleetDisableCmd)

	rootCmd.AddCommand(fleetCmd)
}
