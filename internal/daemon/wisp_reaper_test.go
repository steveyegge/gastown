package daemon

import (
	"testing"
	"time"
)

func TestWispReaperInterval(t *testing.T) {
	// Default
	if got := wispReaperInterval(nil); got != defaultWispReaperInterval {
		t.Errorf("expected default %v, got %v", defaultWispReaperInterval, got)
	}

	// Custom
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:     true,
				IntervalStr: "1h",
			},
		},
	}
	if got := wispReaperInterval(config); got != time.Hour {
		t.Errorf("expected 1h, got %v", got)
	}

	// Invalid falls back to default
	config.Patrols.WispReaper.IntervalStr = "nope"
	if got := wispReaperInterval(config); got != defaultWispReaperInterval {
		t.Errorf("expected default for invalid, got %v", got)
	}
}

func TestWispReaperMaxAge(t *testing.T) {
	if got := wispReaperMaxAge(nil); got != defaultWispMaxAge {
		t.Errorf("expected default %v, got %v", defaultWispMaxAge, got)
	}

	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:   true,
				MaxAgeStr: "48h",
			},
		},
	}
	if got := wispReaperMaxAge(config); got != 48*time.Hour {
		t.Errorf("expected 48h, got %v", got)
	}
}

func TestWispDeleteAge(t *testing.T) {
	if got := wispDeleteAge(nil); got != defaultWispDeleteAge {
		t.Errorf("expected default %v, got %v", defaultWispDeleteAge, got)
	}

	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:      true,
				DeleteAgeStr: "336h",
			},
		},
	}
	if got := wispDeleteAge(config); got != 14*24*time.Hour {
		t.Errorf("expected 336h, got %v", got)
	}
}

func TestStaleIssueAge(t *testing.T) {
	// Default: 30 days
	if got := staleIssueAge(nil); got != defaultStaleIssueAge {
		t.Errorf("expected default %v, got %v", defaultStaleIssueAge, got)
	}

	// Custom: 60 days
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			WispReaper: &WispReaperConfig{
				Enabled:          true,
				StaleIssueAgeStr: "1440h",
			},
		},
	}
	if got := staleIssueAge(config); got != 60*24*time.Hour {
		t.Errorf("expected 1440h (60 days), got %v", got)
	}

	// Invalid falls back to default
	config.Patrols.WispReaper.StaleIssueAgeStr = "bad"
	if got := staleIssueAge(config); got != defaultStaleIssueAge {
		t.Errorf("expected default for invalid, got %v", got)
	}

	// Zero falls back to default
	config.Patrols.WispReaper.StaleIssueAgeStr = "0s"
	if got := staleIssueAge(config); got != defaultStaleIssueAge {
		t.Errorf("expected default for zero, got %v", got)
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		parts []string
		sep   string
		want  string
	}{
		{nil, ",", ""},
		{[]string{"a"}, ",", "a"},
		{[]string{"a", "b", "c"}, ",", "a,b,c"},
		{[]string{"?", "?", "?"}, ",", "?,?,?"},
	}
	for _, tt := range tests {
		got := joinStrings(tt.parts, tt.sep)
		if got != tt.want {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.parts, tt.sep, got, tt.want)
		}
	}
}
