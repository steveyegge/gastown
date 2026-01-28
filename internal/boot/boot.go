// Package boot manages the Boot watchdog - the daemon's entry point for Deacon triage.
// Boot is a dog that runs fresh on each daemon tick, deciding whether to wake/nudge/interrupt
// the Deacon or let it continue. This centralizes the "when to wake" decision in an agent.
package boot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// MarkerFileName is the lock file for Boot startup coordination.
const MarkerFileName = ".boot-running"

// StatusFileName stores Boot's last execution status.
const StatusFileName = ".boot-status.json"

// Status represents Boot's execution status.
type Status struct {
	Running     bool      `json:"running"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	LastAction  string    `json:"last_action,omitempty"` // start/wake/nudge/nothing
	Target      string    `json:"target,omitempty"`      // deacon, witness, etc.
	Error       string    `json:"error,omitempty"`
}

// Boot manages the Boot watchdog lifecycle.
type Boot struct {
	townRoot  string
	bootDir   string // ~/gt/deacon/dogs/boot/
	deaconDir string // ~/gt/deacon/
	tmux      *tmux.Tmux
	degraded  bool
}

// New creates a new Boot manager.
func New(townRoot string) *Boot {
	return &Boot{
		townRoot:  townRoot,
		bootDir:   filepath.Join(townRoot, "deacon", "dogs", "boot"),
		deaconDir: filepath.Join(townRoot, "deacon"),
		tmux:      tmux.NewTmux(),
		degraded:  os.Getenv("GT_DEGRADED") == "true",
	}
}

// EnsureDir ensures the Boot directory exists.
func (b *Boot) EnsureDir() error {
	return os.MkdirAll(b.bootDir, 0755)
}

// markerPath returns the path to the marker file.
func (b *Boot) markerPath() string {
	return filepath.Join(b.bootDir, MarkerFileName)
}

// statusPath returns the path to the status file.
func (b *Boot) statusPath() string {
	return filepath.Join(b.bootDir, StatusFileName)
}

// IsRunning checks if Boot is currently running.
// Queries tmux directly for observable reality (ZFC principle).
func (b *Boot) IsRunning() bool {
	return b.IsSessionAlive()
}

// IsSessionAlive checks if the Boot tmux session exists.
func (b *Boot) IsSessionAlive() bool {
	has, err := b.tmux.HasSession(session.BootSessionName())
	return err == nil && has
}

// AcquireLock creates the marker file to indicate Boot is starting.
// Returns error if Boot is already running.
func (b *Boot) AcquireLock() error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running (session exists)")
	}

	if err := b.EnsureDir(); err != nil {
		return fmt.Errorf("ensuring boot dir: %w", err)
	}

	// Create marker file
	f, err := os.Create(b.markerPath())
	if err != nil {
		return fmt.Errorf("creating marker: %w", err)
	}
	return f.Close()
}

// ReleaseLock removes the marker file.
func (b *Boot) ReleaseLock() error {
	return os.Remove(b.markerPath())
}

// SaveStatus saves Boot's execution status.
func (b *Boot) SaveStatus(status *Status) error {
	if err := b.EnsureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(b.statusPath(), data, 0644) //nolint:gosec // G306: boot status is non-sensitive operational data
}

// LoadStatus loads Boot's last execution status.
func (b *Boot) LoadStatus() (*Status, error) {
	data, err := os.ReadFile(b.statusPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Status{}, nil
		}
		return nil, err
	}

	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// Spawn starts Boot in a fresh tmux session.
// Boot runs the mol-boot-triage molecule and exits when done.
// In degraded mode (no tmux), it runs in a subprocess.
// The agentOverride parameter allows specifying an agent alias to use instead of the town default.
func (b *Boot) Spawn(agentOverride string) error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running")
	}

	// Check for degraded mode
	if b.degraded {
		return b.spawnDegraded()
	}

	return b.spawnTmux(agentOverride)
}

// spawnTmux spawns Boot in a tmux session.
func (b *Boot) spawnTmux(agentOverride string) error {
	// Kill any stale session first.
	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	if b.IsSessionAlive() {
		_ = b.tmux.KillSessionWithProcesses(session.BootSessionName())
	}

	// Ensure boot directory exists (it should have CLAUDE.md with Boot context)
	if err := b.EnsureDir(); err != nil {
		return fmt.Errorf("ensuring boot dir: %w", err)
	}

	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: "boot",
		Sender:    "daemon",
		Topic:     "triage",
	}, "Run `gt boot triage` now.")

	var startCmd string
	if agentOverride != "" {
		var err error
		startCmd, err = config.BuildAgentStartupCommandWithAgentOverride("boot", "", b.townRoot, "", initialPrompt, agentOverride)
		if err != nil {
			return fmt.Errorf("building startup command with agent override: %w", err)
		}
	} else {
		startCmd = config.BuildAgentStartupCommand("boot", "", b.townRoot, "", initialPrompt)
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := b.tmux.NewSessionWithCommand(session.BootSessionName(), b.bootDir, startCmd); err != nil {
		return fmt.Errorf("creating boot session: %w", err)
	}

	// Set environment using centralized AgentEnv for consistency
	doltServer, err := doltserver.EnsureRunningIfMigrated(b.townRoot)
	if err != nil {
		return fmt.Errorf("dolt server check: %w", err)
	}
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:               "boot",
		TownRoot:           b.townRoot,
		DoltServerMode:     doltServer,
		DoltServerDatabase: "hq",
	})
	for k, v := range envVars {
		_ = b.tmux.SetEnvironment(session.BootSessionName(), k, v)
	}

	return nil
}

// spawnDegraded spawns Boot in degraded mode (no tmux).
// Boot runs to completion and exits without handoff.
func (b *Boot) spawnDegraded() error {
	// In degraded mode, we run gt boot triage directly
	// This performs the triage logic without a full Claude session
	cmd := exec.Command("gt", "boot", "triage", "--degraded")
	cmd.Dir = b.deaconDir

	// Use centralized AgentEnv for consistency with tmux mode
	doltServerDegraded, err := doltserver.EnsureRunningIfMigrated(b.townRoot)
	if err != nil {
		return fmt.Errorf("dolt server check: %w", err)
	}
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:               "boot",
		TownRoot:           b.townRoot,
		DoltServerMode:     doltServerDegraded,
		DoltServerDatabase: "hq",
	})
	cmd.Env = config.EnvForExecCommand(envVars)
	cmd.Env = append(cmd.Env, "GT_DEGRADED=true")

	// Run async - don't wait for completion
	return cmd.Start()
}

// IsDegraded returns whether Boot is in degraded mode.
func (b *Boot) IsDegraded() bool {
	return b.degraded
}

// Dir returns Boot's working directory.
func (b *Boot) Dir() string {
	return b.bootDir
}

// DeaconDir returns the Deacon's directory.
func (b *Boot) DeaconDir() string {
	return b.deaconDir
}

// Tmux returns the tmux manager.
func (b *Boot) Tmux() *tmux.Tmux {
	return b.tmux
}
