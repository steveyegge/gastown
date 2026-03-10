package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinSpecialties(t *testing.T) {
	cfg := BuiltinSpecialties()

	if len(cfg.Specialties) != 5 {
		t.Fatalf("BuiltinSpecialties() has %d specialties, want 5", len(cfg.Specialties))
	}

	expected := []string{"frontend", "backend", "tests", "security", "docs"}
	for i, name := range expected {
		if cfg.Specialties[i].Name != name {
			t.Errorf("Specialties[%d].Name = %q, want %q", i, cfg.Specialties[i].Name, name)
		}
	}
}

func TestBuiltinSpecialties_HaveDescriptions(t *testing.T) {
	cfg := BuiltinSpecialties()
	for _, s := range cfg.Specialties {
		if s.Description == "" {
			t.Errorf("specialty %q has empty description", s.Name)
		}
	}
}

func TestBuiltinSpecialties_HaveLabels(t *testing.T) {
	cfg := BuiltinSpecialties()
	for _, s := range cfg.Specialties {
		if len(s.Labels) == 0 {
			t.Errorf("specialty %q has no labels", s.Name)
		}
	}
}

func TestBuiltinSpecialties_HaveFilePatterns(t *testing.T) {
	cfg := BuiltinSpecialties()
	for _, s := range cfg.Specialties {
		if len(s.FilePatterns) == 0 {
			t.Errorf("specialty %q has no file_patterns", s.Name)
		}
	}
}

func TestGetSpecialty(t *testing.T) {
	cfg := BuiltinSpecialties()

	s := cfg.GetSpecialty("frontend")
	if s == nil {
		t.Fatal("GetSpecialty(\"frontend\") returned nil")
	}
	if s.Name != "frontend" {
		t.Errorf("Name = %q, want %q", s.Name, "frontend")
	}
}

func TestGetSpecialty_NotFound(t *testing.T) {
	cfg := BuiltinSpecialties()
	s := cfg.GetSpecialty("nonexistent")
	if s != nil {
		t.Errorf("GetSpecialty(\"nonexistent\") should return nil, got %v", s)
	}
}

func TestNames(t *testing.T) {
	cfg := BuiltinSpecialties()
	names := cfg.Names()
	if len(names) != 5 {
		t.Fatalf("Names() returned %d names, want 5", len(names))
	}
	if names[0] != "frontend" {
		t.Errorf("Names()[0] = %q, want %q", names[0], "frontend")
	}
}

func TestMatchLabels(t *testing.T) {
	cfg := BuiltinSpecialties()

	matches := cfg.MatchLabels([]string{"api"})
	if len(matches) != 1 {
		t.Fatalf("MatchLabels([\"api\"]) returned %d matches, want 1", len(matches))
	}
	if matches[0].Name != "backend" {
		t.Errorf("match.Name = %q, want %q", matches[0].Name, "backend")
	}
}

func TestMatchLabels_Multiple(t *testing.T) {
	cfg := BuiltinSpecialties()

	matches := cfg.MatchLabels([]string{"ui", "testing"})
	if len(matches) != 2 {
		t.Fatalf("MatchLabels returned %d matches, want 2", len(matches))
	}

	names := make(map[string]bool)
	for _, m := range matches {
		names[m.Name] = true
	}
	if !names["frontend"] {
		t.Error("expected frontend in matches")
	}
	if !names["tests"] {
		t.Error("expected tests in matches")
	}
}

func TestMatchLabels_NoMatch(t *testing.T) {
	cfg := BuiltinSpecialties()
	matches := cfg.MatchLabels([]string{"nonexistent"})
	if len(matches) != 0 {
		t.Errorf("MatchLabels returned %d matches, want 0", len(matches))
	}
}

func TestLoadSpecialties_NoFile(t *testing.T) {
	cfg, err := LoadSpecialties(t.TempDir())
	if err != nil {
		t.Fatalf("LoadSpecialties() error: %v", err)
	}
	if len(cfg.Specialties) != 5 {
		t.Errorf("LoadSpecialties() with no file returned %d specialties, want 5", len(cfg.Specialties))
	}
}

func TestLoadSpecialties_EmptyRigPath(t *testing.T) {
	cfg, err := LoadSpecialties("")
	if err != nil {
		t.Fatalf("LoadSpecialties(\"\") error: %v", err)
	}
	if len(cfg.Specialties) != 5 {
		t.Errorf("expected 5 builtins, got %d", len(cfg.Specialties))
	}
}

