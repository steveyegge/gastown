package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

func TestSeanceDefaults(t *testing.T) {
	cfg := seanceDefaults()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.ColdThreshold != 24*time.Hour {
		t.Errorf("expected ColdThreshold to be 24h, got %v", cfg.ColdThreshold)
	}
}

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
		got := shouldRunAutoSeance(tc.role)
		if got != tc.expect {
			t.Errorf("shouldRunAutoSeance(%s) = %v, want %v", tc.role, got, tc.expect)
		}
	}
}

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
			name: "cwd contains rig",
			event: seanceEvent{
				Actor:   "unknown",
				Payload: map[string]interface{}{"cwd": "/home/user/gt/gastown/crew/joe"},
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
		"type":   "session_start",
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

func TestFindPredecessorSession(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "seance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with no events file
	pred := findPredecessorSession(tmpDir, "testrig", "current-session-id")
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

	// Find predecessor - should get newest-session-id (not current)
	pred = findPredecessorSession(tmpDir, "testrig", "current-session-id")
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
}

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

func TestFormatSeanceDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expect   string
	}{
		{30 * time.Minute, "30 minutes"},
		{1 * time.Hour, "1 hours"},
		{2 * time.Hour, "2 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
		{72 * time.Hour, "3 days"},
	}

	for _, tc := range tests {
		got := formatSeanceDuration(tc.duration)
		if got != tc.expect {
			t.Errorf("formatSeanceDuration(%v) = %q, want %q", tc.duration, got, tc.expect)
		}
	}
}

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
}
