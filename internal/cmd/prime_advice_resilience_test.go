package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests verify that gt prime handles advice-related failures gracefully.
// They use PATH manipulation to inject mock `bd` scripts that simulate various
// failure scenarios.

// createMockBD creates a temporary directory with a mock bd script.
// Returns the directory path that should be prepended to PATH.
func createMockBD(t *testing.T, script string) string {
	t.Helper()
	mockDir := t.TempDir()
	bdPath := filepath.Join(mockDir, "bd")
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	return mockDir
}

// withMockBD runs fn with a modified PATH that includes mockDir first.
func withMockBD(t *testing.T, mockDir string, fn func()) {
	t.Helper()
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", mockDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)
	fn()
}

// TestQueryAdviceForAgent_BDNotInPath verifies graceful degradation when
// bd binary is not available.
func TestQueryAdviceForAgent_BDNotInPath(t *testing.T) {
	// Save and clear PATH to simulate bd not being available
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", oldPath)

	// queryAdviceForAgent should return an error, not panic
	beads, err := queryAdviceForAgent("gastown/polecats/alpha")

	// We expect an error when bd isn't found
	if err == nil {
		t.Fatalf("expected error when bd not in PATH, got nil")
	}

	// Beads should be nil (or empty)
	if beads != nil {
		t.Fatalf("expected nil beads when bd not in PATH, got %v", beads)
	}

	// Error should mention exec or not found
	errStr := err.Error()
	if !strings.Contains(errStr, "bd advice list") {
		t.Errorf("error should mention 'bd advice list', got: %s", errStr)
	}
}

// TestQueryAdviceForAgent_BDNonZeroExit verifies error handling when
// bd returns a non-zero exit code.
func TestQueryAdviceForAgent_BDNonZeroExit(t *testing.T) {
	mockScript := `#!/bin/sh
exit 1
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should return an error
		if err == nil {
			t.Fatalf("expected error when bd exits non-zero, got nil")
		}

		// Beads should be nil
		if beads != nil {
			t.Fatalf("expected nil beads when bd fails, got %v", beads)
		}
	})
}

// TestQueryAdviceForAgent_BDInvalidJSON verifies parsing error handling when
// bd returns invalid JSON.
func TestQueryAdviceForAgent_BDInvalidJSON(t *testing.T) {
	// Mock bd that outputs invalid JSON
	mockScript := `#!/bin/sh
echo 'not valid json at all'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should return an error for invalid JSON
		if err == nil {
			t.Fatalf("expected error when bd returns invalid JSON, got nil")
		}

		// Error should mention parsing
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("error should mention parsing, got: %s", err.Error())
		}

		// Beads should be nil
		if beads != nil {
			t.Fatalf("expected nil beads for invalid JSON, got %v", beads)
		}
	})
}

// TestQueryAdviceForAgent_BDReturnsNull verifies handling when
// bd returns null instead of an empty array.
func TestQueryAdviceForAgent_BDReturnsNull(t *testing.T) {
	// Mock bd that outputs null
	mockScript := `#!/bin/sh
echo 'null'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// This could either be handled gracefully or return an error
		// The current implementation doesn't explicitly handle null
		// Testing to document current behavior and ensure no panic
		if err != nil {
			// If error, that's acceptable handling of null
			t.Logf("bd returning null resulted in error: %v (acceptable)", err)
		} else {
			// If no error, beads should be empty
			if len(beads) != 0 {
				t.Errorf("expected empty beads for null, got %d", len(beads))
			}
		}
	})
}

// TestQueryAdviceForAgent_BDReturnsEmptyArray verifies handling of
// empty array response.
func TestQueryAdviceForAgent_BDReturnsEmptyArray(t *testing.T) {
	mockScript := `#!/bin/sh
echo '[]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should succeed with no error
		if err != nil {
			t.Fatalf("unexpected error for empty array: %v", err)
		}

		// Beads should be nil or empty
		if len(beads) != 0 {
			t.Errorf("expected empty beads for [], got %d", len(beads))
		}
	})
}

// TestQueryAdviceForAgent_BDReturnsEmptyString verifies handling of
// empty output from bd.
func TestQueryAdviceForAgent_BDReturnsEmptyString(t *testing.T) {
	mockScript := `#!/bin/sh
# Output nothing
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should succeed with no error (empty output = no advice)
		if err != nil {
			t.Fatalf("unexpected error for empty output: %v", err)
		}

		// Beads should be nil or empty
		if len(beads) != 0 {
			t.Errorf("expected empty beads for empty output, got %d", len(beads))
		}
	})
}

// TestQueryAdviceForAgent_BDTimeout simulates a slow bd command.
// Note: This is a documentation test - actual timeout handling would
// require refactoring to add context cancellation support.
func TestQueryAdviceForAgent_BDTimeout(t *testing.T) {
	// Mock bd that sleeps (simulating database lock or slow query)
	// We use a short sleep since we're not testing actual timeout behavior
	mockScript := `#!/bin/sh
sleep 0.1
echo '[]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should eventually succeed (or timeout if implemented)
		// Current implementation has no timeout, so this documents the gap
		if err != nil {
			t.Logf("slow bd resulted in error: %v", err)
		} else if len(beads) != 0 {
			t.Errorf("expected empty beads from slow bd, got %d", len(beads))
		}
	})
}

