package sling

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
)

// VerifyBeadExists checks that the bead exists using bd show.
// Uses bd's native prefix-based routing via routes.jsonl.
func VerifyBeadExists(beadID, townRoot string) error {
	cmd := bdcmd.Command( "show", beadID, "--json", "--allow-stale")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("bead '%s' not found (bd show failed)", beadID)
	}
	if len(out) == 0 {
		return fmt.Errorf("bead '%s' not found (empty response)", beadID)
	}
	return nil
}

// GetBeadInfo returns status and assignee for a bead.
func GetBeadInfo(beadID, townRoot string) (*BeadInfo, error) {
	cmd := bdcmd.Command( "show", beadID, "--json", "--allow-stale")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bead '%s' not found (empty response)", beadID)
	}
	var infos []BeadInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return nil, fmt.Errorf("parsing bead info: %w", err)
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	return &infos[0], nil
}

// VerifyFormulaExists checks that the formula exists using bd formula show.
func VerifyFormulaExists(formulaName string) error {
	cmd := bdcmd.Command( "formula", "show", formulaName, "--allow-stale")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return nil
	}
	cmd = bdcmd.Command( "formula", "show", "mol-"+formulaName, "--allow-stale")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return nil
	}
	return fmt.Errorf("formula '%s' not found (check 'bd formula list')", formulaName)
}

// LooksLikeBeadID checks if a string looks like a bead ID.
func LooksLikeBeadID(s string) bool {
	idx := strings.Index(s, "-")
	if idx < 1 || idx > 5 {
		return false
	}
	prefix := s[:idx]
	for _, c := range prefix {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	rest := s[idx+1:]
	if len(rest) == 0 {
		return false
	}
	first := rest[0]
	if !((first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')) {
		return false
	}
	return true
}

// CountHookedBeadsForAgent counts how many hooked beads are assigned to an agent.
func CountHookedBeadsForAgent(townRoot, agentID string) int {
	client := beads.New(beads.GetTownBeadsPath(townRoot))
	issues, err := client.List(beads.ListOptions{
		Status:   "hooked",
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return 0
	}
	return len(issues)
}

// GetBeadInstructions reads the instructions (title) from a bead.
func GetBeadInstructions(beadID string) string {
	cmd := bdcmd.Command( "show", beadID, "--json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		return ""
	}
	return issues[0].Title
}

// GetBeadBase reads the base branch from a bead, defaulting to "main".
func GetBeadBase(beadID string) string {
	cmd := bdcmd.Command( "show", beadID, "--json", "--allow-stale")
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		return "main"
	}
	for _, label := range issues[0].Labels {
		if strings.HasPrefix(label, "base:") {
			return strings.TrimPrefix(label, "base:")
		}
	}
	return "main"
}
