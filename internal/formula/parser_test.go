package formula

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Workflow(t *testing.T) {
	data := []byte(`
description = "Test workflow"
formula = "test-workflow"
type = "workflow"
version = 1

[[steps]]
id = "step1"
title = "First Step"
description = "Do the first thing"

[[steps]]
id = "step2"
title = "Second Step"
description = "Do the second thing"
needs = ["step1"]

[[steps]]
id = "step3"
title = "Third Step"
description = "Do the third thing"
needs = ["step2"]

[vars]
[vars.feature]
description = "The feature to implement"
required = true
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Name != "test-workflow" {
		t.Errorf("Name = %q, want %q", f.Name, "test-workflow")
	}
	if f.Type != TypeWorkflow {
		t.Errorf("Type = %q, want %q", f.Type, TypeWorkflow)
	}
	if len(f.Steps) != 3 {
		t.Errorf("len(Steps) = %d, want 3", len(f.Steps))
	}
	if f.Steps[1].Needs[0] != "step1" {
		t.Errorf("step2.Needs[0] = %q, want %q", f.Steps[1].Needs[0], "step1")
	}
}

func TestParse_Convoy(t *testing.T) {
	data := []byte(`
description = "Test convoy"
formula = "test-convoy"
type = "convoy"
version = 1

[[legs]]
id = "leg1"
title = "Leg One"
focus = "Focus area 1"
description = "First leg"

[[legs]]
id = "leg2"
title = "Leg Two"
focus = "Focus area 2"
description = "Second leg"

[synthesis]
title = "Synthesis"
description = "Combine results"
depends_on = ["leg1", "leg2"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Name != "test-convoy" {
		t.Errorf("Name = %q, want %q", f.Name, "test-convoy")
	}
	if f.Type != TypeConvoy {
		t.Errorf("Type = %q, want %q", f.Type, TypeConvoy)
	}
	if len(f.Legs) != 2 {
		t.Errorf("len(Legs) = %d, want 2", len(f.Legs))
	}
	if f.Synthesis == nil {
		t.Fatal("Synthesis is nil")
	}
	if len(f.Synthesis.DependsOn) != 2 {
		t.Errorf("len(Synthesis.DependsOn) = %d, want 2", len(f.Synthesis.DependsOn))
	}
}

func TestParse_Expansion(t *testing.T) {
	data := []byte(`
description = "Test expansion"
formula = "test-expansion"
type = "expansion"
version = 1

[[template]]
id = "{target}.draft"
title = "Draft: {target.title}"
description = "Initial draft"

[[template]]
id = "{target}.refine"
title = "Refine"
description = "Refine the draft"
needs = ["{target}.draft"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Name != "test-expansion" {
		t.Errorf("Name = %q, want %q", f.Name, "test-expansion")
	}
	if f.Type != TypeExpansion {
		t.Errorf("Type = %q, want %q", f.Type, TypeExpansion)
	}
	if len(f.Template) != 2 {
		t.Errorf("len(Template) = %d, want 2", len(f.Template))
	}
}

func TestParse_WorkflowWithAcceptance(t *testing.T) {
	data := []byte(`
description = "Test workflow with acceptance"
formula = "test-acceptance"
type = "workflow"
version = 1

[[steps]]
id = "design"
title = "Design"
description = "Plan it"
acceptance = "Design doc committed"

[[steps]]
id = "implement"
title = "Implement"
description = "Build it"
needs = ["design"]
acceptance = "All code written and committed"

[[steps]]
id = "test"
title = "Test"
description = "Test it"
needs = ["implement"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Steps) != 3 {
		t.Fatalf("len(Steps) = %d, want 3", len(f.Steps))
	}

	// Steps with acceptance criteria
	if f.Steps[0].Acceptance != "Design doc committed" {
		t.Errorf("step[0].Acceptance = %q, want %q", f.Steps[0].Acceptance, "Design doc committed")
	}
	if f.Steps[1].Acceptance != "All code written and committed" {
		t.Errorf("step[1].Acceptance = %q, want %q", f.Steps[1].Acceptance, "All code written and committed")
	}
	// Step without acceptance criteria
	if f.Steps[2].Acceptance != "" {
		t.Errorf("step[2].Acceptance = %q, want empty", f.Steps[2].Acceptance)
	}
}

func TestParse_PourFlag(t *testing.T) {
	// pour = true: steps should be materialized as sub-wisps
	data := []byte(`
description = "Test pour workflow"
formula = "test-pour"
type = "workflow"
version = 1
pour = true

[[steps]]
id = "step1"
title = "First Step"
description = "Do the first thing"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if !f.Pour {
		t.Error("Pour = false, want true")
	}
}

func TestParse_PourFlagDefault(t *testing.T) {
	// Default: pour is false (inline/root-only)
	data := []byte(`
description = "Test inline workflow"
formula = "test-inline"
type = "workflow"
version = 1

[[steps]]
id = "step1"
title = "First Step"
description = "Do the first thing"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Pour {
		t.Error("Pour = true, want false (default)")
	}
}

func TestValidate_MissingName(t *testing.T) {
	data := []byte(`
type = "workflow"
version = 1
[[steps]]
id = "step1"
title = "Step"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for missing formula name")
	}
}

func TestValidate_InvalidType(t *testing.T) {
	data := []byte(`
formula = "test"
type = "invalid"
version = 1
[[steps]]
id = "step1"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestValidate_DuplicateStepID(t *testing.T) {
	data := []byte(`
formula = "test"
type = "workflow"
version = 1
[[steps]]
id = "step1"
title = "Step 1"
[[steps]]
id = "step1"
title = "Step 1 duplicate"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for duplicate step id")
	}
}

func TestValidate_UnknownDependency(t *testing.T) {
	data := []byte(`
formula = "test"
type = "workflow"
version = 1
[[steps]]
id = "step1"
title = "Step 1"
needs = ["nonexistent"]
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for unknown dependency")
	}
}

func TestValidate_Cycle(t *testing.T) {
	data := []byte(`
formula = "test"
type = "workflow"
version = 1
[[steps]]
id = "step1"
title = "Step 1"
needs = ["step2"]
[[steps]]
id = "step2"
title = "Step 2"
needs = ["step1"]
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for cycle")
	}
}

func TestTopologicalSort(t *testing.T) {
	data := []byte(`
formula = "test"
type = "workflow"
version = 1
[[steps]]
id = "step3"
title = "Step 3"
needs = ["step2"]
[[steps]]
id = "step1"
title = "Step 1"
[[steps]]
id = "step2"
title = "Step 2"
needs = ["step1"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	order, err := f.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// step1 must come before step2, step2 must come before step3
	indexOf := func(id string) int {
		for i, x := range order {
			if x == id {
				return i
			}
		}
		return -1
	}

	if indexOf("step1") > indexOf("step2") {
		t.Error("step1 should come before step2")
	}
	if indexOf("step2") > indexOf("step3") {
		t.Error("step2 should come before step3")
	}
}

func TestReadySteps(t *testing.T) {
	data := []byte(`
formula = "test"
type = "workflow"
version = 1
[[steps]]
id = "step1"
title = "Step 1"
[[steps]]
id = "step2"
title = "Step 2"
needs = ["step1"]
[[steps]]
id = "step3"
title = "Step 3"
needs = ["step1"]
[[steps]]
id = "step4"
title = "Step 4"
needs = ["step2", "step3"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Initially only step1 is ready
	ready := f.ReadySteps(map[string]bool{})
	if len(ready) != 1 || ready[0] != "step1" {
		t.Errorf("ReadySteps({}) = %v, want [step1]", ready)
	}

	// After completing step1, step2 and step3 are ready
	ready = f.ReadySteps(map[string]bool{"step1": true})
	if len(ready) != 2 {
		t.Errorf("ReadySteps({step1}) = %v, want [step2, step3]", ready)
	}

	// After completing step1, step2, step3 is still ready
	ready = f.ReadySteps(map[string]bool{"step1": true, "step2": true})
	if len(ready) != 1 || ready[0] != "step3" {
		t.Errorf("ReadySteps({step1, step2}) = %v, want [step3]", ready)
	}

	// After completing step1, step2, step3, only step4 is ready
	ready = f.ReadySteps(map[string]bool{"step1": true, "step2": true, "step3": true})
	if len(ready) != 1 || ready[0] != "step4" {
		t.Errorf("ReadySteps({step1, step2, step3}) = %v, want [step4]", ready)
	}
}

func TestConvoyReadySteps(t *testing.T) {
	data := []byte(`
formula = "test"
type = "convoy"
version = 1
[[legs]]
id = "leg1"
title = "Leg 1"
[[legs]]
id = "leg2"
title = "Leg 2"
[[legs]]
id = "leg3"
title = "Leg 3"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// All legs are ready initially (parallel)
	ready := f.ReadySteps(map[string]bool{})
	if len(ready) != 3 {
		t.Errorf("ReadySteps({}) = %v, want 3 legs", ready)
	}

	// After completing leg1, leg2 and leg3 still ready
	ready = f.ReadySteps(map[string]bool{"leg1": true})
	if len(ready) != 2 {
		t.Errorf("ReadySteps({leg1}) = %v, want 2 legs", ready)
	}
}

func TestParse_ConvoyWithAgent(t *testing.T) {
	t.Parallel()
	data := []byte(`
formula = "agent-test"
type = "convoy"
version = 1
agent = "gemini"

[[legs]]
id = "default-agent"
title = "Uses formula default"
description = "No per-leg agent"

[[legs]]
id = "custom-agent"
title = "Uses custom agent"
description = "Has per-leg agent"
agent = "codex"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Agent != "gemini" {
		t.Errorf("Formula.Agent = %q, want %q", f.Agent, "gemini")
	}
	if len(f.Legs) != 2 {
		t.Fatalf("len(Legs) = %d, want 2", len(f.Legs))
	}
	if f.Legs[0].Agent != "" {
		t.Errorf("Legs[0].Agent = %q, want empty", f.Legs[0].Agent)
	}
	if f.Legs[1].Agent != "codex" {
		t.Errorf("Legs[1].Agent = %q, want %q", f.Legs[1].Agent, "codex")
	}
}

func TestParse_ConvoyWithoutAgent(t *testing.T) {
	t.Parallel()
	// Existing formulas without agent field should continue to work
	data := []byte(`
formula = "no-agent"
type = "convoy"
version = 1

[[legs]]
id = "leg1"
title = "Normal leg"
description = "No agent override"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Agent != "" {
		t.Errorf("Formula.Agent = %q, want empty", f.Agent)
	}
	if f.Legs[0].Agent != "" {
		t.Errorf("Legs[0].Agent = %q, want empty", f.Legs[0].Agent)
	}
}

// TestResolve_ShinyEnterprise verifies that Resolve correctly processes the
// shiny-enterprise formula: inheriting steps from shiny and expanding the
// "implement" step with the rule-of-five template.
func TestResolve_ShinyEnterprise(t *testing.T) {
	data, err := GetEmbeddedFormulaContent("shiny-enterprise")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	// Parse should succeed even though shiny-enterprise has no own steps.
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Extends) == 0 {
		t.Fatal("expected shiny-enterprise to have extends")
	}

	// Resolve should merge shiny steps and apply rule-of-five expansion.
	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// shiny has: design, implement, review, test, submit (5 steps).
	// Expand "implement" with rule-of-five (5 template steps) → 9 total steps.
	wantIDs := []string{
		"design",
		"implement.draft",
		"implement.refine-1",
		"implement.refine-2",
		"implement.refine-3",
		"implement.refine-4",
		"review",
		"test",
		"submit",
	}

	if len(resolved.Steps) != len(wantIDs) {
		t.Fatalf("got %d steps, want %d: %v", len(resolved.Steps), len(wantIDs), stepIDs(resolved))
	}
	for i, want := range wantIDs {
		if got := resolved.Steps[i].ID; got != want {
			t.Errorf("step[%d] = %q, want %q", i, got, want)
		}
	}

	// First expanded step should inherit design's successor role: needs nothing
	// since design has no needs, implement.draft inherits implement's needs
	// which is also empty (design is a needs of implement).
	// Actually: implement.needs = ["design"], so implement.draft.needs = ["design"].
	draftStep := resolved.Steps[1]
	if len(draftStep.Needs) != 1 || draftStep.Needs[0] != "design" {
		t.Errorf("implement.draft.Needs = %v, want [design]", draftStep.Needs)
	}

	// review should now need implement.refine-4 (last expanded step).
	reviewStep := resolved.Steps[6]
	if reviewStep.ID != "review" {
		t.Fatalf("step[6].ID = %q, want review", reviewStep.ID)
	}
	if len(reviewStep.Needs) != 1 || reviewStep.Needs[0] != "implement.refine-4" {
		t.Errorf("review.Needs = %v, want [implement.refine-4]", reviewStep.Needs)
	}
}

// TestResolve_ShinySecure verifies that shiny-secure (extends shiny, aspects only)
// resolves to the shiny steps without error.
func TestResolve_ShinySecure(t *testing.T) {
	data, err := GetEmbeddedFormulaContent("shiny-secure")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// shiny-secure has no expand rules; it should produce the same 5 steps as shiny.
	wantIDs := []string{"design", "implement", "review", "test", "submit"}
	if len(resolved.Steps) != len(wantIDs) {
		t.Fatalf("got %d steps, want %d: %v", len(resolved.Steps), len(wantIDs), stepIDs(resolved))
	}
	for i, want := range wantIDs {
		if got := resolved.Steps[i].ID; got != want {
			t.Errorf("step[%d] = %q, want %q", i, got, want)
		}
	}
}

// TestResolve_CycleDetection verifies that circular extends chains are rejected.
func TestResolve_CycleDetection(t *testing.T) {
	// Create two formulas that extend each other via a temp directory.
	dir := t.TempDir()

	// Formula A extends B.
	aContent := []byte(`formula = "cycle-a"
type = "workflow"
version = 1
extends = ["cycle-b"]
`)
	// Formula B extends A.
	bContent := []byte(`formula = "cycle-b"
type = "workflow"
version = 1
extends = ["cycle-a"]
`)
	if err := os.WriteFile(filepath.Join(dir, "cycle-a.formula.toml"), aContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cycle-b.formula.toml"), bContent, 0644); err != nil {
		t.Fatal(err)
	}

	a, err := Parse(aContent)
	if err != nil {
		t.Fatalf("Parse cycle-a: %v", err)
	}

	_, err = Resolve(a, []string{dir})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular extends") {
		t.Errorf("expected circular extends error, got: %v", err)
	}
}

// TestResolve_NoExtends verifies formulas without extends pass through unchanged.
func TestResolve_NoExtends(t *testing.T) {
	data, err := GetEmbeddedFormulaContent("shiny")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(resolved.Steps) != len(f.Steps) {
		t.Errorf("step count changed: got %d, want %d", len(resolved.Steps), len(f.Steps))
	}
}

// TestParse_ExpansionWithAcceptance verifies that expansion templates can define
// acceptance criteria and that they propagate to generated steps.
func TestParse_ExpansionWithAcceptance(t *testing.T) {
	data := []byte(`
description = "Expansion with acceptance"
formula = "test-acceptance-expansion"
type = "expansion"
version = 1

[[template]]
id = "{target}.step-a"
title = "Step A"
description = "Do A"
acceptance = "A is done"

[[template]]
id = "{target}.step-b"
title = "Step B"
description = "Do B for {target.title}"
needs = ["{target}.step-a"]
acceptance = "B is done for {target.title}"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(f.Template) != 2 {
		t.Fatalf("len(Template) = %d, want 2", len(f.Template))
	}
	if f.Template[0].Acceptance != "A is done" {
		t.Errorf("Template[0].Acceptance = %q, want %q", f.Template[0].Acceptance, "A is done")
	}
	if f.Template[1].Acceptance != "B is done for {target.title}" {
		t.Errorf("Template[1].Acceptance = %q, want %q", f.Template[1].Acceptance, "B is done for {target.title}")
	}

	// Now test that acceptance propagates through expansion.
	workflow := []byte(`
description = "Test workflow"
formula = "test-wf"
type = "workflow"
version = 1

[[steps]]
id = "prep"
title = "Prepare"
description = "Prepare things"

[[steps]]
id = "work"
title = "Do the work"
description = "Main work"
needs = ["prep"]

[[steps]]
id = "finish"
title = "Finish"
description = "Wrap up"
needs = ["work"]
`)

	wf, err := Parse(workflow)
	if err != nil {
		t.Fatalf("Parse workflow: %v", err)
	}

	// Manually apply expansion to test acceptance propagation.
	expanded, err := applyExpandRule(wf.Steps, &ExpandRule{Target: "work", With: "test-acceptance-expansion"}, nil)
	if err != nil {
		// The expansion formula isn't embedded, so load from parsed data.
		// Use a temp dir approach instead.
		dir := t.TempDir()
		if writeErr := os.WriteFile(filepath.Join(dir, "test-acceptance-expansion.formula.toml"), data, 0644); writeErr != nil {
			t.Fatal(writeErr)
		}
		expanded, err = applyExpandRule(wf.Steps, &ExpandRule{Target: "work", With: "test-acceptance-expansion"}, []string{dir})
		if err != nil {
			t.Fatalf("applyExpandRule: %v", err)
		}
	}

	// Should be: prep, work.step-a, work.step-b, finish
	wantIDs := []string{"prep", "work.step-a", "work.step-b", "finish"}
	if len(expanded) != len(wantIDs) {
		t.Fatalf("got %d steps, want %d", len(expanded), len(wantIDs))
	}
	for i, want := range wantIDs {
		if expanded[i].ID != want {
			t.Errorf("step[%d].ID = %q, want %q", i, expanded[i].ID, want)
		}
	}

	// Check acceptance was propagated and placeholders expanded.
	stepA := expanded[1]
	if stepA.Acceptance != "A is done" {
		t.Errorf("work.step-a.Acceptance = %q, want %q", stepA.Acceptance, "A is done")
	}
	stepB := expanded[2]
	if stepB.Acceptance != "B is done for Do the work" {
		t.Errorf("work.step-b.Acceptance = %q, want %q", stepB.Acceptance, "B is done for Do the work")
	}
}

// TestResolve_TDDCycle verifies the tdd-cycle expansion formula parses and validates.
func TestResolve_TDDCycle(t *testing.T) {
	data, err := GetEmbeddedFormulaContent("tdd-cycle")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if f.Type != TypeExpansion {
		t.Errorf("Type = %q, want %q", f.Type, TypeExpansion)
	}
	if len(f.Template) != 5 {
		t.Fatalf("len(Template) = %d, want 5", len(f.Template))
	}

	// Verify template IDs follow the {target}.* pattern.
	wantSuffixes := []string{".write-tests", ".verify-red", ".implement", ".verify-green", ".refactor"}
	for i, want := range wantSuffixes {
		if !strings.HasSuffix(f.Template[i].ID, want) {
			t.Errorf("Template[%d].ID = %q, want suffix %q", i, f.Template[i].ID, want)
		}
	}

	// Verify hard-gate steps have acceptance criteria.
	for _, tmpl := range f.Template {
		if tmpl.Acceptance == "" {
			t.Errorf("Template %q has no acceptance criteria", tmpl.ID)
		}
	}
}

// TestResolve_MonorepoTDD verifies mol-polecat-work-monorepo-tdd resolves correctly:
// the implement step is expanded into 5 TDD sub-steps.
func TestResolve_MonorepoTDD(t *testing.T) {
	data, err := GetEmbeddedFormulaContent("mol-polecat-work-monorepo-tdd")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent: %v", err)
	}

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	resolved, err := Resolve(f, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// mol-polecat-work-monorepo has 10 steps. Expanding "implement" with tdd-cycle
	// (5 template steps) replaces 1 step with 5 → 14 total steps.
	// Original: load-context, branch-setup, implement, self-review, build-check,
	//           commit-changes, push-and-create-pr, validate-ci, handle-ai-reviews, submit-and-exit
	// After: load-context, branch-setup, implement.write-tests, implement.verify-red,
	//        implement.implement, implement.verify-green, implement.refactor,
	//        self-review, build-check, commit-changes, push-and-create-pr,
	//        validate-ci, handle-ai-reviews, submit-and-exit

	// Check that expanded TDD steps exist.
	tddStepIDs := []string{
		"implement.write-tests",
		"implement.verify-red",
		"implement.implement",
		"implement.verify-green",
		"implement.refactor",
	}

	resolvedIDs := stepIDs(resolved)
	for _, want := range tddStepIDs {
		found := false
		for _, got := range resolvedIDs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected step %q in resolved formula; got %v", want, resolvedIDs)
		}
	}

	// The first TDD step should depend on branch-setup (inheriting implement's needs).
	writeTests := resolved.GetStep("implement.write-tests")
	if writeTests == nil {
		t.Fatal("implement.write-tests step not found")
	}
	if len(writeTests.Needs) != 1 || writeTests.Needs[0] != "branch-setup" {
		t.Errorf("implement.write-tests.Needs = %v, want [branch-setup]", writeTests.Needs)
	}

	// self-review should depend on implement.refactor (the last TDD step).
	selfReview := resolved.GetStep("self-review")
	if selfReview == nil {
		t.Fatal("self-review step not found")
	}
	if len(selfReview.Needs) != 1 || selfReview.Needs[0] != "implement.refactor" {
		t.Errorf("self-review.Needs = %v, want [implement.refactor]", selfReview.Needs)
	}

	// Verify hard-gate steps have acceptance criteria propagated.
	verifyRed := resolved.GetStep("implement.verify-red")
	if verifyRed == nil {
		t.Fatal("implement.verify-red step not found")
	}
	if verifyRed.Acceptance == "" {
		t.Error("implement.verify-red should have acceptance criteria")
	}

	verifyGreen := resolved.GetStep("implement.verify-green")
	if verifyGreen == nil {
		t.Fatal("implement.verify-green step not found")
	}
	if verifyGreen.Acceptance == "" {
		t.Error("implement.verify-green should have acceptance criteria")
	}
}

// stepIDs returns the IDs of all steps in a formula for test diagnostics.
func stepIDs(f *Formula) []string {
	ids := make([]string, len(f.Steps))
	for i, s := range f.Steps {
		ids[i] = s.ID
	}
	return ids
}
