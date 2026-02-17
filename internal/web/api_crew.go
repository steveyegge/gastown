package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/session"
)

// CrewMember represents a crew member's status for the dashboard.
type CrewMember struct {
	Name       string `json:"name"`
	Rig        string `json:"rig"`
	State      string `json:"state"`       // spinning, finished, ready, questions
	Hook       string `json:"hook,omitempty"`
	HookTitle  string `json:"hook_title,omitempty"`
	Session    string `json:"session"`     // attached, detached, none
	LastActive string `json:"last_active"`
}

// CrewResponse is the response for /api/crew.
type CrewResponse struct {
	Crew  []CrewMember            `json:"crew"`
	ByRig map[string][]CrewMember `json:"by_rig"`
	Total int                     `json:"total"`
}

// ReadyItem represents a ready work item.
type ReadyItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
	Source   string `json:"source"` // "town" or rig name
	Type     string `json:"type"`   // issue, mr, etc.
}

// ReadyResponse is the response for /api/ready.
type ReadyResponse struct {
	Items    []ReadyItem            `json:"items"`
	BySource map[string][]ReadyItem `json:"by_source"`
	Summary  struct {
		Total   int `json:"total"`
		P1Count int `json:"p1_count"`
		P2Count int `json:"p2_count"`
		P3Count int `json:"p3_count"`
	} `json:"summary"`
}

// handleCrew returns crew status across all rigs with proper state detection.
func (h *APIHandler) handleCrew(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Run gt crew list --all --json to get crew across all rigs
	output, err := h.runGtCommand(ctx, 10*time.Second, []string{"crew", "list", "--all", "--json"})

	resp := CrewResponse{
		Crew:  make([]CrewMember, 0),
		ByRig: make(map[string][]CrewMember),
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Parse the JSON output
	var crewData []struct {
		Name    string `json:"name"`
		Rig     string `json:"rig"`
		Branch  string `json:"branch"`
		Session string `json:"session,omitempty"`
		Hook    string `json:"hook,omitempty"`
	}

	if err := json.Unmarshal([]byte(output), &crewData); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Convert to CrewMember format with state detection
	for _, c := range crewData {
		sessionName := session.CrewSessionName(session.PrefixFor(c.Rig), c.Name)
		state, lastActive, sessionStatus := h.detectCrewState(ctx, sessionName, c.Hook)

		member := CrewMember{
			Name:       c.Name,
			Rig:        c.Rig,
			State:      state,
			Hook:       c.Hook,
			Session:    sessionStatus,
			LastActive: lastActive,
		}
		resp.Crew = append(resp.Crew, member)
		resp.ByRig[c.Rig] = append(resp.ByRig[c.Rig], member)
	}
	resp.Total = len(resp.Crew)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// detectCrewState determines crew member state from tmux session.
// Returns: state (spinning/finished/questions/ready), lastActive string, session status
func (h *APIHandler) detectCrewState(ctx context.Context, sessionName, hook string) (string, string, string) {
	// Check if tmux session exists and get activity
	cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}|#{window_activity}|#{session_attached}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// tmux not running - crew is ready (no session)
		return "ready", "", "none"
	}

	// Find our session
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 3 || parts[0] != sessionName {
			continue
		}

		// Found session
		var activityUnix int64
		if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err != nil {
			continue
		}
		attached := parts[2] == "1"

		sessionStatus := "detached"
		if attached {
			sessionStatus = "attached"
		}

		// Calculate activity age
		activityAge := time.Since(time.Unix(activityUnix, 0))
		lastActive := formatCrewActivityAge(activityAge)

		// Check if Claude is running in the session
		isClaudeRunning := h.isClaudeRunningInSession(ctx, sessionName)

		// Determine state based on activity and Claude status
		state := determineCrewState(activityAge, isClaudeRunning, hook)

		// Check for questions if state is potentially finished
		if state == "finished" || (state == "ready" && hook != "") {
			if h.hasQuestionInPane(ctx, sessionName) {
				state = "questions"
			}
		}

		return state, lastActive, sessionStatus
	}

	// Session not found
	return "ready", "", "none"
}

