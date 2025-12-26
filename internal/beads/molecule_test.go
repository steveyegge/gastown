package beads

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMoleculeSteps_EmptyDescription(t *testing.T) {
	steps, err := ParseMoleculeSteps("")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestParseMoleculeSteps_NoSteps(t *testing.T) {
	desc := `This is a molecule description without any steps.
Just some prose text.`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

func TestParseMoleculeSteps_SingleStep(t *testing.T) {
	desc := `## Step: implement
Write the code carefully.
Follow existing patterns.`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	step := steps[0]
	if step.Ref != "implement" {
		t.Errorf("Ref = %q, want implement", step.Ref)
	}
	if step.Title != "Write the code carefully." {
		t.Errorf("Title = %q, want 'Write the code carefully.'", step.Title)
	}
	if step.Instructions != "Write the code carefully.\nFollow existing patterns." {
		t.Errorf("Instructions = %q", step.Instructions)
	}
	if len(step.Needs) != 0 {
		t.Errorf("Needs = %v, want empty", step.Needs)
	}
}

func TestParseMoleculeSteps_MultipleSteps(t *testing.T) {
	desc := `This workflow takes a task through multiple stages.

## Step: design
Think about architecture and patterns.
Consider edge cases.

## Step: implement
Write the implementation.
Needs: design

## Step: test
Write comprehensive tests.
Needs: implement

## Step: submit
Submit for review.
Needs: implement, test`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(steps))
	}

	// Check design step
	if steps[0].Ref != "design" {
		t.Errorf("step[0].Ref = %q, want design", steps[0].Ref)
	}
	if len(steps[0].Needs) != 0 {
		t.Errorf("step[0].Needs = %v, want empty", steps[0].Needs)
	}

	// Check implement step
	if steps[1].Ref != "implement" {
		t.Errorf("step[1].Ref = %q, want implement", steps[1].Ref)
	}
	if !reflect.DeepEqual(steps[1].Needs, []string{"design"}) {
		t.Errorf("step[1].Needs = %v, want [design]", steps[1].Needs)
	}

	// Check test step
	if steps[2].Ref != "test" {
		t.Errorf("step[2].Ref = %q, want test", steps[2].Ref)
	}
	if !reflect.DeepEqual(steps[2].Needs, []string{"implement"}) {
		t.Errorf("step[2].Needs = %v, want [implement]", steps[2].Needs)
	}

	// Check submit step with multiple dependencies
	if steps[3].Ref != "submit" {
		t.Errorf("step[3].Ref = %q, want submit", steps[3].Ref)
	}
	if !reflect.DeepEqual(steps[3].Needs, []string{"implement", "test"}) {
		t.Errorf("step[3].Needs = %v, want [implement, test]", steps[3].Needs)
	}
}

func TestParseMoleculeSteps_WithTier(t *testing.T) {
	desc := `## Step: quick-task
Do something simple.
Tier: haiku

## Step: complex-task
Do something complex.
Needs: quick-task
Tier: opus`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if steps[0].Tier != "haiku" {
		t.Errorf("step[0].Tier = %q, want haiku", steps[0].Tier)
	}
	if steps[1].Tier != "opus" {
		t.Errorf("step[1].Tier = %q, want opus", steps[1].Tier)
	}
}

func TestParseMoleculeSteps_WithWaitsFor(t *testing.T) {
	desc := `## Step: survey
Discover work items.

## Step: aggregate
Collect results from dynamically bonded children.
WaitsFor: all-children
Needs: survey

## Step: finish
Wrap up.
WaitsFor: all-children, external-signal
Needs: aggregate`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}

	// survey has no WaitsFor
	if len(steps[0].WaitsFor) != 0 {
		t.Errorf("step[0].WaitsFor = %v, want empty", steps[0].WaitsFor)
	}

	// aggregate waits for all-children
	if !reflect.DeepEqual(steps[1].WaitsFor, []string{"all-children"}) {
		t.Errorf("step[1].WaitsFor = %v, want [all-children]", steps[1].WaitsFor)
	}

	// finish waits for multiple conditions
	if !reflect.DeepEqual(steps[2].WaitsFor, []string{"all-children", "external-signal"}) {
		t.Errorf("step[2].WaitsFor = %v, want [all-children, external-signal]", steps[2].WaitsFor)
	}
}

