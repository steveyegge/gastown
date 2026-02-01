package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupRigNameTestDir(t *testing.T, rigName string, rigConfig *rigConfigLocal, rigsJSON *rigsConfigFile) string {
	t.Helper()
	townRoot := t.TempDir()

	// Create rig directory with config.json
	rigDir := filepath.Join(townRoot, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	if rigConfig != nil {
		data, err := json.MarshalIndent(rigConfig, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rigDir, "config.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create mayor/rigs.json if provided
	if rigsJSON != nil {
		mayorDir := filepath.Join(townRoot, "mayor")
		if err := os.MkdirAll(mayorDir, 0755); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(rigsJSON, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	return townRoot
}

func TestRigNameMismatchCheck_AllMatch(t *testing.T) {
	rigCfg := &rigConfigLocal{
		Type:    "rig",
		Version: 1,
		Name:    "myrig",
		Beads:   &rigConfigBeadsLocal{Prefix: "mr"},
	}
	rigsJSON := &rigsConfigFile{
		Version: 1,
		Rigs: map[string]rigsConfigEntry{
			"myrig": {
				BeadsConfig: &rigsConfigBeadsConfig{Prefix: "mr"},
			},
		},
	}

	townRoot := setupRigNameTestDir(t, "myrig", rigCfg, rigsJSON)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %v", result.Status, result.Details)
	}
}

func TestRigNameMismatchCheck_NameMismatch(t *testing.T) {
	rigCfg := &rigConfigLocal{
		Type:    "rig",
		Version: 1,
		Name:    "oldname",
		Beads:   &rigConfigBeadsLocal{Prefix: "mr"},
	}
	rigsJSON := &rigsConfigFile{
		Version: 1,
		Rigs: map[string]rigsConfigEntry{
			"newname": {
				BeadsConfig: &rigsConfigBeadsConfig{Prefix: "mr"},
			},
		},
	}

	townRoot := setupRigNameTestDir(t, "newname", rigCfg, rigsJSON)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "newname"}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d: %v", len(result.Details), result.Details)
	}
}

func TestRigNameMismatchCheck_PrefixMismatch(t *testing.T) {
	rigCfg := &rigConfigLocal{
		Type:    "rig",
		Version: 1,
		Name:    "myrig",
		Beads:   &rigConfigBeadsLocal{Prefix: "ab"},
	}
	rigsJSON := &rigsConfigFile{
		Version: 1,
		Rigs: map[string]rigsConfigEntry{
			"myrig": {
				BeadsConfig: &rigsConfigBeadsConfig{Prefix: "xy"},
			},
		},
	}

	townRoot := setupRigNameTestDir(t, "myrig", rigCfg, rigsJSON)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d: %v", len(result.Details), result.Details)
	}
}

func TestRigNameMismatchCheck_BothMismatch(t *testing.T) {
	rigCfg := &rigConfigLocal{
		Type:    "rig",
		Version: 1,
		Name:    "wrongname",
		Beads:   &rigConfigBeadsLocal{Prefix: "ab"},
	}
	rigsJSON := &rigsConfigFile{
		Version: 1,
		Rigs: map[string]rigsConfigEntry{
			"myrig": {
				BeadsConfig: &rigsConfigBeadsConfig{Prefix: "xy"},
			},
		},
	}

	townRoot := setupRigNameTestDir(t, "myrig", rigCfg, rigsJSON)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if len(result.Details) != 2 {
		t.Errorf("expected 2 details, got %d: %v", len(result.Details), result.Details)
	}
}

func TestRigNameMismatchCheck_NoConfigJson(t *testing.T) {
	townRoot := t.TempDir()

	// Create rig directory but no config.json
	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for missing config.json, got %v", result.Status)
	}
}

func TestRigNameMismatchCheck_Fix(t *testing.T) {
	rigCfg := &rigConfigLocal{
		Type:      "rig",
		Version:   1,
		Name:      "wrongname",
		GitURL:    "https://example.com/repo.git",
		CreatedAt: json.RawMessage(`"2025-01-01T00:00:00Z"`),
		Beads:     &rigConfigBeadsLocal{Prefix: "ab"},
	}
	rigsJSON := &rigsConfigFile{
		Version: 1,
		Rigs: map[string]rigsConfigEntry{
			"myrig": {
				BeadsConfig: &rigsConfigBeadsConfig{Prefix: "xy"},
			},
		},
	}

	townRoot := setupRigNameTestDir(t, "myrig", rigCfg, rigsJSON)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}

	// Verify mismatch is detected
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Re-read config.json and verify both fields are corrected
	fixed, err := loadRigConfigLocal(filepath.Join(townRoot, "myrig"))
	if err != nil {
		t.Fatalf("failed to load fixed config: %v", err)
	}

	if fixed.Name != "myrig" {
		t.Errorf("expected Name=%q after fix, got %q", "myrig", fixed.Name)
	}
	if fixed.Beads == nil || fixed.Beads.Prefix != "xy" {
		prefix := ""
		if fixed.Beads != nil {
			prefix = fixed.Beads.Prefix
		}
		t.Errorf("expected Beads.Prefix=%q after fix, got %q", "xy", prefix)
	}

	// Re-run check to confirm clean
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %v", result.Status, result.Details)
	}
}

func TestRigNameMismatchCheck_NoRig(t *testing.T) {
	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: t.TempDir(), RigName: ""}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rig specified, got %v", result.Status)
	}
}

func TestRigNameMismatchCheck_NoRigsJson(t *testing.T) {
	// Config name matches directory, but no rigs.json — should only check name
	rigCfg := &rigConfigLocal{
		Type:    "rig",
		Version: 1,
		Name:    "myrig",
		Beads:   &rigConfigBeadsLocal{Prefix: "mr"},
	}

	townRoot := setupRigNameTestDir(t, "myrig", rigCfg, nil)

	check := NewRigNameMismatchCheck()
	ctx := &CheckContext{TownRoot: townRoot, RigName: "myrig"}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when name matches and no rigs.json, got %v: %v", result.Status, result.Details)
	}
}
