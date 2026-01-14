// Package cmd implements gt commands.
// This file implements auto-seance context recovery for cold projects.
package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wisp"
)

// SeanceConfig holds auto-seance configuration values.
type SeanceConfig struct {
	Enabled       bool
	ColdThreshold time.Duration
}

// seanceDefaults returns the default seance configuration.
func seanceDefaults() SeanceConfig {
	return SeanceConfig{
		Enabled:       true,
		ColdThreshold: 24 * time.Hour,
	}
}

// loadSeanceConfig loads seance configuration from wisp config.
// Falls back to defaults for any missing values.
func loadSeanceConfig(townRoot, rigName string) SeanceConfig {
	cfg := seanceDefaults()

	wispCfg := wisp.NewConfig(townRoot, rigName)

	// Check seance.enabled
	if val := wispCfg.Get("seance.enabled"); val != nil {
		switch v := val.(type) {
		case bool:
			cfg.Enabled = v
		case string:
			cfg.Enabled = v == "true" || v == "1" || v == "yes"
		}
	}

	// Check seance.cold_threshold
	if val := wispCfg.Get("seance.cold_threshold"); val != nil {
		if s, ok := val.(string); ok {
			if d, err := time.ParseDuration(s); err == nil {
				cfg.ColdThreshold = d
			}
		}
	}

	return cfg
}

// seanceEvent represents a session event from the events log.
type seanceEvent struct {
	Timestamp string                 `json:"ts"`
	Type      string                 `json:"type"`
	Actor     string                 `json:"actor"`
	Payload   map[string]interface{} `json:"payload"`
}

// checkColdRig returns true if the rig is "cold" (no activity within threshold).
// Also returns the timestamp of the last activity.
func checkColdRig(townRoot, rigName string, threshold time.Duration) (bool, time.Time) {
	eventsPath := filepath.Join(townRoot, events.EventsFile)

	file, err := os.Open(eventsPath)
	if err != nil {
		// No events file means new rig - definitely cold
		return true, time.Time{}
	}
	defer file.Close()

	var lastActivity time.Time
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var event seanceEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Check if event is for this rig
		if !isEventForRig(event, rigName) {
			continue
		}

		// Parse timestamp
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			continue
		}

		if ts.After(lastActivity) {
			lastActivity = ts
		}
	}

	if lastActivity.IsZero() {
		// No activity found - rig is cold (or new)
		return true, lastActivity
	}

	// Check if last activity is older than threshold
	age := time.Since(lastActivity)
	return age > threshold, lastActivity
}

// isEventForRig checks if an event is related to the specified rig.
func isEventForRig(event seanceEvent, rigName string) bool {
	// Check actor field - format is typically "rig/role/name" or just "role"
	actor := strings.ToLower(event.Actor)

	// Check if actor starts with rig name
	if strings.HasPrefix(actor, strings.ToLower(rigName)+"/") {
		return true
	}

	// Check payload for rig field
	if rig, ok := event.Payload["rig"].(string); ok {
		if strings.EqualFold(rig, rigName) {
			return true
		}
	}

	// Check payload for cwd that matches rig
	if cwd, ok := event.Payload["cwd"].(string); ok {
		if strings.Contains(cwd, "/"+rigName+"/") {
			return true
		}
	}

	return false
}

// findPredecessorSession finds the most recent session in the rig.
// Returns nil if no predecessor found.
func findPredecessorSession(townRoot, rigName, currentSessionID string) *seanceEvent {
	eventsPath := filepath.Join(townRoot, events.EventsFile)

	file, err := os.Open(eventsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var sessions []seanceEvent
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var event seanceEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Only look at session_start events
		if event.Type != events.TypeSessionStart {
			continue
		}

		// Check if event is for this rig
		if !isEventForRig(event, rigName) {
			continue
		}

		// Skip our own session
		if sessionID := getSeancePayloadString(event.Payload, "session_id"); sessionID != "" {
			if sessionID == currentSessionID {
				continue
			}
		}

		sessions = append(sessions, event)
	}

	if len(sessions) == 0 {
		return nil
	}

	// Find most recent session
	var mostRecent *seanceEvent
	var mostRecentTime time.Time

	for i := range sessions {
		ts, err := time.Parse(time.RFC3339, sessions[i].Timestamp)
		if err != nil {
			continue
		}
		if mostRecent == nil || ts.After(mostRecentTime) {
			mostRecent = &sessions[i]
			mostRecentTime = ts
		}
	}

	return mostRecent
}

