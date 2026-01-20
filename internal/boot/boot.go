// Package boot manages the Boot watchdog - the daemon's entry point for Deacon triage.
// Boot is a dog that runs fresh on each daemon tick, deciding whether to wake/nudge/interrupt
// the Deacon or let it continue. This centralizes the "when to wake" decision in an agent.
package boot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
)

// SessionName is the tmux session name for Boot.
// Exported from session package; aliased here for backwards compatibility.
// Note: We use "gt-boot" instead of "hq-deacon-boot" to avoid tmux prefix
// matching collisions. Tmux matches session names by prefix, so "hq-deacon-boot"
// would match when checking for "hq-deacon", causing HasSession("hq-deacon")
// to return true when only Boot is running.
const SessionName = session.BootSessionName

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
	id      agent.AgentID
	workDir string
}

// New creates a Boot for the given town.
// workDir is computed from townRoot using the standard boot location.
func New(townRoot string) (*Boot, error) {
	workDir := filepath.Join(townRoot, "deacon", "dogs", constants.RoleBoot)

	// Ensure runtime settings exist
	runtimeConfig := config.LoadRuntimeConfig(townRoot)
	if err := runtime.EnsureSettingsForRole(workDir, constants.RoleBoot, runtimeConfig); err != nil {
		return nil, fmt.Errorf("ensuring runtime settings: %w", err)
	}

	return &Boot{
		id:      agent.BootAddress,
		workDir: workDir,
	}, nil
}

// WorkDir returns the working directory for Boot.
func (b *Boot) WorkDir() string {
	return b.workDir
}

// IsRunning checks if the Boot session is active.
func (b *Boot) IsRunning() bool {
	return factory.Agents().Exists(b.id)
}

// statusPath returns the path to the status file.
func statusPath(workDir string) string {
	return filepath.Join(workDir, StatusFileName)
}

// markerPath returns the path to the marker file.
func markerPath(workDir string) string {
	return filepath.Join(workDir, MarkerFileName)
}

// AcquireLock creates the marker file to indicate Boot is starting.
// Returns error if Boot is already running.
func (b *Boot) AcquireLock() error {
	if b.IsRunning() {
		return fmt.Errorf("boot is already running (session exists)")
	}

	// Ensure boot directory exists
	if err := os.MkdirAll(b.workDir, 0755); err != nil {
		return fmt.Errorf("ensuring boot dir: %w", err)
	}

	// Create marker file
	f, err := os.Create(markerPath(b.workDir))
	if err != nil {
		return fmt.Errorf("creating marker: %w", err)
	}
	return f.Close()
}

// ReleaseLock removes the marker file.
func (b *Boot) ReleaseLock() error {
	return os.Remove(markerPath(b.workDir))
}

// SaveStatus saves Boot's execution status.
func SaveStatus(workDir string, status *Status) error {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statusPath(workDir), data, 0644) //nolint:gosec // G306: boot status is non-sensitive operational data
}

// LoadStatus loads Boot's last execution status.
func LoadStatus(workDir string) (*Status, error) {
	data, err := os.ReadFile(statusPath(workDir))
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

// IsDegraded returns whether Boot is in degraded mode.
func IsDegraded() bool {
	return os.Getenv("GT_DEGRADED") == "true"
}
