package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeDashboardJSON(t *testing.T) {
	input := json.RawMessage(`{
		"id": 42,
		"uid": "gastown-overview",
		"title": "Gas Town Overview",
		"version": 13,
		"panels": [{"type": "graph"}]
	}`)

	got, err := normalizeDashboardJSON(input)
	if err != nil {
		t.Fatalf("normalizeDashboardJSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(got, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// id and version should be removed
	if _, ok := result["id"]; ok {
		t.Error("expected 'id' to be removed")
	}
	if _, ok := result["version"]; ok {
		t.Error("expected 'version' to be removed")
	}

	// uid, title, panels should be preserved
	if result["uid"] != "gastown-overview" {
		t.Errorf("uid = %v, want gastown-overview", result["uid"])
	}
	if result["title"] != "Gas Town Overview" {
		t.Errorf("title = %v, want Gas Town Overview", result["title"])
	}

	// Should end with trailing newline
	if got[len(got)-1] != '\n' {
		t.Error("expected trailing newline")
	}
}

func TestNormalizeDashboardJSON_SortedKeys(t *testing.T) {
	input := json.RawMessage(`{"zebra": 1, "alpha": 2, "middle": 3}`)

	got, err := normalizeDashboardJSON(input)
	if err != nil {
		t.Fatalf("normalizeDashboardJSON: %v", err)
	}

	expected := "{\n  \"alpha\": 2,\n  \"middle\": 3,\n  \"zebra\": 1\n}\n"
	if string(got) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestClassifyChange(t *testing.T) {
	dir := t.TempDir()

	// Test "new" — file doesn't exist
	newPath := filepath.Join(dir, "new.json")
	if got := classifyChange(newPath, []byte(`{}`)); got != "new" {
		t.Errorf("classifyChange (new) = %q, want new", got)
	}

	// Test "unchanged" — same content after normalization
	content := `{"title": "Test", "uid": "test-1"}` + "\n"
	normalized, _ := normalizeDashboardJSON(json.RawMessage(content))
	existPath := filepath.Join(dir, "exist.json")
	os.WriteFile(existPath, normalized, 0644)

	if got := classifyChange(existPath, normalized); got != "unchanged" {
		t.Errorf("classifyChange (unchanged) = %q, want unchanged", got)
	}

	// Test "unchanged" even when existing has id/version (they get stripped for comparison)
	withVolatile := `{"id": 5, "title": "Test", "uid": "test-1", "version": 3}` + "\n"
	os.WriteFile(existPath, []byte(withVolatile), 0644)
	if got := classifyChange(existPath, normalized); got != "unchanged" {
		t.Errorf("classifyChange (unchanged with volatile) = %q, want unchanged", got)
	}

	// Test "updated" — different content
	different, _ := normalizeDashboardJSON(json.RawMessage(`{"title": "Changed", "uid": "test-1"}`))
	if got := classifyChange(existPath, different); got != "updated" {
		t.Errorf("classifyChange (updated) = %q, want updated", got)
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		path, root, want string
	}{
		{"/home/user/gt/sfgastown/dashboards", "/home/user/gt", "sfgastown/dashboards"},
		{"/other/path", "/home/user/gt", "../../other/path"},
	}

	for _, tt := range tests {
		got := shortenPath(tt.path, tt.root)
		// For paths that go above root, shortenPath returns the absolute path
		if tt.want == "../../other/path" {
			if got != tt.path {
				t.Errorf("shortenPath(%q, %q) = %q, want %q", tt.path, tt.root, got, tt.path)
			}
		} else if got != tt.want {
			t.Errorf("shortenPath(%q, %q) = %q, want %q", tt.path, tt.root, got, tt.want)
		}
	}
}
