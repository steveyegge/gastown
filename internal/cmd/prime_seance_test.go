package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

// =============================================================================
// SeanceConfig Tests
// =============================================================================

func TestSeanceDefaults(t *testing.T) {
	cfg := seanceDefaults()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.ColdThreshold != 24*time.Hour {
		t.Errorf("expected ColdThreshold to be 24h, got %v", cfg.ColdThreshold)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout to be 30s, got %v", cfg.Timeout)
	}
	if cfg.MinSessionAge != 1*time.Hour {
		t.Errorf("expected MinSessionAge to be 1h, got %v", cfg.MinSessionAge)
	}
	if cfg.CacheTTL != 1*time.Hour {
		t.Errorf("expected CacheTTL to be 1h, got %v", cfg.CacheTTL)
	}
}

func TestLoadSeanceConfigDefaults(t *testing.T) {
	// Test with empty rig name - should return defaults
	cfg := loadSeanceConfig("/tmp/nonexistent", "")
	defaults := seanceDefaults()

	if cfg.Enabled != defaults.Enabled {
		t.Errorf("expected default Enabled, got %v", cfg.Enabled)
	}
	if cfg.ColdThreshold != defaults.ColdThreshold {
		t.Errorf("expected default ColdThreshold, got %v", cfg.ColdThreshold)
	}
}