// getSeancePayloadString extracts a string from a payload map.
func getSeancePayloadString(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// seanceHandoffPrompt is the standardized prompt for extracting handoff context.
const seanceHandoffPrompt = `Provide a brief handoff summary for the next agent:
1. What were you working on? (1-2 sentences)
2. What did you complete? (bullet points)
3. What's still in progress or blocked? (bullet points)
4. Any decisions or context the next agent should know?
5. Any gotchas or things that didn't work?
Keep total response under 500 words.`

// runSeanceSummary runs seance to get a summary from the predecessor session.
// Returns the summary text, or empty string on failure.
func runSeanceSummary(ctx context.Context, sessionID string) string {
	// Build the command: claude --fork-session --resume <id> --print "<prompt>"
	cmd := exec.CommandContext(ctx, "claude", "--fork-session", "--resume", sessionID, "--print", seanceHandoffPrompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Log warning but don't fail
		if stderr.Len() > 0 {
			fmt.Fprintf(os.Stderr, "seance summary: %s\n", strings.TrimSpace(stderr.String()))
		}
		return ""
	}

	return strings.TrimSpace(stdout.String())
}

// SeanceContext holds the extracted predecessor context.
type SeanceContext struct {
	PredecessorActor     string
	PredecessorSessionID string
	LastActive           time.Time
	Summary              string
}

// formatSeanceContext formats the seance context for injection into prime output.
func formatSeanceContext(sc *SeanceContext) string {
	var sb strings.Builder

	sb.WriteString("## Auto-Seance Context Recovery\n\n")
	sb.WriteString(fmt.Sprintf("Previous agent: %s (session: %s)\n", sc.PredecessorActor, sc.PredecessorSessionID))

	if !sc.LastActive.IsZero() {
		age := time.Since(sc.LastActive)
		ageStr := formatSeanceDuration(age)
		sb.WriteString(fmt.Sprintf("Last active: %s (%s ago)\n", sc.LastActive.Local().Format("2006-01-02 15:04"), ageStr))
	}

	sb.WriteString("\n### Handoff Summary\n\n")
	sb.WriteString(sc.Summary)
	sb.WriteString("\n")

	return sb.String()
}

// formatSeanceDuration formats a duration in human-readable form.
func formatSeanceDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// shouldRunAutoSeance determines if auto-seance should run for this role.
// Only crew and polecat roles benefit from auto-seance.
func shouldRunAutoSeance(role Role) bool {
	switch role {
	case RoleCrew, RolePolecat:
		return true
	default:
		// Mayor, Deacon, Witness, Refinery, Boot - infrastructure roles
		return false
	}
}

// runAutoSeance performs auto-seance context recovery if conditions are met.
// Returns the context string to inject, or empty string if seance shouldn't run.
func runAutoSeance(ctx RoleContext, explain func(bool, string)) string {
	// Check role - only crew and polecat benefit from auto-seance
	if !shouldRunAutoSeance(ctx.Role) {
		explain(false, "Auto-seance: skipped (infrastructure role)")
		return ""
	}

	// Load config
	cfg := loadSeanceConfig(ctx.TownRoot, ctx.Rig)

	// Check if enabled
	if !cfg.Enabled {
		explain(false, "Auto-seance: disabled in config")
		return ""
	}

	// Check if rig is cold
	isCold, lastActivity := checkColdRig(ctx.TownRoot, ctx.Rig, cfg.ColdThreshold)
	if !isCold {
		explain(false, fmt.Sprintf("Auto-seance: skipped (rig warm, last activity %s ago)", formatSeanceDuration(time.Since(lastActivity))))
		return ""
	}

	explain(true, fmt.Sprintf("Auto-seance: rig is cold (threshold: %s)", cfg.ColdThreshold))

	// Get current session ID to exclude from predecessor search
	currentSessionID := os.Getenv("GT_SESSION_ID")
	if currentSessionID == "" {
		currentSessionID = os.Getenv("CLAUDE_SESSION_ID")
	}

	// Find predecessor session
	predecessor := findPredecessorSession(ctx.TownRoot, ctx.Rig, currentSessionID)
	if predecessor == nil {
		explain(false, "Auto-seance: no predecessor session found")
		return ""
	}

	predecessorSessionID := getSeancePayloadString(predecessor.Payload, "session_id")
	if predecessorSessionID == "" {
		explain(false, "Auto-seance: predecessor has no session ID")
		return ""
	}

	explain(true, fmt.Sprintf("Auto-seance: found predecessor %s (session: %s)", predecessor.Actor, predecessorSessionID))

	// Run seance with timeout
	seanceCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(os.Stderr, "%s Running auto-seance to recover context from %s...\n", style.Dim.Render("⏳"), predecessor.Actor)

	summary := runSeanceSummary(seanceCtx, predecessorSessionID)
	if summary == "" {
		explain(false, "Auto-seance: failed to get summary (timeout or error)")
		return ""
	}

	explain(true, "Auto-seance: successfully recovered context")

	// Parse predecessor timestamp
	predecessorTime, _ := time.Parse(time.RFC3339, predecessor.Timestamp)

	// Format and return context
	sc := &SeanceContext{
		PredecessorActor:     predecessor.Actor,
		PredecessorSessionID: predecessorSessionID,
		LastActive:           predecessorTime,
		Summary:              summary,
	}

	return formatSeanceContext(sc)
}

// outputAutoSeanceContext runs auto-seance and outputs the result.
// This should be called early in prime, before role context output.
func outputAutoSeanceContext(ctx RoleContext) {
	// Skip in dry-run mode
	if primeDryRun {
		explain(false, "Auto-seance: skipped in dry-run mode")
		return
	}

	context := runAutoSeance(ctx, explain)
	if context != "" {
		fmt.Println()
		fmt.Println(context)
	}
}
