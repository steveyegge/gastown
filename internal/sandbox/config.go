package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// DefaultRemoteWorkDir is the default working directory inside remote sandboxes.
// This is where worktree files are synced and where agents work on code.
const DefaultRemoteWorkDir = "/home/daytona/work"

// Config represents sandbox backend configuration.
// This can be specified at town level (settings/sandbox.json)
// or rig level (rig/settings/sandbox.json).
type Config struct {
	// Backend selects the execution backend: "local" or "daytona".
	// Default: "local"
	Backend BackendType `json:"backend,omitempty"`

	// Daytona contains Daytona-specific configuration.
	// Only used when Backend is "daytona".
	Daytona *DaytonaConfig `json:"daytona,omitempty"`

	// Local contains local backend-specific configuration.
	// Only used when Backend is "local".
	Local *LocalConfig `json:"local,omitempty"`

	// AgentBackends maps agent roles to their preferred backend.
	// Keys: "polecat", "witness", "refinery", "crew"
	// If an agent role is not listed, it uses the default Backend.
	// Example: {"polecat": "daytona", "witness": "local"}
	AgentBackends map[string]BackendType `json:"agent_backends,omitempty"`
}

// DaytonaConfig contains Daytona-specific settings.
type DaytonaConfig struct {
	// APIKeyEnv is the environment variable containing the Daytona API key.
	// Default: "DAYTONA_API_KEY"
	APIKeyEnv string `json:"api_key_env,omitempty"`

	// AnthropicAPIKeyEnv is the env var for the Anthropic API key to pass to sandboxes.
	// Default: "ANTHROPIC_API_KEY"
	AnthropicAPIKeyEnv string `json:"anthropic_api_key_env,omitempty"`

	// AutoStopMinutes is how long a sandbox can be idle before auto-stopping.
	// Default: 15. Set to 0 to disable auto-stop.
	AutoStopMinutes int `json:"auto_stop_minutes,omitempty"`

	// AutoArchiveMinutes is how long a stopped sandbox is kept before archiving.
	// Default: 10080 (7 days). Set to 0 to use maximum interval.
	AutoArchiveMinutes int `json:"auto_archive_minutes,omitempty"`

	// AutoDeleteMinutes is how long a stopped sandbox is kept before deletion.
	// Default: -1 (disabled). Set to 0 to delete immediately upon stopping.
	AutoDeleteMinutes int `json:"auto_delete_minutes,omitempty"`

	// Snapshot is the pre-built snapshot ID to use for sandboxes.
	// If empty, sandboxes are created from the default image.
	Snapshot string `json:"snapshot,omitempty"`

	// SnapshotHasClaudeCode indicates whether the snapshot already has Claude Code installed.
	// When true, skips Claude Code installation during sandbox creation.
	// Default: false
	SnapshotHasClaudeCode bool `json:"snapshot_has_claude_code,omitempty"`

	// Target is the target location where the sandbox runs.
	// Available values: "us", "eu"
	// Default: "" (uses Daytona's default)
	Target string `json:"target,omitempty"`

	// RemoteWorkDir is the directory inside the sandbox where worktree files are synced.
	// This is where the agent will find the project code to work on.
	// Default: "/home/daytona/work"
	RemoteWorkDir string `json:"remote_work_dir,omitempty"`
}

// LocalConfig contains local backend-specific settings.
type LocalConfig struct {
	// TmuxSocketPath is a custom tmux socket path.
	// Default: uses tmux default
	TmuxSocketPath string `json:"tmux_socket_path,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Backend:       BackendLocal,
		AgentBackends: make(map[string]BackendType),
	}
}

// DefaultDaytonaConfig returns a DaytonaConfig with sensible defaults.
// Daytona defaults: auto_stop=15min, auto_archive=7days(10080min), auto_delete=disabled(-1)
func DefaultDaytonaConfig() *DaytonaConfig {
	return &DaytonaConfig{
		APIKeyEnv:          "DAYTONA_API_KEY",
		AnthropicAPIKeyEnv: "ANTHROPIC_API_KEY",
		AutoStopMinutes:    15,
		AutoArchiveMinutes: 10080, // 7 days
		AutoDeleteMinutes:  -1,    // disabled
		RemoteWorkDir:      DefaultRemoteWorkDir,
	}
}

// GetBackendForRole returns the backend type to use for a specific agent role.
func (c *Config) GetBackendForRole(role string) BackendType {
	if c == nil {
		return BackendLocal
	}

	// Check role-specific override first
	if backend, ok := c.AgentBackends[role]; ok {
		return backend
	}

	// Fall back to default backend
	if c.Backend == "" {
		return BackendLocal
	}
	return c.Backend
}

// configCache caches loaded configurations by path.
var (
	configCache   = make(map[string]*Config)
	configCacheMu sync.RWMutex
)

// LoadConfig loads sandbox configuration from a path.
// It checks for settings/sandbox.json relative to the given path.
// Returns default config if file doesn't exist.
func LoadConfig(basePath string) (*Config, error) {
	configPath := filepath.Join(basePath, "settings", "sandbox.json")
	return LoadConfigFromFile(configPath)
}

// LoadConfigFromFile loads sandbox configuration from a specific file.
// Returns default config if file doesn't exist.
func LoadConfigFromFile(path string) (*Config, error) {
	// Check cache first
	configCacheMu.RLock()
	if cached, ok := configCache[path]; ok {
		configCacheMu.RUnlock()
		return cached, nil
	}
	configCacheMu.RUnlock()

	// Try to load from file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Cache the config
	configCacheMu.Lock()
	configCache[path] = &config
	configCacheMu.Unlock()

	return &config, nil
}

// SaveConfig saves sandbox configuration to a path.
// It writes to settings/sandbox.json relative to the given path.
func SaveConfig(basePath string, config *Config) error {
	configPath := filepath.Join(basePath, "settings", "sandbox.json")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Invalidate cache
	configCacheMu.Lock()
	delete(configCache, configPath)
	configCacheMu.Unlock()

	return os.WriteFile(configPath, data, 0644)
}

// ClearConfigCache clears the configuration cache.
// Useful for testing or when config files are modified externally.
func ClearConfigCache() {
	configCacheMu.Lock()
	configCache = make(map[string]*Config)
	configCacheMu.Unlock()
}

// MergeConfigs merges rig-level config over town-level config.
// Rig-level settings override town-level settings.
func MergeConfigs(town, rig *Config) *Config {
	if rig == nil {
		return town
	}
	if town == nil {
		return rig
	}

	// Start with town config
	result := &Config{
		Backend:       town.Backend,
		Daytona:       town.Daytona,
		Local:         town.Local,
		AgentBackends: make(map[string]BackendType),
	}

	// Copy town agent backends
	for k, v := range town.AgentBackends {
		result.AgentBackends[k] = v
	}

	// Override with rig config
	if rig.Backend != "" {
		result.Backend = rig.Backend
	}
	if rig.Daytona != nil {
		result.Daytona = rig.Daytona
	}
	if rig.Local != nil {
		result.Local = rig.Local
	}
	for k, v := range rig.AgentBackends {
		result.AgentBackends[k] = v
	}

	return result
}
