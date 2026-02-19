package crew

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePersonaFile_RigOverridesTown(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create town-level persona
	townDir := filepath.Join(townRoot, ".personas")
	if err := os.MkdirAll(townDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townDir, "toast.md"), []byte("town toast"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig-level persona (should win)
	rigDir := filepath.Join(rigPath, ".personas")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "toast.md"), []byte("rig toast"), 0644); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolvePersonaFile(townRoot, rigPath, "toast")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "rig toast" {
		t.Errorf("expected 'rig toast', got '%s'", content)
	}
	if source != "rig" {
		t.Errorf("expected source 'rig', got '%s'", source)
	}
}

func TestResolvePersonaFile_FallbackToTown(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Only town-level persona
	townDir := filepath.Join(townRoot, ".personas")
	if err := os.MkdirAll(townDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townDir, "toast.md"), []byte("town toast"), 0644); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolvePersonaFile(townRoot, rigPath, "toast")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "town toast" {
		t.Errorf("expected 'town toast', got '%s'", content)
	}
	if source != "town" {
		t.Errorf("expected source 'town', got '%s'", source)
	}
}

func TestResolvePersonaFile_NotFound(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolvePersonaFile(townRoot, rigPath, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got '%s'", content)
	}
	if source != "" {
		t.Errorf("expected empty source, got '%s'", source)
	}
}

func TestResolvePersonaFile_HyphenatedName(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create persona file with hyphenated name
	rigDir := filepath.Join(rigPath, ".personas")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "rust-expert.md"), []byte("rust expert"), 0644); err != nil {
		t.Fatal(err)
	}

	// Resolve using hyphenated name (regression: validateCrewName would reject this)
	content, _, err := ResolvePersonaFile(townRoot, rigPath, "rust-expert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "rust expert" {
		t.Errorf("expected 'rust expert', got '%s'", content)
	}
}

func TestResolvePersonaFile_TraversalRejected(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	_, _, err := ResolvePersonaFile(townRoot, rigPath, "../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal attempt")
	}
}

func TestListPersonas(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create town personas
	townDir := filepath.Join(townRoot, ".personas")
	if err := os.MkdirAll(townDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townDir, "alpha.md"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townDir, "beta.md"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig persona (overrides alpha)
	rigDir := filepath.Join(rigPath, ".personas")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "alpha.md"), []byte("a-rig"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigDir, "gamma.md"), []byte("g"), 0644); err != nil {
		t.Fatal(err)
	}

	personas, err := ListPersonas(townRoot, rigPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(personas) != 3 {
		t.Fatalf("expected 3 personas, got %d", len(personas))
	}

	// Check that alpha is from rig (override)
	found := false
	for _, p := range personas {
		if p.Name == "alpha" {
			found = true
			if p.Source != "rig" {
				t.Errorf("alpha should be from rig, got '%s'", p.Source)
			}
			if !p.Overrides {
				t.Error("alpha should have Overrides=true")
			}
		}
	}
	if !found {
		t.Error("alpha not found in personas")
	}
}

func TestValidatePersonaName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty rejected", "", true},
		{"slash rejected", "foo/bar", true},
		{"backslash rejected", `foo\bar`, true},
		{"dotdot rejected", "../etc", true},
		{"hyphen allowed", "rust-expert", false},
		{"dot allowed", "my.persona", false},
		{"underscore allowed", "senior_dev", false},
		{"simple name allowed", "alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePersonaName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePersonaName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