// TestQueryAdviceForAgent_BDDatabaseError verifies handling of database errors.
// This simulates the "database locked" or "connection refused" scenarios.
func TestQueryAdviceForAgent_BDDatabaseError(t *testing.T) {
	// Mock bd that outputs error message and exits non-zero
	mockScript := `#!/bin/sh
echo "Error: database is locked" >&2
exit 1
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should return an error
		if err == nil {
			t.Fatalf("expected error when bd has database error, got nil")
		}

		// Beads should be nil
		if beads != nil {
			t.Fatalf("expected nil beads for database error, got %v", beads)
		}
	})
}

// TestQueryAdviceForAgent_BDPartialJSON verifies handling of truncated JSON.
func TestQueryAdviceForAgent_BDPartialJSON(t *testing.T) {
	// Mock bd that outputs truncated JSON (simulating crash mid-output)
	mockScript := `#!/bin/sh
echo '[{"id":"test-123","title":"Test'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		_, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should return an error for truncated JSON
		if err == nil {
			t.Fatalf("expected error for truncated JSON, got nil")
		}

		// Error should mention parsing
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("error should mention parsing, got: %s", err.Error())
		}
	})
}

// TestQueryAdviceForAgent_ValidResponse verifies normal operation with valid response.
func TestQueryAdviceForAgent_ValidResponse(t *testing.T) {
	mockScript := `#!/bin/sh
echo '[{"id":"test-123","title":"Test Advice","description":"Test description"}]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		// Should succeed
		if err != nil {
			t.Fatalf("unexpected error for valid response: %v", err)
		}

		// Should have one bead
		if len(beads) != 1 {
			t.Fatalf("expected 1 bead, got %d", len(beads))
		}

		// Verify fields
		if beads[0].ID != "test-123" {
			t.Errorf("expected ID 'test-123', got %q", beads[0].ID)
		}
		if beads[0].Title != "Test Advice" {
			t.Errorf("expected title 'Test Advice', got %q", beads[0].Title)
		}
		if beads[0].Description != "Test description" {
			t.Errorf("expected description 'Test description', got %q", beads[0].Description)
		}
	})
}

// TestFetchLabelsForBeads_BDFailure verifies label fetching handles bd failures gracefully.
func TestFetchLabelsForBeads_BDFailure(t *testing.T) {
	// Mock bd that fails when trying to show beads
	mockScript := `#!/bin/sh
if echo "$@" | grep -q "show"; then
    exit 1
fi
echo '[]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		testBeads := []AdviceBead{
			{ID: "test-123", Title: "Test"},
		}

		err := fetchLabelsForBeads(testBeads)

		// Should return an error
		if err == nil {
			t.Fatalf("expected error when bd show fails, got nil")
		}
	})
}

// TestFetchLabelsForBeads_InvalidJSON verifies label fetching handles invalid JSON.
func TestFetchLabelsForBeads_InvalidJSON(t *testing.T) {
	mockScript := `#!/bin/sh
if echo "$@" | grep -q "show"; then
    echo 'invalid json'
    exit 0
fi
echo '[]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		testBeads := []AdviceBead{
			{ID: "test-123", Title: "Test"},
		}

		err := fetchLabelsForBeads(testBeads)

		// Should return a parsing error
		if err == nil {
			t.Fatalf("expected error for invalid JSON from bd show, got nil")
		}
		if !strings.Contains(err.Error(), "parsing") {
			t.Errorf("error should mention parsing, got: %s", err.Error())
		}
	})
}

// TestFetchLabelsForBeads_EmptyBeads verifies handling of empty beads slice.
func TestFetchLabelsForBeads_EmptyBeads(t *testing.T) {
	// fetchLabelsForBeads should handle empty slice without calling bd
	err := fetchLabelsForBeads([]AdviceBead{})
	if err != nil {
		t.Fatalf("unexpected error for empty beads: %v", err)
	}

	err = fetchLabelsForBeads(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil beads: %v", err)
	}
}

// TestFetchLabelsForBeads_ValidResponse verifies normal label fetching.
func TestFetchLabelsForBeads_ValidResponse(t *testing.T) {
	mockScript := `#!/bin/sh
if echo "$@" | grep -q "show"; then
    echo '[{"id":"test-123","title":"Test","labels":["global","rig:gastown"]}]'
    exit 0
fi
echo '[]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		testBeads := []AdviceBead{
			{ID: "test-123", Title: "Test"},
		}

		err := fetchLabelsForBeads(testBeads)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Labels should be populated
		if len(testBeads[0].Labels) != 2 {
			t.Errorf("expected 2 labels, got %d", len(testBeads[0].Labels))
		}
	})
}

