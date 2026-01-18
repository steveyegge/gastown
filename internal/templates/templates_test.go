package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
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

	// Check that the custom default branch is used in git commands
	if !strings.Contains(output, "origin/develop") {
		t.Error("output missing 'origin/develop' - DefaultBranch not being used for rebase")
	}
	if !strings.Contains(output, "git checkout develop") {
		t.Error("output missing 'git checkout develop' - DefaultBranch not being used for checkout")
	}
	if !strings.Contains(output, "git push origin develop") {
		t.Error("output missing 'git push origin develop' - DefaultBranch not being used for push")
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
	expected := []string{"mayor", "witness", "refinery", "polecat", "crew", "deacon"}

	if len(names) != len(expected) {
		t.Errorf("RoleNames() = %v, want %v", names, expected)
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("RoleNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestGetAllRoleTemplates(t *testing.T) {
	templates, err := GetAllRoleTemplates()
	if err != nil {
		t.Fatalf("GetAllRoleTemplates() error = %v", err)
	}

	if len(templates) == 0 {
		t.Fatal("GetAllRoleTemplates() returned empty map")
	}

	expectedFiles := []string{
		"deacon.md.tmpl",
		"witness.md.tmpl",
		"refinery.md.tmpl",
		"mayor.md.tmpl",
		"polecat.md.tmpl",
		"crew.md.tmpl",
	}

	for _, file := range expectedFiles {
		content, ok := templates[file]
		if !ok {
			t.Errorf("GetAllRoleTemplates() missing %s", file)
			continue
		}
		if len(content) == 0 {
			t.Errorf("GetAllRoleTemplates()[%s] has empty content", file)
		}
	}
}

func TestGetAllRoleTemplates_ContentValidity(t *testing.T) {
	templates, err := GetAllRoleTemplates()
	if err != nil {
		t.Fatalf("GetAllRoleTemplates() error = %v", err)
	}

	for name, content := range templates {
		if !strings.HasSuffix(name, ".md.tmpl") {
			t.Errorf("unexpected file %s (should end with .md.tmpl)", name)
		}
		contentStr := string(content)
		if !strings.Contains(contentStr, "Context") {
			t.Errorf("%s doesn't contain 'Context' - may not be a valid role template", name)
		}
	}
}

func TestGetRoleContent_NoOverride(t *testing.T) {
	// With nil settings, should fall back to embedded template
	data := RoleData{
		Role:          "mayor",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := GetRoleContent("mayor", data, nil, nil)
	if err != nil {
		t.Fatalf("GetRoleContent() error = %v", err)
	}

	// Should contain content from embedded template
	if !strings.Contains(output, "Mayor Context") {
		t.Error("output missing 'Mayor Context' from embedded template")
	}
}

func TestGetRoleContent_TownOverride(t *testing.T) {
	customContent := "# Custom Mayor Context\n\nThis is a custom mayor context."
	townSettings := &config.TownSettings{
		Context: &config.ContextConfig{
			Roles: map[string]string{
				"mayor": customContent,
			},
		},
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
	}

	output, err := GetRoleContent("mayor", data, townSettings, nil)
	if err != nil {
		t.Fatalf("GetRoleContent() error = %v", err)
	}

	if output != customContent {
		t.Errorf("GetRoleContent() = %q, want %q", output, customContent)
	}
}

func TestGetRoleContent_RigOverride(t *testing.T) {
	customContent := "# Rig-specific Mayor Context\n\nCustom for this rig."
	rigSettings := &config.RigSettings{
		Context: &config.ContextConfig{
			Roles: map[string]string{
				"mayor": customContent,
			},
		},
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
	}

	output, err := GetRoleContent("mayor", data, nil, rigSettings)
	if err != nil {
		t.Fatalf("GetRoleContent() error = %v", err)
	}

	if output != customContent {
		t.Errorf("GetRoleContent() = %q, want %q", output, customContent)
	}
}

func TestGetRoleContent_RigOverridesTownPrecedence(t *testing.T) {
	townContent := "# Town Mayor Context"
	rigContent := "# Rig Mayor Context"

	townSettings := &config.TownSettings{
		Context: &config.ContextConfig{
			Roles: map[string]string{
				"mayor": townContent,
			},
		},
	}
	rigSettings := &config.RigSettings{
		Context: &config.ContextConfig{
			Roles: map[string]string{
				"mayor": rigContent,
			},
		},
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
	}

	output, err := GetRoleContent("mayor", data, townSettings, rigSettings)
	if err != nil {
		t.Fatalf("GetRoleContent() error = %v", err)
	}

	// Rig should take precedence over town
	if output != rigContent {
		t.Errorf("GetRoleContent() = %q, want %q (rig should override town)", output, rigContent)
	}
}

func TestGetRoleContent_PartialOverride(t *testing.T) {
	// Town has override for mayor, but we're asking for witness
	townSettings := &config.TownSettings{
		Context: &config.ContextConfig{
			Roles: map[string]string{
				"mayor": "# Custom Mayor",
			},
		},
	}

	data := RoleData{
		Role:          "witness",
		RigName:       "testrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/testrig/witness/rig",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := GetRoleContent("witness", data, townSettings, nil)
	if err != nil {
		t.Fatalf("GetRoleContent() error = %v", err)
	}

	// Should fall back to embedded template for witness
	if !strings.Contains(output, "Witness Context") {
		t.Error("output missing 'Witness Context' - should fall back to embedded template")
	}
}

// Tests for Create*CLAUDEmd functions

func TestCreateMayorCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("creating mayor dir: %v", err)
	}

	err := CreateMayorCLAUDEmd(mayorDir, tmpDir, "testtown", "gt-testtown-mayor", "gt-testtown-deacon")
	if err != nil {
		t.Fatalf("CreateMayorCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(mayorDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain rendered template content
	if !strings.Contains(string(content), "Mayor Context") {
		t.Error("CLAUDE.md missing 'Mayor Context'")
	}
	if !strings.Contains(string(content), tmpDir) {
		t.Error("CLAUDE.md missing town root path")
	}
}

func TestCreateWitnessCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	witnessDir := filepath.Join(rigDir, "witness")
	if err := os.MkdirAll(witnessDir, 0755); err != nil {
		t.Fatalf("creating witness dir: %v", err)
	}

	polecats := []string{"alice", "bob"}
	err := CreateWitnessCLAUDEmd(witnessDir, rigDir, "testrig", polecats)
	if err != nil {
		t.Fatalf("CreateWitnessCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(witnessDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain rendered template content
	if !strings.Contains(string(content), "Witness Context") {
		t.Error("CLAUDE.md missing 'Witness Context'")
	}
	if !strings.Contains(string(content), "testrig") {
		t.Error("CLAUDE.md missing rig name")
	}
}

func TestCreateDeaconCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	deaconDir := filepath.Join(tmpDir, "deacon")
	if err := os.MkdirAll(deaconDir, 0755); err != nil {
		t.Fatalf("creating deacon dir: %v", err)
	}

	err := CreateDeaconCLAUDEmd(deaconDir, tmpDir)
	if err != nil {
		t.Fatalf("CreateDeaconCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(deaconDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain rendered template content
	if !strings.Contains(string(content), "Deacon Context") {
		t.Error("CLAUDE.md missing 'Deacon Context'")
	}
	if !strings.Contains(string(content), tmpDir) {
		t.Error("CLAUDE.md missing town root path")
	}
}

func TestCreateRefineryCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	refineryDir := filepath.Join(rigDir, "refinery")
	if err := os.MkdirAll(refineryDir, 0755); err != nil {
		t.Fatalf("creating refinery dir: %v", err)
	}

	err := CreateRefineryCLAUDEmd(refineryDir, rigDir, "testrig", "develop")
	if err != nil {
		t.Fatalf("CreateRefineryCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(refineryDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain rendered template content
	if !strings.Contains(string(content), "Refinery Context") {
		t.Error("CLAUDE.md missing 'Refinery Context'")
	}
	if !strings.Contains(string(content), "testrig") {
		t.Error("CLAUDE.md missing rig name")
	}
	// Should use the custom default branch
	if !strings.Contains(string(content), "develop") {
		t.Error("CLAUDE.md missing custom default branch 'develop'")
	}
}

func TestCreateCrewCLAUDEmd(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	workDir := filepath.Join(rigDir, "polecats", "alice", "rig")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("creating work dir: %v", err)
	}

	err := CreateCrewCLAUDEmd(workDir, rigDir, "testrig", "alice")
	if err != nil {
		t.Fatalf("CreateCrewCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(workDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain rendered template content
	if !strings.Contains(string(content), "Crew Worker Context") {
		t.Error("CLAUDE.md missing 'Crew Worker Context'")
	}
	if !strings.Contains(string(content), "testrig") {
		t.Error("CLAUDE.md missing rig name")
	}
	if !strings.Contains(string(content), "alice") {
		t.Error("CLAUDE.md missing crew name")
	}
}

func TestCreateMayorCLAUDEmd_WithConfigOverride(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	settingsDir := filepath.Join(tmpDir, "settings")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("creating mayor dir: %v", err)
	}
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("creating settings dir: %v", err)
	}

	// Create a settings file with a custom context override
	customContext := "# Custom Mayor Instructions\n\nThis is a custom context for testing."
	settingsContent := `{
		"context": {
			"roles": {
				"mayor": "# Custom Mayor Instructions\n\nThis is a custom context for testing."
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(settingsContent), 0644); err != nil {
		t.Fatalf("writing settings: %v", err)
	}

	err := CreateMayorCLAUDEmd(mayorDir, tmpDir, "testtown", "gt-testtown-mayor", "gt-testtown-deacon")
	if err != nil {
		t.Fatalf("CreateMayorCLAUDEmd() error = %v", err)
	}

	claudePath := filepath.Join(mayorDir, "CLAUDE.md")
	content, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	// Should contain the custom context, not the default template
	if !strings.Contains(string(content), "Custom Mayor Instructions") {
		t.Error("CLAUDE.md should contain custom context override")
	}
	if string(content) != customContext {
		t.Errorf("CLAUDE.md content = %q, want %q", string(content), customContext)
	}
}
