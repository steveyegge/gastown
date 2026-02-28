package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureTownTrustedAt_MissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".copilot", "config.json")
	townRoot := filepath.Join(dir, "mytown")

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTrustedFolders(t, configPath)
	abs, _ := filepath.Abs(townRoot)
	if len(got) != 1 || got[0] != abs {
		t.Errorf("expected [%s], got %v", abs, got)
	}
}

func TestEnsureTownTrustedAt_EmptyConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	townRoot := filepath.Join(dir, "mytown")

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTrustedFolders(t, configPath)
	abs, _ := filepath.Abs(townRoot)
	if len(got) != 1 || got[0] != abs {
		t.Errorf("expected [%s], got %v", abs, got)
	}
}

func TestEnsureTownTrustedAt_AlreadyTrusted(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	townRoot := filepath.Join(dir, "mytown")
	abs, _ := filepath.Abs(townRoot)

	writeConfig(t, configPath, map[string]interface{}{
		"trusted_folders": []interface{}{abs},
	})

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTrustedFolders(t, configPath)
	if len(got) != 1 {
		t.Errorf("expected 1 entry (no duplicate), got %v", got)
	}
}

func TestEnsureTownTrustedAt_ParentAlreadyTrusted(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	parent := filepath.Join(dir, "parent")
	townRoot := filepath.Join(parent, "child", "grandchild")
	absParent, _ := filepath.Abs(parent)

	writeConfig(t, configPath, map[string]interface{}{
		"trusted_folders": []interface{}{absParent},
	})

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTrustedFolders(t, configPath)
	if len(got) != 1 {
		t.Errorf("expected 1 entry (parent covers), got %v", got)
	}
}

func TestEnsureTownTrustedAt_SubdirTrusted_StillAdds(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	townRoot := filepath.Join(dir, "mytown")
	subdir := filepath.Join(townRoot, "sub", "dir")
	absSubdir, _ := filepath.Abs(subdir)

	writeConfig(t, configPath, map[string]interface{}{
		"trusted_folders": []interface{}{absSubdir},
	})

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should add townRoot without removing the existing subdir entry.
	got := readTrustedFolders(t, configPath)
	abs, _ := filepath.Abs(townRoot)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (subdir preserved + town added), got %v", got)
	}
	if got[0] != absSubdir {
		t.Errorf("expected first entry %s, got %s", absSubdir, got[0])
	}
	if got[1] != abs {
		t.Errorf("expected second entry %s, got %s", abs, got[1])
	}
}

func TestEnsureTownTrustedAt_PreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	townRoot := filepath.Join(dir, "mytown")

	writeConfig(t, configPath, map[string]interface{}{
		"some_other_key": "some_value",
		"nested":         map[string]interface{}{"a": 1},
	})

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["some_other_key"] != "some_value" {
		t.Errorf("other key lost: %v", config)
	}
	nested, ok := config["nested"].(map[string]interface{})
	if !ok || nested["a"] != float64(1) {
		t.Errorf("nested key lost: %v", config)
	}
}

func TestEnsureTownTrustedAt_MissingTrustedFoldersKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	townRoot := filepath.Join(dir, "mytown")

	writeConfig(t, configPath, map[string]interface{}{
		"other": "value",
	})

	if err := ensureTownTrustedAt(configPath, townRoot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTrustedFolders(t, configPath)
	abs, _ := filepath.Abs(townRoot)
	if len(got) != 1 || got[0] != abs {
		t.Errorf("expected [%s], got %v", abs, got)
	}
}

func TestIsEqualOrParent(t *testing.T) {
	tests := []struct {
		candidate string
		target    string
		want      bool
	}{
		{"/a/b", "/a/b", true},
		{"/a", "/a/b/c", true},
		{"/a/b/c", "/a/b", false},
		{"/a/bc", "/a/b", false},
		{"/a/b", "/a/bc", false},
		{"/", "/a/b", true},
	}
	for _, tt := range tests {
		got := isEqualOrParent(tt.candidate, tt.target)
		if got != tt.want {
			t.Errorf("isEqualOrParent(%q, %q) = %v, want %v", tt.candidate, tt.target, got, tt.want)
		}
	}
}

// --- helpers ---

func readTrustedFolders(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	return extractTrustedFolders(config)
}

func writeConfig(t *testing.T, path string, config map[string]interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
