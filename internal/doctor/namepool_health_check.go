package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NamepoolHealthCheck verifies namepool state consistency and detects stale entries.
// The namepool tracks which polecat names are in use, but can become inconsistent
// if polecats are terminated abnormally (crashes, manual kills).
type NamepoolHealthCheck struct {
	FixableCheck
	staleEntries map[string][]string // rigPath -> stale polecat names
	townRoot     string
}

// NewNamepoolHealthCheck creates a new namepool health check.
func NewNamepoolHealthCheck() *NamepoolHealthCheck {
	return &NamepoolHealthCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "namepool-health",
				CheckDescription: "Verify namepool consistency and detect stale entries",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// namepoolState represents the structure of namepool-state.json
type namepoolState struct {
	RigName      string          `json:"rig_name"`
	Theme        string          `json:"theme"`
	InUse        map[string]bool `json:"in_use"`
	OverflowNext int             `json:"overflow_next"`
	MaxSize      int             `json:"max_size"`
}

// Run checks namepool consistency across all rigs.
func (c *NamepoolHealthCheck) Run(ctx *CheckContext) *CheckResult {
	c.townRoot = ctx.TownRoot
	c.staleEntries = make(map[string][]string)

	// Load rigs from registry
	rigsPath := filepath.Join(ctx.TownRoot, "mayor", "rigs.json")
	rigsData, err := os.ReadFile(rigsPath)
	if err != nil {
		// No rigs registered, nothing to check
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs registered",
		}
	}

	var rigs map[string]interface{}
	if err := json.Unmarshal(rigsData, &rigs); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Failed to parse rigs registry",
			Details: []string{err.Error()},
		}
	}

	for rigName := range rigs {
		rigPath := filepath.Join(ctx.TownRoot, rigName)
		c.checkRigNamepool(rigPath, rigName)
	}

	if len(c.staleEntries) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All namepool entries are valid",
		}
	}

	// Build summary
	var details []string
	totalStale := 0
	for rigPath, names := range c.staleEntries {
		rel, _ := filepath.Rel(ctx.TownRoot, rigPath)
		if rel == "" {
			rel = rigPath
		}
		details = append(details, fmt.Sprintf("%s: %s", rel, strings.Join(names, ", ")))
		totalStale += len(names)
	}
	sort.Strings(details)

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d stale namepool entry(s) found (polecats that no longer exist)", totalStale),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to prune stale entries",
	}
}

// checkRigNamepool checks a single rig's namepool for stale entries.
func (c *NamepoolHealthCheck) checkRigNamepool(rigPath, rigName string) {
	statePath := filepath.Join(rigPath, ".runtime", "namepool-state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return // No namepool state, nothing to check
	}

	var state namepoolState
	if err := json.Unmarshal(data, &state); err != nil {
		return // Invalid JSON, handled by runtime_state_check
	}

	if len(state.InUse) == 0 {
		return // Nothing in use
	}

	// Get actual polecat directories
	polecatsDir := filepath.Join(rigPath, "polecats")
	actualPolecats := make(map[string]bool)

	entries, err := os.ReadDir(polecatsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				actualPolecats[entry.Name()] = true
			}
		}
	}

	// Find entries marked as in-use but no matching polecat directory exists
	var stale []string
	for name, inUse := range state.InUse {
		if inUse && !actualPolecats[name] {
			stale = append(stale, name)
		}
	}

	if len(stale) > 0 {
		sort.Strings(stale)
		c.staleEntries[rigPath] = stale
	}
}

// Fix prunes stale entries from namepool state files.
func (c *NamepoolHealthCheck) Fix(ctx *CheckContext) error {
	for rigPath, staleNames := range c.staleEntries {
		statePath := filepath.Join(rigPath, ".runtime", "namepool-state.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			continue
		}

		var state namepoolState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}

		// Remove stale entries
		for _, name := range staleNames {
			delete(state.InUse, name)
		}

		// Write back
		newData, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			rel, _ := filepath.Rel(ctx.TownRoot, rigPath)
			return fmt.Errorf("failed to marshal namepool state for %s: %w", rel, err)
		}

		if err := os.WriteFile(statePath, newData, 0644); err != nil {
			rel, _ := filepath.Rel(ctx.TownRoot, rigPath)
			return fmt.Errorf("failed to write namepool state for %s: %w", rel, err)
		}
	}
	return nil
}
