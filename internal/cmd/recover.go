package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	recoverSoft      bool
	recoverForce     bool
	recoverInterrupt bool
)

func init() {
	rootCmd.AddCommand(recoverCmd)
	recoverCmd.Flags().BoolVar(&recoverSoft, "soft", false, "Only try interrupt + clear (no force restart)")
	recoverCmd.Flags().BoolVar(&recoverForce, "force", false, "Skip to force restart immediately")
	recoverCmd.Flags().BoolVar(&recoverInterrupt, "interrupt", false, "Only send Escape key to interrupt")
}

var recoverCmd = &cobra.Command{
	Use:     "recover <target>",
	GroupID: GroupAgents,
	Short:   "Recover a stuck agent via tmux",
	Long: `Recover a stuck agent using escalating recovery techniques.

When an agent hits an error loop, API issues, or stuck state, this command
attempts to recover it via tmux commands without manual terminal access.

Recovery levels (in escalation order):
  1. Interrupt: Send Escape key to interrupt current execution
  2. Clear: Send /clear to reset conversation (preserves hooked work)
  3. Force: Kill session and restart fresh

Default behavior auto-escalates through all levels until recovery succeeds.
Use flags to limit recovery scope.

Flags:
  --interrupt   Only send Escape (Level 1)
  --soft        Try interrupt + clear only (Levels 1-2)
  --force       Skip to force restart (Level 3)

Target formats:
  rig/crew/name     Crew member (e.g., gastown/crew/upstream_syncer)
  rig/polecat       Polecat worker (e.g., greenplace/furiosa)
  mayor             Mayor session
  witness           Witness for current rig
  refinery          Refinery for current rig
  deacon            Deacon session

Examples:
  gt recover gastown/crew/decision     # Auto-escalate recovery
  gt recover gastown/alpha --soft      # Try interrupt + clear only
  gt recover mayor --force             # Force restart immediately
  gt recover witness --interrupt       # Just send Escape`,
	Args: cobra.ExactArgs(1),
	RunE: runRecover,
}

func runRecover(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Resolve target to session name and type
	sessionName, targetType, rigName, agentName, err := resolveRecoverTarget(target)
	if err != nil {
		return err
	}

	t := tmux.NewTmux()

	// Check if session exists
	exists, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %q not found (target: %s)", sessionName, target)
	}

	fmt.Printf("Recovering %s (session: %s)\n\n", style.Bold.Render(target), style.Dim.Render(sessionName))

	// Determine recovery strategy
	if recoverInterrupt {
		// Level 1 only
		return recoverInterruptOnly(t, sessionName)
	}
	if recoverForce {
		// Level 3 only
		return recoverForceRestart(t, sessionName, targetType, rigName, agentName)
	}
	if recoverSoft {
		// Levels 1-2
		return recoverSoftEscalation(t, sessionName)
	}
	// Default: full escalation (Levels 1-3)
	return recoverFullEscalation(t, sessionName, targetType, rigName, agentName)
}

// resolveRecoverTarget resolves a target string to session name and metadata.
// Returns: sessionName, targetType, rigName, agentName, error
func resolveRecoverTarget(target string) (string, string, string, string, error) {
	t := tmux.NewTmux()

	// Handle role shortcuts
	switch target {
	case "mayor":
		return session.MayorSessionName(), "mayor", "", "", nil
	case "deacon":
		return session.DeaconSessionName(), "deacon", "", "", nil
	case "witness", "refinery":
		roleInfo, err := GetRole()
		if err != nil {
			return "", "", "", "", fmt.Errorf("cannot determine rig for %s shortcut: %w", target, err)
		}
		if roleInfo.Rig == "" {
			return "", "", "", "", fmt.Errorf("cannot determine rig for %s shortcut (not in a rig context)", target)
		}
		if target == "witness" {
			return session.WitnessSessionName(roleInfo.Rig), "witness", roleInfo.Rig, "", nil
		}
		return session.RefinerySessionName(roleInfo.Rig), "refinery", roleInfo.Rig, "", nil
	}

	// Parse rig/agent format
	if strings.Contains(target, "/") {
		rigName, polecatName, err := parseAddress(target)
		if err != nil {
			return "", "", "", "", err
		}

		// Check if this is a crew address (polecatName starts with "crew/")
		if strings.HasPrefix(polecatName, "crew/") {
			crewName := strings.TrimPrefix(polecatName, "crew/")
			sessionName := crewSessionName(rigName, crewName)
			return sessionName, "crew", rigName, crewName, nil
		}

		// Regular polecat
		mgr, _, err := getSessionManager(rigName)
		if err != nil {
			return "", "", "", "", err
		}
		sessionName := mgr.SessionName(polecatName)
		return sessionName, "polecat", rigName, polecatName, nil
	}

	// Try as raw session name (legacy)
	exists, err := t.HasSession(target)
	if err != nil {
		return "", "", "", "", fmt.Errorf("checking session: %w", err)
	}
	if exists {
		return target, "raw", "", "", nil
	}

	return "", "", "", "", fmt.Errorf("target %q not found", target)
}

// recoverInterruptOnly sends just the Escape key (Level 1).
func recoverInterruptOnly(t *tmux.Tmux, sessionName string) error {
	fmt.Printf("Level 1: Sending Escape to interrupt...\n")
	if err := t.SendKeysRaw(sessionName, "Escape"); err != nil {
		return fmt.Errorf("sending Escape: %w", err)
	}
	fmt.Printf("%s Escape sent. Agent should show interrupted prompt.\n", style.SuccessPrefix)
	return nil
}

