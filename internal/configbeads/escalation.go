// Package configbeads provides functions for loading configuration from beads.
// This file adds escalation config bead loading.
package configbeads

import (
	"encoding/json"
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// LoadEscalationConfigFromBeads loads the EscalationConfig from config beads.
// It queries beads matching the "escalation" category for the given town,
// merging layers from least-specific to most-specific scope.
// Returns nil, nil if no escalation config beads exist.
func LoadEscalationConfigFromBeads(bd *beads.Beads, townName string) (*config.EscalationConfig, error) {
	_, fields, err := bd.ListConfigBeadsForScope(
		beads.ConfigCategoryEscalation, townName, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("listing escalation config beads: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}

	// Use the most specific bead's metadata (last in list, since sorted by specificity)
	lastField := fields[len(fields)-1]
	if lastField.Metadata == "" {
		return nil, nil
	}

	var cfg config.EscalationConfig
	if err := json.Unmarshal([]byte(lastField.Metadata), &cfg); err != nil {
		return nil, fmt.Errorf("parsing escalation config bead metadata: %w", err)
	}

	return &cfg, nil
}

// LoadEscalationConfig loads escalation config, trying beads first with filesystem fallback.
func LoadEscalationConfig(townRoot, townName string) (*config.EscalationConfig, error) {
	// Try beads first
	bd := beads.New(townRoot)
	cfg, err := LoadEscalationConfigFromBeads(bd, townName)
	if err == nil && cfg != nil {
		return cfg, nil
	}

	// Fallback to filesystem
	fsPath := config.EscalationConfigPath(townRoot)
	return config.LoadOrCreateEscalationConfig(fsPath)
}
