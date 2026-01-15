package beads

import (
	"strings"
	"testing"
)

func TestFormatChannelDescription(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		fields *ChannelFields
		want   []string // Lines that should be present
	}{
		{
			name:  "basic channel",
			title: "Channel: alerts",
			fields: &ChannelFields{
				Name:        "alerts",
				Subscribers: []string{"gastown/crew/max", "gastown/witness"},
				Status:      ChannelStatusActive,
				CreatedBy:   "human",
				CreatedAt:   "2024-01-15T10:00:00Z",
			},
			want: []string{
				"Channel: alerts",
				"name: alerts",
				"subscribers: gastown/crew/max,gastown/witness",
				"status: active",
				"created_by: human",
				"created_at: 2024-01-15T10:00:00Z",
			},
		},
		{
			name:  "empty subscribers",
			title: "Channel: empty",
			fields: &ChannelFields{
				Name:        "empty",
				Subscribers: nil,
				Status:      ChannelStatusActive,
				CreatedBy:   "admin",
			},
			want: []string{
				"name: empty",
				"subscribers: null",
				"created_by: admin",
			},
		},
		{
			name:  "with retention",
			title: "Channel: builds",
			fields: &ChannelFields{
				Name:           "builds",
				Subscribers:    []string{"*/witness"},
				RetentionCount: 100,
				RetentionHours: 24,
			},
			want: []string{
				"name: builds",
				"retention_count: 100",
				"retention_hours: 24",
			},
		},
		{
			name:  "closed channel",
			title: "Channel: old",
			fields: &ChannelFields{
				Name:   "old",
				Status: ChannelStatusClosed,
			},
			want: []string{
				"status: closed",
			},
		},
		{
			name:   "nil fields",
			title:  "Just a title",
			fields: nil,
			want:   []string{"Just a title"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatChannelDescription(tt.title, tt.fields)
			for _, line := range tt.want {
				if !strings.Contains(got, line) {
					t.Errorf("FormatChannelDescription() missing line %q\ngot:\n%s", line, got)
				}
			}
		})
	}
}

func TestParseChannelFields(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        *ChannelFields
	}{
		{
			name: "full channel",
			description: `Channel: alerts

name: alerts
subscribers: gastown/crew/max,gastown/witness,*/refinery
status: active
retention_count: 50
retention_hours: 48
created_by: human
created_at: 2024-01-15T10:00:00Z`,
			want: &ChannelFields{
				Name:           "alerts",
				Subscribers:    []string{"gastown/crew/max", "gastown/witness", "*/refinery"},
				Status:         ChannelStatusActive,
				RetentionCount: 50,
				RetentionHours: 48,
				CreatedBy:      "human",
				CreatedAt:      "2024-01-15T10:00:00Z",
			},
		},
		{
			name: "null subscribers",
			description: `Channel: empty

name: empty
subscribers: null
status: active
created_by: admin`,
			want: &ChannelFields{
				Name:        "empty",
				Subscribers: nil,
				Status:      ChannelStatusActive,
				CreatedBy:   "admin",
			},
		},
		{
			name: "single subscriber",
			description: `name: solo
subscribers: gastown/crew/max
status: active`,
			want: &ChannelFields{
				Name:        "solo",
				Subscribers: []string{"gastown/crew/max"},
				Status:      ChannelStatusActive,
			},
		},
		{
			name:        "empty description",
			description: "",
			want: &ChannelFields{
				Status: ChannelStatusActive, // Default
			},
		},
		{
			name: "subscribers with spaces",
			description: `name: spaced
subscribers: a, b , c
status: active`,
			want: &ChannelFields{
				Name:        "spaced",
				Subscribers: []string{"a", "b", "c"},
				Status:      ChannelStatusActive,
			},
		},
		{
			name: "closed status",
			description: `name: archived
status: closed`,
			want: &ChannelFields{
				Name:   "archived",
				Status: ChannelStatusClosed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseChannelFields(tt.description)
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.want.Status)
			}
			if got.RetentionCount != tt.want.RetentionCount {
				t.Errorf("RetentionCount = %d, want %d", got.RetentionCount, tt.want.RetentionCount)
			}
			if got.RetentionHours != tt.want.RetentionHours {
				t.Errorf("RetentionHours = %d, want %d", got.RetentionHours, tt.want.RetentionHours)
			}
			if got.CreatedBy != tt.want.CreatedBy {
				t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, tt.want.CreatedBy)
			}
			if got.CreatedAt != tt.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tt.want.CreatedAt)
			}
			if len(got.Subscribers) != len(tt.want.Subscribers) {
				t.Errorf("Subscribers count = %d, want %d", len(got.Subscribers), len(tt.want.Subscribers))
			} else {
				for i, s := range got.Subscribers {
					if s != tt.want.Subscribers[i] {
						t.Errorf("Subscribers[%d] = %q, want %q", i, s, tt.want.Subscribers[i])
					}
				}
			}
		})
	}
}

func TestChannelBeadID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"alerts", "hq-channel-alerts"},
		{"builds", "hq-channel-builds"},
		{"team-updates", "hq-channel-team-updates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChannelBeadID(tt.name); got != tt.want {
				t.Errorf("ChannelBeadID(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestChannelRoundTrip(t *testing.T) {
	// Test that Format -> Parse preserves data
	original := &ChannelFields{
		Name:           "test-channel",
		Subscribers:    []string{"gastown/crew/max", "*/witness", "@town"},
		Status:         ChannelStatusActive,
		RetentionCount: 100,
		RetentionHours: 72,
		CreatedBy:      "tester",
		CreatedAt:      "2024-01-15T12:00:00Z",
	}

	description := FormatChannelDescription("Channel: test-channel", original)
	parsed := ParseChannelFields(description)

	if parsed.Name != original.Name {
		t.Errorf("Name: got %q, want %q", parsed.Name, original.Name)
	}
	if parsed.Status != original.Status {
		t.Errorf("Status: got %q, want %q", parsed.Status, original.Status)
	}
	if parsed.RetentionCount != original.RetentionCount {
		t.Errorf("RetentionCount: got %d, want %d", parsed.RetentionCount, original.RetentionCount)
	}
	if parsed.RetentionHours != original.RetentionHours {
		t.Errorf("RetentionHours: got %d, want %d", parsed.RetentionHours, original.RetentionHours)
	}
	if parsed.CreatedBy != original.CreatedBy {
		t.Errorf("CreatedBy: got %q, want %q", parsed.CreatedBy, original.CreatedBy)
	}
	if parsed.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt: got %q, want %q", parsed.CreatedAt, original.CreatedAt)
	}
	if len(parsed.Subscribers) != len(original.Subscribers) {
		t.Fatalf("Subscribers count: got %d, want %d", len(parsed.Subscribers), len(original.Subscribers))
	}
	for i, s := range original.Subscribers {
		if parsed.Subscribers[i] != s {
			t.Errorf("Subscribers[%d]: got %q, want %q", i, parsed.Subscribers[i], s)
		}
	}
}
