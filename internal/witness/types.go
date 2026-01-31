// Package witness provides the polecat monitoring agent.
//
// ZFC-compliant: Running state is derived from tmux sessions, not stored in files.
// Configuration is sourced from role beads (hq-witness-role).
package witness

// WitnessConfig contains configuration for the witness.
type WitnessConfig struct {
	// MaxWorkers is the maximum number of concurrent polecats (default: 4).
	MaxWorkers int `json:"max_workers"`

	// SpawnDelayMs is the delay between spawns in milliseconds (default: 5000).
	SpawnDelayMs int `json:"spawn_delay_ms"`

	// AutoSpawn enables automatic spawning for ready issues (default: true).
	AutoSpawn bool `json:"auto_spawn"`

	// EpicID limits spawning to children of this epic (optional).
	EpicID string `json:"epic_id,omitempty"`

	// IssuePrefix limits spawning to issues with this prefix (optional).
	IssuePrefix string `json:"issue_prefix,omitempty"`
}
