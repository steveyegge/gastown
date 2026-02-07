// Package cmd provides OJ nudge helpers for gt nudge and sling.
// When a target polecat is OJ-managed (has oj_job_id in its hooked bead),
// nudges are routed through "oj agent send" instead of tmux (od-ki9.4).
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getOjJobIDForPolecat looks up the OJ job ID for a polecat by resolving:
// polecat agent → agent bead → hook slot → hooked bead → oj_job_id.
// Returns empty string if the polecat is not OJ-managed or on any error.
func getOjJobIDForPolecat(townRoot, rigName, polecatName string) string {
	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return ""
	}

	// Get hook_bead from agent bead via bd slot get
	cmd := exec.Command("bd", "slot", "get", agentBeadID, "hook")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	hookBeadID := strings.TrimSpace(string(out))
	if hookBeadID == "" {
		return ""
	}

	return getOjJobIDFromBead(hookBeadID)
}

// getOjJobIDFromBead reads the oj_job_id from a bead's description.
// Returns empty string if not found or on error.
func getOjJobIDFromBead(beadID string) string {
	cmd := exec.Command("bd", "show", beadID, "--json", "--allow-stale")
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		cmd.Dir = townRoot
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		return ""
	}

	fields := beads.ParseAttachmentFields(&issues[0])
	if fields == nil {
		return ""
	}
	return fields.OjJobID
}

// sendViaOj sends a message to an OJ-managed agent using "oj agent send".
// This is the canonical nudge method for OJ-managed polecats (od-ki9.4).
func sendViaOj(ojJobID, message string) error {
	cmd := exec.Command("oj", "agent", "send", ojJobID, message)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
