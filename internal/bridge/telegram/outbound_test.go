package telegram

import (
	"testing"
)

// TestCategoryFilter verifies that CategoryFilter.Matches correctly allows
// and blocks event types based on the configured category names.
func TestCategoryFilter(t *testing.T) {
	t.Run("matching event passes", func(t *testing.T) {
		f := NewCategoryFilter([]string{"stuck_agents"})
		if !f.Matches("mass_death") {
			t.Error("expected mass_death to match stuck_agents category")
		}
		if !f.Matches("session_death") {
			t.Error("expected session_death to match stuck_agents category")
		}
	})

	t.Run("non-matching event blocked", func(t *testing.T) {
		f := NewCategoryFilter([]string{"stuck_agents"})
		if f.Matches("escalation_sent") {
			t.Error("expected escalation_sent to be blocked when only stuck_agents configured")
		}
		if f.Matches("merge_failed") {
			t.Error("expected merge_failed to be blocked when only stuck_agents configured")
		}
	})

	t.Run("empty categories blocks all", func(t *testing.T) {
		f := NewCategoryFilter([]string{})
		if f.Matches("mass_death") {
			t.Error("expected mass_death to be blocked with empty categories")
		}
		if f.Matches("escalation_sent") {
			t.Error("expected escalation_sent to be blocked with empty categories")
		}
	})

	t.Run("merge_failures category works", func(t *testing.T) {
		f := NewCategoryFilter([]string{"merge_failures"})
		if !f.Matches("merge_failed") {
			t.Error("expected merge_failed to match merge_failures category")
		}
		if f.Matches("mass_death") {
			t.Error("expected mass_death to be blocked when only merge_failures configured")
		}
	})

	t.Run("multiple categories", func(t *testing.T) {
		f := NewCategoryFilter([]string{"stuck_agents", "escalations"})
		if !f.Matches("mass_death") {
			t.Error("expected mass_death to match")
		}
		if !f.Matches("escalation_sent") {
			t.Error("expected escalation_sent to match")
		}
		if f.Matches("merge_failed") {
			t.Error("expected merge_failed to be blocked")
		}
	})
}

// TestFormatNotification verifies the notification text for each event type.
func TestFormatNotification(t *testing.T) {
	t.Run("mass_death format", func(t *testing.T) {
		payload := map[string]interface{}{
			"count":  float64(3),
			"window": "5m",
		}
		got := FormatNotification("mass_death", "overseer", payload)
		want := "[mass_death] 3 agent(s) died in 5m"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("session_death format", func(t *testing.T) {
		payload := map[string]interface{}{
			"session": "my-session",
			"reason":  "timeout",
		}
		got := FormatNotification("session_death", "overseer", payload)
		want := "[session_death] my-session died (timeout)"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("escalation format", func(t *testing.T) {
		payload := map[string]interface{}{
			"message": "need help with merge",
		}
		got := FormatNotification("escalation_sent", "alice", payload)
		want := "[escalation] alice: need help with merge"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("merge_failed format", func(t *testing.T) {
		payload := map[string]interface{}{
			"branch": "feature/foo",
		}
		got := FormatNotification("merge_failed", "bob", payload)
		want := "[merge_failed] bob on feature/foo"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("default format", func(t *testing.T) {
		got := FormatNotification("unknown_event", "carol", nil)
		want := "[unknown_event] carol"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// TestParseFeedLine verifies that ParseFeedLine correctly parses valid JSON.
func TestParseFeedLine(t *testing.T) {
	t.Run("valid JSON parses correctly", func(t *testing.T) {
		line := `{"ts":"2024-01-01T00:00:00Z","type":"mass_death","actor":"overseer","summary":"3 agents died","payload":{"count":3,"window":"5m"}}`
		fl, err := ParseFeedLine(line)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fl.Timestamp != "2024-01-01T00:00:00Z" {
			t.Errorf("Timestamp: got %q, want %q", fl.Timestamp, "2024-01-01T00:00:00Z")
		}
		if fl.Type != "mass_death" {
			t.Errorf("Type: got %q, want %q", fl.Type, "mass_death")
		}
		if fl.Actor != "overseer" {
			t.Errorf("Actor: got %q, want %q", fl.Actor, "overseer")
		}
		if fl.Summary != "3 agents died" {
			t.Errorf("Summary: got %q", fl.Summary)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := ParseFeedLine("not json")
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("empty line returns error", func(t *testing.T) {
		_, err := ParseFeedLine("")
		if err == nil {
			t.Error("expected error for empty line")
		}
	})

	t.Run("source field parsed", func(t *testing.T) {
		line := `{"ts":"2024-01-01T00:00:00Z","source":"refinery","type":"merge_failed","actor":"bot"}`
		fl, err := ParseFeedLine(line)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fl.Source != "refinery" {
			t.Errorf("Source: got %q, want %q", fl.Source, "refinery")
		}
	})

	t.Run("count field parsed", func(t *testing.T) {
		line := `{"ts":"2024-01-01T00:00:00Z","type":"mass_death","actor":"overseer","count":5}`
		fl, err := ParseFeedLine(line)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fl.Count != 5 {
			t.Errorf("Count: got %d, want 5", fl.Count)
		}
	})
}
