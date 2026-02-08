// Package copilot provides helpers for managing GitHub Copilot CLI config.
package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

const configFileName = "config.json"

// Config represents the subset of Copilot config we manage.
type Config struct {
	TrustedFolders []string `json:"trusted_folders,omitempty"`
}

// ConfigPath returns the default Copilot config path for the given home directory.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".copilot", configFileName)
}

// LoadConfig reads the Copilot config file. If the file doesn't exist, returns (nil, false, nil).
func LoadConfig(path string) (*Config, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, true, err
	}
	return &cfg, true, nil
}

// SaveConfig writes the Copilot config file with stable formatting.
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644) //nolint:gosec // G306: config file
}

// EnsureTrustedFolder updates trusted_folders to include the provided path.
// Returns (updated, exists, err). If exists is false, the config file was missing.
func EnsureTrustedFolder(path, folder string) (bool, bool, error) {
	cfg, exists, err := LoadConfig(path)
	if err != nil {
		return false, exists, err
	}
	if !exists || cfg == nil {
		return false, false, nil
	}

	if cfg.TrustedFolders == nil {
		cfg.TrustedFolders = []string{}
	}

	if slices.Contains(cfg.TrustedFolders, folder) {
		return false, true, nil
	}

	cfg.TrustedFolders = append(cfg.TrustedFolders, folder)
	if err := SaveConfig(path, cfg); err != nil {
		return false, true, fmt.Errorf("writing copilot config: %w", err)
	}
	return true, true, nil
}
