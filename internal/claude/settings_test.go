package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleTypeFor(t *testing.T) {
	tests := []struct {
		role   string
		expect RoleType
	}{
		{"polecat", Autonomous},
		{"witness", Autonomous},
		{"refinery", Autonomous},
		{"deacon", Autonomous},
		{"boot", Autonomous},
		{"mayor", Interactive},
		{"crew", Interactive},
		{"unknown", Interactive},
		{"", Interactive},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := RoleTypeFor(tt.role)
			if got != tt.expect {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", tt.role, got, tt.expect)
			}
		})
	}
}

func TestEnsureSettingsAt_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Interactive, ".claude", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt failed: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("settings file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("settings file is empty")
	}
}

func TestEnsureSettingsAt_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}

	err := EnsureSettingsAt(dir, Interactive, ".claude", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt failed: %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "custom" {
		t.Errorf("settings file was overwritten; got %q, want %q", string(content), "custom")
	}
}

func TestEnsureSettingsAt_Autonomous(t *testing.T) {
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Autonomous, ".claude", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt failed: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}
	if len(content) == 0 {
		t.Error("autonomous settings file is empty")
	}
}

func TestEnsureSettingsAt_CustomDir(t *testing.T) {
	dir := t.TempDir()

	err := EnsureSettingsAt(dir, Interactive, "my-settings", "config.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt failed: %v", err)
	}

	settingsPath := filepath.Join(dir, "my-settings", "config.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not created at custom path: %v", err)
	}
}

func TestEnsureSettings(t *testing.T) {
	dir := t.TempDir()

	err := EnsureSettings(dir, Interactive)
	if err != nil {
		t.Fatalf("EnsureSettings failed: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not created: %v", err)
	}
}

func TestEnsureSettingsForRole(t *testing.T) {
	dir := t.TempDir()

	err := EnsureSettingsForRole(dir, "polecat")
	if err != nil {
		t.Fatalf("EnsureSettingsForRole failed: %v", err)
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not created: %v", err)
	}
}
