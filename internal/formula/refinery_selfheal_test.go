package formula

import (
	"strings"
	"testing"
)

// TestRefineryFormulaBackoffMax60s verifies that the refinery patrol formula
// uses a 60s max backoff (not 5m) to ensure the refinery polls at least once
// per minute. This is the self-healing fallback for broken signal chains.
func TestRefineryFormulaBackoffMax60s(t *testing.T) {
	content, err := formulasFS.ReadFile("formulas/mol-refinery-patrol.formula.toml")
	if err != nil {
		t.Fatalf("reading refinery formula: %v", err)
	}

	contentStr := string(content)

	// The burn-or-loop step should have --backoff-max 60s
	if !strings.Contains(contentStr, "--backoff-max 60s") {
		t.Error("refinery formula should use --backoff-max 60s (not 5m) for self-healing")
	}

	// Should NOT have the old 5m backoff
	if strings.Contains(contentStr, "--backoff-max 5m") {
		t.Error("refinery formula should not use --backoff-max 5m (too slow for self-healing)")
	}
}

// TestRefineryFormulaHasGHPRFallback verifies that the refinery patrol formula
// includes a GitHub PR polling fallback in the queue-scan step.
func TestRefineryFormulaHasGHPRFallback(t *testing.T) {
	content, err := formulasFS.ReadFile("formulas/mol-refinery-patrol.formula.toml")
	if err != nil {
		t.Fatalf("reading refinery formula: %v", err)
	}

	contentStr := string(content)

	// queue-scan step should mention gh pr list as a fallback
	if !strings.Contains(contentStr, "gh pr list") {
		t.Error("refinery formula queue-scan should include 'gh pr list' fallback for orphaned PRs")
	}

	// Should mention self-healing
	if !strings.Contains(contentStr, "Self-healing fallback") {
		t.Error("refinery formula should document the self-healing fallback pattern")
	}
}
