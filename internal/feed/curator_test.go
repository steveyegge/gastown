package feed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
)

func TestCurator_FiltersByVisibility(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "feed-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create events file with test events
	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	feedPath := filepath.Join(tmpDir, FeedFile)

	// Write a feed-visible event
	feedEvent := events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       events.TypeSling,
		Actor:      "mayor",
		Payload:    map[string]interface{}{"bead": "gt-123", "target": "gastown/slit"},
		Visibility: events.VisibilityFeed,
	}
	feedData, _ := json.Marshal(feedEvent)

	// Write an audit-only event (should be filtered out)
	auditEvent := events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       "internal_check",
		Actor:      "daemon",
		Visibility: events.VisibilityAudit,
	}
	auditData, _ := json.Marshal(auditEvent)

	// Create events file
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		t.Fatalf("creating events file: %v", err)
	}

	// Start curator
	curator := NewCurator(tmpDir)
	if err := curator.Start(); err != nil {
		t.Fatalf("starting curator: %v", err)
	}
	defer curator.Stop()

	// Give curator time to start
	time.Sleep(50 * time.Millisecond)

	// Append events
	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("opening events file: %v", err)
	}
	f.Write(append(feedData, '\n'))
	f.Write(append(auditData, '\n'))
	f.Close()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Check feed file
	feedContent, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatalf("reading feed file: %v", err)
	}

	// Should contain feed event but not audit event
	if len(feedContent) == 0 {
		t.Error("feed file is empty, expected at least one event")
	}

	var writtenEvent FeedEvent
	if err := json.Unmarshal(feedContent[:len(feedContent)-1], &writtenEvent); err != nil {
		t.Fatalf("parsing feed event: %v", err)
	}

	if writtenEvent.Type != events.TypeSling {
		t.Errorf("expected type %s, got %s", events.TypeSling, writtenEvent.Type)
	}
	if writtenEvent.Actor != "mayor" {
		t.Errorf("expected actor 'mayor', got %s", writtenEvent.Actor)
	}
}

func TestCurator_DedupesDoneEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "feed-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	feedPath := filepath.Join(tmpDir, FeedFile)

	// Create events file
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		t.Fatalf("creating events file: %v", err)
	}

	// Start curator
	curator := NewCurator(tmpDir)
	if err := curator.Start(); err != nil {
		t.Fatalf("starting curator: %v", err)
	}
	defer curator.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write 3 identical done events from same actor
	f, _ := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	for i := 0; i < 3; i++ {
		doneEvent := events.Event{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Source:     "gt",
			Type:       events.TypeDone,
			Actor:      "gastown/slit",
			Payload:    map[string]interface{}{"bead": "slit-12345"},
			Visibility: events.VisibilityFeed,
		}
		data, _ := json.Marshal(doneEvent)
		f.Write(append(data, '\n'))
	}
	f.Close()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Count feed events
	feedContent, _ := os.ReadFile(feedPath)
	lines := 0
	for _, b := range feedContent {
		if b == '\n' {
			lines++
		}
	}

	// Should only have 1 event due to deduplication
	if lines != 1 {
		t.Errorf("expected 1 feed event after deduplication, got %d", lines)
	}
}

// --- Config loading tests ---

func TestCurator_DefaultConfig_NoSettingsFile(t *testing.T) {
	// When no settings file exists, NewCurator should use defaults.
	tmpDir := t.TempDir()

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.doneDedupeWindow != 10*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 10s", curator.doneDedupeWindow)
	}
	if curator.slingAggregateWindow != 30*time.Second {
		t.Errorf("slingAggregateWindow = %v, want 30s", curator.slingAggregateWindow)
	}
	if curator.minAggregateCount != 3 {
		t.Errorf("minAggregateCount = %d, want 3", curator.minAggregateCount)
	}
}

func TestCurator_DefaultConfig_EmptyTownRoot(t *testing.T) {
	// Empty townRoot should use defaults without crashing.
	curator := NewCurator("")
	defer curator.Stop()

	if curator.doneDedupeWindow != 10*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 10s", curator.doneDedupeWindow)
	}
	if curator.minAggregateCount != 3 {
		t.Errorf("minAggregateCount = %d, want 3", curator.minAggregateCount)
	}
}

