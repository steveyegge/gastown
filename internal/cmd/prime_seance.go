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
	Timeout       time.Duration // Timeout for seance API calls
	MinSessionAge time.Duration // Skip predecessors younger than this (likely still in handoff)
	CacheTTL      time.Duration // How long to cache seance results
}

// SeanceResult captures the outcome of an auto-seance attempt for diagnostics.
type SeanceResult struct {
	Ran           bool
	SkipReason    string
	PredecessorID string
	Duration      time.Duration
	Error         error
	CacheHit      bool
}

// seanceCacheEntry represents a cached seance summary.
type seanceCacheEntry struct {
	SessionID string
	Summary   string
	Timestamp time.Time
}

// seanceCache stores cached seance summaries to avoid redundant API calls.
// Key is predecessor session ID, value is the cached summary.
var seanceCache = make(map[string]*seanceCacheEntry)

// getSeanceCachePath returns the path to the seance cache file.
func getSeanceCachePath(townRoot string) string {
	return filepath.Join(townRoot, ".beads-wisp", "seance-cache.json")
}

// loadSeanceCache loads the seance cache from disk.
func loadSeanceCache(townRoot string) {
	cachePath := getSeanceCachePath(townRoot)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return // Cache miss is fine
	}

	var entries map[string]*seanceCacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}

	seanceCache = entries
}

