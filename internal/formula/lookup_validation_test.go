package formula

import (
	"testing"
)

// --- GetLeg tests ---

func TestGetLeg_Found(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "alpha", Title: "Alpha Leg", Focus: "focus-a"},
			{ID: "beta", Title: "Beta Leg", Focus: "focus-b"},
			{ID: "gamma", Title: "Gamma Leg", Focus: "focus-c"},
		},
	}

	leg := f.GetLeg("beta")
	if leg == nil {
		t.Fatal("GetLeg(\"beta\") returned nil, want non-nil")
	}
	if leg.ID != "beta" {
		t.Errorf("GetLeg(\"beta\").ID = %q, want %q", leg.ID, "beta")
	}
	if leg.Title != "Beta Leg" {
		t.Errorf("GetLeg(\"beta\").Title = %q, want %q", leg.Title, "Beta Leg")
	}
}

func TestGetLeg_NotFound(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "alpha", Title: "Alpha Leg"},
		},
	}

	leg := f.GetLeg("nonexistent")
	if leg != nil {
		t.Errorf("GetLeg(\"nonexistent\") = %v, want nil", leg)
	}
}

func TestGetLeg_EmptyLegs(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
	}

	leg := f.GetLeg("any")
	if leg != nil {
		t.Errorf("GetLeg on empty legs = %v, want nil", leg)
	}
}

// --- GetTemplate tests ---

func TestGetTemplate_Found(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
		Template: []Template{
			{ID: "draft", Title: "Draft", Description: "Write draft"},
			{ID: "review", Title: "Review", Description: "Review draft", Needs: []string{"draft"}},
			{ID: "publish", Title: "Publish", Description: "Publish final", Needs: []string{"review"}},
		},
	}

	tmpl := f.GetTemplate("review")
	if tmpl == nil {
		t.Fatal("GetTemplate(\"review\") returned nil, want non-nil")
	}
	if tmpl.ID != "review" {
		t.Errorf("GetTemplate(\"review\").ID = %q, want %q", tmpl.ID, "review")
	}
	if tmpl.Title != "Review" {
		t.Errorf("GetTemplate(\"review\").Title = %q, want %q", tmpl.Title, "Review")
	}
	if len(tmpl.Needs) != 1 || tmpl.Needs[0] != "draft" {
		t.Errorf("GetTemplate(\"review\").Needs = %v, want [\"draft\"]", tmpl.Needs)
	}
}

func TestGetTemplate_NotFound(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
		Template: []Template{
			{ID: "draft", Title: "Draft"},
		},
	}

	tmpl := f.GetTemplate("nonexistent")
	if tmpl != nil {
		t.Errorf("GetTemplate(\"nonexistent\") = %v, want nil", tmpl)
	}
}

func TestGetTemplate_EmptyTemplates(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
	}

	tmpl := f.GetTemplate("any")
	if tmpl != nil {
		t.Errorf("GetTemplate on empty templates = %v, want nil", tmpl)
	}
}

// --- GetAspect tests ---

func TestGetAspect_Found(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf", Title: "Performance", Focus: "latency"},
			{ID: "security", Title: "Security", Focus: "auth"},
			{ID: "ux", Title: "User Experience", Focus: "usability"},
		},
	}

	aspect := f.GetAspect("security")
	if aspect == nil {
		t.Fatal("GetAspect(\"security\") returned nil, want non-nil")
	}
	if aspect.ID != "security" {
		t.Errorf("GetAspect(\"security\").ID = %q, want %q", aspect.ID, "security")
	}
	if aspect.Title != "Security" {
		t.Errorf("GetAspect(\"security\").Title = %q, want %q", aspect.Title, "Security")
	}
	if aspect.Focus != "auth" {
		t.Errorf("GetAspect(\"security\").Focus = %q, want %q", aspect.Focus, "auth")
	}
}

func TestGetAspect_NotFound(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf", Title: "Performance"},
		},
	}

	aspect := f.GetAspect("nonexistent")
	if aspect != nil {
		t.Errorf("GetAspect(\"nonexistent\") = %v, want nil", aspect)
	}
}

func TestGetAspect_EmptyAspects(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
	}

	aspect := f.GetAspect("any")
	if aspect != nil {
		t.Errorf("GetAspect on empty aspects = %v, want nil", aspect)
	}
}

// --- GetDependencies tests ---

