package formula

import (
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

func TestParse_Extends(t *testing.T) {
	data := []byte(`
description = "Shiny workflow with security audit aspect applied."
extends = ["shiny"]
formula = "shiny-secure"
type = "workflow"
version = 1

[compose]
aspects = ["security-audit"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Name != "shiny-secure" {
		t.Errorf("Name = %q, want %q", f.Name, "shiny-secure")
	}
	if len(f.Extends) != 1 || f.Extends[0] != "shiny" {
		t.Errorf("Extends = %v, want [shiny]", f.Extends)
	}
	if f.Compose == nil {
		t.Fatal("Compose is nil")
	}
	if len(f.Compose.Aspects) != 1 || f.Compose.Aspects[0] != "security-audit" {
		t.Errorf("Compose.Aspects = %v, want [security-audit]", f.Compose.Aspects)
	}
}

func TestParse_ComposeExpand(t *testing.T) {
	data := []byte(`
description = "Enterprise workflow"
extends = ["shiny"]
formula = "shiny-enterprise"
type = "workflow"
version = 1

[compose]

[[compose.expand]]
target = "implement"
with = "rule-of-five"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Compose == nil {
		t.Fatal("Compose is nil")
	}
	if len(f.Compose.Expand) != 1 {
		t.Fatalf("len(Compose.Expand) = %d, want 1", len(f.Compose.Expand))
	}
	if f.Compose.Expand[0].Target != "implement" {
		t.Errorf("Compose.Expand[0].Target = %q, want %q", f.Compose.Expand[0].Target, "implement")
	}
	if f.Compose.Expand[0].With != "rule-of-five" {
		t.Errorf("Compose.Expand[0].With = %q, want %q", f.Compose.Expand[0].With, "rule-of-five")
	}
}

func TestParse_Advice(t *testing.T) {
	data := []byte(`
description = "Security aspect"
formula = "security-audit"
type = "aspect"
version = 1

[[advice]]
target = "implement"
[advice.around]

[[advice.around.after]]
description = "Post-scan"
id = "{step.id}-security-postscan"
title = "Security postscan for {step.id}"

[[advice.around.before]]
description = "Pre-scan"
id = "{step.id}-security-prescan"
title = "Security prescan for {step.id}"

[[pointcuts]]
glob = "implement"

[[pointcuts]]
glob = "submit"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Advice) != 1 {
		t.Fatalf("len(Advice) = %d, want 1", len(f.Advice))
	}
	if f.Advice[0].Target != "implement" {
		t.Errorf("Advice[0].Target = %q, want %q", f.Advice[0].Target, "implement")
	}
	if f.Advice[0].Around == nil {
		t.Fatal("Advice[0].Around is nil")
	}
	if len(f.Advice[0].Around.Before) != 1 {
		t.Errorf("len(Around.Before) = %d, want 1", len(f.Advice[0].Around.Before))
	}
	if len(f.Advice[0].Around.After) != 1 {
		t.Errorf("len(Around.After) = %d, want 1", len(f.Advice[0].Around.After))
	}
	if f.Advice[0].Around.Before[0].ID != "{step.id}-security-prescan" {
		t.Errorf("Before[0].ID = %q, want %q", f.Advice[0].Around.Before[0].ID, "{step.id}-security-prescan")
	}
	if len(f.Pointcuts) != 2 {
		t.Errorf("len(Pointcuts) = %d, want 2", len(f.Pointcuts))
	}
	if f.Pointcuts[0].Glob != "implement" {
		t.Errorf("Pointcuts[0].Glob = %q, want %q", f.Pointcuts[0].Glob, "implement")
	}
}

func TestParse_Squash(t *testing.T) {
	data := []byte(`
formula = "test-squash"
type = "workflow"
version = 1

[squash]
trigger = "on_complete"
template_type = "work"
include_metrics = true

[[steps]]
id = "step1"
title = "Step 1"
description = "Do something"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Squash == nil {
		t.Fatal("Squash is nil")
	}
	if f.Squash.Trigger != "on_complete" {
		t.Errorf("Squash.Trigger = %q, want %q", f.Squash.Trigger, "on_complete")
	}
	if f.Squash.TemplateType != "work" {
		t.Errorf("Squash.TemplateType = %q, want %q", f.Squash.TemplateType, "work")
	}
	if !f.Squash.IncludeMetrics {
		t.Error("Squash.IncludeMetrics = false, want true")
	}
}

func TestParse_Gate(t *testing.T) {
	data := []byte(`
formula = "test-gate"
type = "workflow"
version = 1

[[steps]]
id = "step1"
title = "Step 1"
description = "Normal step"

[[steps]]
id = "step2"
title = "Step 2"
description = "Gated step"
needs = ["step1"]
gate = { type = "conditional", condition = "no_response_1" }
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(f.Steps))
	}
	if f.Steps[0].Gate != nil {
		t.Error("Steps[0].Gate should be nil")
	}
	if f.Steps[1].Gate == nil {
		t.Fatal("Steps[1].Gate is nil")
	}
	if f.Steps[1].Gate.Type != "conditional" {
		t.Errorf("Gate.Type = %q, want %q", f.Steps[1].Gate.Type, "conditional")
	}
	if f.Steps[1].Gate.Condition != "no_response_1" {
		t.Errorf("Gate.Condition = %q, want %q", f.Steps[1].Gate.Condition, "no_response_1")
	}
}

func TestParse_Presets(t *testing.T) {
	data := []byte(`
formula = "test-presets"
type = "convoy"
version = 1

[[legs]]
id = "leg1"
title = "Leg 1"

[[legs]]
id = "leg2"
title = "Leg 2"

[presets]
[presets.gate]
description = "Light review"
legs = ["leg1"]

[presets.full]
description = "Full review"
legs = ["leg1", "leg2"]
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Presets) != 2 {
		t.Fatalf("len(Presets) = %d, want 2", len(f.Presets))
	}
	gate := f.Presets["gate"]
	if gate.Description != "Light review" {
		t.Errorf("Presets[gate].Description = %q, want %q", gate.Description, "Light review")
	}
	if len(gate.Legs) != 1 || gate.Legs[0] != "leg1" {
		t.Errorf("Presets[gate].Legs = %v, want [leg1]", gate.Legs)
	}
	full := f.Presets["full"]
	if len(full.Legs) != 2 {
		t.Errorf("len(Presets[full].Legs) = %d, want 2", len(full.Legs))
	}
}

func TestValidate_WorkflowNoStepsNoExtends(t *testing.T) {
	data := []byte(`
formula = "test-empty"
type = "workflow"
version = 1
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for workflow with no steps and no extends")
	}
}

func TestValidate_WorkflowExtendsNoSteps(t *testing.T) {
	data := []byte(`
formula = "test-extends"
extends = ["base"]
type = "workflow"
version = 1
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse should succeed for composition formula: %v", err)
	}
	if len(f.Extends) != 1 {
		t.Errorf("Extends = %v, want [base]", f.Extends)
	}
}

func TestValidate_AspectWithAdvice(t *testing.T) {
	data := []byte(`
formula = "test-aspect-advice"
type = "aspect"
version = 1

[[advice]]
target = "implement"
[advice.around]

[[advice.around.before]]
id = "pre-check"
title = "Pre check"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse should succeed for aspect with advice: %v", err)
	}
	if len(f.Advice) != 1 {
		t.Errorf("len(Advice) = %d, want 1", len(f.Advice))
	}
}

func TestValidate_AspectNoAspectsNoAdvice(t *testing.T) {
	data := []byte(`
formula = "test-empty-aspect"
type = "aspect"
version = 1
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for aspect with no aspects and no advice")
	}
}

func TestParse_EmbeddedFormulasWithSquash(t *testing.T) {
	// Parse mol-digest-generate which uses [squash]
	content, err := GetEmbeddedFormulaContent("mol-digest-generate")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent() error: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if f.Squash == nil {
		t.Fatal("Squash should not be nil for mol-digest-generate")
	}
	if f.Squash.Trigger != "on_complete" {
		t.Errorf("Squash.Trigger = %q, want %q", f.Squash.Trigger, "on_complete")
	}
}

func TestParse_EmbeddedFormulasWithGate(t *testing.T) {
	// Parse mol-shutdown-dance which uses gate on steps
	content, err := GetEmbeddedFormulaContent("mol-shutdown-dance")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent() error: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Find a step with a gate
	gateFound := false
	for _, step := range f.Steps {
		if step.Gate != nil {
			gateFound = true
			if step.Gate.Type != "conditional" {
				t.Errorf("Step %q Gate.Type = %q, want %q", step.ID, step.Gate.Type, "conditional")
			}
			break
		}
	}
	if !gateFound {
		t.Error("mol-shutdown-dance should have at least one step with a gate")
	}
}

func TestParse_EmbeddedCompositionFormulas(t *testing.T) {
	// Parse shiny-secure which uses extends + compose
	content, err := GetEmbeddedFormulaContent("shiny-secure")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent() error: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Extends) != 1 || f.Extends[0] != "shiny" {
		t.Errorf("Extends = %v, want [shiny]", f.Extends)
	}
	if f.Compose == nil {
		t.Fatal("Compose should not be nil")
	}
	if len(f.Compose.Aspects) != 1 || f.Compose.Aspects[0] != "security-audit" {
		t.Errorf("Compose.Aspects = %v, want [security-audit]", f.Compose.Aspects)
	}
}

func TestParse_EmbeddedAspectFormula(t *testing.T) {
	// Parse security-audit which uses advice + pointcuts
	content, err := GetEmbeddedFormulaContent("security-audit")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent() error: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Advice) == 0 {
		t.Fatal("security-audit should have advice")
	}
	if len(f.Pointcuts) == 0 {
		t.Fatal("security-audit should have pointcuts")
	}
}

func TestParse_EmbeddedConvoyWithPresets(t *testing.T) {
	// Parse code-review which uses [presets]
	content, err := GetEmbeddedFormulaContent("code-review")
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaContent() error: %v", err)
	}

	f, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(f.Presets) == 0 {
		t.Fatal("code-review should have presets")
	}
	if _, ok := f.Presets["gate"]; !ok {
		t.Error("code-review should have a 'gate' preset")
	}
	if _, ok := f.Presets["full"]; !ok {
		t.Error("code-review should have a 'full' preset")
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
