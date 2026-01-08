// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// StaleHookConfig holds configurable parameters for stale hook detection.
type StaleHookConfig struct {
	// MaxAge is how long a bead can be hooked before being considered stale.
	MaxAge time.Duration `json:"max_age"`
	// DryRun if true, only reports what would be done without making changes.
	DryRun bool `json:"dry_run"`
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
	t := tmux.NewTmux()

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
		if bead.Assignee != "" {
			sessionName := assigneeToSessionName(bead.Assignee)
			if sessionName != "" {
				alive, _ := t.HasSession(sessionName)
				hookResult.AgentAlive = alive
			}
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

// assigneeToSessionName converts an assignee address to a tmux session name.
// Supports formats like "gastown/polecats/max", "gastown/crew/joe", etc.
func assigneeToSessionName(assignee string) string {
	parts := strings.Split(assignee, "/")

	switch len(parts) {
	case 1:
		// Simple names like "deacon", "mayor"
		switch assignee {
		case "deacon":
			return session.DeaconSessionName()
		case "mayor":
			return session.MayorSessionName()
		default:
			return ""
		}
	case 2:
		// rig/role: "gastown/witness", "gastown/refinery"
		rig, role := parts[0], parts[1]
		switch role {
		case "witness", "refinery":
			return fmt.Sprintf("gt-%s-%s", rig, role)
		default:
			return ""
		}
	case 3:
		// rig/type/name: "gastown/polecats/max", "gastown/crew/joe"
		rig, agentType, name := parts[0], parts[1], parts[2]
		switch agentType {
		case "polecats":
			return fmt.Sprintf("gt-%s-%s", rig, name)
		case "crew":
			return fmt.Sprintf("gt-%s-crew-%s", rig, name)
		default:
			return ""
		}
	default:
		return ""
	}
}

// unhookBead sets a bead's status back to 'open'.
func unhookBead(townRoot, beadID string) error {
	cmd := exec.Command("bd", "update", beadID, "--status=open")
	cmd.Dir = townRoot
	return cmd.Run()
}
