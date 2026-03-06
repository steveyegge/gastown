package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallForRole_RoleAware(t *testing.T) {
	// Claude has autonomous/interactive variants
	tests := []struct {
		name     string
		role     string
		wantFile string // expected template used
	}{
		{"autonomous polecat", "polecat", "settings-autonomous.json"},
		{"autonomous witness", "witness", "settings-autonomous.json"},
		{"interactive crew", "crew", "settings-interactive.json"},
		{"interactive mayor", "mayor", "settings-interactive.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			err := InstallForRole("claude", dir, dir, tt.role, ".claude", "settings.json", true)
			if err != nil {
				t.Fatalf("InstallForRole: %v", err)
			}

			path := filepath.Join(dir, ".claude", "settings.json")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatal("settings.json not created")
			}

			// Verify content matches expected template
			got, _ := os.ReadFile(path)
			want, _ := templateFS.ReadFile("templates/claude/" + tt.wantFile)
			if string(got) != string(want) {
				t.Errorf("content mismatch: got %d bytes, want %d bytes (from %s)", len(got), len(want), tt.wantFile)
			}
		})
	}
}

func TestInstallForRole_RoleAgnostic(t *testing.T) {
	// OpenCode, Pi, OMP have single templates
	tests := []struct {
		provider  string
		hooksDir  string
		hooksFile string
	}{
		{"opencode", ".opencode/plugins", "gastown.js"},
		{"pi", ".pi/extensions", "gastown-hooks.js"},
		{"omp", ".omp/hooks", "gastown-hook.ts"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			dir := t.TempDir()
			err := InstallForRole(tt.provider, dir, dir, "polecat", tt.hooksDir, tt.hooksFile, false)
			if err != nil {
				t.Fatalf("InstallForRole(%s): %v", tt.provider, err)
			}

			path := filepath.Join(dir, tt.hooksDir, tt.hooksFile)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("%s not created", tt.hooksFile)
			}
		})
	}
}

func TestInstallForRole_SkipsExisting(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(hooksPath), 0755)
	os.WriteFile(hooksPath, []byte("custom"), 0644)

	err := InstallForRole("claude", dir, dir, "crew", ".claude", "settings.json", true)
	if err != nil {
		t.Fatalf("InstallForRole: %v", err)
	}

	got, _ := os.ReadFile(hooksPath)
	if string(got) != "custom" {
		t.Error("existing file was overwritten")
	}
}

func TestInstallForRole_SettingsDirVsWorkDir(t *testing.T) {
	settingsDir := t.TempDir()
	workDir := t.TempDir()

	// Claude uses settingsDir (useSettingsDir=true)
	err := InstallForRole("claude", settingsDir, workDir, "crew", ".claude", "settings.json", true)
	if err != nil {
		t.Fatalf("InstallForRole (claude): %v", err)
	}
	if _, err := os.Stat(filepath.Join(settingsDir, ".claude", "settings.json")); os.IsNotExist(err) {
		t.Error("claude: file not in settingsDir")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error("claude: file should not be in workDir")
	}

	// OpenCode uses workDir (useSettingsDir=false)
	err = InstallForRole("opencode", settingsDir, workDir, "polecat", ".opencode/plugins", "gastown.js", false)
	if err != nil {
		t.Fatalf("InstallForRole (opencode): %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".opencode/plugins", "gastown.js")); os.IsNotExist(err) {
		t.Error("opencode: file not in workDir")
	}
}

func TestInstallForRole_EmptyProvider(t *testing.T) {
	dir := t.TempDir()
	err := InstallForRole("", dir, dir, "crew", ".claude", "settings.json", false)
	if err != nil {
		t.Fatalf("expected nil error for empty provider, got: %v", err)
	}
}

func TestInstallForRole_Permissions(t *testing.T) {
	dir := t.TempDir()

	// JSON files should get 0600
	err := InstallForRole("claude", dir, dir, "crew", ".claude", "settings.json", true)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, ".claude", "settings.json"))
	if info.Mode().Perm() != 0600 {
		t.Errorf("JSON file perm = %o, want 0600", info.Mode().Perm())
	}

	// Non-JSON files should get 0644
	dir2 := t.TempDir()
	err = InstallForRole("pi", dir2, dir2, "polecat", ".pi/extensions", "gastown-hooks.js", false)
	if err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(filepath.Join(dir2, ".pi/extensions", "gastown-hooks.js"))
	if info.Mode().Perm() != 0644 {
		t.Errorf("JS file perm = %o, want 0644", info.Mode().Perm())
	}
}

func TestInstallForRole_CursorRoleAware(t *testing.T) {
	// Cursor uses hooks-autonomous.json / hooks-interactive.json naming
	dir := t.TempDir()
	err := InstallForRole("cursor", dir, dir, "polecat", ".cursor", "hooks.json", false)
	if err != nil {
		t.Fatalf("InstallForRole(cursor, polecat): %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json"))
	want, _ := templateFS.ReadFile("templates/cursor/hooks-autonomous.json")
	if string(got) != string(want) {
		t.Error("cursor autonomous: content mismatch")
	}

	dir2 := t.TempDir()
	err = InstallForRole("cursor", dir2, dir2, "crew", ".cursor", "hooks.json", false)
	if err != nil {
		t.Fatalf("InstallForRole(cursor, crew): %v", err)
	}

	got, _ = os.ReadFile(filepath.Join(dir2, ".cursor", "hooks.json"))
	want, _ = templateFS.ReadFile("templates/cursor/hooks-interactive.json")
	if string(got) != string(want) {
		t.Error("cursor interactive: content mismatch")
	}
}

func TestInstallForRole_GeminiRoleAware(t *testing.T) {
	dir := t.TempDir()
	err := InstallForRole("gemini", dir, dir, "witness", ".gemini", "settings.json", false)
	if err != nil {
		t.Fatalf("InstallForRole(gemini, witness): %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	want, _ := templateFS.ReadFile("templates/gemini/settings-autonomous.json")
	if string(got) != string(want) {
		t.Error("gemini autonomous: content mismatch")
	}
}

func TestInstallForRole_CopilotRoleAware(t *testing.T) {
	// Copilot uses gastown-autonomous.json / gastown-interactive.json naming
	dir := t.TempDir()
	err := InstallForRole("copilot", dir, dir, "polecat", ".github/hooks", "gastown.json", false)
	if err != nil {
		t.Fatalf("InstallForRole(copilot, polecat): %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, ".github/hooks", "gastown.json"))
	want, _ := templateFS.ReadFile("templates/copilot/gastown-autonomous.json")
	if string(got) != string(want) {
		t.Error("copilot autonomous: content mismatch")
	}

	dir2 := t.TempDir()
	err = InstallForRole("copilot", dir2, dir2, "crew", ".github/hooks", "gastown.json", false)
	if err != nil {
		t.Fatalf("InstallForRole(copilot, crew): %v", err)
	}

	got, _ = os.ReadFile(filepath.Join(dir2, ".github/hooks", "gastown.json"))
	want, _ = templateFS.ReadFile("templates/copilot/gastown-interactive.json")
	if string(got) != string(want) {
		t.Error("copilot interactive: content mismatch")
	}
}