func TestParseMoleculeSteps_CaseInsensitive(t *testing.T) {
	desc := `## STEP: Design
Plan the work.
NEEDS: nothing
TIER: SONNET
WAITSFOR: All-Children

## step: implement
Write code.
needs: Design
tier: Haiku
waitsfor: some-condition`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	// Note: refs preserve original case
	if steps[0].Ref != "Design" {
		t.Errorf("step[0].Ref = %q, want Design", steps[0].Ref)
	}
	if steps[0].Tier != "sonnet" {
		t.Errorf("step[0].Tier = %q, want sonnet", steps[0].Tier)
	}
	// WaitsFor values preserve case
	if !reflect.DeepEqual(steps[0].WaitsFor, []string{"All-Children"}) {
		t.Errorf("step[0].WaitsFor = %v, want [All-Children]", steps[0].WaitsFor)
	}

	if steps[1].Ref != "implement" {
		t.Errorf("step[1].Ref = %q, want implement", steps[1].Ref)
	}
	if steps[1].Tier != "haiku" {
		t.Errorf("step[1].Tier = %q, want haiku", steps[1].Tier)
	}
	if !reflect.DeepEqual(steps[1].WaitsFor, []string{"some-condition"}) {
		t.Errorf("step[1].WaitsFor = %v, want [some-condition]", steps[1].WaitsFor)
	}
}

func TestParseMoleculeSteps_Shiny(t *testing.T) {
	// The canonical example from the design doc
	desc := `This workflow takes a task from design to merge.

## Step: design
Think carefully about architecture. Consider existing patterns,
trade-offs, testability.

## Step: implement
Write clean code. Follow codebase conventions.
Needs: design

## Step: review
Review for bugs, edge cases, style issues.
Needs: implement

## Step: test
Write and run tests. Cover happy path and edge cases.
Needs: implement

## Step: submit
Submit for merge via refinery.
Needs: review, test`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}

	expected := []struct {
		ref   string
		needs []string
	}{
		{"design", nil},
		{"implement", []string{"design"}},
		{"review", []string{"implement"}},
		{"test", []string{"implement"}},
		{"submit", []string{"review", "test"}},
	}

	for i, exp := range expected {
		if steps[i].Ref != exp.ref {
			t.Errorf("step[%d].Ref = %q, want %q", i, steps[i].Ref, exp.ref)
		}
		if exp.needs == nil {
			if len(steps[i].Needs) != 0 {
				t.Errorf("step[%d].Needs = %v, want empty", i, steps[i].Needs)
			}
		} else if !reflect.DeepEqual(steps[i].Needs, exp.needs) {
			t.Errorf("step[%d].Needs = %v, want %v", i, steps[i].Needs, exp.needs)
		}
	}
}

