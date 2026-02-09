package krc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AutoPruneStateFile is the filename for persisting auto-prune state.
const AutoPruneStateFile = ".krc-autoprune.json"

// AutoPruneState tracks when the last prune occurred to enforce PruneInterval.
type AutoPruneState struct {
	LastPruneTime   time.Time    `json:"last_prune_time"`
	LastResult      *PruneResult `json:"last_result,omitempty"`
	PruneCount      int          `json:"prune_count"`
	TotalPruned     int          `json:"total_pruned"`
	TotalBytesFreed int64        `json:"total_bytes_freed"`
}

// autoStatePath returns the path to the auto-prune state file.
func autoStatePath(townRoot string) string {
	return filepath.Join(townRoot, AutoPruneStateFile)
}

// LoadAutoPruneState loads auto-prune state from disk.
// Returns a fresh state if the file doesn't exist.
func LoadAutoPruneState(townRoot string) (*AutoPruneState, error) {
	data, err := os.ReadFile(autoStatePath(townRoot))
	if os.IsNotExist(err) {
		return &AutoPruneState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading auto-prune state: %w", err)
	}

	var state AutoPruneState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing auto-prune state: %w", err)
	}

	return &state, nil
}

// SaveAutoPruneState persists auto-prune state to disk.
func SaveAutoPruneState(townRoot string, state *AutoPruneState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling auto-prune state: %w", err)
	}

	path := autoStatePath(townRoot)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil { //nolint:gosec // G306: state file is non-sensitive
		return fmt.Errorf("writing auto-prune state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming auto-prune state: %w", err)
	}

	return nil
}

// ShouldPrune returns true if enough time has elapsed since the last prune.
func (s *AutoPruneState) ShouldPrune(interval time.Duration) bool {
	if s.LastPruneTime.IsZero() {
		return true // never pruned before
	}
	return time.Since(s.LastPruneTime) >= interval
}

// TimeSinceLastPrune returns how long since the last prune, or -1 if never.
func (s *AutoPruneState) TimeSinceLastPrune() time.Duration {
	if s.LastPruneTime.IsZero() {
		return -1
	}
	return time.Since(s.LastPruneTime)
}

// RecordPrune updates state after a successful prune.
func (s *AutoPruneState) RecordPrune(result *PruneResult) {
	s.LastPruneTime = time.Now()
	s.LastResult = result
	s.PruneCount++
	s.TotalPruned += result.EventsPruned
	s.TotalBytesFreed += result.BytesBefore - result.BytesAfter
}

// AutoPrune runs a prune if the interval has elapsed. Returns the result
// (nil if skipped), whether it ran, and any error.
func AutoPrune(townRoot string, config *Config) (result *PruneResult, ran bool, err error) {
	state, err := LoadAutoPruneState(townRoot)
	if err != nil {
		return nil, false, fmt.Errorf("loading auto-prune state: %w", err)
	}

	if !state.ShouldPrune(config.PruneInterval) {
		return nil, false, nil
	}

	pruner := NewPruner(townRoot, config)
	result, err = pruner.Prune()
	if err != nil {
		return nil, false, fmt.Errorf("pruning: %w", err)
	}

	state.RecordPrune(result)
	if saveErr := SaveAutoPruneState(townRoot, state); saveErr != nil {
		// Non-fatal: prune succeeded, state save failed
		return result, true, nil
	}

	return result, true, nil
}
