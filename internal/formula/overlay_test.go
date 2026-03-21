package formula

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFormulaOverlay_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	overlay, err := LoadFormulaOverlay("mol-polecat-work", tmpDir, "gastown")
	require.NoError(t, err)
	assert.Nil(t, overlay)
}

func TestLoadFormulaOverlay_TownLevel(t *testing.T) {
	tmpDir := t.TempDir()
	overlayDir := filepath.Join(tmpDir, "formula-overlays")
	require.NoError(t, os.MkdirAll(overlayDir, 0o755))

	content := `
[[step-overrides]]
step_id = "submit-review"
mode = "replace"
description = "Custom submission instructions"
`
	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "mol-polecat-work.toml"), []byte(content), 0o644))

	overlay, err := LoadFormulaOverlay("mol-polecat-work", tmpDir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, overlay)
	require.Len(t, overlay.StepOverrides, 1)
	assert.Equal(t, "submit-review", overlay.StepOverrides[0].StepID)
	assert.Equal(t, ModeReplace, overlay.StepOverrides[0].Mode)
	assert.Equal(t, "Custom submission instructions", overlay.StepOverrides[0].Description)
}

func TestLoadFormulaOverlay_RigLevel(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "gastown", "formula-overlays")
	require.NoError(t, os.MkdirAll(rigDir, 0o755))

	content := `
[[step-overrides]]
step_id = "build"
mode = "append"
description = "Also run integration tests"
`
	require.NoError(t, os.WriteFile(filepath.Join(rigDir, "mol-polecat-work.toml"), []byte(content), 0o644))

	overlay, err := LoadFormulaOverlay("mol-polecat-work", tmpDir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, overlay)
	require.Len(t, overlay.StepOverrides, 1)
	assert.Equal(t, "build", overlay.StepOverrides[0].StepID)
	assert.Equal(t, ModeAppend, overlay.StepOverrides[0].Mode)
}

func TestLoadFormulaOverlay_RigPrecedence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level overlay.
	townDir := filepath.Join(tmpDir, "formula-overlays")
	require.NoError(t, os.MkdirAll(townDir, 0o755))
	townContent := `
[[step-overrides]]
step_id = "submit-review"
mode = "replace"
description = "Town-level override"
`
	require.NoError(t, os.WriteFile(filepath.Join(townDir, "mol-polecat-work.toml"), []byte(townContent), 0o644))

	// Create rig-level overlay (should win).
	rigDir := filepath.Join(tmpDir, "gastown", "formula-overlays")
	require.NoError(t, os.MkdirAll(rigDir, 0o755))
	rigContent := `
[[step-overrides]]
step_id = "build"
mode = "skip"
`
	require.NoError(t, os.WriteFile(filepath.Join(rigDir, "mol-polecat-work.toml"), []byte(rigContent), 0o644))

	overlay, err := LoadFormulaOverlay("mol-polecat-work", tmpDir, "gastown")
	require.NoError(t, err)
	require.NotNil(t, overlay)
	// Rig-level wins entirely — only its overrides appear.
	require.Len(t, overlay.StepOverrides, 1)
	assert.Equal(t, "build", overlay.StepOverrides[0].StepID)
	assert.Equal(t, ModeSkip, overlay.StepOverrides[0].Mode)
}

func TestLoadFormulaOverlay_InvalidMode(t *testing.T) {
	tmpDir := t.TempDir()
	overlayDir := filepath.Join(tmpDir, "formula-overlays")
	require.NoError(t, os.MkdirAll(overlayDir, 0o755))

	content := `
[[step-overrides]]
step_id = "build"
mode = "delete"
description = "Bad mode"
`
	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "test-formula.toml"), []byte(content), 0o644))

	overlay, err := LoadFormulaOverlay("test-formula", tmpDir, "rig")
	assert.Error(t, err)
	assert.Nil(t, overlay)
	assert.Contains(t, err.Error(), `invalid mode "delete"`)
}

func TestLoadFormulaOverlay_MissingStepID(t *testing.T) {
	tmpDir := t.TempDir()
	overlayDir := filepath.Join(tmpDir, "formula-overlays")
	require.NoError(t, os.MkdirAll(overlayDir, 0o755))

	content := `
[[step-overrides]]
mode = "replace"
description = "No step_id"
`
	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "test-formula.toml"), []byte(content), 0o644))

	overlay, err := LoadFormulaOverlay("test-formula", tmpDir, "rig")
	assert.Error(t, err)
	assert.Nil(t, overlay)
	assert.Contains(t, err.Error(), "step_id is required")
}