func TestLoadSeanceConfigFromWisp(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "seance-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create wisp config directory
	configDir := filepath.Join(tmpDir, ".beads-wisp", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write config file
	configData := map[string]interface{}{
		"rig": "testrig",
		"values": map[string]interface{}{
			"seance.enabled":         false,
			"seance.cold_threshold":  "12h",
			"seance.timeout":         "45s",
			"seance.min_session_age": "30m",
			"seance.cache_ttl":       "2h",
		},
		"blocked": []string{},
	}
	data, _ := json.MarshalIndent(configData, "", "  ")
	configPath := filepath.Join(configDir, "testrig.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config
	cfg := loadSeanceConfig(tmpDir, "testrig")

	if cfg.Enabled != false {
		t.Errorf("expected Enabled=false, got %v", cfg.Enabled)
	}
	if cfg.ColdThreshold != 12*time.Hour {
		t.Errorf("expected ColdThreshold=12h, got %v", cfg.ColdThreshold)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("expected Timeout=45s, got %v", cfg.Timeout)
	}
	if cfg.MinSessionAge != 30*time.Minute {
		t.Errorf("expected MinSessionAge=30m, got %v", cfg.MinSessionAge)
	}
	if cfg.CacheTTL != 2*time.Hour {
		t.Errorf("expected CacheTTL=2h, got %v", cfg.CacheTTL)
	}
}

func TestLoadSeanceConfigInvalidDurations(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "seance-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create wisp config with invalid values
	configDir := filepath.Join(tmpDir, ".beads-wisp", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configData := map[string]interface{}{
		"rig": "testrig",
		"values": map[string]interface{}{
			"seance.cold_threshold": "invalid",
			"seance.timeout":        "-5s", // negative should be rejected
		},
		"blocked": []string{},
	}
	data, _ := json.MarshalIndent(configData, "", "  ")
	configPath := filepath.Join(configDir, "testrig.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config - should fall back to defaults for invalid values
	cfg := loadSeanceConfig(tmpDir, "testrig")
	defaults := seanceDefaults()

	if cfg.ColdThreshold != defaults.ColdThreshold {
		t.Errorf("expected default ColdThreshold for invalid value, got %v", cfg.ColdThreshold)
	}
	if cfg.Timeout != defaults.Timeout {
		t.Errorf("expected default Timeout for negative value, got %v", cfg.Timeout)
	}
}

// =============================================================================
// Role Filter Tests
// =============================================================================

func TestShouldRunAutoSeance(t *testing.T) {
	tests := []struct {
		role   Role
		expect bool
	}{
		{RoleCrew, true},
		{RolePolecat, true},
		{RoleMayor, false},
		{RoleDeacon, false},
		{RoleWitness, false},
		{RoleRefinery, false},
		{RoleBoot, false},
		{RoleUnknown, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.role), func(t *testing.T) {
			got := shouldRunAutoSeance(tc.role)
			if got != tc.expect {
				t.Errorf("shouldRunAutoSeance(%s) = %v, want %v", tc.role, got, tc.expect)
			}
		})
	}
}

// =============================================================================
// Event Filtering Tests
// =============================================================================

func TestIsEventForRig(t *testing.T) {
	tests := []struct {
		name    string
		event   seanceEvent
		rigName string
		expect  bool
	}{
		{
			name: "actor starts with rig",
			event: seanceEvent{
				Actor: "gastown/crew/joe",
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "actor different rig",
			event: seanceEvent{
				Actor: "beads/crew/wolf",
			},
			rigName: "gastown",
			expect:  false,
		},
		{
			name: "payload has rig field",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"rig": "gastown"},
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "cwd contains rig path",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"cwd": "/home/user/gt/gastown/crew/joe"},
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "cwd ends with rig",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"cwd": "/home/user/gt/gastown"},
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "target field matches rig",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"target": "gastown/crew/joe"},
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "target field exact match",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"target": "gastown"},
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "actor exact match",
			event: seanceEvent{
				Actor: "gastown",
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "case insensitive match",
			event: seanceEvent{
				Actor: "GASTOWN/crew/joe",
			},
			rigName: "gastown",
			expect:  true,
		},
		{
			name: "no match",
			event: seanceEvent{
				Actor:   "deacon",
				Payload: map[string]interface{}{},
			},
			rigName: "gastown",
			expect:  false,
		},
		{
			name: "empty rig name",
			event: seanceEvent{
				Actor: "gastown/crew/joe",
			},
			rigName: "",
			expect:  false,
		},
		{
			name: "partial rig name in path should not match",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"cwd": "/home/user/gt/gastown-old/crew/joe"},
			},
			rigName: "gastown",
			expect:  false, // gastown-old does NOT contain /gastown/ as a path segment
		},
		{
			name: "nil payload",
			event: seanceEvent{
				Actor:   "other",
				Payload: nil,
			},
			rigName: "gastown",
			expect:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isEventForRig(tc.event, tc.rigName)
			if got != tc.expect {
				t.Errorf("isEventForRig() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestActivityEventTypes(t *testing.T) {
	// Verify the correct event types are included
	expectedTypes := []string{
		events.TypeSessionStart,
		events.TypeSessionEnd,
		events.TypeSling,
		events.TypeHook,
		events.TypeDone,
		events.TypeHandoff,
	}

	for _, eventType := range expectedTypes {
		if !activityEventTypes[eventType] {
			t.Errorf("expected %s to be an activity event type", eventType)
		}
	}

	// Verify some types are NOT included
	nonActivityTypes := []string{
		events.TypeMail,
		events.TypeSpawn,
		events.TypeKill,
		events.TypePatrolStarted,
	}

	for _, eventType := range nonActivityTypes {
		if activityEventTypes[eventType] {
			t.Errorf("expected %s to NOT be an activity event type", eventType)
		}
	}
}

// =============================================================================
// Cold Rig Detection Tests
// =============================================================================

func TestCheckColdRig(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with no events file - should be cold
	isCold, lastActivity := checkColdRig(tmpDir, "testrig", 24*time.Hour)
	if !isCold {
		t.Error("expected rig to be cold with no events file")
	}
	if !lastActivity.IsZero() {
		t.Errorf("expected zero time for no activity, got %v", lastActivity)
	}

	// Create events file with recent activity
	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	recentTime := time.Now().Add(-1 * time.Hour).UTC()
	event := map[string]interface{}{
		"ts":     recentTime.Format(time.RFC3339),
		"type":   events.TypeSessionStart, // Use activity event type
		"actor":  "testrig/crew/joe",
		"source": "gt",
	}
	data, _ := json.Marshal(event)
	data = append(data, '\n')
	if err := os.WriteFile(eventsPath, data, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// Test with recent activity - should be warm
	isCold, lastActivity = checkColdRig(tmpDir, "testrig", 24*time.Hour)
	if isCold {
		t.Error("expected rig to be warm with recent activity")
	}
	if time.Since(lastActivity) > 2*time.Hour {
		t.Errorf("expected recent activity, got %v ago", time.Since(lastActivity))
	}

	// Create events file with old activity
	oldTime := time.Now().Add(-48 * time.Hour).UTC()
	event["ts"] = oldTime.Format(time.RFC3339)
	data, _ = json.Marshal(event)
	data = append(data, '\n')
	if err := os.WriteFile(eventsPath, data, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// Test with old activity - should be cold
	isCold, _ = checkColdRig(tmpDir, "testrig", 24*time.Hour)
	if !isCold {
		t.Error("expected rig to be cold with old activity")
	}
}

func TestCheckColdRigFiltersEventTypes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)

	// Add recent mail event (should NOT count as activity)
	mailEvent := map[string]interface{}{
		"ts":     time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeMail, // Not an activity type
		"actor":  "testrig/crew/joe",
		"source": "gt",
	}
	data, _ := json.Marshal(mailEvent)
	data = append(data, '\n')

	// Add old session_start event (should count)
	sessionEvent := map[string]interface{}{
		"ts":     time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "testrig/crew/joe",
		"source": "gt",
	}
	sessionData, _ := json.Marshal(sessionEvent)
	data = append(data, sessionData...)
	data = append(data, '\n')

	if err := os.WriteFile(eventsPath, data, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// Should be cold - mail event doesn't count, session is old
	isCold, _ := checkColdRig(tmpDir, "testrig", 24*time.Hour)
	if !isCold {
		t.Error("expected rig to be cold (mail event shouldn't count)")
	}
}

func TestCheckColdRigMultipleRigs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	var eventsData []byte

	// Add recent activity for rig1
	rig1Event := map[string]interface{}{
		"ts":     time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "rig1/crew/joe",
		"source": "gt",
	}
	data, _ := json.Marshal(rig1Event)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	// Add old activity for rig2
	rig2Event := map[string]interface{}{
		"ts":     time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "rig2/crew/max",
		"source": "gt",
	}
	data, _ = json.Marshal(rig2Event)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	if err := os.WriteFile(eventsPath, eventsData, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// rig1 should be warm
	isCold, _ := checkColdRig(tmpDir, "rig1", 24*time.Hour)
	if isCold {
		t.Error("expected rig1 to be warm")
	}

	// rig2 should be cold
	isCold, _ = checkColdRig(tmpDir, "rig2", 24*time.Hour)
	if !isCold {
		t.Error("expected rig2 to be cold")
	}

	// rig3 (no events) should be cold
	isCold, _ = checkColdRig(tmpDir, "rig3", 24*time.Hour)
	if !isCold {
		t.Error("expected rig3 to be cold (no events)")
	}
}

// =============================================================================
// Predecessor Session Tests
// =============================================================================

func TestFindPredecessorSession(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with no events file
	pred := findPredecessorSession(tmpDir, "testrig", "current-session-id", 0)
	if pred != nil {
		t.Error("expected nil predecessor with no events file")
	}

	// Create events file with session_start events
	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	var eventsData []byte

	// Add a session from different rig (should be ignored)
	otherEvent := map[string]interface{}{
		"ts":     time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "otherrig/crew/bob",
		"source": "gt",
		"payload": map[string]interface{}{
			"session_id": "other-session-id",
		},
	}
	data, _ := json.Marshal(otherEvent)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	// Add an older session for our rig
	oldEvent := map[string]interface{}{
		"ts":     time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "testrig/crew/joe",
		"source": "gt",
		"payload": map[string]interface{}{
			"session_id": "old-session-id",
		},
	}
	data, _ = json.Marshal(oldEvent)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	// Add a newer session for our rig (should be found)
	newEvent := map[string]interface{}{
		"ts":     time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "testrig/crew/max",
		"source": "gt",
		"payload": map[string]interface{}{
			"session_id": "newest-session-id",
		},
	}
	data, _ = json.Marshal(newEvent)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	// Add current session (should be excluded)
	currentEvent := map[string]interface{}{
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"type":   events.TypeSessionStart,
		"actor":  "testrig/crew/joe",
		"source": "gt",
		"payload": map[string]interface{}{
			"session_id": "current-session-id",
		},
	}
	data, _ = json.Marshal(currentEvent)
	eventsData = append(eventsData, data...)
	eventsData = append(eventsData, '\n')

	if err := os.WriteFile(eventsPath, eventsData, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// Find predecessor with minAge=0 - should get newest-session-id (1 hour old)
	pred = findPredecessorSession(tmpDir, "testrig", "current-session-id", 0)
	if pred == nil {
		t.Fatal("expected to find predecessor")
	}

	sessionID := getSeancePayloadString(pred.Payload, "session_id")
	if sessionID != "newest-session-id" {
		t.Errorf("expected newest-session-id, got %s", sessionID)
	}
	if pred.Actor != "testrig/crew/max" {
		t.Errorf("expected testrig/crew/max, got %s", pred.Actor)
	}

	// Find predecessor with minAge=2h - should get old-session-id (3 hours old)
	// The 1-hour-old session should be skipped
	pred = findPredecessorSession(tmpDir, "testrig", "current-session-id", 2*time.Hour)
	if pred == nil {
		t.Fatal("expected to find predecessor with minAge=2h")
	}
	sessionID = getSeancePayloadString(pred.Payload, "session_id")
	if sessionID != "old-session-id" {
		t.Errorf("expected old-session-id with minAge=2h, got %s", sessionID)
	}
}

func TestFindPredecessorSessionAllTooRecent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	var eventsData []byte

	// Add only recent sessions
	for i := 0; i < 3; i++ {
		event := map[string]interface{}{
			"ts":     time.Now().Add(-time.Duration(i+1) * 10 * time.Minute).UTC().Format(time.RFC3339),
			"type":   events.TypeSessionStart,
			"actor":  "testrig/crew/joe",
			"source": "gt",
			"payload": map[string]interface{}{
				"session_id": "session-" + string(rune('a'+i)),
			},
		}
		data, _ := json.Marshal(event)
		eventsData = append(eventsData, data...)
		eventsData = append(eventsData, '\n')
	}

	if err := os.WriteFile(eventsPath, eventsData, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// All sessions are < 1 hour old, minAge=1h should return nil
	pred := findPredecessorSession(tmpDir, "testrig", "current", 1*time.Hour)
	if pred != nil {
		t.Error("expected nil when all sessions are too recent")
	}
}

func TestFindPredecessorSessionNoSessionID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)

	// Add session without session_id in payload
	event := map[string]interface{}{
		"ts":      time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		"type":    events.TypeSessionStart,
		"actor":   "testrig/crew/joe",
		"source":  "gt",
		"payload": map[string]interface{}{}, // No session_id
	}
	data, _ := json.Marshal(event)
	data = append(data, '\n')

	if err := os.WriteFile(eventsPath, data, 0644); err != nil {
		t.Fatalf("failed to write events file: %v", err)
	}

	// Should still find the event (session_id check is done later in runAutoSeance)
	pred := findPredecessorSession(tmpDir, "testrig", "current", 0)
	if pred == nil {
		t.Fatal("expected to find predecessor even without session_id")
	}
}

// =============================================================================
// Cache Tests
// =============================================================================

func TestSeanceCache(t *testing.T) {
	// Clear global cache for test
	seanceCache = make(map[string]*seanceCacheEntry)

	// Test cache miss
	summary, hit := getCachedSeanceSummary("session-1", 1*time.Hour)
	if hit {
		t.Error("expected cache miss")
	}
	if summary != "" {
		t.Error("expected empty summary for cache miss")
	}

	// Set cache entry
	setCachedSeanceSummary("session-1", "Test summary content")

	// Test cache hit
	summary, hit = getCachedSeanceSummary("session-1", 1*time.Hour)
	if !hit {
		t.Error("expected cache hit")
	}
	if summary != "Test summary content" {
		t.Errorf("expected 'Test summary content', got %q", summary)
	}

	// Test TTL expiration
	seanceCache["session-1"].Timestamp = time.Now().Add(-2 * time.Hour)
	summary, hit = getCachedSeanceSummary("session-1", 1*time.Hour)
	if hit {
		t.Error("expected cache miss after TTL expiration")
	}

	// Cache entry should be deleted
	if _, exists := seanceCache["session-1"]; exists {
		t.Error("expected expired cache entry to be deleted")
	}
}

func TestCleanStaleCache(t *testing.T) {
	// Clear global cache for test
	seanceCache = make(map[string]*seanceCacheEntry)

	// Add some entries
	setCachedSeanceSummary("fresh-1", "Fresh content 1")
	setCachedSeanceSummary("fresh-2", "Fresh content 2")
	setCachedSeanceSummary("stale-1", "Stale content 1")
	setCachedSeanceSummary("stale-2", "Stale content 2")

	// Make some entries stale
	seanceCache["stale-1"].Timestamp = time.Now().Add(-2 * time.Hour)
	seanceCache["stale-2"].Timestamp = time.Now().Add(-3 * time.Hour)

	// Clean with 1h TTL
	cleanStaleCache(1 * time.Hour)

	// Check fresh entries remain
	if _, exists := seanceCache["fresh-1"]; !exists {
		t.Error("expected fresh-1 to remain")
	}
	if _, exists := seanceCache["fresh-2"]; !exists {
		t.Error("expected fresh-2 to remain")
	}

	// Check stale entries are removed
	if _, exists := seanceCache["stale-1"]; exists {
		t.Error("expected stale-1 to be removed")
	}
	if _, exists := seanceCache["stale-2"]; exists {
		t.Error("expected stale-2 to be removed")
	}
}

func TestSeanceCachePersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seance-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clear and populate cache
	seanceCache = make(map[string]*seanceCacheEntry)
	setCachedSeanceSummary("session-1", "Summary 1")
	setCachedSeanceSummary("session-2", "Summary 2")

	// Save cache
	saveSeanceCache(tmpDir)

	// Verify file exists
	cachePath := getSeanceCachePath(tmpDir)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatal("expected cache file to exist")
	}

	// Clear cache and reload
	seanceCache = make(map[string]*seanceCacheEntry)
	loadSeanceCache(tmpDir)

	// Verify entries loaded
	if len(seanceCache) != 2 {
		t.Errorf("expected 2 cache entries, got %d", len(seanceCache))
	}
	if entry, ok := seanceCache["session-1"]; !ok || entry.Summary != "Summary 1" {
		t.Error("expected session-1 entry with 'Summary 1'")
	}
	if entry, ok := seanceCache["session-2"]; !ok || entry.Summary != "Summary 2" {
		t.Error("expected session-2 entry with 'Summary 2'")
	}
}