func TestLoadSpecialties_Override(t *testing.T) {
	rigPath := t.TempDir()
	conductorDir := filepath.Join(rigPath, "conductor")
	os.MkdirAll(conductorDir, 0o755)

	// Override frontend description and add a new specialty
	tomlContent := `
[[specialty]]
name = "frontend"
description = "React and Next.js components"
file_patterns = ["src/**/*.tsx"]

[[specialty]]
name = "infra"
description = "Infrastructure and deployment"
file_patterns = ["terraform/**", "k8s/**"]
labels = ["infra", "devops"]
`
	os.WriteFile(filepath.Join(conductorDir, "specialties.toml"), []byte(tomlContent), 0o644)

	cfg, err := LoadSpecialties(rigPath)
	if err != nil {
		t.Fatalf("LoadSpecialties() error: %v", err)
	}

	// 5 builtins + 1 new = 6
	if len(cfg.Specialties) != 6 {
		t.Fatalf("expected 6 specialties, got %d", len(cfg.Specialties))
	}

	// Frontend should be overridden
	fe := cfg.GetSpecialty("frontend")
	if fe == nil {
		t.Fatal("frontend specialty missing")
	}
	if fe.Description != "React and Next.js components" {
		t.Errorf("frontend description = %q, want overridden value", fe.Description)
	}
	if len(fe.FilePatterns) != 1 || fe.FilePatterns[0] != "src/**/*.tsx" {
		t.Errorf("frontend file_patterns = %v, want overridden value", fe.FilePatterns)
	}

	// Infra should be appended
	infra := cfg.GetSpecialty("infra")
	if infra == nil {
		t.Fatal("infra specialty missing")
	}
	if infra.Description != "Infrastructure and deployment" {
		t.Errorf("infra description = %q", infra.Description)
	}
}

func TestLoadSpecialties_OverridePreservesUnchanged(t *testing.T) {
	rigPath := t.TempDir()
	conductorDir := filepath.Join(rigPath, "conductor")
	os.MkdirAll(conductorDir, 0o755)

	// Only override backend, leave others alone
	tomlContent := `
[[specialty]]
name = "backend"
description = "Go microservices"
`
	os.WriteFile(filepath.Join(conductorDir, "specialties.toml"), []byte(tomlContent), 0o644)

	cfg, err := LoadSpecialties(rigPath)
	if err != nil {
		t.Fatalf("LoadSpecialties() error: %v", err)
	}

	if len(cfg.Specialties) != 5 {
		t.Fatalf("expected 5 specialties, got %d", len(cfg.Specialties))
	}

	// Backend overridden
	be := cfg.GetSpecialty("backend")
	if be.Description != "Go microservices" {
		t.Errorf("backend description = %q, want overridden value", be.Description)
	}
	// But labels preserved from builtin since override didn't set them
	if len(be.Labels) != 3 {
		t.Errorf("backend labels = %v, want preserved builtins", be.Labels)
	}

	// Frontend unchanged
	fe := cfg.GetSpecialty("frontend")
	if fe.Description != "UI components, styling, browser interactions, accessibility" {
		t.Errorf("frontend description changed unexpectedly: %q", fe.Description)
	}
}

func TestLoadSpecialties_InvalidTOML(t *testing.T) {
	rigPath := t.TempDir()
	conductorDir := filepath.Join(rigPath, "conductor")
	os.MkdirAll(conductorDir, 0o755)

	os.WriteFile(filepath.Join(conductorDir, "specialties.toml"), []byte("{{invalid"), 0o644)

	_, err := LoadSpecialties(rigPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestMergeSpecialties_NilOverride(t *testing.T) {
	base := BuiltinSpecialties()
	result := mergeSpecialties(base, nil)
	if len(result.Specialties) != 5 {
		t.Errorf("nil override should return base unchanged, got %d", len(result.Specialties))
	}
}

func TestMergeSpecialties_EmptyOverride(t *testing.T) {
	base := BuiltinSpecialties()
	result := mergeSpecialties(base, &SpecialtyConfig{})
	if len(result.Specialties) != 5 {
		t.Errorf("empty override should return base unchanged, got %d", len(result.Specialties))
	}
}