func TestCurator_CustomConfig_FromSettingsFile(t *testing.T) {
	// Write a settings/config.json with custom FeedCurator values.
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts := config.NewTownSettings()
	ts.FeedCurator = &config.FeedCuratorConfig{
		DoneDedupeWindow:     "20s",
		SlingAggregateWindow: "1m",
		MinAggregateCount:    7,
	}
	if err := config.SaveTownSettings(filepath.Join(settingsDir, "config.json"), ts); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.doneDedupeWindow != 20*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 20s", curator.doneDedupeWindow)
	}
	if curator.slingAggregateWindow != 1*time.Minute {
		t.Errorf("slingAggregateWindow = %v, want 1m", curator.slingAggregateWindow)
	}
	if curator.minAggregateCount != 7 {
		t.Errorf("minAggregateCount = %d, want 7", curator.minAggregateCount)
	}
}

func TestCurator_PartialConfig_FallsBackToDefaults(t *testing.T) {
	// Settings file exists but FeedCurator section is absent → defaults.
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts := config.NewTownSettings()
	// No FeedCurator set
	if err := config.SaveTownSettings(filepath.Join(settingsDir, "config.json"), ts); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.doneDedupeWindow != 10*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 10s (default)", curator.doneDedupeWindow)
	}
	if curator.slingAggregateWindow != 30*time.Second {
		t.Errorf("slingAggregateWindow = %v, want 30s (default)", curator.slingAggregateWindow)
	}
	if curator.minAggregateCount != 3 {
		t.Errorf("minAggregateCount = %d, want 3 (default)", curator.minAggregateCount)
	}
}

func TestCurator_PartialFeedCuratorConfig_EmptyDurations(t *testing.T) {
	// FeedCurator section exists but some duration fields are empty → fallback defaults.
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts := config.NewTownSettings()
	ts.FeedCurator = &config.FeedCuratorConfig{
		DoneDedupeWindow: "25s",
		// SlingAggregateWindow left empty → fallback
		MinAggregateCount: 10,
	}
	if err := config.SaveTownSettings(filepath.Join(settingsDir, "config.json"), ts); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.doneDedupeWindow != 25*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 25s", curator.doneDedupeWindow)
	}
	// Empty string → ParseDurationOrDefault falls back to 30s
	if curator.slingAggregateWindow != 30*time.Second {
		t.Errorf("slingAggregateWindow = %v, want 30s (fallback)", curator.slingAggregateWindow)
	}
	if curator.minAggregateCount != 10 {
		t.Errorf("minAggregateCount = %d, want 10", curator.minAggregateCount)
	}
}

func TestCurator_PartialFeedCuratorConfig_ZeroMinAggregate(t *testing.T) {
	// FeedCurator section exists with durations but MinAggregateCount is omitted (zero value).
	// Must fall back to default 3, NOT use 0 (which would aggregate every sling event).
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts := config.NewTownSettings()
	ts.FeedCurator = &config.FeedCuratorConfig{
		DoneDedupeWindow:     "25s",
		SlingAggregateWindow: "1m",
		// MinAggregateCount intentionally omitted (zero value)
	}
	if err := config.SaveTownSettings(filepath.Join(settingsDir, "config.json"), ts); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.minAggregateCount != 3 {
		t.Errorf("minAggregateCount = %d, want 3 (default for zero/omitted value)", curator.minAggregateCount)
	}
}

func TestCurator_InvalidDurationString_FallsBack(t *testing.T) {
	// Invalid duration string in config → falls back to default.
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts := config.NewTownSettings()
	ts.FeedCurator = &config.FeedCuratorConfig{
		DoneDedupeWindow:     "not-valid",
		SlingAggregateWindow: "also-bad",
		MinAggregateCount:    3,
	}
	if err := config.SaveTownSettings(filepath.Join(settingsDir, "config.json"), ts); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	curator := NewCurator(tmpDir)
	defer curator.Stop()

	if curator.doneDedupeWindow != 10*time.Second {
		t.Errorf("doneDedupeWindow = %v, want 10s (fallback for invalid)", curator.doneDedupeWindow)
	}
	if curator.slingAggregateWindow != 30*time.Second {
		t.Errorf("slingAggregateWindow = %v, want 30s (fallback for invalid)", curator.slingAggregateWindow)
	}
}