func TestGetDependencies_Workflow(t *testing.T) {
	f := &Formula{
		Type: TypeWorkflow,
		Name: "test",
		Steps: []Step{
			{ID: "step1", Title: "Step 1"},
			{ID: "step2", Title: "Step 2", Needs: []string{"step1"}},
			{ID: "step3", Title: "Step 3", Needs: []string{"step1", "step2"}},
		},
	}

	// Step with no deps
	deps := f.GetDependencies("step1")
	if deps != nil {
		t.Errorf("GetDependencies(\"step1\") = %v, want nil", deps)
	}

	// Step with one dep
	deps = f.GetDependencies("step2")
	if len(deps) != 1 || deps[0] != "step1" {
		t.Errorf("GetDependencies(\"step2\") = %v, want [\"step1\"]", deps)
	}

	// Step with multiple deps
	deps = f.GetDependencies("step3")
	if len(deps) != 2 {
		t.Errorf("GetDependencies(\"step3\") = %v, want 2 deps", deps)
	}

	// Unknown step
	deps = f.GetDependencies("unknown")
	if deps != nil {
		t.Errorf("GetDependencies(\"unknown\") = %v, want nil", deps)
	}
}

func TestGetDependencies_Expansion(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
		Template: []Template{
			{ID: "draft", Title: "Draft"},
			{ID: "review", Title: "Review", Needs: []string{"draft"}},
		},
	}

	deps := f.GetDependencies("draft")
	if deps != nil {
		t.Errorf("GetDependencies(\"draft\") = %v, want nil", deps)
	}

	deps = f.GetDependencies("review")
	if len(deps) != 1 || deps[0] != "draft" {
		t.Errorf("GetDependencies(\"review\") = %v, want [\"draft\"]", deps)
	}
}

func TestGetDependencies_Convoy(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "leg1", Title: "Leg 1"},
			{ID: "leg2", Title: "Leg 2"},
		},
		Synthesis: &Synthesis{
			Title:     "Combine",
			DependsOn: []string{"leg1", "leg2"},
		},
	}

	// Regular legs have no dependencies in convoy
	deps := f.GetDependencies("leg1")
	if deps != nil {
		t.Errorf("GetDependencies(\"leg1\") = %v, want nil", deps)
	}

	// Synthesis depends on legs
	deps = f.GetDependencies("synthesis")
	if len(deps) != 2 {
		t.Errorf("GetDependencies(\"synthesis\") = %v, want 2 deps", deps)
	}
}

func TestGetDependencies_ConvoyNoSynthesis(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "leg1", Title: "Leg 1"},
		},
	}

	// Without synthesis, "synthesis" ID returns nil
	deps := f.GetDependencies("synthesis")
	if deps != nil {
		t.Errorf("GetDependencies(\"synthesis\") without synthesis = %v, want nil", deps)
	}
}

func TestGetDependencies_Aspect(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf", Title: "Performance"},
		},
	}

	// Aspect type not handled by GetDependencies, should return nil
	deps := f.GetDependencies("perf")
	if deps != nil {
		t.Errorf("GetDependencies for aspect type = %v, want nil", deps)
	}
}

// --- GetAllIDs tests ---

func TestGetAllIDs_Workflow(t *testing.T) {
	f := &Formula{
		Type: TypeWorkflow,
		Name: "test",
		Steps: []Step{
			{ID: "plan"},
			{ID: "build"},
			{ID: "test"},
		},
	}

	ids := f.GetAllIDs()
	if len(ids) != 3 {
		t.Fatalf("GetAllIDs() returned %d IDs, want 3", len(ids))
	}
	expected := []string{"plan", "build", "test"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("GetAllIDs()[%d] = %q, want %q", i, id, expected[i])
		}
	}
}

func TestGetAllIDs_Convoy(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "leg-a"},
			{ID: "leg-b"},
		},
	}

	ids := f.GetAllIDs()
	if len(ids) != 2 {
		t.Fatalf("GetAllIDs() returned %d IDs, want 2", len(ids))
	}
	if ids[0] != "leg-a" || ids[1] != "leg-b" {
		t.Errorf("GetAllIDs() = %v, want [leg-a, leg-b]", ids)
	}
}

func TestGetAllIDs_Expansion(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
		Template: []Template{
			{ID: "tmpl-1"},
			{ID: "tmpl-2"},
			{ID: "tmpl-3"},
		},
	}

	ids := f.GetAllIDs()
	if len(ids) != 3 {
		t.Fatalf("GetAllIDs() returned %d IDs, want 3", len(ids))
	}
	expected := []string{"tmpl-1", "tmpl-2", "tmpl-3"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("GetAllIDs()[%d] = %q, want %q", i, id, expected[i])
		}
	}
}