// isClaudeRunningInSession checks if Claude/agent is actively running.
func (h *APIHandler) isClaudeRunningInSession(ctx context.Context, sessionName string) bool {
	// Target pane 0 explicitly (:0.0) to avoid false positives from
	// user-created split panes running shells or other commands.
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-t", sessionName+":0.0", "-p", "#{pane_current_command}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false
	}

	output := strings.ToLower(strings.TrimSpace(stdout.String()))
	if output == "" {
		return false
	}
	// Check for common agent commands
	return strings.Contains(output, "claude") ||
		strings.Contains(output, "node") ||
		strings.Contains(output, "codex") ||
		strings.Contains(output, "opencode")
}

// hasQuestionInPane checks the last output for question indicators.
func (h *APIHandler) hasQuestionInPane(ctx context.Context, sessionName string) bool {
	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", sessionName, "-p", "-J")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false
	}

	// Get last few lines
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	lastLines := ""
	if len(lines) > 10 {
		lastLines = strings.Join(lines[len(lines)-10:], "\n")
	} else {
		lastLines = strings.Join(lines, "\n")
	}
	lastLines = strings.ToLower(lastLines)

	// Look for question indicators
	questionIndicators := []string{
		"?",
		"what do you think",
		"should i",
		"would you like",
		"please confirm",
		"waiting for",
		"need your input",
		"your thoughts",
		"let me know",
	}

	for _, indicator := range questionIndicators {
		if strings.Contains(lastLines, indicator) {
			return true
		}
	}
	return false
}

// determineCrewState determines state from activity and Claude status.
func determineCrewState(activityAge time.Duration, isClaudeRunning bool, hook string) string {
	if !isClaudeRunning {
		// Claude not running
		if hook != "" {
			return "finished" // Had work, Claude stopped = finished
		}
		return "ready" // No work, Claude stopped = ready for work
	}

	// Claude is running
	switch {
	case activityAge < 2*time.Minute:
		return "spinning" // Active recently
	case activityAge < 10*time.Minute:
		return "spinning" // Still probably working
	default:
		return "questions" // Running but no activity = likely waiting for input
	}
}

// formatCrewActivityAge formats activity age for display.
func formatCrewActivityAge(age time.Duration) string {
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	}
}

// handleReady returns ready work items across town.
func (h *APIHandler) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Run gt ready --json to get ready work
	output, err := h.runGtCommand(ctx, 12*time.Second, []string{"ready", "--json"})

	resp := ReadyResponse{
		Items:    make([]ReadyItem, 0),
		BySource: make(map[string][]ReadyItem),
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Parse the JSON output from gt ready
	var readyData struct {
		Sources []struct {
			Name   string `json:"name"`
			Issues []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				Priority int    `json:"priority"`
				Type     string `json:"type"`
			} `json:"issues"`
		} `json:"sources"`
		Summary struct {
			Total   int `json:"total"`
			P1Count int `json:"p1_count"`
			P2Count int `json:"p2_count"`
			P3Count int `json:"p3_count"`
		} `json:"summary"`
	}

	if err := json.Unmarshal([]byte(output), &readyData); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Convert to ReadyItem format
	for _, src := range readyData.Sources {
		for _, issue := range src.Issues {
			item := ReadyItem{
				ID:       issue.ID,
				Title:    issue.Title,
				Priority: issue.Priority,
				Source:   src.Name,
				Type:     issue.Type,
			}
			resp.Items = append(resp.Items, item)
			resp.BySource[src.Name] = append(resp.BySource[src.Name], item)

			// Count priorities
			switch issue.Priority {
			case 1:
				resp.Summary.P1Count++
			case 2:
				resp.Summary.P2Count++
			case 3:
				resp.Summary.P3Count++
			}
		}
	}
	resp.Summary.Total = len(resp.Items)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
