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

func TestMessageNames(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	names := tmpl.MessageNames()
	expected := []string{"spawn", "nudge", "escalation", "handoff"}

	if len(names) != len(expected) {
		t.Errorf("MessageNames() = %v, want %v", names, expected)
	}

	for i, name := range names {
		if name != expected[i] {
			t.Errorf("MessageNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestRenderRole_Witness(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "witness",
		RigName:       "myrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/myrig/witness/rig",
		DefaultBranch: "main",
		Polecats:      []string{"alpha", "bravo"},
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("witness", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	if !strings.Contains(output, "Witness Context") {
		t.Error("output missing 'Witness Context'")
	}
	if !strings.Contains(output, "myrig") {
		t.Error("output missing rig name")
	}
}

func TestRenderRole_Crew(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "crew",
		RigName:       "myrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/myrig/crew/engineer",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("crew", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	if !strings.Contains(output, "Crew") {
		t.Error("output missing 'Crew'")
	}
	if !strings.Contains(output, "/test/town") {
		t.Error("output missing town root")
	}
}

func TestRenderRole_Boot(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "boot",
		RigName:       "myrig",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/myrig/boot/rig",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("boot", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	if output == "" {
		t.Error("RenderRole(boot) returned empty output")
	}
}

func TestRenderRole_InvalidRole(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = tmpl.RenderRole("nonexistent", RoleData{})
	if err == nil {
		t.Error("RenderRole(nonexistent) should error")
	}
}

func TestRenderMessage_Escalation(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := EscalationData{
		Polecat:     "TestCat",
		Issue:       "gt-456",
		Reason:      "Max nudges exceeded",
		NudgeCount:  3,
		LastStatus:  "in_progress",
		Suggestions: []string{"kill and respawn", "reassign to different polecat"},
	}

	output, err := tmpl.RenderMessage("escalation", data)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	if !strings.Contains(output, "TestCat") {
		t.Error("output missing polecat name")
	}
	if !strings.Contains(output, "gt-456") {
		t.Error("output missing issue ID")
	}
}

func TestRenderMessage_Handoff(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := HandoffData{
		Role:        "polecat",
		CurrentWork: "Fixing bug gt-123",
		Status:      "in_progress",
		NextSteps:   []string{"Run tests", "Push to remote"},
		Notes:       "Almost done",
		PendingMail: 2,
		GitBranch:   "feature/fix",
		GitDirty:    true,
	}

	output, err := tmpl.RenderMessage("handoff", data)
	if err != nil {
		t.Fatalf("RenderMessage() error = %v", err)
	}

	if !strings.Contains(output, "polecat") {
		t.Error("output missing role")
	}
	if !strings.Contains(output, "Fixing bug gt-123") {
		t.Error("output missing current work")
	}
}

func TestRenderMessage_Invalid(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = tmpl.RenderMessage("nonexistent", nil)
	if err == nil {
		t.Error("RenderMessage(nonexistent) should error")
	}
}

func TestCmdName_Default(t *testing.T) {
	// CmdName uses sync.Once so we can only test the current state
	name := CmdName()
	if name == "" {
		t.Error("CmdName() should not return empty string")
	}
}

func TestCreateMayorCLAUDEmd(t *testing.T) {
	dir := t.TempDir()

	created, err := CreateMayorCLAUDEmd(dir, "/test/town", "town", "gt-town-mayor", "gt-town-deacon")
	if err != nil {
		t.Fatalf("CreateMayorCLAUDEmd() error = %v", err)
	}
	if !created {
		t.Error("first call should create file")
	}

	// Second call should not overwrite
	created2, err := CreateMayorCLAUDEmd(dir, "/test/town", "town", "gt-town-mayor", "gt-town-deacon")
	if err != nil {
		t.Fatalf("second CreateMayorCLAUDEmd() error = %v", err)
	}
	if created2 {
		t.Error("second call should not overwrite existing file")
	}
}

func TestAllRolesRender(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, role := range tmpl.RoleNames() {
		t.Run(role, func(t *testing.T) {
			data := RoleData{
				Role:          role,
				RigName:       "testrig",
				TownRoot:      "/test/town",
				TownName:      "town",
				WorkDir:       "/test/town/testrig",
				DefaultBranch: "main",
				Polecat:       "TestCat",
				Polecats:      []string{"alpha"},
				MayorSession:  "gt-town-mayor",
				DeaconSession: "gt-town-deacon",
			}

			output, err := tmpl.RenderRole(role, data)
			if err != nil {
				t.Fatalf("RenderRole(%s) error = %v", role, err)
			}
			if output == "" {
				t.Errorf("RenderRole(%s) returned empty output", role)
			}
		})
	}
}

func TestAllMessagesRender(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	dataMap := map[string]interface{}{
		"spawn": SpawnData{
			Issue: "gt-1", Title: "Test", Priority: 1,
			Description: "desc", Branch: "main", RigName: "rig", Polecat: "cat",
		},
		"nudge": NudgeData{
			Polecat: "cat", Reason: "timeout", NudgeCount: 1, MaxNudges: 3,
			Issue: "gt-1", Status: "in_progress",
		},
		"escalation": EscalationData{
			Polecat: "cat", Issue: "gt-1", Reason: "stuck", NudgeCount: 3,
			LastStatus: "in_progress", Suggestions: []string{"restart"},
		},
		"handoff": HandoffData{
			Role: "polecat", CurrentWork: "task", Status: "done",
			NextSteps: []string{"push"}, GitBranch: "main",
		},
	}

	for _, name := range tmpl.MessageNames() {
		t.Run(name, func(t *testing.T) {
			data, ok := dataMap[name]
			if !ok {
				t.Fatalf("no test data for message %q", name)
			}
			output, err := tmpl.RenderMessage(name, data)
			if err != nil {
				t.Fatalf("RenderMessage(%s) error = %v", name, err)
			}
			if output == "" {
				t.Errorf("RenderMessage(%s) returned empty output", name)
			}
		})
	}
}

func TestCommandNames(t *testing.T) {
	names := CommandNames()
	if len(names) == 0 {
		t.Error("CommandNames() returned empty")
	}
}

func TestHasCommands_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if HasCommands(dir) {
		t.Error("HasCommands() should be false for empty dir")
	}
}

func TestMissingCommands_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	missing := MissingCommands(dir)
	if len(missing) == 0 {
		t.Error("MissingCommands() should list commands for empty dir")
	}
}

func TestProvisionCommands(t *testing.T) {
	dir := t.TempDir()

	if err := ProvisionCommands(dir); err != nil {
		t.Fatalf("ProvisionCommands() error = %v", err)
	}

	if !HasCommands(dir) {
		t.Error("HasCommands() should be true after provisioning")
	}

	missing := MissingCommands(dir)
	if len(missing) != 0 {
		t.Errorf("MissingCommands() = %v after provisioning", missing)
	}
}

func TestProvisionCommands_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := ProvisionCommands(dir); err != nil {
		t.Fatal(err)
	}
	// Second call should not error
	if err := ProvisionCommands(dir); err != nil {
		t.Fatalf("second ProvisionCommands() error = %v", err)
	}
}