func TestExpandTemplateVars(t *testing.T) {
	tests := []struct {
		name string
		text string
		ctx  map[string]string
		want string
	}{
		{
			name: "no variables",
			text: "Just plain text",
			ctx:  map[string]string{"foo": "bar"},
			want: "Just plain text",
		},
		{
			name: "single variable",
			text: "Implement {{feature_name}} feature",
			ctx:  map[string]string{"feature_name": "authentication"},
			want: "Implement authentication feature",
		},
		{
			name: "multiple variables",
			text: "Implement {{feature}} in {{file}}",
			ctx:  map[string]string{"feature": "login", "file": "auth.go"},
			want: "Implement login in auth.go",
		},
		{
			name: "unknown variable left as-is",
			text: "Value is {{unknown}}",
			ctx:  map[string]string{"known": "value"},
			want: "Value is {{unknown}}",
		},
		{
			name: "nil context",
			text: "Value is {{var}}",
			ctx:  nil,
			want: "Value is {{var}}",
		},
		{
			name: "empty context",
			text: "Value is {{var}}",
			ctx:  map[string]string{},
			want: "Value is {{var}}",
		},
		{
			name: "repeated variable",
			text: "{{x}} and {{x}} again",
			ctx:  map[string]string{"x": "foo"},
			want: "foo and foo again",
		},
		{
			name: "multiline",
			text: "First line with {{a}}.\nSecond line with {{b}}.",
			ctx:  map[string]string{"a": "alpha", "b": "beta"},
			want: "First line with alpha.\nSecond line with beta.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandTemplateVars(tt.text, tt.ctx)
			if got != tt.want {
				t.Errorf("ExpandTemplateVars() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMoleculeSteps_WithTemplateVars(t *testing.T) {
	desc := `## Step: implement
Implement {{feature_name}} in {{target_file}}.
Follow the existing patterns.`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	// Template vars should be preserved in parsed instructions
	if steps[0].Instructions != "Implement {{feature_name}} in {{target_file}}.\nFollow the existing patterns." {
		t.Errorf("Instructions = %q", steps[0].Instructions)
	}

	// Now expand them
	expanded := ExpandTemplateVars(steps[0].Instructions, map[string]string{
		"feature_name": "user auth",
		"target_file":  "auth.go",
	})

	if expanded != "Implement user auth in auth.go.\nFollow the existing patterns." {
		t.Errorf("expanded = %q", expanded)
	}
}

func TestValidateMolecule_Valid(t *testing.T) {
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: design
Plan the work.

## Step: implement
Write code.
Needs: design`,
	}

	err := ValidateMolecule(mol)
	if err != nil {
		t.Errorf("ValidateMolecule() = %v, want nil", err)
	}
}

func TestValidateMolecule_WrongType(t *testing.T) {
	mol := &Issue{
		ID:          "task-xyz",
		Type:        "task",
		Description: `## Step: design\nPlan.`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for wrong type")
	}
}

func TestValidateMolecule_NoSteps(t *testing.T) {
	mol := &Issue{
		ID:          "mol-xyz",
		Type:        "molecule",
		Description: "Just some description without steps.",
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for no steps")
	}
}

func TestValidateMolecule_DuplicateRef(t *testing.T) {
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: design
Plan the work.

## Step: design
Plan again.`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for duplicate ref")
	}
}

func TestValidateMolecule_UnknownDependency(t *testing.T) {
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: implement
Write code.
Needs: nonexistent`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for unknown dependency")
	}
}

func TestValidateMolecule_SelfDependency(t *testing.T) {
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: implement
Write code.
Needs: implement`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for self-dependency")
	}
}

func TestValidateMolecule_Nil(t *testing.T) {
	err := ValidateMolecule(nil)
	if err == nil {
		t.Error("ValidateMolecule(nil) = nil, want error")
	}
}

func TestParseMoleculeSteps_WhitespaceHandling(t *testing.T) {
	desc := `## Step:   spaced
  Indented instructions.

  More indented content.

Needs:  dep1 , dep2 ,dep3
Tier:   opus  `

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	// Ref preserves original (though trimmed)
	if steps[0].Ref != "spaced" {
		t.Errorf("Ref = %q, want spaced", steps[0].Ref)
	}

	// Dependencies should be trimmed
	expectedDeps := []string{"dep1", "dep2", "dep3"}
	if !reflect.DeepEqual(steps[0].Needs, expectedDeps) {
		t.Errorf("Needs = %v, want %v", steps[0].Needs, expectedDeps)
	}

	// Tier should be lowercase and trimmed
	if steps[0].Tier != "opus" {
		t.Errorf("Tier = %q, want opus", steps[0].Tier)
	}
}

func TestParseMoleculeSteps_EmptyInstructions(t *testing.T) {
	desc := `## Step: empty

## Step: next
Has content.`

	steps, err := ParseMoleculeSteps(desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	// First step has empty instructions, title defaults to ref
	if steps[0].Instructions != "" {
		t.Errorf("step[0].Instructions = %q, want empty", steps[0].Instructions)
	}
	if steps[0].Title != "empty" {
		t.Errorf("step[0].Title = %q, want empty", steps[0].Title)
	}

	// Second step has content
	if steps[1].Instructions != "Has content." {
		t.Errorf("step[1].Instructions = %q", steps[1].Instructions)
	}
}

func TestValidateMolecule_SimpleCycle(t *testing.T) {
	// A -> B -> A (simple 2-node cycle)
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: a
First step.
Needs: b

## Step: b
Second step.
Needs: a`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for cycle")
	}
	if err != nil && !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q should mention 'cycle'", err.Error())
	}
}

func TestValidateMolecule_LongerCycle(t *testing.T) {
	// A -> B -> C -> A (3-node cycle)
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: a
First step.
Needs: c

## Step: b
Second step.
Needs: a

## Step: c
Third step.
Needs: b`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for cycle")
	}
	if err != nil && !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q should mention 'cycle'", err.Error())
	}
}

func TestValidateMolecule_DiamondNoCycle(t *testing.T) {
	// Diamond pattern: A -> B, A -> C, B -> D, C -> D
	// This has no cycle, should pass
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: a
Root step.

## Step: b
Branch 1.
Needs: a

## Step: c
Branch 2.
Needs: a

## Step: d
Merge point.
Needs: b, c`,
	}

	err := ValidateMolecule(mol)
	if err != nil {
		t.Errorf("ValidateMolecule() = %v, want nil (diamond has no cycle)", err)
	}
}

func TestValidateMolecule_CycleInSubgraph(t *testing.T) {
	// Root -> A, A -> B -> C -> A (cycle not involving root)
	mol := &Issue{
		ID:   "mol-xyz",
		Type: "molecule",
		Description: `## Step: root
Starting point.

## Step: a
First in cycle.
Needs: root, c

## Step: b
Second in cycle.
Needs: a

## Step: c
Third in cycle.
Needs: b`,
	}

	err := ValidateMolecule(mol)
	if err == nil {
		t.Error("ValidateMolecule() = nil, want error for cycle in subgraph")
	}
}
