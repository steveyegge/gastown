package templates

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if tmpl == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRenderRole_Mayor(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "mayor",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "Mayor Context") {
		t.Error("output missing 'Mayor Context'")
	}
	if !strings.Contains(output, "/test/town") {
		t.Error("output missing town root")
	}
	if !strings.Contains(output, "global coordinator") {
		t.Error("output missing role description")
	}
}

func TestRenderRole_Polecat(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "polecat",
		RigName:       "myrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/myrig/polecats/TestCat",
		DefaultBranch: "main",
		Polecat:       "TestCat",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("polecat", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "Polecat Context") {
		t.Error("output missing 'Polecat Context'")
	}
	if !strings.Contains(output, "TestCat") {
		t.Error("output missing polecat name")
	}
	if !strings.Contains(output, "myrig") {
		t.Error("output missing rig name")
	}
}

func TestRenderRole_Deacon(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "deacon",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("deacon", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "Deacon Context") {
		t.Error("output missing 'Deacon Context'")
	}
	if !strings.Contains(output, "/test/town") {
		t.Error("output missing town root")
	}
	if !strings.Contains(output, "Patrol Executor") {
		t.Error("output missing role description")
	}
	if !strings.Contains(output, "Startup Protocol: Propulsion") {
		t.Error("output missing startup protocol section")
	}
	if !strings.Contains(output, "mol-deacon-patrol") {
		t.Error("output missing patrol molecule reference")
	}
}

func TestRenderRole_Refinery_DefaultBranch(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test with custom default branch (e.g., "develop")
	data := RoleData{
		Role:          "refinery",
		RigName:       "myrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/myrig/refinery/rig",
		DefaultBranch: "develop",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("refinery", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	// Check that the custom default branch is used in target-resolution guidance.
	// The refinery template now uses placeholders (<rebase-target>/<merge-target>)
	// instead of hardcoding literal branch commands.
	if !strings.Contains(output, "fallback develop") {
		t.Error("output missing 'fallback develop' - DefaultBranch not being used in target fallback guidance")
	}
	if !strings.Contains(output, "always use develop") {
		t.Error("output missing 'always use develop' - DefaultBranch not being used in integration-disabled guidance")
	}
	if !strings.Contains(output, "git rebase origin/<rebase-target>") {
		t.Error("output missing placeholder rebase command")
	}
	if !strings.Contains(output, "git checkout <merge-target>") {
		t.Error("output missing placeholder checkout command")
	}
	if !strings.Contains(output, "git push origin <merge-target>") {
		t.Error("output missing placeholder push command")
	}

	// Verify it does NOT contain hardcoded "main" in git commands
	// (main may appear in other contexts like "main branch" descriptions, so we check specific patterns)
	if strings.Contains(output, "git rebase origin/main") {
		t.Error("output still contains hardcoded 'git rebase origin/main' - should use DefaultBranch")
	}
	if strings.Contains(output, "git checkout main") {
		t.Error("output still contains hardcoded 'git checkout main' - should use DefaultBranch")
	}
	if strings.Contains(output, "git push origin main") {
		t.Error("output still contains hardcoded 'git push origin main' - should use DefaultBranch")
	}
}

func TestRenderMessage_Spawn(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := SpawnData{
		Issue:       "gt-123",
		Title:       "Test Issue",
		Priority:    1,
		Description: "Test description",
		Branch:      "feature/test",
		RigName:     "myrig",
		Polecat:     "TestCat",
	}

	output, err := tmpl.RenderMessage("spawn", data)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "gt-123") {
		t.Error("output missing issue ID")
	}
	if !strings.Contains(output, "Test Issue") {
		t.Error("output missing issue title")
	}
}

func TestRenderMessage_Nudge(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := NudgeData{
		Polecat:    "TestCat",
		Reason:     "No progress for 30 minutes",
		NudgeCount: 2,
		MaxNudges:  3,
		Issue:      "gt-123",
		Status:     "in_progress",
	}

	output, err := tmpl.RenderMessage("nudge", data)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "TestCat") {
		t.Error("output missing polecat name")
	}
	if !strings.Contains(output, "2/3") {
		t.Error("output missing nudge count")
	}
}

func TestRoleNames(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	names := tmpl.RoleNames()
	expected := []string{"mayor", "witness", "refinery", "polecat", "crew", "deacon", "boot"}

	if len(names) != len(expected) {
		t.Errorf("RoleNames() = %v, want %v", names, expected)
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("RoleNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}