func TestLoadFormulaOverlay_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	overlayDir := filepath.Join(tmpDir, "formula-overlays")
	require.NoError(t, os.MkdirAll(overlayDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "test-formula.toml"), []byte("[[invalid"), 0o644))

	overlay, err := LoadFormulaOverlay("test-formula", tmpDir, "rig")
	assert.Error(t, err)
	assert.Nil(t, overlay)
}

func TestApplyOverlays_Nil(t *testing.T) {
	f := &Formula{Steps: []Step{{ID: "a", Description: "original"}}}
	warnings := ApplyOverlays(f, nil)
	assert.Nil(t, warnings)
	assert.Equal(t, "original", f.Steps[0].Description)
}

func TestApplyOverlays_EmptyOverrides(t *testing.T) {
	f := &Formula{Steps: []Step{{ID: "a", Description: "original"}}}
	warnings := ApplyOverlays(f, &FormulaOverlay{})
	assert.Nil(t, warnings)
	assert.Equal(t, "original", f.Steps[0].Description)
}

func TestApplyOverlays_Replace(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "Original description"},
			{ID: "step-2", Description: "Keep this"},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "step-1", Mode: ModeReplace, Description: "Replaced description"},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	assert.Empty(t, warnings)
	assert.Equal(t, "Replaced description", f.Steps[0].Description)
	assert.Equal(t, "Keep this", f.Steps[1].Description)
}

func TestApplyOverlays_Append(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "build", Description: "Run the build"},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "build", Mode: ModeAppend, Description: "Also run integration tests"},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	assert.Empty(t, warnings)
	assert.Equal(t, "Run the build\nAlso run integration tests", f.Steps[0].Description)
}

func TestApplyOverlays_Skip(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "First"},
			{ID: "step-2", Description: "Second", Needs: []string{"step-1"}},
			{ID: "step-3", Description: "Third", Needs: []string{"step-2"}},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "step-2", Mode: ModeSkip},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	assert.Empty(t, warnings)
	// step-2 should be removed.
	require.Len(t, f.Steps, 2)
	assert.Equal(t, "step-1", f.Steps[0].ID)
	assert.Equal(t, "step-3", f.Steps[1].ID)
	// step-3 should now depend on step-1 (inherited from skipped step-2).
	assert.Equal(t, []string{"step-1"}, f.Steps[1].Needs)
}

func TestApplyOverlays_SkipFirstStep(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "First"},
			{ID: "step-2", Description: "Second", Needs: []string{"step-1"}},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "step-1", Mode: ModeSkip},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	assert.Empty(t, warnings)
	require.Len(t, f.Steps, 1)
	assert.Equal(t, "step-2", f.Steps[0].ID)
	// step-2 inherits step-1's needs (empty).
	assert.Empty(t, f.Steps[0].Needs)
}

func TestApplyOverlays_StaleOverride(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "First"},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "nonexistent-step", Mode: ModeReplace, Description: "Won't match"},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "nonexistent-step")
	assert.Contains(t, warnings[0], "stale override")
	// Original step should be unchanged.
	assert.Equal(t, "First", f.Steps[0].Description)
}

func TestApplyOverlays_MultipleOverrides(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "First"},
			{ID: "step-2", Description: "Second", Needs: []string{"step-1"}},
			{ID: "step-3", Description: "Third", Needs: []string{"step-2"}},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "step-1", Mode: ModeReplace, Description: "New first"},
			{StepID: "step-3", Mode: ModeAppend, Description: "Extra instructions"},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	assert.Empty(t, warnings)
	assert.Equal(t, "New first", f.Steps[0].Description)
	assert.Equal(t, "Second", f.Steps[1].Description)
	assert.Equal(t, "Third\nExtra instructions", f.Steps[2].Description)
}

func TestApplyOverlays_MixedStaleAndValid(t *testing.T) {
	f := &Formula{
		Steps: []Step{
			{ID: "step-1", Description: "First"},
		},
	}
	overlay := &FormulaOverlay{
		StepOverrides: []StepOverride{
			{StepID: "step-1", Mode: ModeReplace, Description: "Updated"},
			{StepID: "ghost-step", Mode: ModeSkip},
		},
	}

	warnings := ApplyOverlays(f, overlay)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "ghost-step")
	assert.Equal(t, "Updated", f.Steps[0].Description)
}
