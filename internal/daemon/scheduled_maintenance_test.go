package daemon

import (
	"testing"
	"time"
)

func TestParseWindowTime(t *testing.T) {
	tests := []struct {
		input      string
		wantHour   int
		wantMinute int
		wantErr    bool
	}{
		{"03:00", 3, 0, false},
		{"00:00", 0, 0, false},
		{"23:59", 23, 59, false},
		{"12:30", 12, 30, false},
		{"3:00", 3, 0, false},
		// Invalid
		{"24:00", 0, 0, true},
		{"12:60", 0, 0, true},
		{"-1:00", 0, 0, true},
		{"abc", 0, 0, true},
		{"", 0, 0, true},
		{"12", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			hour, minute, err := parseWindowTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseWindowTime(%q) expected error, got hour=%d minute=%d", tt.input, hour, minute)
				}
				return
			}
			if err != nil {
				t.Errorf("parseWindowTime(%q) unexpected error: %v", tt.input, err)
				return
			}
			if hour != tt.wantHour || minute != tt.wantMinute {
				t.Errorf("parseWindowTime(%q) = (%d, %d), want (%d, %d)", tt.input, hour, minute, tt.wantHour, tt.wantMinute)
			}
		})
	}
}

func TestIsInMaintenanceWindow(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name   string
		now    time.Time
		window string
		want   bool
	}{
		{
			name:   "exactly at window start",
			now:    time.Date(2026, 2, 28, 3, 0, 0, 0, loc),
			window: "03:00",
			want:   true,
		},
		{
			name:   "during window",
			now:    time.Date(2026, 2, 28, 3, 30, 0, 0, loc),
			window: "03:00",
			want:   true,
		},
		{
			name:   "just before window end",
			now:    time.Date(2026, 2, 28, 3, 59, 59, 0, loc),
			window: "03:00",
			want:   true,
		},
		{
			name:   "at window end (1 hour later)",
			now:    time.Date(2026, 2, 28, 4, 0, 0, 0, loc),
			window: "03:00",
			want:   false,
		},
		{
			name:   "before window",
			now:    time.Date(2026, 2, 28, 2, 59, 0, 0, loc),
			window: "03:00",
			want:   false,
		},
		{
			name:   "much later",
			now:    time.Date(2026, 2, 28, 15, 0, 0, 0, loc),
			window: "03:00",
			want:   false,
		},
		{
			name:   "midnight window",
			now:    time.Date(2026, 2, 28, 0, 15, 0, 0, loc),
			window: "00:00",
			want:   true,
		},
		{
			name:   "invalid window",
			now:    time.Date(2026, 2, 28, 3, 0, 0, 0, loc),
			window: "bad",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInMaintenanceWindow(tt.now, tt.window)
			if got != tt.want {
				t.Errorf("isInMaintenanceWindow(%v, %q) = %v, want %v", tt.now, tt.window, got, tt.want)
			}
		})
	}
}

func TestShouldRunMaintenance(t *testing.T) {
	now := time.Date(2026, 2, 28, 3, 0, 0, 0, time.Local)

	tests := []struct {
		name     string
		lastRun  time.Time
		interval string
		want     bool
	}{
		{
			name:     "never run before",
			lastRun:  time.Time{},
			interval: "daily",
			want:     true,
		},
		{
			name:     "daily - ran 25 hours ago",
			lastRun:  now.Add(-25 * time.Hour),
			interval: "daily",
			want:     true,
		},
		{
			name:     "daily - ran 10 hours ago",
			lastRun:  now.Add(-10 * time.Hour),
			interval: "daily",
			want:     false,
		},
		{
			name:     "weekly - ran 7 days ago",
			lastRun:  now.Add(-7 * 24 * time.Hour),
			interval: "weekly",
			want:     true,
		},
		{
			name:     "weekly - ran 3 days ago",
			lastRun:  now.Add(-3 * 24 * time.Hour),
			interval: "weekly",
			want:     false,
		},
		{
			name:     "monthly - ran 30 days ago",
			lastRun:  now.Add(-30 * 24 * time.Hour),
			interval: "monthly",
			want:     true,
		},
		{
			name:     "monthly - ran 10 days ago",
			lastRun:  now.Add(-10 * 24 * time.Hour),
			interval: "monthly",
			want:     false,
		},
		{
			name:     "custom duration 48h - ran 50h ago",
			lastRun:  now.Add(-50 * time.Hour),
			interval: "48h",
			want:     true,
		},
		{
			name:     "custom duration 48h - ran 30h ago",
			lastRun:  now.Add(-30 * time.Hour),
			interval: "48h",
			want:     false,
		},
		{
			name:     "invalid interval - falls back to daily",
			lastRun:  now.Add(-25 * time.Hour),
			interval: "nope",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRunMaintenance(now, tt.lastRun, tt.interval)
			if got != tt.want {
				t.Errorf("shouldRunMaintenance(now, %v, %q) = %v, want %v", tt.lastRun, tt.interval, got, tt.want)
			}
		})
	}
}

func TestMaintenanceThreshold(t *testing.T) {
	// Nil config returns default
	if got := maintenanceThreshold(nil); got != defaultMaintenanceThreshold {
		t.Errorf("expected default %d, got %d", defaultMaintenanceThreshold, got)
	}

	// Configured threshold
	threshold := 500
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ScheduledMaintenance: &ScheduledMaintenanceConfig{
				Enabled:   true,
				Threshold: &threshold,
			},
		},
	}
	if got := maintenanceThreshold(config); got != 500 {
		t.Errorf("expected 500, got %d", got)
	}
}

func TestMaintenanceWindow(t *testing.T) {
	// Nil config returns empty
	if got := maintenanceWindow(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Configured window
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ScheduledMaintenance: &ScheduledMaintenanceConfig{
				Enabled: true,
				Window:  "03:00",
			},
		},
	}
	if got := maintenanceWindow(config); got != "03:00" {
		t.Errorf("expected 03:00, got %q", got)
	}
}

func TestMaintenanceInterval(t *testing.T) {
	// Nil config returns "daily"
	if got := maintenanceInterval(nil); got != "daily" {
		t.Errorf("expected daily, got %q", got)
	}

	// Configured interval
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ScheduledMaintenance: &ScheduledMaintenanceConfig{
				Enabled:  true,
				Interval: "weekly",
			},
		},
	}
	if got := maintenanceInterval(config); got != "weekly" {
		t.Errorf("expected weekly, got %q", got)
	}

	// Empty interval returns default
	config.Patrols.ScheduledMaintenance.Interval = ""
	if got := maintenanceInterval(config); got != "daily" {
		t.Errorf("expected daily for empty, got %q", got)
	}
}

func TestIsPatrolEnabledScheduledMaintenance(t *testing.T) {
	// Nil config — disabled (opt-in)
	if IsPatrolEnabled(nil, "scheduled_maintenance") {
		t.Error("expected scheduled_maintenance disabled with nil config")
	}

	// Explicitly disabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ScheduledMaintenance: &ScheduledMaintenanceConfig{
				Enabled: false,
			},
		},
	}
	if IsPatrolEnabled(config, "scheduled_maintenance") {
		t.Error("expected scheduled_maintenance disabled when Enabled=false")
	}

	// Enabled
	config.Patrols.ScheduledMaintenance.Enabled = true
	if !IsPatrolEnabled(config, "scheduled_maintenance") {
		t.Error("expected scheduled_maintenance enabled when Enabled=true")
	}
}
