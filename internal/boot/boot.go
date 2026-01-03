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
	"github.com/steveyegge/gastown/internal/tmux"
)

// SessionName is the tmux session name for Boot.
// Note: We use "gt-boot" instead of "gt-deacon-boot" to avoid tmux prefix
// matching collisions. Tmux matches session names by prefix, so "gt-deacon-boot"
// would match when checking for "gt-deacon", causing HasSession("gt-deacon")
// to return true when only Boot is running.
const SessionName = "gt-boot"

// MarkerFileName is the file that indicates Boot is currently running.
const MarkerFileName = ".boot-running"

// StatusFileName stores Boot's last execution status.
const StatusFileName = ".boot-status.json"

// DefaultMarkerTTL is how long a marker is considered valid before it's stale.
const DefaultMarkerTTL = 5 * time.Minute

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
	townRoot   string
	bootDir    string // ~/gt/deacon/dogs/boot/
	deaconDir  string // ~/gt/deacon/
	tmux       *tmux.Tmux
	degraded   bool
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
// Returns true if marker exists and isn't stale, false otherwise.
func (b *Boot) IsRunning() bool {
	info, err := os.Stat(b.markerPath())
	if err != nil {
		return false
	}

	// Check if marker is stale (older than TTL)
	age := time.Since(info.ModTime())
	if age > DefaultMarkerTTL {
		// Stale marker - clean it up
		_ = os.Remove(b.markerPath())
		return false
	}

	return true
}

// IsSessionAlive checks if the Boot tmux session exists.
func (b *Boot) IsSessionAlive() bool {
	has, err := b.tmux.HasSession(SessionName)
	return err == nil && has
}

// AcquireLock creates the marker file to indicate Boot is starting.
// Returns error if Boot is already running.
func (b *Boot) AcquireLock() error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running (marker exists)")
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
func (b *Boot) Spawn() error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running")
	}

	// Check for degraded mode
	if b.degraded {
		return b.spawnDegraded()
	}

	return b.spawnTmux()
}

// spawnTmux spawns Boot in a tmux session.
func (b *Boot) spawnTmux() error {
	// Kill any stale session first
	if b.IsSessionAlive() {
		_ = b.tmux.KillSession(SessionName)
	}

	// Ensure boot directory exists (it should have CLAUDE.md with Boot context)
	if err := b.EnsureDir(); err != nil {
		return fmt.Errorf("ensuring boot dir: %w", err)
	}

	// Create new session in boot directory (not deacon dir) so Claude reads Boot's CLAUDE.md
	if err := b.tmux.NewSession(SessionName, b.bootDir); err != nil {
		return fmt.Errorf("creating boot session: %w", err)
	}

	// Set environment
	_ = b.tmux.SetEnvironment(SessionName, "GT_ROLE", "boot")
	_ = b.tmux.SetEnvironment(SessionName, "BD_ACTOR", "deacon-boot")

	// Launch Claude with environment exported inline and initial triage prompt
	// The "gt boot triage" prompt tells Boot to immediately start triage (GUPP principle)
	startCmd := config.BuildAgentStartupCommand("boot", "deacon-boot", "", "gt boot triage")
	if err := b.tmux.SendKeys(SessionName, startCmd); err != nil {
		return fmt.Errorf("sending startup command: %w", err)
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
	cmd.Env = append(os.Environ(),
		"GT_ROLE=boot",
		"BD_ACTOR=deacon-boot",
		"GT_DEGRADED=true",
	)

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
