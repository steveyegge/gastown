package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWispIDFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantID  string
		wantErr bool
	}{
		{
			name:   "new_epic_id",
			json:   `{"new_epic_id":"gt-wisp-abc","created":7,"phase":"vapor"}`,
			wantID: "gt-wisp-abc",
		},
		{
			name:   "root_id legacy",
			json:   `{"root_id":"gt-wisp-legacy"}`,
			wantID: "gt-wisp-legacy",
		},
		{
			name:   "result_id forward compat",
			json:   `{"result_id":"gt-wisp-result"}`,
			wantID: "gt-wisp-result",
		},
		{
			name:   "precedence prefers new_epic_id",
			json:   `{"root_id":"gt-wisp-legacy","new_epic_id":"gt-wisp-new"}`,
			wantID: "gt-wisp-new",
		},
		{
			name:    "missing id keys",
			json:    `{"created":7,"phase":"vapor"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			json:    `{"new_epic_id":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := parseWispIDFromJSON([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWispIDFromJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotID != tt.wantID {
				t.Fatalf("parseWispIDFromJSON() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

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

func TestCalculateWorkDirFromSession(t *testing.T) {
	// Create a temporary town structure
	townRoot := t.TempDir()

	// Create mayor/town.json to make it look like a town (workspace detection marker)
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Change to town root so workspace.FindFromCwd works
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tests := []struct {
		name        string
		sessionName string
		wantSuffix  string // Expected suffix of the work dir (relative to townRoot)
	}{
		// Town-level agents
		{
			name:        "mayor session",
			sessionName: "hq-mayor",
			wantSuffix:  "mayor",
		},
		{
			name:        "deacon session",
			sessionName: "hq-deacon",
			wantSuffix:  "deacon",
		},

		// Rig-level singletons
		{
			name:        "witness session",
			sessionName: "gt-testrig-witness",
			wantSuffix:  filepath.Join("testrig", "witness", "rig"),
		},
		{
			name:        "refinery session",
			sessionName: "gt-testrig-refinery",
			wantSuffix:  filepath.Join("testrig", "refinery", "rig"),
		},

		// Named workers
		{
			name:        "crew session",
			sessionName: "gt-testrig-crew-max",
			wantSuffix:  filepath.Join("testrig", "crew", "max"),
		},
		{
			name:        "polecat session",
			sessionName: "gt-testrig-nux",
			wantSuffix:  filepath.Join("testrig", "polecats", "nux"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateWorkDirFromSession(tt.sessionName)

			// Calculate expected path
			var want string
			if tt.wantSuffix == "" {
				want = townRoot
			} else {
				want = filepath.Join(townRoot, tt.wantSuffix)
			}

			if got != want {
				t.Errorf("calculateWorkDirFromSession(%q) = %q, want %q", tt.sessionName, got, want)
			}
		})
	}
}

func TestCalculateWorkDirFromSession_InvalidSession(t *testing.T) {
	// Create a temporary town structure
	townRoot := t.TempDir()

	// Create mayor/town.json to make it look like a town
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Change to town root so workspace.FindFromCwd works
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tests := []struct {
		name        string
		sessionName string
	}{
		{
			name:        "invalid prefix",
			sessionName: "invalid-session",
		},
		{
			name:        "empty session",
			sessionName: "",
		},
		{
			name:        "just prefix",
			sessionName: "gt-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateWorkDirFromSession(tt.sessionName)
			if got != "" {
				t.Errorf("calculateWorkDirFromSession(%q) = %q, want empty string", tt.sessionName, got)
			}
		})
	}
}