func TestGetAllIDs_Aspect(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf"},
			{ID: "security"},
		},
	}

	ids := f.GetAllIDs()
	if len(ids) != 2 {
		t.Fatalf("GetAllIDs() returned %d IDs, want 2", len(ids))
	}
	if ids[0] != "perf" || ids[1] != "security" {
		t.Errorf("GetAllIDs() = %v, want [perf, security]", ids)
	}
}

func TestGetAllIDs_Empty(t *testing.T) {
	f := &Formula{
		Type: TypeWorkflow,
		Name: "test",
	}

	ids := f.GetAllIDs()
	if len(ids) != 0 {
		t.Errorf("GetAllIDs() on empty formula = %v, want empty", ids)
	}
}

// --- inferType tests ---

func TestInferType_AlreadySet(t *testing.T) {
	f := &Formula{
		Type: TypeWorkflow,
	}
	f.inferType()
	if f.Type != TypeWorkflow {
		t.Errorf("inferType() changed already-set type to %q", f.Type)
	}
}

func TestInferType_FromSteps(t *testing.T) {
	f := &Formula{
		Steps: []Step{{ID: "s1"}},
	}
	f.inferType()
	if f.Type != TypeWorkflow {
		t.Errorf("inferType() with steps = %q, want %q", f.Type, TypeWorkflow)
	}
}

func TestInferType_FromLegs(t *testing.T) {
	f := &Formula{
		Legs: []Leg{{ID: "l1"}},
	}
	f.inferType()
	if f.Type != TypeConvoy {
		t.Errorf("inferType() with legs = %q, want %q", f.Type, TypeConvoy)
	}
}

func TestInferType_FromTemplate(t *testing.T) {
	f := &Formula{
		Template: []Template{{ID: "t1"}},
	}
	f.inferType()
	if f.Type != TypeExpansion {
		t.Errorf("inferType() with template = %q, want %q", f.Type, TypeExpansion)
	}
}

func TestInferType_FromAspects(t *testing.T) {
	f := &Formula{
		Aspects: []Aspect{{ID: "a1"}},
	}
	f.inferType()
	if f.Type != TypeAspect {
		t.Errorf("inferType() with aspects = %q, want %q", f.Type, TypeAspect)
	}
}

func TestInferType_Empty(t *testing.T) {
	f := &Formula{}
	f.inferType()
	if f.Type != "" {
		t.Errorf("inferType() with no content = %q, want empty", f.Type)
	}
}

func TestInferType_StepsPrecedence(t *testing.T) {
	// Steps should take precedence over legs when both present
	f := &Formula{
		Steps: []Step{{ID: "s1"}},
		Legs:  []Leg{{ID: "l1"}},
	}
	f.inferType()
	if f.Type != TypeWorkflow {
		t.Errorf("inferType() with steps+legs = %q, want %q (steps precedence)", f.Type, TypeWorkflow)
	}
}

// --- validateAspect tests ---

func TestValidateAspect_Valid(t *testing.T) {
	data := []byte(`
formula = "test-aspect"
type = "aspect"
version = 1

[[aspects]]
id = "perf"
title = "Performance"
focus = "latency and throughput"

[[aspects]]
id = "security"
title = "Security"
focus = "authentication and authorization"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if f.Type != TypeAspect {
		t.Errorf("Type = %q, want %q", f.Type, TypeAspect)
	}
	if len(f.Aspects) != 2 {
		t.Errorf("len(Aspects) = %d, want 2", len(f.Aspects))
	}
}

func TestValidateAspect_Empty(t *testing.T) {
	// Aspect formula with no aspects should fail
	f := &Formula{
		Name: "test",
		Type: TypeAspect,
	}
	err := f.Validate()
	if err == nil {
		t.Error("expected error for aspect formula with no aspects")
	}
}

func TestValidateAspect_MissingID(t *testing.T) {
	data := []byte(`
formula = "test-aspect"
type = "aspect"
version = 1

[[aspects]]
title = "Performance"
focus = "latency"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for aspect missing id")
	}
}

