package beads

import (
	"strings"
	"testing"
)

func TestFormatGroupDescription(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		fields *GroupFields
		want   []string // Lines that should be present
	}{
		{
			name:  "basic group",
			title: "Group: ops-team",
			fields: &GroupFields{
				Name:      "ops-team",
				Members:   []string{"gastown/crew/max", "gastown/witness"},
				CreatedBy: "human",
				CreatedAt: "2024-01-15T10:00:00Z",
			},
			want: []string{
				"Group: ops-team",
				"name: ops-team",
				"members: gastown/crew/max,gastown/witness",
				"created_by: human",
				"created_at: 2024-01-15T10:00:00Z",
			},
		},
		{
			name:  "empty members",
			title: "Group: empty",
			fields: &GroupFields{
				Name:      "empty",
				Members:   nil,
				CreatedBy: "admin",
			},
			want: []string{
				"name: empty",
				"members: null",
				"created_by: admin",
			},
		},
		{
			name:  "patterns in members",
			title: "Group: all-witnesses",
			fields: &GroupFields{
				Name:    "all-witnesses",
				Members: []string{"*/witness", "@crew"},
			},
			want: []string{
				"members: */witness,@crew",
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
			got := FormatGroupDescription(tt.title, tt.fields)
			for _, line := range tt.want {
				if !strings.Contains(got, line) {
					t.Errorf("FormatGroupDescription() missing line %q\ngot:\n%s", line, got)
				}
			}
		})
	}
}

func TestParseGroupFields(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        *GroupFields
	}{
		{
			name: "full group",
			description: `Group: ops-team

name: ops-team
members: gastown/crew/max,gastown/witness,*/refinery
created_by: human
created_at: 2024-01-15T10:00:00Z`,
			want: &GroupFields{
				Name:      "ops-team",
				Members:   []string{"gastown/crew/max", "gastown/witness", "*/refinery"},
				CreatedBy: "human",
				CreatedAt: "2024-01-15T10:00:00Z",
			},
		},
		{
			name: "null members",
			description: `Group: empty

name: empty
members: null
created_by: admin`,
			want: &GroupFields{
				Name:      "empty",
				Members:   nil,
				CreatedBy: "admin",
			},
		},
		{
			name: "single member",
			description: `name: solo
members: gastown/crew/max`,
			want: &GroupFields{
				Name:    "solo",
				Members: []string{"gastown/crew/max"},
			},
		},
		{
			name:        "empty description",
			description: "",
			want:        &GroupFields{},
		},
		{
			name: "members with spaces",
			description: `name: spaced
members: a, b , c`,
			want: &GroupFields{
				Name:    "spaced",
				Members: []string{"a", "b", "c"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGroupFields(tt.description)
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.CreatedBy != tt.want.CreatedBy {
				t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, tt.want.CreatedBy)
			}
			if got.CreatedAt != tt.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tt.want.CreatedAt)
			}
			if len(got.Members) != len(tt.want.Members) {
				t.Errorf("Members count = %d, want %d", len(got.Members), len(tt.want.Members))
			} else {
				for i, m := range got.Members {
					if m != tt.want.Members[i] {
						t.Errorf("Members[%d] = %q, want %q", i, m, tt.want.Members[i])
					}
				}
			}
		})
	}
}

func TestGroupBeadIDWithPrefix(t *testing.T) {
	tests := []struct {
		prefix string
		name   string
		want   string
	}{
		{"hq", "ops-team", "hq-group-ops-team"},
		{"gt", "ops-team", "gt-group-ops-team"},
		{"bd", "all", "bd-group-all"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix+"-"+tt.name, func(t *testing.T) {
			if got := GroupBeadIDWithPrefix(tt.prefix, tt.name); got != tt.want {
				t.Errorf("GroupBeadIDWithPrefix(%q, %q) = %q, want %q", tt.prefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestGroupBeadID(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"ops-team", "hq-group-ops-team"},
		{"all", "hq-group-all"},
		{"crew-leads", "hq-group-crew-leads"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GroupBeadID(tt.name); got != tt.want {
				t.Errorf("GroupBeadID(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that Format -> Parse preserves data
	original := &GroupFields{
		Name:      "test-group",
		Members:   []string{"gastown/crew/max", "*/witness", "@town"},
		CreatedBy: "tester",
		CreatedAt: "2024-01-15T12:00:00Z",
	}

	description := FormatGroupDescription("Group: test-group", original)
	parsed := ParseGroupFields(description)

	if parsed.Name != original.Name {
		t.Errorf("Name: got %q, want %q", parsed.Name, original.Name)
	}
	if parsed.CreatedBy != original.CreatedBy {
		t.Errorf("CreatedBy: got %q, want %q", parsed.CreatedBy, original.CreatedBy)
	}
	if parsed.CreatedAt != original.CreatedAt {
		t.Errorf("CreatedAt: got %q, want %q", parsed.CreatedAt, original.CreatedAt)
	}
	if len(parsed.Members) != len(original.Members) {
		t.Fatalf("Members count: got %d, want %d", len(parsed.Members), len(original.Members))
	}
	for i, m := range original.Members {
		if parsed.Members[i] != m {
			t.Errorf("Members[%d]: got %q, want %q", i, parsed.Members[i], m)
		}
	}
}

func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		// Valid names
		{"ops-team", false},
		{"all", false},
		{"crew-leads", false},
		{"a", false},
		{"abc_def", false},
		{"team123", false},
		{"a-b_c-d", false},

		// Invalid: empty
		{"", true},

		// Invalid: leading/trailing whitespace
		{" ops-team", true},
		{"ops-team ", true},
		{" ops ", true},

		// Invalid: uppercase
		{"Ops-Team", true},
		{"ALL", true},

		// Invalid: special characters
		{"ops team", true},
		{"ops.team", true},
		{"ops/team", true},
		{"ops@team", true},
		{"ops:team", true},

		// Invalid: starts with hyphen or underscore
		{"-ops", true},
		{"_ops", true},

		// Invalid: too long (65 chars)
		{strings.Repeat("a", 65), true},

		// Valid: exactly at limit (64 chars)
		{strings.Repeat("a", 64), false},
	}

	for _, tt := range tests {
		label := tt.name
		if label == "" {
			label = "(empty)"
		}
		t.Run(label, func(t *testing.T) {
			err := ValidateGroupName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGroupName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}
