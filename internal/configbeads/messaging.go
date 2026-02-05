// Package configbeads provides functions for loading configuration from beads.
// This file adds messaging config bead loading.
package configbeads

import (
	"encoding/json"
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// LoadMessagingConfigFromBeads loads the MessagingConfig from config beads.
// It queries beads matching the "messaging" category for the given town,
// merging layers from least-specific to most-specific scope.
// Returns nil, nil if no messaging config beads exist.
func LoadMessagingConfigFromBeads(bd *beads.Beads, townName string) (*config.MessagingConfig, error) {
	_, fields, err := bd.ListConfigBeadsForScope(
		beads.ConfigCategoryMessaging, townName, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("listing messaging config beads: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}

	// Use the most specific bead's metadata (last in list, since sorted by specificity)
	lastField := fields[len(fields)-1]
	if lastField.Metadata == "" {
		return nil, nil
	}

	var cfg config.MessagingConfig
	if err := json.Unmarshal([]byte(lastField.Metadata), &cfg); err != nil {
		return nil, fmt.Errorf("parsing messaging config bead metadata: %w", err)
	}

	return &cfg, nil
}

// LoadMessagingConfig loads messaging config, trying beads first with filesystem fallback.
func LoadMessagingConfig(townRoot, townName string) (*config.MessagingConfig, error) {
	// Try beads first
	bd := beads.New(townRoot)
	cfg, err := LoadMessagingConfigFromBeads(bd, townName)
	if err == nil && cfg != nil {
		return cfg, nil
	}

	// Fallback to filesystem
	fsPath := config.MessagingConfigPath(townRoot)
	return config.LoadOrCreateMessagingConfig(fsPath)
}
