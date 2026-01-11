package townlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatLogLine(t *testing.T) {
	ts := time.Date(2025, 12, 26, 15, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		event    Event
		contains []string
	}{
		{
			name: "spawn event",
			event: Event{
				Timestamp: ts,
				Type:      EventSpawn,
				Agent:     "gastown/crew/max",
				Context:   "gt-xyz",
			},
			contains: []string{"2025-12-26 15:30:45", "[spawn]", "gastown/crew/max", "spawned for gt-xyz"},
		},
		{
			name: "nudge event",
			event: Event{
				Timestamp: ts,
				Type:      EventNudge,
				Agent:     "gastown/crew/max",
				Context:   "start work",
			},
			contains: []string{"[nudge]", "gastown/crew/max", "nudged with"},
		},
		{
			name: "done event",
			event: Event{
				Timestamp: ts,
				Type:      EventDone,
				Agent:     "gastown/crew/max",
				Context:   "gt-abc",
			},
			contains: []string{"[done]", "completed gt-abc"},
		},
		{
			name: "crash event",
			event: Event{
				Timestamp: ts,
				Type:      EventCrash,
				Agent:     "gastown/polecats/Toast",
				Context:   "signal 9",
			},
			contains: []string{"[crash]", "exited unexpectedly", "signal 9"},
		},
		{
			name: "kill event",
			event: Event{
				Timestamp: ts,
				Type:      EventKill,
				Agent:     "gastown/polecats/Toast",
				Context:   "gt stop",
			},
			contains: []string{"[kill]", "killed", "gt stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := formatLogLine(tt.event)
			for _, want := range tt.contains {
				if !strings.Contains(line, want) {
					t.Errorf("formatLogLine() = %q, want it to contain %q", line, want)
				}
			}
		})
	}
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		check   func(Event) bool
	}{
		{
			name: "valid spawn line",
			line: "2025-12-26 15:30:45 [spawn] gastown/crew/max spawned for gt-xyz",
			check: func(e Event) bool {
				return e.Type == EventSpawn && e.Agent == "gastown/crew/max"
			},
		},
		{
			name: "valid nudge line",
			line: "2025-12-26 15:31:02 [nudge] gastown/crew/max nudged with \"start\"",
			check: func(e Event) bool {
				return e.Type == EventNudge && e.Agent == "gastown/crew/max"
			},
		},
		{
			name:    "too short",
			line:    "short",
			wantErr: true,
		},
		{
			name:    "missing bracket",
			line:    "2025-12-26 15:30:45 spawn gastown/crew/max",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseLogLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseLogLine() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseLogLine() unexpected error: %v", err)
				return
			}
			if tt.check != nil && !tt.check(event) {
				t.Errorf("parseLogLine() check failed for event: %+v", event)
			}
		})
	}
}

func TestLoggerLogEvent(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "townlog-test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := NewLogger(tmpDir)

	// Log an event
	err = logger.Log(EventSpawn, "gastown/crew/max", "gt-xyz")
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	// Verify log file was created
	logPath := filepath.Join(tmpDir, "logs", "town.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	if !strings.Contains(string(content), "[spawn]") {
		t.Errorf("log file should contain [spawn], got: %s", content)
	}
	if !strings.Contains(string(content), "gastown/crew/max") {
		t.Errorf("log file should contain agent name, got: %s", content)
	}
}

func TestFilterEvents(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Timestamp: now.Add(-2 * time.Hour), Type: EventSpawn, Agent: "gastown/crew/max", Context: "gt-1"},
		{Timestamp: now.Add(-1 * time.Hour), Type: EventNudge, Agent: "gastown/crew/max", Context: "hi"},
		{Timestamp: now.Add(-30 * time.Minute), Type: EventDone, Agent: "gastown/polecats/Toast", Context: "gt-2"},
		{Timestamp: now.Add(-10 * time.Minute), Type: EventSpawn, Agent: "wyvern/crew/joe", Context: "gt-3"},
	}

	tests := []struct {
		name      string
		filter    Filter
		wantCount int
	}{
		{
			name:      "no filter",
			filter:    Filter{},
			wantCount: 4,
		},
		{
			name:      "filter by type",
			filter:    Filter{Type: EventSpawn},
			wantCount: 2,
		},
		{
			name:      "filter by agent prefix",
			filter:    Filter{Agent: "gastown/"},
			wantCount: 3,
		},
		{
			name:      "filter by time",
			filter:    Filter{Since: now.Add(-45 * time.Minute)},
			wantCount: 2,
		},
		{
			name:      "combined filters",
			filter:    Filter{Type: EventSpawn, Agent: "gastown/"},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterEvents(events, tt.filter)
			if len(result) != tt.wantCount {
				t.Errorf("FilterEvents() got %d events, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is a longer string", 10, "this is..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
