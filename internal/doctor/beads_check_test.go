package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewBeadsDatabaseCheck(t *testing.T) {
	check := NewBeadsDatabaseCheck()

	if check.Name() != "beads-database" {
		t.Errorf("expected name 'beads-database', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBeadsDatabaseCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty database
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Create JSONL with content
	jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"id":"test-1","title":"Test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for empty db with content in jsonl, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_PopulatedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create database with content
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for populated db, got %v", result.Status)
	}
}

func TestNewPrefixMismatchCheck(t *testing.T) {
	check := NewPrefixMismatchCheck()

	if check.Name() != "prefix-mismatch" {
		t.Errorf("expected name 'prefix-mismatch', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestPrefixMismatchCheck_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for no routes, got %v", result.Status)
	}
}

func TestPrefixMismatchCheck_NoRigsJson(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rigs.json, got %v", result.Status)
	}
}

func TestPrefixMismatchCheck_Matching(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with matching gt prefix
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "gt"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for matching prefixes, got %v: %s", result.Status, result.Message)
	}
}

func TestPrefixMismatchCheck_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with WRONG prefix (ga instead of gt)
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "ga"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for prefix mismatch, got %v: %s", result.Status, result.Message)
	}

	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d", len(result.Details))
	}
}

func TestPrefixMismatchCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with WRONG prefix (ga instead of gt)
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "ga"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// First verify there's a mismatch
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected mismatch before fix, got %v", result.Status)
	}

	// Fix it
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify it's now fixed
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}

	// Verify rigs.json was updated
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("failed to load fixed rigs.json: %v (content: %s)", err, data)
	}
	if cfg.Rigs["gastown"].BeadsConfig.Prefix != "gt" {
		t.Errorf("expected prefix 'gt' after fix, got %q", cfg.Rigs["gastown"].BeadsConfig.Prefix)
	}
}
