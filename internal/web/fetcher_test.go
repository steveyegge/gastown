package web

import (
	"testing"

	"github.com/steveyegge/gastown/internal/activity"
)

func TestCalculateWorkStatus(t *testing.T) {
	tests := []struct {
		name          string
		completed     int
		total         int
		activityColor string
		want          string
	}{
		{
			name:          "complete when all done",
			completed:     5,
			total:         5,
			activityColor: activity.ColorGreen,
			want:          "complete",
		},
		{
			name:          "complete overrides activity color",
			completed:     3,
			total:         3,
			activityColor: activity.ColorRed,
			want:          "complete",
		},
		{
			name:          "active when green",
			completed:     2,
			total:         5,
			activityColor: activity.ColorGreen,
			want:          "active",
		},
		{
			name:          "stale when yellow",
			completed:     2,
			total:         5,
			activityColor: activity.ColorYellow,
			want:          "stale",
		},
		{
			name:          "stuck when red",
			completed:     2,
			total:         5,
			activityColor: activity.ColorRed,
			want:          "stuck",
		},
		{
			name:          "waiting when unknown color",
			completed:     2,
			total:         5,
			activityColor: activity.ColorUnknown,
			want:          "waiting",
		},
		{
			name:          "waiting when empty color",
			completed:     0,
			total:         5,
			activityColor: "",
			want:          "waiting",
		},
		{
			name:          "waiting when no work yet",
			completed:     0,
			total:         0,
			activityColor: activity.ColorUnknown,
			want:          "waiting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateWorkStatus(tt.completed, tt.total, tt.activityColor)
			if got != tt.want {
				t.Errorf("calculateWorkStatus(%d, %d, %q) = %q, want %q",
					tt.completed, tt.total, tt.activityColor, got, tt.want)
			}
		})
	}
}

func TestDetermineCIStatus(t *testing.T) {
	tests := []struct {
		name   string
		checks []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}
		want string
	}{
		{
			name:   "pending when no checks",
			checks: nil,
			want:   "pending",
		},
		{
			name: "pass when all success",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "success"},
				{Conclusion: "success"},
			},
			want: "pass",
		},
		{
			name: "pass with skipped checks",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "success"},
				{Conclusion: "skipped"},
			},
			want: "pass",
		},
		{
			name: "fail when any failure",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "success"},
				{Conclusion: "failure"},
			},
			want: "fail",
		},
		{
			name: "fail when cancelled",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "cancelled"},
			},
			want: "fail",
		},
		{
			name: "fail when timed_out",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "timed_out"},
			},
			want: "fail",
		},
		{
			name: "pending when in_progress",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "success"},
				{Status: "in_progress"},
			},
			want: "pending",
		},
		{
			name: "pending when queued",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Status: "queued"},
			},
			want: "pending",
		},
		{
			name: "fail from state FAILURE",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{State: "FAILURE"},
			},
			want: "fail",
		},
		{
			name: "pending from state PENDING",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{State: "PENDING"},
			},
			want: "pending",
		},
		{
			name: "failure takes precedence over pending",
			checks: []struct {
				State      string `json:"state"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}{
				{Conclusion: "failure"},
				{Status: "in_progress"},
			},
			want: "fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineCIStatus(tt.checks)
			if got != tt.want {
				t.Errorf("determineCIStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetermineMergeableStatus(t *testing.T) {
	tests := []struct {
		name      string
		mergeable string
		want      string
	}{
		{"ready when MERGEABLE", "MERGEABLE", "ready"},
		{"ready when lowercase mergeable", "mergeable", "ready"},
		{"conflict when CONFLICTING", "CONFLICTING", "conflict"},
		{"conflict when lowercase conflicting", "conflicting", "conflict"},
		{"pending when UNKNOWN", "UNKNOWN", "pending"},
		{"pending when empty", "", "pending"},
		{"pending when other value", "something_else", "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineMergeableStatus(tt.mergeable)
			if got != tt.want {
				t.Errorf("determineMergeableStatus(%q) = %q, want %q",
					tt.mergeable, got, tt.want)
			}
		})
	}
}

func TestDetermineColorClass(t *testing.T) {
	tests := []struct {
		name      string
		ciStatus  string
		mergeable string
		want      string
	}{
		{"green when pass and ready", "pass", "ready", "mq-green"},
		{"red when CI fails", "fail", "ready", "mq-red"},
		{"red when conflict", "pass", "conflict", "mq-red"},
		{"red when both fail and conflict", "fail", "conflict", "mq-red"},
		{"yellow when CI pending", "pending", "ready", "mq-yellow"},
		{"yellow when merge pending", "pass", "pending", "mq-yellow"},
		{"yellow when both pending", "pending", "pending", "mq-yellow"},
		{"yellow for unknown states", "unknown", "unknown", "mq-yellow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineColorClass(tt.ciStatus, tt.mergeable)
			if got != tt.want {
				t.Errorf("determineColorClass(%q, %q) = %q, want %q",
					tt.ciStatus, tt.mergeable, got, tt.want)
			}
		})
	}
}

func TestGetRefineryStatusHint(t *testing.T) {
	// Create a minimal fetcher for testing
	f := &LiveConvoyFetcher{}

	tests := []struct {
		name            string
		mergeQueueCount int
		want            string
	}{
		{"idle when no PRs", 0, "Idle - Waiting for PRs"},
		{"singular PR", 1, "Processing 1 PR"},
		{"multiple PRs", 2, "Processing 2 PRs"},
		{"many PRs", 10, "Processing 10 PRs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.getRefineryStatusHint(tt.mergeQueueCount)
			if got != tt.want {
				t.Errorf("getRefineryStatusHint(%d) = %q, want %q",
					tt.mergeQueueCount, got, tt.want)
			}
		})
	}
}

func TestParseActivityTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantUnix  int64
		wantValid bool
	}{
		{"valid timestamp", "1704312345", 1704312345, true},
		{"zero timestamp", "0", 0, false},
		{"empty string", "", 0, false},
		{"invalid string", "abc", 0, false},
		{"negative", "-123", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unix, valid := parseActivityTimestamp(tt.input)
			if valid != tt.wantValid {
				t.Errorf("parseActivityTimestamp(%q) valid = %v, want %v",
					tt.input, valid, tt.wantValid)
			}
			if valid && unix != tt.wantUnix {
				t.Errorf("parseActivityTimestamp(%q) = %d, want %d",
					tt.input, unix, tt.wantUnix)
			}
		})
	}
}
