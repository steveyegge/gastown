package templates

import (
	"fmt"
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
	// The refinery template intentionally uses placeholders
	// (<rebase-target>/<merge-target>) instead of literal branch commands, so this
	// test verifies the rendered rule text + placeholders.
	fallback := fmt.Sprintf("fallback `%s`", data.DefaultBranch)
	alwaysUse := fmt.Sprintf("always use `%s`", data.DefaultBranch)
	if !strings.Contains(output, "Target Resolution Rule (single source):") {
		t.Error("output missing target resolution rule heading")
	}
	if !strings.Contains(output, fallback) {
		t.Errorf("output missing %q - DefaultBranch not being used in target fallback guidance", fallback)
	}
	if !strings.Contains(output, alwaysUse) {
		t.Errorf("output missing %q - DefaultBranch not being used in integration-disabled guidance", alwaysUse)
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

func TestRenderRole_Dog(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:          "dog",
		DogName:       "Fido",
		TownRoot:      "/test/town",
		TownName:      "town",
		WorkDir:       "/test/town/deacon/dogs/Fido",
		DefaultBranch: "main",
		MayorSession:  "gt-town-mayor",
		DeaconSession: "gt-town-deacon",
	}

	output, err := tmpl.RenderRole("dog", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	// Check for key content
	if !strings.Contains(output, "Dog Context") {
		t.Error("output missing 'Dog Context'")
	}
	if !strings.Contains(output, "Fido") {
		t.Error("output missing dog name")
	}
	if !strings.Contains(output, "/test/town") {
		t.Error("output missing town root")
	}
}

// TestRenderRole_Dog_NoHardcodedGtPath verifies the dog template uses {{ .TownRoot }}
// and does not contain hardcoded ~/gt paths.
func TestRenderRole_Dog_NoHardcodedGtPath(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const customTownRoot = "/custom/test/instance"

	data := RoleData{
		Role:          "dog",
		DogName:       "Rover",
		TownRoot:      customTownRoot,
		TownName:      "instance",
		WorkDir:       customTownRoot + "/deacon/dogs/Rover",
		DefaultBranch: "main",
		MayorSession:  "gt-instance-mayor",
		DeaconSession: "gt-instance-deacon",
	}

	output, err := tmpl.RenderRole("dog", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	if strings.Contains(output, "~/gt") {
		var offending []string
		for i, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "~/gt") {
				offending = append(offending, fmt.Sprintf("  line %d: %s", i+1, strings.TrimSpace(line)))
			}
		}
		t.Errorf("rendered dog template still contains hardcoded ~/gt (TownRoot=%q):\n%s",
			customTownRoot, strings.Join(offending, "\n"))
	}

	if !strings.Contains(output, customTownRoot) {
		t.Errorf("rendered dog template does not contain TownRoot %q — paths may be hardcoded", customTownRoot)
	}
}

// TestRenderRole_NoHardcodedGtPath verifies that no role template renders
// a literal "~/gt" path — all path references must use {{ .TownRoot }}.
// This is a regression test for instances running outside ~/gt
// (e.g., test instances at a custom path).
func TestRenderRole_NoHardcodedGtPath(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const customTownRoot2 = "/custom/test/instance"

	roles := []struct {
		role string
		data RoleData
	}{
		{
			role: "polecat",
			data: RoleData{
				Role: "polecat", RigName: "myrig", Polecat: "TestCat",
				TownRoot: customTownRoot2, TownName: "instance",
				WorkDir:       customTownRoot2 + "/myrig/polecats/TestCat",
				DefaultBranch: "main",
				MayorSession:  "gt-instance-mayor", DeaconSession: "gt-instance-deacon",
			},
		},
		{
			role: "mayor",
			data: RoleData{
				Role: "mayor", TownRoot: customTownRoot2, TownName: "instance",
				WorkDir:       customTownRoot2,
				DefaultBranch: "main",
				MayorSession:  "gt-instance-mayor", DeaconSession: "gt-instance-deacon",
			},
		},
		{
			role: "witness",
			data: RoleData{
				Role: "witness", RigName: "myrig",
				TownRoot: customTownRoot2, TownName: "instance",
				WorkDir:       customTownRoot2 + "/myrig/witness",
				DefaultBranch: "main",
				Polecats:      []string{"Cat1", "Cat2"},
				MayorSession:  "gt-instance-mayor", DeaconSession: "gt-instance-deacon",
			},
		},
		{
			role: "crew",
			data: RoleData{
				Role: "crew", RigName: "myrig", Polecat: "TestCrew",
				TownRoot: customTownRoot2, TownName: "instance",
				WorkDir:       customTownRoot2 + "/myrig/crew/TestCrew",
				DefaultBranch: "main",
				MayorSession:  "gt-instance-mayor", DeaconSession: "gt-instance-deacon",
			},
		},
		{
			role: "deacon",
			data: RoleData{
				Role: "deacon", TownRoot: customTownRoot2, TownName: "instance",
				WorkDir:       customTownRoot2,
				DefaultBranch: "main",
				MayorSession:  "gt-instance-mayor", DeaconSession: "gt-instance-deacon",
			},
		},
		// dog tested separately in TestRenderRole_Dog_NoHardcodedGtPath
		// (requires DogName field)
	}

	for _, tc := range roles {
		t.Run(tc.role, func(t *testing.T) {
			output, err := tmpl.RenderRole(tc.role, tc.data)
			if err != nil {
				t.Fatalf("RenderRole(%q) error = %v", tc.role, err)
			}
			if strings.Contains(output, "~/gt") {
				var offending []string
				for i, line := range strings.Split(output, "\n") {
					if strings.Contains(line, "~/gt") {
						offending = append(offending, fmt.Sprintf("  line %d: %s", i+1, strings.TrimSpace(line)))
					}
				}
				t.Errorf("rendered %q template still contains hardcoded ~/gt (TownRoot=%q):\n%s",
					tc.role, customTownRoot2, strings.Join(offending, "\n"))
			}
		})
	}
}

// TestRenderRole_TownRootInOutput verifies that the actual TownRoot value
// appears in the rendered output for roles that reference it in path instructions.
func TestRenderRole_TownRootInOutput(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const customRoot = "/Users/pa/dev/gastown-tests/my-instance"

	roles := []struct {
		role string
		data RoleData
	}{
		{
			role: "polecat",
			data: RoleData{
				Role: "polecat", RigName: "myrig", Polecat: "Sparky",
				TownRoot: customRoot, TownName: "my-instance",
				WorkDir: customRoot + "/myrig/polecats/Sparky", DefaultBranch: "main",
				MayorSession: "gt-my-instance-mayor", DeaconSession: "gt-my-instance-deacon",
			},
		},
		{
			role: "mayor",
			data: RoleData{
				Role: "mayor", TownRoot: customRoot, TownName: "my-instance",
				WorkDir: customRoot, DefaultBranch: "main",
				MayorSession: "gt-my-instance-mayor", DeaconSession: "gt-my-instance-deacon",
			},
		},
		{
			role: "witness",
			data: RoleData{
				Role: "witness", RigName: "myrig",
				TownRoot: customRoot, TownName: "my-instance",
				WorkDir: customRoot + "/myrig/witness", DefaultBranch: "main",
				MayorSession: "gt-my-instance-mayor", DeaconSession: "gt-my-instance-deacon",
			},
		},
		{
			role: "crew",
			data: RoleData{
				Role: "crew", RigName: "myrig", Polecat: "Sparky",
				TownRoot: customRoot, TownName: "my-instance",
				WorkDir: customRoot + "/myrig/crew/Sparky", DefaultBranch: "main",
				MayorSession: "gt-my-instance-mayor", DeaconSession: "gt-my-instance-deacon",
			},
		},
		{
			role: "deacon",
			data: RoleData{
				Role: "deacon", TownRoot: customRoot, TownName: "my-instance",
				WorkDir: customRoot, DefaultBranch: "main",
				MayorSession: "gt-my-instance-mayor", DeaconSession: "gt-my-instance-deacon",
			},
		},
	}

	for _, tc := range roles {
		t.Run(tc.role, func(t *testing.T) {
			output, err := tmpl.RenderRole(tc.role, tc.data)
			if err != nil {
				t.Fatalf("RenderRole(%q) error = %v", tc.role, err)
			}
			if !strings.Contains(output, customRoot) {
				t.Errorf("rendered %q template does not contain TownRoot %q — paths may be hardcoded", tc.role, customRoot)
			}
		})
	}
}

// TestRenderRole_Polecat_CwdInstruction verifies the critical cwd instruction
// uses the actual town root, not a hardcoded ~/gt path.
// Regression test: agents were following hardcoded ~/gt even in test instances.
func TestRenderRole_Polecat_CwdInstruction(t *testing.T) {
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const customRoot = "/srv/gastown-ci"

	data := RoleData{
		Role: "polecat", RigName: "rig1", Polecat: "Worker",
		TownRoot: customRoot, TownName: "gastown-ci",
		WorkDir: customRoot + "/rig1/polecats/Worker", DefaultBranch: "main",
		MayorSession: "gt-gastown-ci-mayor", DeaconSession: "gt-gastown-ci-deacon",
	}

	output, err := tmpl.RenderRole("polecat", data)
	if err != nil {
		t.Fatalf("RenderRole() error = %v", err)
	}

	wantCwd := customRoot + "/rig1/polecats/Worker/"
	if !strings.Contains(output, wantCwd) {
		t.Errorf("cwd instruction missing %q\n(agent would use wrong path for non-default instance)", wantCwd)
	}

	wantNeverEdit := customRoot + "/rig1/"
	if !strings.Contains(output, wantNeverEdit) {
		t.Errorf("NEVER edit instruction missing %q", wantNeverEdit)
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
