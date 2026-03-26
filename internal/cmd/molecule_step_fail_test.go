package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/formula"
)

// TestLoadRollbackStepsFromFormula verifies that rollback_steps are parsed from formula TOML.
func TestLoadRollbackStepsFromFormula(t *testing.T) {
	// Locate the embedded test formula
	formulaDir := filepath.Join("..", "formula", "formulas")
	formulaPath := filepath.Join(formulaDir, "test-rollback-failure.formula.toml")

	if _, err := os.Stat(formulaPath); os.IsNotExist(err) {
		t.Skipf("test formula not found at %s", formulaPath)
	}

	f, err := formula.ParseFile(formulaPath)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", formulaPath, err)
	}

	if f.Name != "test-rollback-failure" {
		t.Errorf("formula name: got %q, want %q", f.Name, "test-rollback-failure")
	}

	if len(f.RollbackSteps) != 3 {
		t.Fatalf("len(rollback_steps): got %d, want 3", len(f.RollbackSteps))
	}

	// Verify order matches definition order (execution reverses this)
	want := []struct {
		id    string
		hasRun bool
	}{
		{"undo-finalize", false},
		{"undo-work", true},
		{"undo-setup", true},
	}
	for i, w := range want {
		got := f.RollbackSteps[i]
		if got.ID != w.id {
			t.Errorf("rollback_steps[%d].id: got %q, want %q", i, got.ID, w.id)
		}
		if w.hasRun && got.Run == "" {
			t.Errorf("rollback_steps[%d] (%s): expected non-empty run field", i, got.ID)
		}
		if !w.hasRun && got.Run != "" {
			t.Errorf("rollback_steps[%d] (%s): expected empty run field, got %q", i, got.ID, got.Run)
		}
	}
}

// TestExecuteRollbackStepsReverseOrder verifies that rollback steps run in reverse (last → first).
func TestExecuteRollbackStepsReverseOrder(t *testing.T) {
	steps := []formula.RollbackStep{
		{ID: "step-a", Title: "Step A", Run: ""},
		{ID: "step-b", Title: "Step B", Run: ""},
		{ID: "step-c", Title: "Step C", Run: ""},
	}

	var executionOrder []string

	// Override executeRollbackCommand to capture order without running real commands
	origFn := rollbackExecFn
	defer func() { rollbackExecFn = origFn }()
	rollbackExecFn = func(id, _ string) error {
		executionOrder = append(executionOrder, id)
		return nil
	}

	// Inject the IDs into the run commands so the capture works
	for i := range steps {
		steps[i].Run = steps[i].ID // non-empty = will call rollbackExecFn
	}

	errs := executeRollbackSteps(steps, "mol-test", "test-formula", "step-x", "test-actor", false)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	wantOrder := []string{"step-c", "step-b", "step-a"}
	if len(executionOrder) != len(wantOrder) {
		t.Fatalf("execution order length: got %d, want %d", len(executionOrder), len(wantOrder))
	}
	for i, want := range wantOrder {
		if executionOrder[i] != want {
			t.Errorf("execution order[%d]: got %q, want %q", i, executionOrder[i], want)
		}
	}
}

// TestExecuteRollbackStepsDryRun verifies dry-run skips command execution.
func TestExecuteRollbackStepsDryRun(t *testing.T) {
	steps := []formula.RollbackStep{
		{ID: "undo-a", Title: "Undo A", Run: "echo undo-a"},
	}

	executed := false
	origFn := rollbackExecFn
	defer func() { rollbackExecFn = origFn }()
	rollbackExecFn = func(_, _ string) error {
		executed = true
		return nil
	}

	errs := executeRollbackSteps(steps, "mol-test", "test-formula", "step-x", "test-actor", true /* dryRun */)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if executed {
		t.Error("dry-run should not execute rollback commands")
	}
}

// TestExecuteRollbackStepsNoRunField verifies steps without a run field log events but don't execute.
func TestExecuteRollbackStepsNoRunField(t *testing.T) {
	steps := []formula.RollbackStep{
		{ID: "undo-x", Title: "Undo X", Run: ""}, // no run command
	}

	executed := false
	origFn := rollbackExecFn
	defer func() { rollbackExecFn = origFn }()
	rollbackExecFn = func(_, _ string) error {
		executed = true
		return nil
	}

	errs := executeRollbackSteps(steps, "mol-test", "test-formula", "step-y", "test-actor", false)

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if executed {
		t.Error("step with empty run field should not invoke rollbackExecFn")
	}
}

// TestExecuteRollbackStepsCommandError verifies errors are collected but all steps run.
func TestExecuteRollbackStepsCommandError(t *testing.T) {
	steps := []formula.RollbackStep{
		{ID: "undo-a", Title: "Undo A", Run: "fail"},
		{ID: "undo-b", Title: "Undo B", Run: "succeed"},
	}

	var executed []string
	origFn := rollbackExecFn
	defer func() { rollbackExecFn = origFn }()
	rollbackExecFn = func(id, cmd string) error {
		executed = append(executed, id)
		if cmd == "fail" {
			return fmt.Errorf("simulated failure")
		}
		return nil
	}

	errs := executeRollbackSteps(steps, "mol-test", "test-formula", "step-z", "test-actor", false)

	// Both steps should have been attempted (all steps run even on error)
	if len(executed) != 2 {
		t.Errorf("expected 2 steps executed, got %d (%v)", len(executed), executed)
	}
	// undo-b runs first (reverse), undo-a runs second
	if len(executed) >= 2 && executed[0] != "undo-b" {
		t.Errorf("first executed should be undo-b (reverse), got %q", executed[0])
	}
	// undo-a fails
	if len(errs) == 0 {
		t.Error("expected error from failed rollback step undo-a")
	}
}
