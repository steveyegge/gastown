package mq

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateMRIDWithTime(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		branch    string
		timestamp time.Time
		want      string
	}{
		{
			name:      "basic gastown MR",
			prefix:    "gt",
			branch:    "polecat/Nux/gt-xyz",
			timestamp: time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC),
			want:      "gt-mr-", // Will verify prefix, actual hash varies
		},
		{
			name:      "different prefix",
			prefix:    "hop",
			branch:    "feature/auth",
			timestamp: time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC),
			want:      "hop-mr-",
		},
		{
			name:      "empty prefix",
			prefix:    "",
			branch:    "main",
			timestamp: time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC),
			want:      "-mr-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateMRIDWithTime(tt.prefix, tt.branch, tt.timestamp)

			// Verify prefix format
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("GenerateMRIDWithTime() = %q, want prefix %q", got, tt.want)
			}

			// Verify total format: prefix-mr-XXXXXXXXXX (10 hex chars)
			parts := strings.Split(got, "-mr-")
			if len(parts) != 2 {
				t.Errorf("GenerateMRIDWithTime() = %q, expected format <prefix>-mr-<hash>", got)
				return
			}

			if parts[0] != tt.prefix {
				t.Errorf("GenerateMRIDWithTime() prefix = %q, want %q", parts[0], tt.prefix)
			}

			if len(parts[1]) != 10 {
				t.Errorf("GenerateMRIDWithTime() hash length = %d, want 10", len(parts[1]))
			}

			// Verify hash is valid hex
			for _, c := range parts[1] {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("GenerateMRIDWithTime() hash contains invalid hex char: %c", c)
				}
			}
		})
	}
}

func TestGenerateMRIDWithTime_Deterministic(t *testing.T) {
	// Same inputs should produce same output
	prefix := "gt"
	branch := "polecat/Nux/gt-xyz"
	ts := time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC)

	id1 := GenerateMRIDWithTime(prefix, branch, ts)
	id2 := GenerateMRIDWithTime(prefix, branch, ts)

	if id1 != id2 {
		t.Errorf("Same inputs produced different outputs: %q != %q", id1, id2)
	}
}

func TestGenerateMRIDWithTime_DifferentTimestamps(t *testing.T) {
	// Different timestamps should produce different IDs
	prefix := "gt"
	branch := "polecat/Nux/gt-xyz"
	ts1 := time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 12, 17, 10, 0, 0, 1, time.UTC) // 1 nanosecond later

	id1 := GenerateMRIDWithTime(prefix, branch, ts1)
	id2 := GenerateMRIDWithTime(prefix, branch, ts2)

	if id1 == id2 {
		t.Errorf("Different timestamps produced same ID: %q", id1)
	}
}

func TestGenerateMRIDWithTime_DifferentBranches(t *testing.T) {
	// Different branches should produce different IDs
	prefix := "gt"
	ts := time.Date(2025, 12, 17, 10, 0, 0, 0, time.UTC)

	id1 := GenerateMRIDWithTime(prefix, "branch-a", ts)
	id2 := GenerateMRIDWithTime(prefix, "branch-b", ts)

	if id1 == id2 {
		t.Errorf("Different branches produced same ID: %q", id1)
	}
}

func TestGenerateMRID(t *testing.T) {
	// GenerateMRID uses current time, so we just verify format
	id := GenerateMRID("gt", "polecat/Nux/gt-xyz")

	if !strings.HasPrefix(id, "gt-mr-") {
		t.Errorf("GenerateMRID() = %q, want prefix gt-mr-", id)
	}

	parts := strings.Split(id, "-mr-")
	if len(parts) != 2 || len(parts[1]) != 10 {
		t.Errorf("GenerateMRID() = %q, invalid format", id)
	}
}

func TestGenerateMRID_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they're unique
	ids := make(map[string]bool)
	prefix := "gt"
	branch := "test-branch"

	for i := 0; i < 100; i++ {
		id := GenerateMRID(prefix, branch)
		if ids[id] {
			t.Errorf("Duplicate ID generated: %q", id)
		}
		ids[id] = true
	}
}
