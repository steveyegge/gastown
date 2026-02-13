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
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/terminal"
)

// coopDefault returns a CoopBackend with no sessions as a safe default.
// The real backend is injected via SetBackend() by the daemon in K8s mode.
func coopDefault() terminal.Backend {
	return terminal.NewCoopBackend(terminal.CoopConfig{})
}

// deaconSession is the deacon session name, duplicated here to avoid import cycle with session package.
const deaconSession = "hq-deacon"

// SessionName is the tmux session name for Boot.
// Note: We use "gt-boot" instead of "hq-deacon-boot" to avoid tmux prefix
// matching collisions. Tmux matches session names by prefix, so "hq-deacon-boot"
// would match when checking for "hq-deacon", causing HasSession("hq-deacon")
// to return true when only Boot is running.
const SessionName = "gt-boot"

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
	backend   terminal.Backend
	degraded  bool
}

// New creates a new Boot manager.
func New(townRoot string) *Boot {
	return &Boot{
		townRoot:  townRoot,
		bootDir:   filepath.Join(townRoot, "deacon", "dogs", "boot"),
		deaconDir: filepath.Join(townRoot, "deacon"),
		backend:   coopDefault(),
		degraded:  os.Getenv("GT_DEGRADED") == "true",
	}
}

// SetBackend overrides the terminal backend used for session liveness checks.
func (b *Boot) SetBackend(be terminal.Backend) {
	b.backend = be
}

// hasSession checks if a terminal session exists via the CoopBackend.
func (b *Boot) hasSession(sessionID string) (bool, error) {
	return b.backend.HasSession(sessionID)
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

// IsSessionAlive checks if the Boot session exists.
func (b *Boot) IsSessionAlive() bool {
	has, err := b.hasSession(session.BootSessionName())
	return err == nil && has
}

// IsCurrentSession checks if we're already running inside the Boot session.
// In K8s mode this is determined by the GT_SESSION environment variable.
func (b *Boot) IsCurrentSession() bool {
	return os.Getenv("GT_SESSION") == session.BootSessionName()
}

// AcquireLock creates the marker file to indicate Boot is starting.
// Returns error if Boot is already running (unless we ARE the current boot session).
// FIX (hq-yitkgc): Skip lock check if we're already inside the boot session.
func (b *Boot) AcquireLock() error {
	// If we're already the boot session, we don't need to check/acquire lock
	if b.IsCurrentSession() {
		// Still create the marker file for consistency
		if err := b.EnsureDir(); err != nil {
			return fmt.Errorf("ensuring boot dir: %w", err)
		}
		f, err := os.Create(b.markerPath())
		if err != nil {
			return fmt.Errorf("creating marker: %w", err)
		}
		return f.Close()
	}

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

// Spawn starts Boot as a subprocess.
// Boot runs the mol-boot-triage molecule and exits when done.
// The agentOverride parameter is unused in K8s mode (kept for API compatibility).
func (b *Boot) Spawn(_ string) error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running")
	}

	return b.spawnDegraded()
}

// spawnDegraded spawns Boot in degraded mode (no tmux).
// Boot runs to completion and exits without handoff.
func (b *Boot) spawnDegraded() error {
	// In degraded mode, we run gt boot triage directly
	// This performs the triage logic without a full Claude session
	cmd := exec.Command("gt", "boot", "triage", "--degraded")
	cmd.Dir = b.deaconDir

	// Use centralized AgentEnv for consistency with tmux mode
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:         "boot",
		TownRoot:     b.townRoot,
		BDDaemonHost: os.Getenv("BD_DAEMON_HOST"),
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

// Backend returns the terminal backend.
func (b *Boot) Backend() terminal.Backend {
	return b.backend
}
