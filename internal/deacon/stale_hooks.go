// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/ids"
)

// AgentChecker is the minimal interface for checking if agents exist.
type AgentChecker interface {
	Exists(id agent.AgentID) bool
}

// StaleHookConfig holds configurable parameters for stale hook detection.
type StaleHookConfig struct {
	// MaxAge is how long a bead can be hooked before being considered stale.
	MaxAge time.Duration `json:"max_age"`
	// DryRun if true, only reports what would be done without making changes.
	DryRun bool `json:"dry_run"`
	// AgentChecker is used to check if agents exist.
	// If nil, agent.Default() is used.
	// This field enables testing without real tmux sessions.
	AgentChecker AgentChecker `json:"-"`
}

// DefaultStaleHookConfig returns the default stale hook config.
func DefaultStaleHookConfig() *StaleHookConfig {
	return &StaleHookConfig{
		MaxAge: 1 * time.Hour,
		DryRun: false,
	}
}

// HookedBead represents a bead in hooked status from bd list output.
type HookedBead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Assignee  string    `json:"assignee"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StaleHookResult represents the result of processing a stale hooked bead.
type StaleHookResult struct {
	BeadID      string `json:"bead_id"`
	Title       string `json:"title"`
	Assignee    string `json:"assignee"`
	Age         string `json:"age"`
	AgentAlive  bool   `json:"agent_alive"`
	Unhooked    bool   `json:"unhooked"`
	Error       string `json:"error,omitempty"`
}

// StaleHookScanResult contains the full results of a stale hook scan.
type StaleHookScanResult struct {
	ScannedAt   time.Time          `json:"scanned_at"`
	TotalHooked int                `json:"total_hooked"`
	StaleCount  int                `json:"stale_count"`
	Unhooked    int                `json:"unhooked"`
	Results     []*StaleHookResult `json:"results"`
}

// ScanStaleHooks finds hooked beads older than the threshold and optionally unhooks them.
func ScanStaleHooks(townRoot string, cfg *StaleHookConfig) (*StaleHookScanResult, error) {
	if cfg == nil {
		cfg = DefaultStaleHookConfig()
	}

	result := &StaleHookScanResult{
		ScannedAt: time.Now().UTC(),
		Results:   make([]*StaleHookResult, 0),
	}

	// Get all hooked beads
	hookedBeads, err := listHookedBeads(townRoot)
	if err != nil {
		return nil, fmt.Errorf("listing hooked beads: %w", err)
	}

	result.TotalHooked = len(hookedBeads)

	// Filter to stale ones (older than threshold)
	threshold := time.Now().Add(-cfg.MaxAge)

	// Use injected AgentChecker if provided, otherwise use agent.Default
	checker := cfg.AgentChecker
	if checker == nil {
		checker = agent.Default()
	}

	for _, bead := range hookedBeads {
		// Skip if updated recently (not stale)
		if bead.UpdatedAt.After(threshold) {
			continue
		}

		result.StaleCount++

		hookResult := &StaleHookResult{
			BeadID:   bead.ID,
			Title:    bead.Title,
			Assignee: bead.Assignee,
			Age:      time.Since(bead.UpdatedAt).Round(time.Minute).String(),
		}

		// Check if assignee agent is still alive
		// The assignee format (e.g., "gastown/polecat/max") IS the AgentID format
		if bead.Assignee != "" {
			hookResult.AgentAlive = checker.Exists(ids.ParseAddress(bead.Assignee))
		}

		// If agent is dead/gone and not dry run, unhook the bead
		if !hookResult.AgentAlive && !cfg.DryRun {
			if err := unhookBead(townRoot, bead.ID); err != nil {
				hookResult.Error = err.Error()
			} else {
				hookResult.Unhooked = true
				result.Unhooked++
			}
		}

		result.Results = append(result.Results, hookResult)
	}

	return result, nil
}

// listHookedBeads returns all beads with status=hooked.
func listHookedBeads(townRoot string) ([]*HookedBead, error) {
	cmd := exec.Command("bd", "list", "--status=hooked", "--json", "--limit=0")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		// No hooked beads is not an error
		if strings.Contains(string(output), "no issues found") {
			return nil, nil
		}
		return nil, err
	}

	if len(output) == 0 || string(output) == "[]" || string(output) == "null\n" {
		return nil, nil
	}

	var beads []*HookedBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing hooked beads: %w", err)
	}

	return beads, nil
}

// unhookBead sets a bead's status back to 'open'.
func unhookBead(townRoot, beadID string) error {
	cmd := exec.Command("bd", "update", beadID, "--status=open")
	cmd.Dir = townRoot
	return cmd.Run()
}
