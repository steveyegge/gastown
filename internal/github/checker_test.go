package github

import (
	"fmt"
	"testing"
)

func TestBeadTitle(t *testing.T) {
	pr := PR{Number: 1500, Title: "feat/copilot-runtime"}
	check := CheckRun{Name: "integration-tests"}

	got := BeadTitle(pr, check)
	want := "CI failure: integration-tests on PR #1500"
	if got != want {
		t.Errorf("BeadTitle() = %q, want %q", got, want)
	}
}

func TestBeadTitle_SpecialCharacters(t *testing.T) {
	pr := PR{Number: 42, Title: "fix: handle edge-case"}
	check := CheckRun{Name: "build (ubuntu-latest)"}

	got := BeadTitle(pr, check)
	want := "CI failure: build (ubuntu-latest) on PR #42"
	if got != want {
		t.Errorf("BeadTitle() = %q, want %q", got, want)
	}
}

func TestCheckResult_EmptyPRs(t *testing.T) {
	result := &CheckResult{
		Repo:    "owner/repo",
		PRCount: 0,
	}

	if len(result.Failures) != 0 {
		t.Error("expected no failures for empty PR list")
	}
	if len(result.Created) != 0 {
		t.Error("expected no created beads for empty PR list")
	}
}

func TestCheckFailure_Fields(t *testing.T) {
	f := CheckFailure{
		PR:    PR{Number: 100, Title: "test pr", URL: "https://github.com/owner/repo/pull/100"},
		Check: CheckRun{Name: "lint", Conclusion: "failure", URL: "https://github.com/owner/repo/actions/123"},
		Repo:  "owner/repo",
	}

	if f.PR.Number != 100 {
		t.Errorf("PR.Number = %d, want 100", f.PR.Number)
	}
	if f.Check.Name != "lint" {
		t.Errorf("Check.Name = %q, want %q", f.Check.Name, "lint")
	}
	if !f.Check.IsFailed() {
		t.Error("expected IsFailed() = true for conclusion=failure")
	}

	// Verify bead title format
	title := BeadTitle(f.PR, f.Check)
	expected := fmt.Sprintf("CI failure: %s on PR #%d", f.Check.Name, f.PR.Number)
	if title != expected {
		t.Errorf("BeadTitle() = %q, want %q", title, expected)
	}
}
