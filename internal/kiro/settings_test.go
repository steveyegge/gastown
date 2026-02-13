package kiro

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleTypeFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role string
		want RoleType
	}{
		{"polecat", Autonomous},
		{"witness", Autonomous},
		{"refinery", Autonomous},
		{"deacon", Autonomous},
		{"mayor", Interactive},
		{"crew", Interactive},
		{"", Interactive},
		{"unknown", Interactive},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := RoleTypeFor(tt.role); got != tt.want {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestEnsureSettingsAt_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Autonomous, ".kiro", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	path := filepath.Join(dir, ".kiro", "settings.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("settings file not created: %v", err)
	}

	// Verify file permissions (0600)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	// Verify file has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading settings file: %v", err)
	}
	if len(data) == 0 {
		t.Error("settings file is empty")
	}
}

func TestEnsureSettingsAt_SkipsExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create the file first with known content
	kiroDir := filepath.Join(dir, ".kiro")
	if err := os.MkdirAll(kiroDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := []byte(`{"existing": true}`)
	path := filepath.Join(kiroDir, "settings.json")
	if err := os.WriteFile(path, existing, 0600); err != nil {
		t.Fatal(err)
	}

	// EnsureSettingsAt should not overwrite
	err := EnsureSettingsAt(dir, Autonomous, ".kiro", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(existing) {
		t.Error("EnsureSettingsAt overwrote existing file")
	}
}

func TestEnsureSettingsAt_CreatesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nested := filepath.Join(dir, "deep", "nested")

	err := EnsureSettingsAt(nested, Interactive, ".kiro", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	path := filepath.Join(nested, ".kiro", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("settings file not created at nested path: %v", err)
	}
}

func TestEnsureSettingsAt_AutonomousTemplate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Autonomous, ".kiro", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Autonomous template should include mail check --inject in SessionStart
	if !contains(content, "gt mail check --inject") {
		t.Error("autonomous template missing 'gt mail check --inject' in SessionStart")
	}
}

func TestEnsureSettingsAt_InteractiveTemplate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Interactive, ".kiro", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Interactive SessionStart should NOT include mail check --inject
	// (mail is handled via UserPromptSubmit instead)
	// Check that SessionStart line doesn't contain mail check
	// Both templates have gt prime --hook in SessionStart, but only autonomous has mail check there
	if !contains(content, "gt prime --hook") {
		t.Error("interactive template missing 'gt prime --hook'")
	}
}

func TestEnsureSettingsForRoleAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role     string
		wantFile bool
	}{
		{"polecat", true},
		{"crew", true},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			dir := t.TempDir()
			err := EnsureSettingsForRoleAt(dir, tt.role, ".kiro", "settings.json")
			if err != nil {
				t.Fatalf("EnsureSettingsForRoleAt() error = %v", err)
			}
			path := filepath.Join(dir, ".kiro", "settings.json")
			if _, err := os.Stat(path); err != nil {
				t.Errorf("settings file not created for role %q", tt.role)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