// TestOutputAdviceContext_BDNotAvailable verifies that outputAdviceContext
// handles bd unavailability gracefully without panicking.
func TestOutputAdviceContext_BDNotAvailable(t *testing.T) {
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", oldPath)

	// Enable explain mode to capture the warning
	oldExplain := primeExplain
	primeExplain = true
	defer func() { primeExplain = oldExplain }()

	// This should not panic
	ctx := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "alpha",
	}

	// Call should complete without panic
	// Note: We can't easily capture stdout here, but ensuring no panic is key
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("outputAdviceContext panicked: %v", r)
		}
	}()

	outputAdviceContext(ctx)
}

// TestOutputAdviceContext_InvalidAgentID verifies handling of contexts
// that don't produce valid agent IDs.
func TestOutputAdviceContext_InvalidAgentID(t *testing.T) {
	// Context with missing required fields
	ctx := RoleInfo{
		Role: RolePolecat,
		// Missing Rig and Polecat
	}

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("outputAdviceContext panicked for invalid context: %v", r)
		}
	}()

	outputAdviceContext(ctx)
}

// TestQueryAdviceForAgent_SpecialCharactersInAgentID verifies handling of
// agent IDs with special characters.
func TestQueryAdviceForAgent_SpecialCharactersInAgentID(t *testing.T) {
	mockScript := `#!/bin/sh
echo '[]'
`
	mockDir := createMockBD(t, mockScript)

	testCases := []string{
		"gastown/polecats/alpha-1",
		"gas-town/polecats/alpha",
		"gastown/crew/decision_notify",
		"beads/polecats/alpha_beta",
	}

	withMockBD(t, mockDir, func() {
		for _, agentID := range testCases {
			t.Run(agentID, func(t *testing.T) {
				_, err := queryAdviceForAgent(agentID)
				if err != nil {
					t.Errorf("unexpected error for agent ID %q: %v", agentID, err)
				}
			})
		}
	})
}

// TestQueryAdviceForAgent_LargeBinaryOutput verifies handling of large responses.
func TestQueryAdviceForAgent_LargeBinaryOutput(t *testing.T) {
	// Generate a large valid JSON response
	mockScript := `#!/bin/sh
printf '['
for i in $(seq 1 100); do
    if [ $i -gt 1 ]; then printf ','; fi
    printf '{"id":"test-%s","title":"Test Advice %s","description":"This is a longer description for test advice number %s to simulate more realistic data."}' "$i" "$i" "$i"
done
echo ']'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		if err != nil {
			t.Fatalf("unexpected error for large response: %v", err)
		}

		if len(beads) != 100 {
			t.Errorf("expected 100 beads, got %d", len(beads))
		}
	})
}

// TestQueryAdviceForAgent_UnicodeContent verifies handling of Unicode in advice.
func TestQueryAdviceForAgent_UnicodeContent(t *testing.T) {
	mockScript := `#!/bin/sh
echo '[{"id":"test-123","title":"Unicode: \u00e9\u00e8\u00ea \u4e2d\u6587 \ud83d\ude00","description":"Emoji test: \ud83d\ude80\ud83c\udf1f"}]'
`
	mockDir := createMockBD(t, mockScript)
	withMockBD(t, mockDir, func() {
		beads, err := queryAdviceForAgent("gastown/polecats/alpha")

		if err != nil {
			t.Fatalf("unexpected error for Unicode content: %v", err)
		}

		if len(beads) != 1 {
			t.Fatalf("expected 1 bead, got %d", len(beads))
		}

		// Just verify it parsed without error - content validation is optional
		if beads[0].Title == "" {
			t.Error("expected non-empty title")
		}
	})
}

// BenchmarkQueryAdviceForAgent provides baseline performance measurement.
func BenchmarkQueryAdviceForAgent(b *testing.B) {
	// Only run if bd is available
	if _, err := exec.LookPath("bd"); err != nil {
		b.Skip("bd not installed")
	}

	mockScript := `#!/bin/sh
echo '[{"id":"test","title":"Test","description":"Test"}]'
`
	mockDir := b.TempDir()
	bdPath := filepath.Join(mockDir, "bd")
	if err := os.WriteFile(bdPath, []byte(mockScript), 0755); err != nil {
		b.Fatalf("write mock bd: %v", err)
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", mockDir+":"+oldPath)
	defer os.Setenv("PATH", oldPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = queryAdviceForAgent("gastown/polecats/alpha")
	}
}
