package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestGetFormulaNames(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	formulasDir := filepath.Join(tmpDir, "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatalf("creating formulas dir: %v", err)
	}

	// Create some formula files
	formulas := []string{
		"mol-deacon-patrol.formula.toml",
		"mol-witness-patrol.formula.toml",
		"shiny.formula.toml",
	}
	for _, f := range formulas {
		path := filepath.Join(formulasDir, f)
		if err := os.WriteFile(path, []byte("# test"), 0644); err != nil {
			t.Fatalf("writing %s: %v", f, err)
		}
	}

	// Also create a non-formula file (should be ignored)
	if err := os.WriteFile(filepath.Join(formulasDir, ".installed.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("writing .installed.json: %v", err)
	}

	// Test
	names := getFormulaNames(tmpDir)
	if names == nil {
		t.Fatal("getFormulaNames returned nil")
	}

	expected := []string{"mol-deacon-patrol", "mol-witness-patrol", "shiny"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected formula name %q not found", name)
		}
	}

	// Should not include the .installed.json file
	if names[".installed"] {
		t.Error(".installed should not be in formula names")
	}

	if len(names) != len(expected) {
		t.Errorf("got %d formula names, want %d", len(names), len(expected))
	}
}

func TestGetFormulaNames_NonexistentDir(t *testing.T) {
	names := getFormulaNames("/nonexistent/path")
	if names != nil {
		t.Error("expected nil for nonexistent directory")
	}
}

func TestFilterFormulaScaffolds(t *testing.T) {
	formulaNames := map[string]bool{
		"mol-deacon-patrol":  true,
		"mol-witness-patrol": true,
	}

	issues := []*beads.Issue{
		{ID: "mol-deacon-patrol", Title: "mol-deacon-patrol"},
		{ID: "mol-deacon-patrol.inbox-check", Title: "Handle callbacks"},
		{ID: "mol-deacon-patrol.health-scan", Title: "Check health"},
		{ID: "mol-witness-patrol", Title: "mol-witness-patrol"},
		{ID: "mol-witness-patrol.loop-or-exit", Title: "Loop or exit"},
		{ID: "hq-123", Title: "Real work item"},
		{ID: "hq-wisp-abc", Title: "Actual wisp"},
		{ID: "gt-456", Title: "Project issue"},
	}

	filtered := filterFormulaScaffolds(issues, formulaNames)

	// Should only have the non-scaffold issues
	if len(filtered) != 3 {
		t.Errorf("got %d filtered issues, want 3", len(filtered))
	}

	expectedIDs := map[string]bool{
		"hq-123":      true,
		"hq-wisp-abc": true,
		"gt-456":      true,
	}
	for _, issue := range filtered {
		if !expectedIDs[issue.ID] {
			t.Errorf("unexpected issue in filtered result: %s", issue.ID)
		}
	}
}

func TestFilterFormulaScaffolds_NilFormulaNames(t *testing.T) {
	issues := []*beads.Issue{
		{ID: "hq-123", Title: "Real work"},
		{ID: "mol-deacon-patrol", Title: "Would be filtered"},
	}

	// With nil formula names, should return all issues unchanged
	filtered := filterFormulaScaffolds(issues, nil)
	if len(filtered) != len(issues) {
		t.Errorf("got %d issues, want %d (nil formulaNames should return all)", len(filtered), len(issues))
	}
}

func TestFilterFormulaScaffolds_EmptyFormulaNames(t *testing.T) {
	issues := []*beads.Issue{
		{ID: "hq-123", Title: "Real work"},
		{ID: "mol-deacon-patrol", Title: "Would be filtered"},
	}

	// With empty formula names, should return all issues unchanged
	filtered := filterFormulaScaffolds(issues, map[string]bool{})
	if len(filtered) != len(issues) {
		t.Errorf("got %d issues, want %d (empty formulaNames should return all)", len(filtered), len(issues))
	}
}

func TestFilterFormulaScaffolds_EmptyIssues(t *testing.T) {
	formulaNames := map[string]bool{"mol-deacon-patrol": true}
	filtered := filterFormulaScaffolds([]*beads.Issue{}, formulaNames)
	if len(filtered) != 0 {
		t.Errorf("got %d issues, want 0", len(filtered))
	}
}

func TestFilterFormulaScaffolds_DotInNonScaffold(t *testing.T) {
	// Issue ID has a dot but prefix is not a formula name
	formulaNames := map[string]bool{"mol-deacon-patrol": true}

	issues := []*beads.Issue{
		{ID: "hq-cv.synthesis-step", Title: "Convoy synthesis"},
		{ID: "some.other.thing", Title: "Random dotted ID"},
	}

	filtered := filterFormulaScaffolds(issues, formulaNames)
	if len(filtered) != 2 {
		t.Errorf("got %d issues, want 2 (non-formula dots should not filter)", len(filtered))
	}
}