// saveSeanceCache saves the seance cache to disk.
func saveSeanceCache(townRoot string) {
	cachePath := getSeanceCachePath(townRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return
	}

	data, err := json.MarshalIndent(seanceCache, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(cachePath, data, 0644)
}

// getCachedSeanceSummary returns a cached summary if valid, or empty string if not.
func getCachedSeanceSummary(sessionID string, ttl time.Duration) (string, bool) {
	entry, ok := seanceCache[sessionID]
	if !ok {
		return "", false
	}

	// Check if cache entry is still valid
	if time.Since(entry.Timestamp) > ttl {
		delete(seanceCache, sessionID)
		return "", false
	}

	return entry.Summary, true
}

// setCachedSeanceSummary stores a summary in the cache.
func setCachedSeanceSummary(sessionID, summary string) {
	seanceCache[sessionID] = &seanceCacheEntry{
		SessionID: sessionID,
		Summary:   summary,
		Timestamp: time.Now(),
	}
}

// cleanStaleCache removes expired entries from the cache.
func cleanStaleCache(ttl time.Duration) {
	now := time.Now()
	for id, entry := range seanceCache {
		if now.Sub(entry.Timestamp) > ttl {
			delete(seanceCache, id)
		}
	}
}

// isSameAgent checks if the predecessor is the same agent (self-restart case).
func isSameAgent(predecessorActor, currentActor string) bool {
	if predecessorActor == "" || currentActor == "" {
		return false
	}
	return strings.EqualFold(predecessorActor, currentActor)
}

// seanceDefaults returns the default seance configuration.
func seanceDefaults() SeanceConfig {
	return SeanceConfig{
		Enabled:       true,
		ColdThreshold: 24 * time.Hour,
		Timeout:       30 * time.Second,
		MinSessionAge: 1 * time.Hour,  // Skip very recent sessions (handoff mail suffices)
		CacheTTL:      1 * time.Hour,  // Cache seance summaries for 1 hour
	}
}

// loadSeanceConfig loads seance configuration from wisp config.
// Falls back to defaults for any missing values.
func loadSeanceConfig(townRoot, rigName string) SeanceConfig {
	cfg := seanceDefaults()

	// Handle empty rig name gracefully
	if rigName == "" {
		return cfg
	}

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
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				cfg.ColdThreshold = d
			}
		}
	}

	// Check seance.timeout
	if val := wispCfg.Get("seance.timeout"); val != nil {
		if s, ok := val.(string); ok {
			if d, err := time.ParseDuration(s); err == nil && d > 0 {
				cfg.Timeout = d
			}
		}
	}

	// Check seance.min_session_age
	if val := wispCfg.Get("seance.min_session_age"); val != nil {
		if s, ok := val.(string); ok {
			if d, err := time.ParseDuration(s); err == nil && d >= 0 {
				cfg.MinSessionAge = d
			}
		}
	}

	// Check seance.cache_ttl
	if val := wispCfg.Get("seance.cache_ttl"); val != nil {
		if s, ok := val.(string); ok {
			if d, err := time.ParseDuration(s); err == nil && d >= 0 {
				cfg.CacheTTL = d
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

// activityEventTypes defines which event types count as "activity" for cold detection.
// Not all events indicate meaningful activity worth seancing for.
var activityEventTypes = map[string]bool{
	events.TypeSessionStart: true,
	events.TypeSessionEnd:   true,
	events.TypeSling:        true,
	events.TypeHook:         true,
	events.TypeDone:         true,
	events.TypeHandoff:      true,
}

// maxEventsToScan limits how many events we scan for cold detection.
// This prevents performance issues with very large event files.
const maxEventsToScan = 10000

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
	var scannedCount int
	now := time.Now()
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	// Collect recent events in a slice for efficient processing
	// (We read forward but events are appended, so recent ones are at the end)
	var recentEvents []seanceEvent

	for scanner.Scan() {
		scannedCount++
		if scannedCount > maxEventsToScan {
			// Only keep events from last maxEventsToScan
			// (older events will be dropped from beginning of slice)
			if len(recentEvents) > 0 {
				recentEvents = recentEvents[1:]
			}
		}

		var event seanceEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Only consider meaningful activity event types
		if !activityEventTypes[event.Type] {
			continue
		}

		// Check if event is for this rig
		if !isEventForRig(event, rigName) {
			continue
		}

		recentEvents = append(recentEvents, event)
	}

	// Process events in reverse order (most recent first)
	for i := len(recentEvents) - 1; i >= 0; i-- {
		event := recentEvents[i]

		// Parse timestamp
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			continue
		}

		if ts.After(lastActivity) {
			lastActivity = ts

			// Early termination: if we found activity within threshold, rig is warm
			if now.Sub(ts) <= threshold {
				return false, lastActivity
			}
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
	if rigName == "" {
		return false
	}

	rigLower := strings.ToLower(rigName)

	// Check actor field - format is typically "rig/role/name" or just "role"
	actor := strings.ToLower(event.Actor)

	// Check if actor starts with rig name followed by slash
	if strings.HasPrefix(actor, rigLower+"/") {
		return true
	}

	// Check if actor exactly matches rig name (rare but possible)
	if actor == rigLower {
		return true
	}

	// Check payload for rig field (most reliable)
	if rig, ok := event.Payload["rig"].(string); ok {
		if strings.EqualFold(rig, rigName) {
			return true
		}
	}

	// Check payload for cwd that contains rig path
	if cwd, ok := event.Payload["cwd"].(string); ok {
		// Look for /rigname/ pattern to avoid false matches
		// e.g., /home/user/gt/gastown/crew/joe should match "gastown"
		cwdLower := strings.ToLower(cwd)
		if strings.Contains(cwdLower, "/"+rigLower+"/") {
			return true
		}
		// Also check if cwd ends with /rigname (rig root directory)
		if strings.HasSuffix(cwdLower, "/"+rigLower) {
			return true
		}
	}

	// Check payload for target field (used in sling events)
	if target, ok := event.Payload["target"].(string); ok {
		targetLower := strings.ToLower(target)
		if strings.HasPrefix(targetLower, rigLower+"/") || targetLower == rigLower {
			return true
		}
	}

	return false
}

// maxSessionsToTrack limits how many sessions we track when looking for predecessor.
const maxSessionsToTrack = 100

// findPredecessorSession finds the most recent session in the rig.
// Sessions younger than minAge are skipped (handoff mail suffices for recent sessions).
// Returns nil if no predecessor found.
func findPredecessorSession(townRoot, rigName, currentSessionID string, minAge time.Duration) *seanceEvent {
	eventsPath := filepath.Join(townRoot, events.EventsFile)

	file, err := os.Open(eventsPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Use a ring buffer approach - keep only recent sessions
	sessions := make([]seanceEvent, 0, maxSessionsToTrack)
	scanner := bufio.NewScanner(file)

	// Increase buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	now := time.Now()

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

		// Skip sessions that are too recent (handoff mail suffices)
		if ts, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
			if now.Sub(ts) < minAge {
				continue
			}
		}

		// Keep only recent sessions (ring buffer behavior)
		if len(sessions) >= maxSessionsToTrack {
			sessions = sessions[1:]
		}
		sessions = append(sessions, event)
	}

	if len(sessions) == 0 {
		return nil
	}

	// Find most recent session (iterate backwards since most recent is likely at end)
	var mostRecent *seanceEvent
	var mostRecentTime time.Time

	for i := len(sessions) - 1; i >= 0; i-- {
		ts, err := time.Parse(time.RFC3339, sessions[i].Timestamp)
		if err != nil {
			continue
		}
		if mostRecent == nil || ts.After(mostRecentTime) {
			mostRecent = &sessions[i]
			mostRecentTime = ts
		}
	}

	// Return a copy to avoid slice reference issues
	if mostRecent != nil {
		result := *mostRecent
		return &result
	}

	return nil
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
// Deprecated: Use runSeanceSummaryWithError for better error handling.
func runSeanceSummary(ctx context.Context, sessionID string) string {
	summary, _ := runSeanceSummaryWithError(ctx, sessionID)
	return summary
}

// runSeanceSummaryWithError runs seance to get a summary from the predecessor session.
// Returns the summary text and any error encountered.
func runSeanceSummaryWithError(ctx context.Context, sessionID string) (string, error) {
	// Validate session ID to avoid command injection
	if sessionID == "" {
		return "", fmt.Errorf("empty session ID")
	}
	if strings.ContainsAny(sessionID, ";&|`$(){}[]<>\\\"'") {
		return "", fmt.Errorf("invalid session ID characters")
	}

	// Build the command: claude --fork-session --resume <id> --print "<prompt>"
	cmd := exec.CommandContext(ctx, "claude", "--fork-session", "--resume", sessionID, "--print", seanceHandoffPrompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check for context cancellation/timeout
		if ctx.Err() != nil {
			return "", fmt.Errorf("seance canceled: %w", ctx.Err())
		}

		// Include stderr in error message if available
		errMsg := err.Error()
		if stderrStr := strings.TrimSpace(stderr.String()); stderrStr != "" {
			errMsg = fmt.Sprintf("%s: %s", errMsg, stderrStr)
		}
		return "", fmt.Errorf("seance failed: %s", errMsg)
	}

	return strings.TrimSpace(stdout.String()), nil
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

	// Truncate session ID for readability (first 12 chars)
	sessionIDDisplay := sc.PredecessorSessionID
	if len(sessionIDDisplay) > 12 {
		sessionIDDisplay = sessionIDDisplay[:12] + "…"
	}

	sb.WriteString(fmt.Sprintf("Previous agent: %s (session: %s)\n", sc.PredecessorActor, sessionIDDisplay))

	if !sc.LastActive.IsZero() {
		age := time.Since(sc.LastActive)
		ageStr := formatSeanceDuration(age)
		sb.WriteString(fmt.Sprintf("Last active: %s (%s ago)\n", sc.LastActive.Local().Format("2006-01-02 15:04"), ageStr))
	}

	sb.WriteString("\n### Handoff Summary\n\n")

	// Ensure summary doesn't have excessive whitespace
	summary := strings.TrimSpace(sc.Summary)
	if summary == "" {
		summary = "(No summary available)"
	}
	sb.WriteString(summary)
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
	startTime := time.Now()
	result := &SeanceResult{}

	defer func() {
		result.Duration = time.Since(startTime)
		// Log result for debugging (only if seance was attempted)
		if result.Ran && primeExplain {
			if result.Error != nil {
				explain(false, fmt.Sprintf("Auto-seance: completed in %v with error: %v", result.Duration, result.Error))
			} else {
				explain(true, fmt.Sprintf("Auto-seance: completed in %v", result.Duration))
			}
		}
	}()

	// Check role - only crew and polecat benefit from auto-seance
	if !shouldRunAutoSeance(ctx.Role) {
		result.SkipReason = "infrastructure role"
		explain(false, "Auto-seance: skipped (infrastructure role)")
		return ""
	}

	// Load config
	cfg := loadSeanceConfig(ctx.TownRoot, ctx.Rig)

	// Check if enabled
	if !cfg.Enabled {
		result.SkipReason = "disabled in config"
		explain(false, "Auto-seance: disabled in config")
		return ""
	}

	// Check if rig is cold
	isCold, lastActivity := checkColdRig(ctx.TownRoot, ctx.Rig, cfg.ColdThreshold)
	if !isCold {
		result.SkipReason = "rig is warm"
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
	predecessor := findPredecessorSession(ctx.TownRoot, ctx.Rig, currentSessionID, cfg.MinSessionAge)
	if predecessor == nil {
		result.SkipReason = "no predecessor found"
		explain(false, "Auto-seance: no predecessor session found")
		return ""
	}

	predecessorSessionID := getSeancePayloadString(predecessor.Payload, "session_id")
	if predecessorSessionID == "" {
		result.SkipReason = "predecessor has no session ID"
		explain(false, "Auto-seance: predecessor has no session ID")
		return ""
	}

	result.PredecessorID = predecessorSessionID
	explain(true, fmt.Sprintf("Auto-seance: found predecessor %s (session: %s)", predecessor.Actor, predecessorSessionID))

	// Load and check cache first
	loadSeanceCache(ctx.TownRoot)
	if cachedSummary, hit := getCachedSeanceSummary(predecessorSessionID, cfg.CacheTTL); hit {
		result.CacheHit = true
		explain(true, "Auto-seance: using cached summary")
		predecessorTime, _ := time.Parse(time.RFC3339, predecessor.Timestamp)
		sc := &SeanceContext{
			PredecessorActor:     predecessor.Actor,
			PredecessorSessionID: predecessorSessionID,
			LastActive:           predecessorTime,
			Summary:              cachedSummary,
		}
		return formatSeanceContext(sc)
	}

	// Run seance with configurable timeout
	result.Ran = true
	seanceCtx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "%s Running auto-seance to recover context from %s...\n", style.Dim.Render("⏳"), predecessor.Actor)

	summary, err := runSeanceSummaryWithError(seanceCtx, predecessorSessionID)
	if err != nil {
		result.Error = err
		if seanceCtx.Err() == context.DeadlineExceeded {
			explain(false, fmt.Sprintf("Auto-seance: timed out after %v", cfg.Timeout))
		} else {
			explain(false, fmt.Sprintf("Auto-seance: failed (%v)", err))
		}
		return ""
	}

	if summary == "" {
		result.SkipReason = "empty summary"
		explain(false, "Auto-seance: predecessor returned empty summary")
		return ""
	}

	// Cache the successful result
	setCachedSeanceSummary(predecessorSessionID, summary)
	saveSeanceCache(ctx.TownRoot)

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
