package krc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.DefaultTTL != 7*24*time.Hour {
		t.Errorf("expected default TTL of 7 days, got %v", config.DefaultTTL)
	}

	if config.PruneInterval != 1*time.Hour {
		t.Errorf("expected prune interval of 1 hour, got %v", config.PruneInterval)
	}

	if config.MinRetainCount != 100 {
		t.Errorf("expected min retain count of 100, got %d", config.MinRetainCount)
	}

	// Check some expected TTL patterns exist
	if _, ok := config.TTLs["patrol_*"]; !ok {
		t.Error("expected patrol_* TTL pattern to exist")
	}

	if _, ok := config.TTLs["session_start"]; !ok {
		t.Error("expected session_start TTL to exist")
	}
}

func TestGetTTL_ExactMatch(t *testing.T) {
	config := DefaultConfig()

	ttl := config.GetTTL("session_start")
	if ttl != 3*24*time.Hour {
		t.Errorf("expected session_start TTL of 3 days, got %v", ttl)
	}
}

func TestGetTTL_GlobMatch(t *testing.T) {
	config := DefaultConfig()

	// patrol_started should match patrol_*
	ttl := config.GetTTL("patrol_started")
	if ttl != 24*time.Hour {
		t.Errorf("expected patrol_started TTL of 1 day (via patrol_*), got %v", ttl)
	}

	// patrol_complete should also match patrol_*
	ttl = config.GetTTL("patrol_complete")
	if ttl != 24*time.Hour {
		t.Errorf("expected patrol_complete TTL of 1 day (via patrol_*), got %v", ttl)
	}

	// merge_started should match merge_*
	ttl = config.GetTTL("merge_started")
	if ttl != 30*24*time.Hour {
		t.Errorf("expected merge_started TTL of 30 days (via merge_*), got %v", ttl)
	}
}

func TestGetTTL_DefaultFallback(t *testing.T) {
	config := DefaultConfig()

	// unknown_event should fall back to default
	ttl := config.GetTTL("unknown_event")
	if ttl != config.DefaultTTL {
		t.Errorf("expected unknown_event TTL to be default (%v), got %v", config.DefaultTTL, ttl)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"patrol_*", "patrol_started", true},
		{"patrol_*", "patrol_complete", true},
		{"patrol_*", "patrol", false},
		{"patrol_*", "xpatrol_started", false},
		{"merge_*", "merge_started", true},
		{"*", "anything", true},
		{"exact", "exact", true},
		{"exact", "notexact", false},
	}

	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestLoadConfig_Default(t *testing.T) {
	// Load from non-existent path should return defaults
	config, err := LoadConfig("/nonexistent/path")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.DefaultTTL != 7*24*time.Hour {
		t.Errorf("expected default TTL, got %v", config.DefaultTTL)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := DefaultConfig()
	config.DefaultTTL = 10 * 24 * time.Hour

	if err := SaveConfig(tmpDir, config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.DefaultTTL != config.DefaultTTL {
		t.Errorf("expected DefaultTTL %v, got %v", config.DefaultTTL, loaded.DefaultTTL)
	}
}

func TestPruner_Prune(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test events file with some expired and some fresh events
	eventsPath := filepath.Join(tmpDir, ".events.jsonl")
	now := time.Now().UTC()

	events := []struct {
		ts     time.Time
		typ    string
		actor  string
	}{
		// Expired (older than 7 days default)
		{now.Add(-10 * 24 * time.Hour), "test_event", "actor1"},
		{now.Add(-8 * 24 * time.Hour), "test_event", "actor2"},
		// Fresh
		{now.Add(-1 * time.Hour), "test_event", "actor3"},
		{now.Add(-30 * time.Minute), "test_event", "actor4"},
	}

	f, err := os.Create(eventsPath)
	if err != nil {
		t.Fatalf("failed to create events file: %v", err)
	}

	for _, e := range events {
		event := map[string]interface{}{
			"ts":    e.ts.Format(time.RFC3339),
			"type":  e.typ,
			"actor": e.actor,
		}
		data, _ := json.Marshal(event)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	// Create pruner with default config
	config := DefaultConfig()
	pruner := NewPruner(tmpDir, config)

	result, err := pruner.Prune()
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	if result.EventsProcessed != 4 {
		t.Errorf("expected 4 events processed, got %d", result.EventsProcessed)
	}

	if result.EventsPruned != 2 {
		t.Errorf("expected 2 events pruned, got %d", result.EventsPruned)
	}

	if result.EventsRetained != 2 {
		t.Errorf("expected 2 events retained, got %d", result.EventsRetained)
	}

	// Verify file was updated
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events file: %v", err)
	}

	// Count lines
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}

	if lines != 2 {
		t.Errorf("expected 2 lines in pruned file, got %d", lines)
	}
}

func TestGetStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test events file
	eventsPath := filepath.Join(tmpDir, ".events.jsonl")
	now := time.Now().UTC()

	events := []struct {
		ts  time.Time
		typ string
	}{
		{now.Add(-1 * time.Hour), "session_start"},
		{now.Add(-2 * time.Hour), "mail"},
		{now.Add(-3 * time.Hour), "nudge"},
	}

	f, err := os.Create(eventsPath)
	if err != nil {
		t.Fatalf("failed to create events file: %v", err)
	}

	for _, e := range events {
		event := map[string]interface{}{
			"ts":   e.ts.Format(time.RFC3339),
			"type": e.typ,
		}
		data, _ := json.Marshal(event)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	config := DefaultConfig()
	stats, err := GetStats(tmpDir, config)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.EventsFile.EventCount != 3 {
		t.Errorf("expected 3 events, got %d", stats.EventsFile.EventCount)
	}

	if stats.ByType["session_start"] != 1 {
		t.Errorf("expected 1 session_start event, got %d", stats.ByType["session_start"])
	}

	if stats.ByType["mail"] != 1 {
		t.Errorf("expected 1 mail event, got %d", stats.ByType["mail"])
	}

	if stats.ByAge["0-1d"] != 3 {
		t.Errorf("expected 3 events in 0-1d bucket, got %d", stats.ByAge["0-1d"])
	}
}