// =============================================================================
// Utility Function Tests
// =============================================================================

func TestGetSeancePayloadString(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		key     string
		expect  string
	}{
		{
			name:    "string value",
			payload: map[string]interface{}{"session_id": "abc123"},
			key:     "session_id",
			expect:  "abc123",
		},
		{
			name:    "missing key",
			payload: map[string]interface{}{"other": "value"},
			key:     "session_id",
			expect:  "",
		},
		{
			name:    "non-string value",
			payload: map[string]interface{}{"count": 42},
			key:     "count",
			expect:  "",
		},
		{
			name:    "nil payload",
			payload: nil,
			key:     "session_id",
			expect:  "",
		},
		{
			name:    "empty string value",
			payload: map[string]interface{}{"session_id": ""},
			key:     "session_id",
			expect:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getSeancePayloadString(tc.payload, tc.key)
			if got != tc.expect {
				t.Errorf("getSeancePayloadString() = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestIsSameAgent(t *testing.T) {
	tests := []struct {
		name        string
		predecessor string
		current     string
		expect      bool
	}{
		{"exact match", "gastown/crew/joe", "gastown/crew/joe", true},
		{"case insensitive", "GASTOWN/CREW/JOE", "gastown/crew/joe", true},
		{"different agent", "gastown/crew/joe", "gastown/crew/max", false},
		{"different rig", "beads/crew/joe", "gastown/crew/joe", false},
		{"empty predecessor", "", "gastown/crew/joe", false},
		{"empty current", "gastown/crew/joe", "", false},
		{"both empty", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSameAgent(tc.predecessor, tc.current)
			if got != tc.expect {
				t.Errorf("isSameAgent(%q, %q) = %v, want %v", tc.predecessor, tc.current, got, tc.expect)
			}
		})
	}
}

func TestFormatSeanceDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expect   string
	}{
		{0, "0 minutes"},
		{30 * time.Minute, "30 minutes"},
		{59 * time.Minute, "59 minutes"},
		{1 * time.Hour, "1 hours"},
		{2 * time.Hour, "2 hours"},
		{23 * time.Hour, "23 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
		{72 * time.Hour, "3 days"},
		{7 * 24 * time.Hour, "7 days"},
	}

	for _, tc := range tests {
		t.Run(tc.expect, func(t *testing.T) {
			got := formatSeanceDuration(tc.duration)
			if got != tc.expect {
				t.Errorf("formatSeanceDuration(%v) = %q, want %q", tc.duration, got, tc.expect)
			}
		})
	}
}

// =============================================================================
// Format Context Tests
// =============================================================================

func TestFormatSeanceContext(t *testing.T) {
	sc := &SeanceContext{
		PredecessorActor:     "gastown/crew/max",
		PredecessorSessionID: "abc123",
		LastActive:           time.Now().Add(-26 * time.Hour),
		Summary:              "Working on feature X.\n- Completed task A\n- In progress: task B",
	}

	output := formatSeanceContext(sc)

	// Check that output contains expected content
	if !strings.Contains(output, "Auto-Seance Context Recovery") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "gastown/crew/max") {
		t.Error("expected predecessor actor in output")
	}
	if !strings.Contains(output, "abc123") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(output, "Working on feature X") {
		t.Error("expected summary in output")
	}
	if !strings.Contains(output, "Handoff Summary") {
		t.Error("expected Handoff Summary section")
	}
}

