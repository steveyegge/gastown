package telegram

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// tokenPattern matches a valid Telegram bot token: numeric-id:secret.
// The secret part may contain letters, digits, underscores, and hyphens.
var tokenPattern = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]+$`)

// Config holds the configuration for the Telegram bridge.
type Config struct {
	Token     string   `json:"token"`
	ChatID    int64    `json:"chat_id"`
	AllowFrom []int64  `json:"allow_from,omitempty"`
	Target    string   `json:"target,omitempty"`
	Enabled   bool     `json:"enabled"`
	Notify    []string `json:"notify,omitempty"`
	RateLimit int      `json:"rate_limit,omitempty"`
}

// Validate checks that the config is well-formed.
// It returns an error if any required field is missing or invalid.
func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("telegram config: token is required")
	}
	if !tokenPattern.MatchString(c.Token) {
		return fmt.Errorf("telegram config: token format invalid (must match \\d+:[A-Za-z0-9_-]+)")
	}
	if c.ChatID == 0 {
		return fmt.Errorf("telegram config: chat_id is required")
	}
	return nil
}

// IsEnabled reports true only when both Enabled is true and Token is non-empty.
func (c *Config) IsEnabled() bool {
	return c.Enabled && c.Token != ""
}

// IsAllowed reports whether the given Telegram user ID is permitted to interact
// with the bridge. Fail-closed: an empty or nil AllowFrom list blocks all users.
func (c *Config) IsAllowed(userID int64) bool {
	for _, id := range c.AllowFrom {
		if id == userID {
			return true
		}
	}
	return false
}

// ApplyDefaults sets default values for fields that have not been configured.
// It does not overwrite values that are already set.
func (c *Config) ApplyDefaults() {
	if c.Target == "" {
		c.Target = "mayor/"
	}
	if c.RateLimit == 0 {
		c.RateLimit = 30
	}
	if c.Notify == nil {
		c.Notify = []string{"escalations"}
	}
}

// MaskedToken returns a redacted version of the token showing only the last 4
// characters, suitable for logging.
func (c *Config) MaskedToken() string {
	tok := c.Token
	if len(tok) <= 4 {
		return "****" + tok
	}
	return "****" + tok[len(tok)-4:]
}

// ConfigPath returns the canonical path for the Telegram config file given
// the Gas Town root directory.
func ConfigPath(townRoot string) string {
	return townRoot + "/mayor/telegram.json"
}

// LoadConfig reads and validates the Telegram config from path.
// It returns an error if the file permissions are not exactly 0600 (i.e. it
// rejects world- or group-readable files).
func LoadConfig(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, err
	}
	if info.Mode().Perm() != 0600 {
		return Config{}, fmt.Errorf("telegram config: file %s has unsafe permissions %04o (want 0600)", path, info.Mode().Perm())
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is caller-supplied and permission-checked above
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("telegram config: parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfig serialises cfg as JSON and writes it to path with 0600 permissions.
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("telegram config: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil { //nolint:gosec // intentional 0600 write
		return fmt.Errorf("telegram config: write %s: %w", path, err)
	}
	return nil
}
