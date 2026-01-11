package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/checkpoint"
	"github.com/steveyegge/gastown/internal/constants"
)

// SessionState represents the detected session state for observability.
type SessionState struct {
	State         string `json:"state"`                    // normal, post-handoff, crash-recovery, autonomous
	Role          Role   `json:"role"`                     // detected role
	PrevSession   string `json:"prev_session,omitempty"`   // for post-handoff
	CheckpointAge string `json:"checkpoint_age,omitempty"` // for crash-recovery
	HookedBead    string `json:"hooked_bead,omitempty"`    // for autonomous
}

// detectSessionState returns the current session state without side effects.
func detectSessionState(ctx RoleContext) SessionState {
	state := SessionState{
		State: "normal",
		Role:  ctx.Role,
	}

	// Check for handoff marker (post-handoff state)
	markerPath := filepath.Join(ctx.WorkDir, constants.DirRuntime, constants.FileHandoffMarker)
	if data, err := os.ReadFile(markerPath); err == nil {
		state.State = "post-handoff"
		state.PrevSession = strings.TrimSpace(string(data))
		return state
	}

	// Check for checkpoint (crash-recovery state) - only for polecat/crew
	if ctx.Role == RolePolecat || ctx.Role == RoleCrew {
		if cp, err := checkpoint.Read(ctx.WorkDir); err == nil && cp != nil && !cp.IsStale(24*time.Hour) {
			state.State = "crash-recovery"
			state.CheckpointAge = cp.Age().Round(time.Minute).String()
			return state
		}
	}

	// Check for hooked work (autonomous state)
	agentID := getAgentIdentity(ctx)
	if agentID != "" {
		b := beads.New(ctx.WorkDir)
		hookedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusHooked,
			Assignee: agentID,
			Priority: -1,
		})
		if err == nil && len(hookedBeads) > 0 {
			state.State = "autonomous"
			state.HookedBead = hookedBeads[0].ID
			return state
		}
		// Also check in_progress beads
		inProgressBeads, err := b.List(beads.ListOptions{
			Status:   "in_progress",
			Assignee: agentID,
			Priority: -1,
		})
		if err == nil && len(inProgressBeads) > 0 {
			state.State = "autonomous"
			state.HookedBead = inProgressBeads[0].ID
			return state
		}
	}

	return state
}

// checkHandoffMarker checks for a handoff marker file and outputs a warning if found.
// This prevents the "handoff loop" bug where a new session sees /handoff in context
// and incorrectly runs it again. The marker tells the new session: "handoff is DONE,
// the /handoff you see in context was from YOUR PREDECESSOR, not a request for you."
func checkHandoffMarker(workDir string) {
	markerPath := filepath.Join(workDir, constants.DirRuntime, constants.FileHandoffMarker)
	data, err := os.ReadFile(markerPath)
	if err != nil {
		// No marker = not post-handoff, normal startup
		return
	}

	// Marker found - this is a post-handoff session
	prevSession := strings.TrimSpace(string(data))

	// Remove the marker FIRST so we don't warn twice
	_ = os.Remove(markerPath)

	// Output prominent warning
	outputHandoffWarning(prevSession)
}

// checkHandoffMarkerDryRun checks for handoff marker without removing it (for --dry-run).
func checkHandoffMarkerDryRun(workDir string) {
	markerPath := filepath.Join(workDir, constants.DirRuntime, constants.FileHandoffMarker)
	data, err := os.ReadFile(markerPath)
	if err != nil {
		// No marker = not post-handoff, normal startup
		explain(true, "Post-handoff: no handoff marker found")
		return
	}

	// Marker found - this is a post-handoff session
	prevSession := strings.TrimSpace(string(data))
	explain(true, fmt.Sprintf("Post-handoff: marker found (predecessor: %s), marker NOT removed in dry-run", prevSession))

	// Output the warning but don't remove marker
	outputHandoffWarning(prevSession)
}
