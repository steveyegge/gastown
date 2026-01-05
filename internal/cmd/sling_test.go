package cmd

import "testing"

func TestFormatTrackBeadID(t *testing.T) {
	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		// HQ beads should remain unchanged
		{
			name:     "hq bead unchanged",
			beadID:   "hq-abc123",
			expected: "hq-abc123",
		},
		{
			name:     "hq convoy unchanged",
			beadID:   "hq-cv-xyz789",
			expected: "hq-cv-xyz789",
		},

		// Cross-rig beads get external: prefix
		{
			name:     "gastown rig bead",
			beadID:   "gt-mol-abc123",
			expected: "external:gt-mol:gt-mol-abc123",
		},
		{
			name:     "beads rig task",
			beadID:   "beads-task-xyz",
			expected: "external:beads-task:beads-task-xyz",
		},
		{
			name:     "two segment ID",
			beadID:   "foo-bar",
			expected: "external:foo-bar:foo-bar",
		},

		// Edge cases
		{
			name:     "single segment fallback",
			beadID:   "orphan",
			expected: "orphan",
		},
		{
			name:     "empty string fallback",
			beadID:   "",
			expected: "",
		},
		{
			name:     "many segments",
			beadID:   "a-b-c-d-e-f",
			expected: "external:a-b:a-b-c-d-e-f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTrackBeadID(tt.beadID)
			if result != tt.expected {
				t.Errorf("formatTrackBeadID(%q) = %q, want %q", tt.beadID, result, tt.expected)
			}
		})
	}
}

// TestFormatTrackBeadIDConsumerCompatibility verifies that the external ref format
// produced by formatTrackBeadID can be correctly parsed by the consumer pattern
// used in convoy.go, model.go, feed/convoy.go, and web/fetcher.go.
func TestFormatTrackBeadIDConsumerCompatibility(t *testing.T) {
	// Consumer pattern from convoy.go:1062-1068:
	// if strings.HasPrefix(issueID, "external:") {
	//     parts := strings.SplitN(issueID, ":", 3)
	//     if len(parts) == 3 {
	//         issueID = parts[2] // Extract the actual issue ID
	//     }
	// }

	tests := []struct {
		name           string
		beadID         string
		wantOriginalID string
	}{
		{
			name:           "cross-rig bead round-trips",
			beadID:         "gt-mol-abc123",
			wantOriginalID: "gt-mol-abc123",
		},
		{
			name:           "beads rig bead round-trips",
			beadID:         "beads-task-xyz",
			wantOriginalID: "beads-task-xyz",
		},
		{
			name:           "hq bead unchanged",
			beadID:         "hq-abc123",
			wantOriginalID: "hq-abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := formatTrackBeadID(tt.beadID)

			// Simulate consumer parsing logic
			parsed := formatted
			if len(formatted) > 9 && formatted[:9] == "external:" {
				parts := make([]string, 0, 3)
				start := 0
				count := 0
				for i := 0; i < len(formatted) && count < 2; i++ {
					if formatted[i] == ':' {
						parts = append(parts, formatted[start:i])
						start = i + 1
						count++
					}
				}
				if count == 2 {
					parts = append(parts, formatted[start:])
				}
				if len(parts) == 3 {
					parsed = parts[2]
				}
			}

			if parsed != tt.wantOriginalID {
				t.Errorf("round-trip failed: formatTrackBeadID(%q) = %q, parsed back to %q, want %q",
					tt.beadID, formatted, parsed, tt.wantOriginalID)
			}
		})
	}
}
