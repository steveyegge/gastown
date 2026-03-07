// ABOUTME: Global state management for Gas Town enable/disable toggle.
// ABOUTME: Uses XDG-compliant paths for per-machine state storage.

package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/util"
)

// State represents the global Gas Town state.
type State struct {
	Enabled          bool      `json:"enabled"`
	Version          string    `json:"version"`
	MachineID        string    `json:"machine_id"`
	InstalledAt      time.Time `json:"installed_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ShellIntegration string    `json:"shell_integration,omitempty"`
	LastDoctorRun    time.Time `json:"last_doctor_run,omitempty"`
}

// StateDir returns the XDG-compliant state directory.
// Uses ~/.local/state/gastown/ (per XDG Base Directory Specification).
func StateDir() string {
	// Check XDG_STATE_HOME first
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "gastown")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "gastown")
}

// ConfigDir returns the XDG-compliant config directory.
// Uses ~/.config/gastown/
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "gastown")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gastown")
}

// CacheDir returns the XDG-compliant cache directory.
// Uses ~/.cache/gastown/
func CacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "gastown")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "gastown")
}

// StatePath returns the path to state.json.
func StatePath() string {
	return filepath.Join(StateDir(), "state.json")
}

// stateLockPath returns the path to the flock sidecar for state.json.
func stateLockPath() string {
	return StatePath() + ".lock"
}

// withStateLock acquires an exclusive cross-process file lock around fn.
// This prevents the read-check-generate-write race where concurrent callers
// (e.g. daemon + CLI) could both see an empty MachineID, generate different
// UUIDs, and clobber each other's writes.
func withStateLock(fn func() error) error {
	dir := StateDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	fl := flock.New(stateLockPath())
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("acquiring state file lock: %w", err)
	}
	defer fl.Unlock() //nolint:errcheck // best-effort unlock

	return fn()
}

// IsEnabled checks if Gas Town is globally enabled.
// Priority: env override > state file > default (false)
func IsEnabled() bool {
	// Environment overrides take priority
	if os.Getenv("GASTOWN_DISABLED") == "1" {
		return false
	}
	if os.Getenv("GASTOWN_ENABLED") == "1" {
		return true
	}

	// Check state file
	state, err := Load()
	if err != nil {
		return false // Default to disabled if state unreadable
	}
	return state.Enabled
}

// Load reads the state from disk.
func Load() (*State, error) {
	data, err := os.ReadFile(StatePath())
	if os.IsNotExist(err) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save writes the state to disk atomically.
// Uses util.EnsureDirAndWriteJSONWithPerm with 0600 permissions for security.
func Save(s *State) error {
	dir := StateDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	s.UpdatedAt = time.Now()

	return util.AtomicWriteJSONWithPerm(StatePath(), s, 0600)
}

// Enable enables Gas Town globally.
func Enable(version string) error {
	return withStateLock(func() error {
		s, err := Load()
		if err != nil {
			// Create new state
			s = &State{
				InstalledAt: time.Now(),
				MachineID:   generateMachineID(),
			}
		}

		s.Enabled = true
		s.Version = version
		return Save(s)
	})
}

// Disable disables Gas Town globally.
func Disable() error {
	return withStateLock(func() error {
		s, err := Load()
		if err != nil {
			// Nothing to disable, create disabled state
			s = &State{
				InstalledAt: time.Now(),
				MachineID:   generateMachineID(),
				Enabled:     false,
			}
			return Save(s)
		}

		s.Enabled = false
		return Save(s)
	})
}

// generateMachineID creates a unique machine identifier.
func generateMachineID() string {
	return uuid.New().String()[:8]
}

// GetMachineID returns the machine ID, creating and persisting one if needed.
// Uses file locking to ensure concurrent callers converge on the same ID.
func GetMachineID() string {
	var machineID string
	err := withStateLock(func() error {
		s, loadErr := Load()
		if loadErr == nil && s.MachineID != "" {
			machineID = s.MachineID
			return nil
		}
		// State missing or MachineID empty — generate and persist.
		if s == nil {
			s = &State{
				InstalledAt: time.Now(),
			}
		}
		s.MachineID = generateMachineID()
		machineID = s.MachineID
		return Save(s)
	})
	if err != nil {
		// Lock/save failed — return a transient ID as fallback so callers
		// always get a usable value, but log nothing (callers don't expect errors).
		return generateMachineID()
	}
	return machineID
}

// SetShellIntegration records which shell integration is installed.
func SetShellIntegration(shell string) error {
	return withStateLock(func() error {
		s, err := Load()
		if err != nil {
			s = &State{
				InstalledAt: time.Now(),
				MachineID:   generateMachineID(),
			}
		}
		s.ShellIntegration = shell
		return Save(s)
	})
}

// RecordDoctorRun records when doctor was last run.
func RecordDoctorRun() error {
	return withStateLock(func() error {
		s, err := Load()
		if err != nil {
			return err
		}
		s.LastDoctorRun = time.Now()
		return Save(s)
	})
}