func TestFormatSeanceContextLongSessionID(t *testing.T) {
	sc := &SeanceContext{
		PredecessorActor:     "gastown/crew/max",
		PredecessorSessionID: "very-long-session-id-that-should-be-truncated",
		LastActive:           time.Now().Add(-1 * time.Hour),
		Summary:              "Test summary",
	}

	output := formatSeanceContext(sc)

	// Should contain truncated session ID
	if strings.Contains(output, "very-long-session-id-that-should-be-truncated") {
		t.Error("expected session ID to be truncated")
	}
	if !strings.Contains(output, "very-long-se…") {
		t.Error("expected truncated session ID with ellipsis")
	}
}

func TestFormatSeanceContextEmptySummary(t *testing.T) {
	sc := &SeanceContext{
		PredecessorActor:     "gastown/crew/max",
		PredecessorSessionID: "abc123",
		LastActive:           time.Now().Add(-1 * time.Hour),
		Summary:              "",
	}

	output := formatSeanceContext(sc)

	if !strings.Contains(output, "(No summary available)") {
		t.Error("expected placeholder for empty summary")
	}
}

func TestFormatSeanceContextWhitespaceSummary(t *testing.T) {
	sc := &SeanceContext{
		PredecessorActor:     "gastown/crew/max",
		PredecessorSessionID: "abc123",
		LastActive:           time.Now().Add(-1 * time.Hour),
		Summary:              "   \n\t\n   ",
	}

	output := formatSeanceContext(sc)

	if !strings.Contains(output, "(No summary available)") {
		t.Error("expected placeholder for whitespace-only summary")
	}
}

