package crew

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveIdentityFile_RigOverridesTown(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create town-level identity
	townIDDir := filepath.Join(townRoot, "identities")
	if err := os.MkdirAll(townIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townIDDir, "toast.md"), []byte("town toast"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig-level identity (should win)
	rigIDDir := filepath.Join(rigPath, "identities")
	if err := os.MkdirAll(rigIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigIDDir, "toast.md"), []byte("rig toast"), 0644); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolveIdentityFile(townRoot, rigPath, "toast")
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

func TestResolveIdentityFile_FallbackToTown(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Only town-level identity
	townIDDir := filepath.Join(townRoot, "identities")
	if err := os.MkdirAll(townIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townIDDir, "toast.md"), []byte("town toast"), 0644); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolveIdentityFile(townRoot, rigPath, "toast")
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

func TestResolveIdentityFile_NotFound(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	content, source, err := ResolveIdentityFile(townRoot, rigPath, "nonexistent")
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

func TestResolveIdentityFile_ExplicitOverride(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create identity file with different name than crew
	rigIDDir := filepath.Join(rigPath, "identities")
	if err := os.MkdirAll(rigIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigIDDir, "senior-rust-dev.md"), []byte("rust expert"), 0644); err != nil {
		t.Fatal(err)
	}

	// Resolve using explicit name (not crew name)
	content, _, err := ResolveIdentityFile(townRoot, rigPath, "senior-rust-dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "rust expert" {
		t.Errorf("expected 'rust expert', got '%s'", content)
	}
}

func TestListIdentities(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create town identities
	townIDDir := filepath.Join(townRoot, "identities")
	if err := os.MkdirAll(townIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townIDDir, "alpha.md"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townIDDir, "beta.md"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig identity (overrides alpha)
	rigIDDir := filepath.Join(rigPath, "identities")
	if err := os.MkdirAll(rigIDDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigIDDir, "alpha.md"), []byte("a-rig"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigIDDir, "gamma.md"), []byte("g"), 0644); err != nil {
		t.Fatal(err)
	}

	identities, err := ListIdentities(townRoot, rigPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(identities) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(identities))
	}

	// Check that alpha is from rig (override)
	found := false
	for _, id := range identities {
		if id.Name == "alpha" {
			found = true
			if id.Source != "rig" {
				t.Errorf("alpha should be from rig, got '%s'", id.Source)
			}
			if !id.Overrides {
				t.Error("alpha should have Overrides=true")
			}
		}
	}
	if !found {
		t.Error("alpha not found in identities")
	}
}
