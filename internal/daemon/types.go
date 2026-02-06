// Package daemon provides the town-level background service for Gas Town.
//
// The daemon is a simple Go process (not a Claude agent) that:
// 1. Pokes agents periodically (heartbeat)
// 2. Processes lifecycle requests (cycle, restart, shutdown)
// 3. Restarts sessions when agents request cycling
//
// The daemon is a "dumb scheduler" - all intelligence is in agents.
package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

// Config holds daemon configuration.
type Config struct {
	// HeartbeatInterval is how often to poke agents.
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// TownRoot is the Gas Town workspace root.
	TownRoot string `json:"town_root"`

	// LogFile is the path to the daemon log file.
	LogFile string `json:"log_file"`

	// PidFile is the path to the PID file.
	PidFile string `json:"pid_file"`
}

// DefaultConfig returns the default daemon configuration.
func DefaultConfig(townRoot string) *Config {
	daemonDir := filepath.Join(townRoot, "daemon")
	return &Config{
		HeartbeatInterval: 5 * time.Minute, // Deacon wakes on mail too, no need to poke often
		TownRoot:          townRoot,
		LogFile:           filepath.Join(daemonDir, "daemon.log"),
		PidFile:           filepath.Join(daemonDir, "daemon.pid"),
	}
}

// State represents the daemon's runtime state.
type State struct {
	// Running indicates if the daemon is running.
	Running bool `json:"running"`

	// PID is the process ID of the daemon.
	PID int `json:"pid"`

	// StartedAt is when the daemon started.
	StartedAt time.Time `json:"started_at"`

	// LastHeartbeat is when the last heartbeat completed.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// HeartbeatCount is how many heartbeats have completed.
	HeartbeatCount int64 `json:"heartbeat_count"`
}

// StateFile returns the path to the state file.
func StateFile(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "state.json")
}

// LoadState loads daemon state from disk.
func LoadState(townRoot string) (*State, error) {
	stateFile := StateFile(townRoot)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveState saves daemon state to disk using atomic write.
func SaveState(townRoot string, state *State) error {
	stateFile := StateFile(townRoot)

	// Ensure daemon directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}

	return util.AtomicWriteJSON(stateFile, state)
}

// PatrolConfig holds configuration for a single patrol.
type PatrolConfig struct {
	// Enabled controls whether this patrol runs during heartbeat.
	Enabled bool `json:"enabled"`

	// Interval is how often to run this patrol (not used yet).
	Interval string `json:"interval,omitempty"`

	// Agent is the agent type for this patrol (not used yet).
	Agent string `json:"agent,omitempty"`

	// Rigs limits this patrol to specific rigs. If empty, all rigs are patrolled.
	Rigs []string `json:"rigs,omitempty"`
}

// PatrolsConfig holds configuration for all patrols.
type PatrolsConfig struct {
	Refinery   *PatrolConfig     `json:"refinery,omitempty"`
	Witness    *PatrolConfig     `json:"witness,omitempty"`
	Deacon     *PatrolConfig     `json:"deacon,omitempty"`
	DoltServer *DoltServerConfig `json:"dolt_server,omitempty"`
}

// DaemonPatrolConfig is the structure of mayor/daemon.json.
type DaemonPatrolConfig struct {
	Type      string         `json:"type"`
	Version   int            `json:"version"`
	Heartbeat *PatrolConfig  `json:"heartbeat,omitempty"`
	Patrols   *PatrolsConfig `json:"patrols,omitempty"`
}

// PatrolConfigFile returns the path to the patrol config file.
func PatrolConfigFile(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "daemon.json")
}

// LoadPatrolConfig loads patrol configuration from mayor/daemon.json.
// Returns nil if the file doesn't exist or can't be parsed.
func LoadPatrolConfig(townRoot string) *DaemonPatrolConfig {
	configFile := PatrolConfigFile(townRoot)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil
	}

	var config DaemonPatrolConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	return &config
}

// IsPatrolEnabled checks if a patrol is enabled in the config.
// Returns true if the config doesn't exist (default enabled for backwards compatibility).
func IsPatrolEnabled(config *DaemonPatrolConfig, patrol string) bool {
	if config == nil || config.Patrols == nil {
		return true // Default: enabled
	}

	switch patrol {
	case "refinery":
		if config.Patrols.Refinery != nil {
			return config.Patrols.Refinery.Enabled
		}
	case "witness":
		if config.Patrols.Witness != nil {
			return config.Patrols.Witness.Enabled
		}
	case "deacon":
		if config.Patrols.Deacon != nil {
			return config.Patrols.Deacon.Enabled
		}
	}
	return true // Default: enabled
}

// GetPatrolRigs returns the list of rigs for a patrol, or nil if all rigs should be patrolled.
func GetPatrolRigs(config *DaemonPatrolConfig, patrol string) []string {
	if config == nil || config.Patrols == nil {
		return nil // All rigs
	}

	switch patrol {
	case "refinery":
		if config.Patrols.Refinery != nil {
			return config.Patrols.Refinery.Rigs
		}
	case "witness":
		if config.Patrols.Witness != nil {
			return config.Patrols.Witness.Rigs
		}
	}
	return nil // All rigs
}

// LifecycleAction represents a lifecycle request action.
type LifecycleAction string

const (
	// ActionCycle restarts the session with handoff.
	ActionCycle LifecycleAction = "cycle"

	// ActionRestart does a fresh restart without handoff.
	ActionRestart LifecycleAction = "restart"

	// ActionShutdown terminates without restart.
	ActionShutdown LifecycleAction = "shutdown"
)

// LifecycleRequest represents a request from an agent to the daemon.
type LifecycleRequest struct {
	// From is the agent requesting the action (e.g., "mayor/", "gastown/witness").
	From string `json:"from"`

	// Action is what lifecycle action to perform.
	Action LifecycleAction `json:"action"`

	// Timestamp is when the request was made.
	Timestamp time.Time `json:"timestamp"`
}
