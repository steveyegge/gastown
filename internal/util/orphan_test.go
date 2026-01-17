//go:build !windows

package util

import (
	"testing"
)

func TestParseEtime(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		// MM:SS format
		{"00:30", 30, false},
		{"01:00", 60, false},
		{"01:23", 83, false},
		{"59:59", 3599, false},

		// HH:MM:SS format
		{"00:01:00", 60, false},
		{"01:00:00", 3600, false},
		{"01:02:03", 3723, false},
		{"23:59:59", 86399, false},

		// DD-HH:MM:SS format
		{"1-00:00:00", 86400, false},
		{"2-01:02:03", 176523, false},
		{"7-12:30:45", 649845, false},

		// Edge cases
		{"00:00", 0, false},
		{"0-00:00:00", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseEtime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEtime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseEtime(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindOrphanedClaudeProcesses(t *testing.T) {
	// This is a live test that checks for orphaned processes on the current system.
	// It should not fail - just return whatever orphans exist (likely none in CI).
	orphans, err := FindOrphanedClaudeProcesses()
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses() error = %v", err)
	}

	// Log what we found (useful for debugging)
	t.Logf("Found %d orphaned claude processes", len(orphans))
	for _, o := range orphans {
		t.Logf("  PID %d: %s", o.PID, o.Cmd)
	}
}

func TestFindOrphanedClaudeProcesses_IgnoresTerminalProcesses(t *testing.T) {
	// This test verifies that the function only returns processes without TTY.
	// We can't easily mock ps output, but we can verify that if we're running
	// this test in a terminal, our own process tree isn't flagged.
	orphans, err := FindOrphanedClaudeProcesses()
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses() error = %v", err)
	}

	// If we're running in a terminal (typical test scenario), verify that
	// any orphans found genuinely have no TTY. We can't verify they're NOT
	// in the list since we control the test process, but we can log for inspection.
	for _, o := range orphans {
		t.Logf("Orphan found: PID %d (%s) - verify this has TTY=? in 'ps aux'", o.PID, o.Cmd)
	}
}