func TestCurator_GeneratesSummary(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "feed-test-*")
	defer os.RemoveAll(tmpDir)

	curator := NewCurator(tmpDir)

	tests := []struct {
		event    *events.Event
		expected string
	}{
		{
			event: &events.Event{
				Type:    events.TypeSling,
				Actor:   "mayor",
				Payload: map[string]interface{}{"bead": "gt-123", "target": "gastown/slit"},
			},
			expected: "mayor assigned gt-123 to gastown/slit",
		},
		{
			event: &events.Event{
				Type:    events.TypeDone,
				Actor:   "gastown/slit",
				Payload: map[string]interface{}{"bead": "slit-12345"},
			},
			expected: "gastown/slit completed work on slit-12345",
		},
		{
			event: &events.Event{
				Type:  events.TypeHandoff,
				Actor: "gastown/witness",
			},
			expected: "gastown/witness handed off to fresh session",
		},
	}

	for _, tc := range tests {
		summary := curator.generateSummary(tc.event)
		if summary != tc.expected {
			t.Errorf("generateSummary(%s): expected %q, got %q", tc.event.Type, tc.expected, summary)
		}
	}
}

// --- Truncation and size limit tests ---

func TestCurator_TruncatesAtMaxSize(t *testing.T) {
	tmpDir := t.TempDir()
	feedPath := filepath.Join(tmpDir, FeedFile)

	curator := NewCurator(tmpDir)
	defer curator.Stop()
	curator.maxFeedFileSize = 1024 // override for testing

	// Write events directly to the feed file to exceed the limit
	f, err := os.OpenFile(feedPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		ev := FeedEvent{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339),
			Source:    "gt",
			Type:      "test",
			Actor:     "test-actor",
			Summary:   fmt.Sprintf("test event %d with some padding to make it longer", i),
		}
		data, _ := json.Marshal(ev)
		f.Write(append(data, '\n'))
	}
	f.Close()

	// Verify file exceeds limit
	info, _ := os.Stat(feedPath)
	if info.Size() <= 1024 {
		t.Fatalf("test setup: file size %d should exceed 1024", info.Size())
	}

	// Write one more event through curator (triggers truncation)
	curator.writeFeedEvent(&events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       events.TypeDone,
		Actor:      "test-actor",
		Payload:    map[string]interface{}{"bead": "test"},
		Visibility: events.VisibilityFeed,
	})

	// Verify file was truncated
	info, err = os.Stat(feedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1024+512 { // allow small overshoot for the new write
		t.Errorf("file size %d should be near or below limit after truncation", info.Size())
	}

	// Verify content is valid JSONL
	content, _ := os.ReadFile(feedPath)
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}
		var ev FeedEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("malformed line after truncation: %v", err)
		}
	}
}

func TestCurator_ReadRecentFeedEventsLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	feedPath := filepath.Join(tmpDir, FeedFile)

	// Write a large feed file with events spanning hours
	f, err := os.OpenFile(feedPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 5000; i++ {
		ts := now.Add(-2*time.Hour + time.Duration(i)*time.Millisecond*1440)
		ev := FeedEvent{
			Timestamp: ts.UTC().Format(time.RFC3339),
			Source:    "gt",
			Type:      events.TypeDone,
			Actor:     "test-actor",
			Summary:   fmt.Sprintf("event %d", i),
		}
		data, _ := json.Marshal(ev)
		f.Write(append(data, '\n'))
	}
	f.Close()

	curator := NewCurator(tmpDir)

	result := curator.readRecentFeedEvents(10 * time.Second)

	if len(result) > 100 {
		t.Errorf("readRecentFeedEvents returned %d events for 10s window, expected << 5000", len(result))
	}
	if len(result) == 0 {
		t.Error("readRecentFeedEvents returned 0 events, expected at least some recent ones")
	}
}

func TestCurator_FeedFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions are not supported on Windows")
	}
	tmpDir := t.TempDir()
	feedPath := filepath.Join(tmpDir, FeedFile)

	curator := NewCurator(tmpDir)

	curator.writeFeedEvent(&events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       events.TypeDone,
		Actor:      "test-actor",
		Payload:    map[string]interface{}{"bead": "test"},
		Visibility: events.VisibilityFeed,
	})

	info, err := os.Stat(feedPath)
	if err != nil {
		t.Fatalf("feed file not created: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("feed file permissions = %o, want 0600", perm)
	}
}

func TestCurator_DefaultMaxFeedFileSize(t *testing.T) {
	curator := NewCurator(t.TempDir())
	if curator.maxFeedFileSize != maxFeedFileSize {
		t.Errorf("maxFeedFileSize = %d, want %d", curator.maxFeedFileSize, maxFeedFileSize)
	}
}