func TestFormatSeanceContextZeroTime(t *testing.T) {
	sc := &SeanceContext{
		PredecessorActor:     "gastown/crew/max",
		PredecessorSessionID: "abc123",
		LastActive:           time.Time{}, // Zero time
		Summary:              "Test summary",
	}

	output := formatSeanceContext(sc)

	// Should not contain "Last active:" for zero time
	if strings.Contains(output, "Last active:") {
		t.Error("expected no 'Last active:' for zero time")
	}
}

// =============================================================================
// SeanceResult Tests
// =============================================================================

func TestSeanceResult(t *testing.T) {
	result := &SeanceResult{
		Ran:           true,
		SkipReason:    "",
		PredecessorID: "session-123",
		Duration:      5 * time.Second,
		Error:         nil,
		CacheHit:      false,
	}

	if !result.Ran {
		t.Error("expected Ran to be true")
	}
	if result.PredecessorID != "session-123" {
		t.Errorf("expected PredecessorID 'session-123', got %q", result.PredecessorID)
	}
	if result.CacheHit {
		t.Error("expected CacheHit to be false")
	}
}

// =============================================================================
// runSeanceSummaryWithError Tests (limited - can't test actual claude command)
// =============================================================================

func TestRunSeanceSummaryWithErrorEmptySessionID(t *testing.T) {
	ctx := context.Background()
	_, err := runSeanceSummaryWithError(ctx, "")
	if err == nil {
		t.Error("expected error for empty session ID")
	}
	if !strings.Contains(err.Error(), "empty session ID") {
		t.Errorf("expected 'empty session ID' error, got %v", err)
	}
}

func TestRunSeanceSummaryWithErrorInvalidChars(t *testing.T) {
	ctx := context.Background()

	invalidIDs := []string{
		"session;id",
		"session&id",
		"session|id",
		"session`id",
		"session$id",
		"session(id)",
		"session{id}",
		"session[id]",
		"session<id>",
		"session\\id",
		"session\"id",
		"session'id",
	}

	for _, id := range invalidIDs {
		_, err := runSeanceSummaryWithError(ctx, id)
		if err == nil {
			t.Errorf("expected error for invalid session ID %q", id)
		}
		if !strings.Contains(err.Error(), "invalid session ID") {
			t.Errorf("expected 'invalid session ID' error for %q, got %v", id, err)
		}
	}
}

func TestRunSeanceSummaryWithErrorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := runSeanceSummaryWithError(ctx, "valid-session-id")
	if err == nil {
		t.Error("expected error for canceled context")
	}
	// The actual error message depends on whether the command starts or not
}
