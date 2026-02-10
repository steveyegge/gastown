package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

// setupBeadsDir creates a .beads/metadata.json with the given backend.
func setupBeadsDir(t *testing.T, dir, backend string) {
	t.Helper()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"backend":"` + backend + `"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// setupHook creates a .git/hooks/post-merge file with the given content.
func setupHook(t *testing.T, dir, content string) {
	t.Helper()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "post-merge"), []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

// setupRigsJSON creates a mayor/rigs.json with the given rig names.
func setupRigsJSON(t *testing.T, townRoot string, rigNames []string) {
	t.Helper()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigs := "{"
	for i, name := range rigNames {
		if i > 0 {
			rigs += ","
		}
		rigs += `"` + name + `":{"git_url":"https://example.com/` + name + `.git","added_at":"2025-01-01T00:00:00Z"}`
	}
	rigs += "}"
	content := `{"version":1,"rigs":` + rigs + `}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDoltHooksCheck_NoBeadsDir(t *testing.T) {
	townRoot := t.TempDir()
	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_BackendSqlite(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "sqlite")
	setupHook(t, townRoot, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for sqlite backend, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_DoltNoHooks(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	// No .git/hooks directory at all

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no hooks exist, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_DoltShimHook(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# bd-shim v1\nexec bd hooks run post-merge \"$@\"\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for shim hook, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_DoltNonBdHook(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# Custom post-merge hook\necho 'merged'\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for non-bd hook, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_DoltHookWithDoltSkipLogic(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# bd (beads) post-merge hook\n# Check backend\nif [ \"$backend\" = \"dolt\" ]; then exit 0; fi\nbd sync --import-only\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for hook with Dolt skip logic, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltHooksCheck_DoltStaleInlineHook(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for stale inline hook, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) == 0 {
		t.Error("expected details about stale hooks")
	}
	if result.FixHint == "" {
		t.Error("expected a fix hint")
	}
}

func TestDoltHooksCheck_TownOKButRigStale(t *testing.T) {
	townRoot := t.TempDir()

	// Town root: sqlite backend (OK)
	setupBeadsDir(t, townRoot, "sqlite")

	// Set up a rig with dolt backend and stale hook
	rigDir := filepath.Join(townRoot, "myrig")
	setupBeadsDir(t, rigDir, "dolt")
	setupHook(t, rigDir, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")
	setupRigsJSON(t, townRoot, []string{"myrig"})

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for stale rig hook, got %v: %s", result.Status, result.Message)
	}
	found := false
	for _, d := range result.Details {
		if d == `Rig "myrig" has stale JSONL-sync hooks` {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected detail mentioning rig 'myrig', got %v", result.Details)
	}
}

func TestDoltHooksCheck_BothTownAndRigStale(t *testing.T) {
	townRoot := t.TempDir()

	// Town root: dolt + stale
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")

	// Rig: dolt + stale
	rigDir := filepath.Join(townRoot, "rigA")
	setupBeadsDir(t, rigDir, "dolt")
	setupHook(t, rigDir, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")
	setupRigsJSON(t, townRoot, []string{"rigA"})

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) != 2 {
		t.Errorf("expected 2 details (town + rig), got %d: %v", len(result.Details), result.Details)
	}
}

func TestDoltHooksCheck_FixCachesStalePathsFromRun(t *testing.T) {
	townRoot := t.TempDir()
	setupBeadsDir(t, townRoot, "dolt")
	setupHook(t, townRoot, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run populates stalePaths
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v", result.Status)
	}
	if len(check.stalePaths) != 1 {
		t.Fatalf("expected 1 stale path, got %d", len(check.stalePaths))
	}
	if check.stalePaths[0] != townRoot {
		t.Errorf("expected stale path %q, got %q", townRoot, check.stalePaths[0])
	}
}

func TestDoltHooksCheck_MultipleRigs(t *testing.T) {
	townRoot := t.TempDir()

	// No town-level beads
	// Two rigs: one dolt+stale, one dolt+shim (OK)
	rig1 := filepath.Join(townRoot, "rig1")
	setupBeadsDir(t, rig1, "dolt")
	setupHook(t, rig1, "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n")

	rig2 := filepath.Join(townRoot, "rig2")
	setupBeadsDir(t, rig2, "dolt")
	setupHook(t, rig2, "#!/bin/sh\n# bd-shim v1\nexec bd hooks run post-merge\n")

	setupRigsJSON(t, townRoot, []string{"rig1", "rig2"})

	check := NewDoltHooksCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail (only rig1), got %d: %v", len(result.Details), result.Details)
	}
}

// Test helpers

func TestReadBeadsBackend(t *testing.T) {
	t.Run("missing dir", func(t *testing.T) {
		got := readBeadsBackend("/nonexistent/path/.beads")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("not json"), 0644); err != nil {
			t.Fatal(err)
		}
		got := readBeadsBackend(dir)
		if got != "" {
			t.Errorf("expected empty for bad json, got %q", got)
		}
	})

	t.Run("dolt", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"backend":"dolt"}`), 0644); err != nil {
			t.Fatal(err)
		}
		got := readBeadsBackend(dir)
		if got != "dolt" {
			t.Errorf("expected 'dolt', got %q", got)
		}
	})
}

func TestIsHookStaleDolt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"no hook", "", false},
		{"shim hook", "#!/bin/sh\n# bd-shim v1\nexec bd hooks run post-merge\n", false},
		{"non-bd hook", "#!/bin/sh\necho merged\n", false},
		{"hook with dolt skip", "#!/bin/sh\n# bd (beads) post-merge\nif backend == dolt; then skip; fi\n", false},
		{"stale inline hook", "#!/bin/sh\n# bd (beads) post-merge hook\nbd sync --import-only\n", true},
		{"stale hook with bd but no marker", "#!/bin/sh\nbd import -i\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "post-merge"), []byte(tt.content), 0755); err != nil {
					t.Fatal(err)
				}
			}
			got := isHookStaleDolt(dir)
			if got != tt.want {
				t.Errorf("isHookStaleDolt() = %v, want %v", got, tt.want)
			}
		})
	}
}
