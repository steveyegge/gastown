// Package daemon provides config bead loading for daemon patrol configuration.
// This bridges the beads-based config system with the daemon's typed config structs.
package daemon

import (
	"encoding/json"
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
)

// LoadDaemonConfigFromBeads loads daemon patrol configuration from config beads.
// It queries config beads matching the "daemon" category for the given town,
// merging global → town-specific layers to produce the final config.
// Returns nil, nil if no daemon config beads exist.
func LoadDaemonConfigFromBeads(b *beads.Beads, townName string) (*DaemonPatrolConfig, error) {
	_, fields, err := b.ListConfigBeadsForScope(
		beads.ConfigCategoryDaemon, townName, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("listing daemon config beads: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil
	}

	// Merge metadata layers (least specific → most specific)
	merged := make(map[string]interface{})
	for _, f := range fields {
		if f.Metadata == "" {
			continue
		}
		var layer map[string]interface{}
		if err := json.Unmarshal([]byte(f.Metadata), &layer); err != nil {
			continue
		}
		deepMergeDaemonConfig(merged, layer)
	}

	// Marshal merged config and unmarshal into typed struct
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshaling merged daemon config: %w", err)
	}

	var config DaemonPatrolConfig
	if err := json.Unmarshal(mergedJSON, &config); err != nil {
		return nil, fmt.Errorf("parsing merged daemon config: %w", err)
	}

	return &config, nil
}

// deepMergeDaemonConfig merges src into dst recursively.
// For nested maps, values are merged recursively.
// For all other types, src overwrites dst.
func deepMergeDaemonConfig(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		srcMap, srcIsMap := srcVal.(map[string]interface{})
		dstMap, dstIsMap := dstVal.(map[string]interface{})
		if srcIsMap && dstIsMap {
			deepMergeDaemonConfig(dstMap, srcMap)
		} else {
			dst[key] = srcVal
		}
	}
}