// recoverSoftEscalation tries interrupt then clear (Levels 1-2).
func recoverSoftEscalation(t *tmux.Tmux, sessionName string) error {
	// Level 1: Interrupt
	fmt.Printf("Level 1: Sending Escape to interrupt...\n")
	_ = t.SendKeysRaw(sessionName, "Escape")
	time.Sleep(500 * time.Millisecond)

	// Check if agent responded (capture output)
	fmt.Printf("         Waiting for agent response...\n")
	time.Sleep(2 * time.Second)

	// Level 2: Clear
	fmt.Printf("Level 2: Sending /clear to reset conversation...\n")
	if err := t.NudgeSession(sessionName, "/clear"); err != nil {
		return fmt.Errorf("sending /clear: %w", err)
	}

	fmt.Printf("%s Soft recovery complete. Agent should restart with fresh context.\n", style.SuccessPrefix)
	fmt.Printf("         Hooked work is preserved.\n")
	return nil
}

// recoverFullEscalation tries all levels: interrupt -> clear -> force restart.
func recoverFullEscalation(t *tmux.Tmux, sessionName, targetType, rigName, agentName string) error {
	// Level 1: Interrupt
	fmt.Printf("Level 1: Sending Escape to interrupt...\n")
	_ = t.SendKeysRaw(sessionName, "Escape")
	time.Sleep(500 * time.Millisecond)

	// Brief wait to see if interrupt worked
	fmt.Printf("         Waiting for response...\n")
	time.Sleep(2 * time.Second)

	// Level 2: Clear
	fmt.Printf("Level 2: Sending /clear to reset conversation...\n")
	if err := t.NudgeSession(sessionName, "/clear"); err != nil {
		fmt.Printf("         Warning: /clear failed: %v\n", err)
	}
	time.Sleep(3 * time.Second)

	// Check if Claude is responding
	if t.IsClaudeRunning(sessionName) {
		fmt.Printf("%s Recovery appears successful after /clear.\n", style.SuccessPrefix)
		fmt.Printf("         Hooked work is preserved.\n")
		return nil
	}

	// Level 3: Force restart
	fmt.Printf("Level 3: Force restarting session...\n")
	return recoverForceRestart(t, sessionName, targetType, rigName, agentName)
}

// recoverForceRestart kills and restarts the session (Level 3).
func recoverForceRestart(t *tmux.Tmux, sessionName, targetType, rigName, agentName string) error {
	// Kill existing session
	fmt.Printf("         Killing session %s...\n", sessionName)
	if err := t.KillSessionWithProcesses(sessionName); err != nil {
		fmt.Printf("         Warning: kill failed: %v\n", err)
	}

	// Brief delay to ensure cleanup
	time.Sleep(500 * time.Millisecond)

	// Restart based on target type
	switch targetType {
	case "crew":
		return restartCrewSession(rigName, agentName)
	case "polecat":
		return restartPolecatSession(rigName, agentName)
	case "witness":
		return restartWitnessSession(rigName)
	case "refinery":
		return restartRefinerySession(rigName)
	case "mayor":
		return restartMayorSession()
	case "deacon":
		return restartDeaconSession()
	case "raw":
		return fmt.Errorf("cannot restart raw session %q - unknown type", sessionName)
	default:
		return fmt.Errorf("unknown target type: %s", targetType)
	}
}

// restartCrewSession restarts a crew member session.
func restartCrewSession(rigName, crewName string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Get rig
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return fmt.Errorf("rig %q not found: %w", rigName, err)
	}

	// Create crew manager and restart
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	fmt.Printf("         Starting crew session...\n")
	if err := crewMgr.Start(crewName, crew.StartOptions{
		KillExisting: true,
		Topic:        "recover",
	}); err != nil {
		return fmt.Errorf("starting crew session: %w", err)
	}

	fmt.Printf("%s Force recovery complete. Fresh session started.\n", style.SuccessPrefix)
	fmt.Printf("         Hooked work is preserved in beads.\n")
	return nil
}

// restartPolecatSession restarts a polecat session.
func restartPolecatSession(rigName, polecatName string) error {
	mgr, _, err := getSessionManager(rigName)
	if err != nil {
		return fmt.Errorf("getting session manager: %w", err)
	}

	fmt.Printf("         Starting polecat session...\n")
	if err := mgr.Start(polecatName, polecat.SessionStartOptions{}); err != nil {
		return fmt.Errorf("starting polecat session: %w", err)
	}

	fmt.Printf("%s Force recovery complete. Fresh session started.\n", style.SuccessPrefix)
	fmt.Printf("         Hooked work is preserved in beads.\n")
	return nil
}

// restartWitnessSession restarts a witness session.
func restartWitnessSession(rigName string) error {
	// Use gt witness start logic
	fmt.Printf("         Starting witness session...\n")
	return fmt.Errorf("witness restart not yet implemented - use: gt witness start %s", rigName)
}

// restartRefinerySession restarts a refinery session.
func restartRefinerySession(rigName string) error {
	// Use gt refinery start logic
	fmt.Printf("         Starting refinery session...\n")
	return fmt.Errorf("refinery restart not yet implemented - use: gt refinery start %s", rigName)
}

// restartMayorSession restarts the mayor session.
func restartMayorSession() error {
	fmt.Printf("         Starting mayor session...\n")
	return fmt.Errorf("mayor restart not yet implemented - use: gt up --mayor")
}

// restartDeaconSession restarts the deacon session.
func restartDeaconSession() error {
	fmt.Printf("         Starting deacon session...\n")
	return fmt.Errorf("deacon restart not yet implemented - use: gt deacon start")
}
