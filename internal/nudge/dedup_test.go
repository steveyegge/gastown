package nudge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHashMessage(t *testing.T) {
	h1 := HashMessage("hello world")
	h2 := HashMessage("hello world")
	h3 := HashMessage("different message")

	if h1 != h2 {
		t.Error("same message should produce same hash")
	}
	if h1 == h3 {
		t.Error("different messages should produce different hashes")
	}
	if len(h1) != 32 {
		t.Errorf("hash should be 32 hex chars, got %d", len(h1))
	}
}

func TestCheckDedup_NoState(t *testing.T) {
	dir := t.TempDir()
	result, err := CheckDedup(dir, "gt-test", "hello", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupDeliver {
		t.Errorf("expected DedupDeliver for no prior state, got %d", result)
	}
}

func TestCheckDedup_DifferentMessage(t *testing.T) {
	dir := t.TempDir()
	if err := RecordDelivery(dir, "gt-test", "first message", "session-1"); err != nil {
		t.Fatal(err)
	}

	result, err := CheckDedup(dir, "gt-test", "different message", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupDeliver {
		t.Errorf("expected DedupDeliver for different message, got %d", result)
	}
}

func TestCheckDedup_SameMessage_SameSession(t *testing.T) {
	dir := t.TempDir()
	if err := RecordDelivery(dir, "gt-test", "repeated nudge", "session-1"); err != nil {
		t.Fatal(err)
	}

	result, err := CheckDedup(dir, "gt-test", "repeated nudge", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupShortRef {
		t.Errorf("expected DedupShortRef for same message in same session, got %d", result)
	}
}

func TestCheckDedup_SameMessage_DifferentSession(t *testing.T) {
	dir := t.TempDir()
	if err := RecordDelivery(dir, "gt-test", "repeated nudge", "session-1"); err != nil {
		t.Fatal(err)
	}

	result, err := CheckDedup(dir, "gt-test", "repeated nudge", "session-2")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupDeliver {
		t.Errorf("expected DedupDeliver for same message in different session, got %d", result)
	}
}

func TestCheckDedup_SameMessage_UnknownSession(t *testing.T) {
	dir := t.TempDir()
	if err := RecordDelivery(dir, "gt-test", "repeated nudge", "session-1"); err != nil {
		t.Fatal(err)
	}

	// Empty session ID = unknown, should deliver full
	result, err := CheckDedup(dir, "gt-test", "repeated nudge", "")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupDeliver {
		t.Errorf("expected DedupDeliver for unknown session, got %d", result)
	}
}

func TestCheckDedup_CooldownExpired(t *testing.T) {
	dir := t.TempDir()

	// Write state with an old timestamp
	if err := RecordDelivery(dir, "gt-test", "old nudge", "session-1"); err != nil {
		t.Fatal(err)
	}

	// Manually backdate the state
	state, err := loadState(dir, "gt-test")
	if err != nil || state == nil {
		t.Fatal("failed to load state")
	}
	state.Timestamp = time.Now().Add(-31 * time.Minute) // Past the 30min WORK cooldown

	data, _ := json.MarshalIndent(state, "", "  ")
	path := stateFilePath(dir, "gt-test")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckDedup(dir, "gt-test", "old nudge", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if result != DedupDeliver {
		t.Errorf("expected DedupDeliver after cooldown expired, got %d", result)
	}
}

func TestClassifyCooldown(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected time.Duration
	}{
		{"work directive", "You have hooked work. Execute it.", CooldownWork},
		{"alert message", "Alert firing: ServiceDown for dolt", CooldownAlert},
		{"critical alert", "CRITICAL: disk full on kota", CooldownAlert},
		{"wake message", "wake up and check mail", CooldownWake},
		{"session started", "session-started", CooldownWake},
		{"generic", "Please check your mail", CooldownWork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCooldown(tt.message)
			if got != tt.expected {
				t.Errorf("classifyCooldown(%q) = %v, want %v", tt.message, got, tt.expected)
			}
		})
	}
}

func TestRecordDelivery_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	target := "gt-test-agent"

	if err := RecordDelivery(dir, target, "test message", "session-1"); err != nil {
		t.Fatal(err)
	}

	// Verify state file exists
	path := stateFilePath(dir, target)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("state file should exist after RecordDelivery")
	}
}

func TestRecordDelivery_OverwritesPrevious(t *testing.T) {
	dir := t.TempDir()

	if err := RecordDelivery(dir, "gt-test", "first", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := RecordDelivery(dir, "gt-test", "second", "s2"); err != nil {
		t.Fatal(err)
	}

	state, err := loadState(dir, "gt-test")
	if err != nil {
		t.Fatal(err)
	}
	if state.Hash != HashMessage("second") {
		t.Error("state should reflect the most recent delivery")
	}
	if state.SessionID != "s2" {
		t.Error("session ID should be updated")
	}
}

func TestStateFilePath_SlashSanitization(t *testing.T) {
	dir := t.TempDir()
	path := stateFilePath(dir, "aegis/crew/malcolm")
	expected := filepath.Join(dedupStateDir(dir), "aegis_crew_malcolm.json")
	if path != expected {
		t.Errorf("stateFilePath = %q, want %q", path, expected)
	}
}

func TestPreview(t *testing.T) {
	short := "short"
	if preview(short, 80) != "short" {
		t.Error("short strings should be returned as-is")
	}

	long := string(make([]byte, 200))
	p := preview(long, 80)
	if len(p) != 83 { // 80 chars + "..."
		t.Errorf("long preview should be 83 chars, got %d", len(p))
	}
}