func TestValidateAspect_DuplicateID(t *testing.T) {
	data := []byte(`
formula = "test-aspect"
type = "aspect"
version = 1

[[aspects]]
id = "perf"
title = "Performance"
focus = "latency"

[[aspects]]
id = "perf"
title = "Performance Dup"
focus = "throughput"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for duplicate aspect id")
	}
}

// --- ReadySteps for aspect and expansion types ---

func TestReadySteps_Aspect(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf"},
			{ID: "security"},
			{ID: "ux"},
		},
	}

	// All aspects ready initially
	ready := f.ReadySteps(map[string]bool{})
	if len(ready) != 3 {
		t.Errorf("ReadySteps({}) = %v, want 3 aspects", ready)
	}

	// After completing one
	ready = f.ReadySteps(map[string]bool{"perf": true})
	if len(ready) != 2 {
		t.Errorf("ReadySteps({perf}) = %v, want 2 aspects", ready)
	}

	// After completing all
	ready = f.ReadySteps(map[string]bool{"perf": true, "security": true, "ux": true})
	if len(ready) != 0 {
		t.Errorf("ReadySteps(all completed) = %v, want empty", ready)
	}
}

func TestReadySteps_Expansion(t *testing.T) {
	f := &Formula{
		Type: TypeExpansion,
		Name: "test",
		Template: []Template{
			{ID: "draft"},
			{ID: "review", Needs: []string{"draft"}},
			{ID: "publish", Needs: []string{"review"}},
		},
	}

	// Only draft is ready initially
	ready := f.ReadySteps(map[string]bool{})
	if len(ready) != 1 || ready[0] != "draft" {
		t.Errorf("ReadySteps({}) = %v, want [draft]", ready)
	}

	// After completing draft, review is ready
	ready = f.ReadySteps(map[string]bool{"draft": true})
	if len(ready) != 1 || ready[0] != "review" {
		t.Errorf("ReadySteps({draft}) = %v, want [review]", ready)
	}
}

// --- TopologicalSort for convoy and aspect types ---

func TestTopologicalSort_Convoy(t *testing.T) {
	f := &Formula{
		Type: TypeConvoy,
		Name: "test",
		Legs: []Leg{
			{ID: "leg1"},
			{ID: "leg2"},
		},
	}

	order, err := f.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}
	if len(order) != 2 {
		t.Errorf("TopologicalSort() returned %d items, want 2", len(order))
	}
}

func TestTopologicalSort_Aspect(t *testing.T) {
	f := &Formula{
		Type: TypeAspect,
		Name: "test",
		Aspects: []Aspect{
			{ID: "perf"},
			{ID: "security"},
		},
	}

	order, err := f.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}
	if len(order) != 2 {
		t.Errorf("TopologicalSort() returned %d items, want 2", len(order))
	}
}

// --- Validate expansion edge cases ---

func TestValidateExpansion_MissingTemplateID(t *testing.T) {
	data := []byte(`
formula = "test"
type = "expansion"
version = 1

[[template]]
title = "No ID"
description = "Missing id field"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for template missing id")
	}
}

func TestValidateExpansion_DuplicateTemplateID(t *testing.T) {
	data := []byte(`
formula = "test"
type = "expansion"
version = 1

[[template]]
id = "draft"
title = "Draft"

[[template]]
id = "draft"
title = "Draft Dup"
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for duplicate template id")
	}
}

func TestValidateExpansion_UnknownNeed(t *testing.T) {
	data := []byte(`
formula = "test"
type = "expansion"
version = 1

[[template]]
id = "draft"
title = "Draft"
needs = ["nonexistent"]
`)

	_, err := Parse(data)
	if err == nil {
		t.Error("expected error for template referencing unknown need")
	}
}

// --- Infer type via Parse (integration) ---

func TestParse_InferTypeConvoy(t *testing.T) {
	data := []byte(`
formula = "inferred-convoy"
version = 1

[[legs]]
id = "leg1"
title = "Leg 1"
focus = "focus"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if f.Type != TypeConvoy {
		t.Errorf("inferred type = %q, want %q", f.Type, TypeConvoy)
	}
}

func TestParse_InferTypeExpansion(t *testing.T) {
	data := []byte(`
formula = "inferred-expansion"
version = 1

[[template]]
id = "tmpl1"
title = "Template 1"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if f.Type != TypeExpansion {
		t.Errorf("inferred type = %q, want %q", f.Type, TypeExpansion)
	}
}

func TestParse_InferTypeAspect(t *testing.T) {
	data := []byte(`
formula = "inferred-aspect"
version = 1

[[aspects]]
id = "a1"
title = "Aspect 1"
focus = "focus1"
`)

	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if f.Type != TypeAspect {
		t.Errorf("inferred type = %q, want %q", f.Type, TypeAspect)
	}
}
