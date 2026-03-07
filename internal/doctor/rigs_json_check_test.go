package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

const testRigsJSON = `{
  "rigs": {
    "gastown": {"beads": {"prefix": "-"}}
  }
}`

func TestRigsJSONCheck_BothPresent_OK(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "rigs.json"), []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRigsJSONCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
}

func TestRigsJSONCheck_CanonicalOnly_Warning(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRigsJSONCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Errorf("expected Warning, got %s: %s", result.Status, result.Message)
	}
}

func TestRigsJSONCheck_FallbackOnly_Warning(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(townRoot, "rigs.json"), []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRigsJSONCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Errorf("expected Warning, got %s: %s", result.Status, result.Message)
	}
}

func TestRigsJSONCheck_BothMissing_Error(t *testing.T) {
	townRoot := t.TempDir()

	check := NewRigsJSONCheck()
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusError {
		t.Errorf("expected Error, got %s: %s", result.Status, result.Message)
	}
}

func TestRigsJSONCheck_Fix_RestoresCanonicalFromFallback(t *testing.T) {
	townRoot := t.TempDir()
	// Only fallback exists.
	if err := os.WriteFile(filepath.Join(townRoot, "rigs.json"), []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRigsJSONCheck()
	// Run first to populate paths.
	result := check.Run(&CheckContext{TownRoot: townRoot})
	if result.Status != StatusWarning {
		t.Fatalf("expected Warning before fix, got %s", result.Status)
	}

	if !check.CanFix() {
		t.Fatal("expected CanFix() to return true")
	}

	if err := check.Fix(&CheckContext{TownRoot: townRoot}); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Canonical should now exist with correct content.
	canonical := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("canonical not created: %v", err)
	}
	if string(data) != testRigsJSON {
		t.Error("restored canonical content does not match fallback")
	}

	// Temp file should not be left behind.
	if _, err := os.Stat(canonical + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file was not cleaned up after Fix()")
	}
}
