package session

import (
	"os"
	"path/filepath"
	"testing"
)

const testRigsJSON = `{
  "rigs": {
    "gastown": {"beads": {"prefix": "-"}},
    "beads":   {"beads": {"prefix": "bd-"}}
  }
}`

func TestBuildPrefixRegistryFromTown_CanonicalExists_FallbackCreated(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(mayorDir, "rigs.json")
	if err := os.WriteFile(canonical, []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := BuildPrefixRegistryFromTown(townRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Registry should be populated.
	if rig := r.RigForPrefix("-"); rig != "gastown" {
		t.Errorf("expected gastown for prefix -, got %q", rig)
	}

	// Fallback copy should have been created at town root.
	fallback := filepath.Join(townRoot, "rigs.json")
	if _, err := os.Stat(fallback); os.IsNotExist(err) {
		t.Error("fallback rigs.json was not created at town root")
	}
}

func TestBuildPrefixRegistryFromTown_CanonicalMissing_FallbackUsed(t *testing.T) {
	townRoot := t.TempDir()
	// No mayor/rigs.json — only fallback at town root.
	fallback := filepath.Join(townRoot, "rigs.json")
	if err := os.WriteFile(fallback, []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := BuildPrefixRegistryFromTown(townRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Registry should be populated from fallback.
	if rig := r.RigForPrefix("bd-"); rig != "beads" {
		t.Errorf("expected beads for prefix bd-, got %q", rig)
	}
}

func TestBuildPrefixRegistryFromTown_BothMissing_EmptyRegistry(t *testing.T) {
	townRoot := t.TempDir()
	// No rigs.json anywhere.

	r, err := BuildPrefixRegistryFromTown(townRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Registry should be empty — RigForPrefix returns the prefix itself when unknown.
	if rig := r.RigForPrefix("-"); rig != "-" {
		t.Errorf("expected fallthrough prefix -, got %q", rig)
	}
	// Verify no rigs were registered by checking a known rig name returns default.
	if prefix := r.PrefixForRig("gastown"); prefix != DefaultPrefix {
		t.Errorf("expected default prefix for unknown rig, got %q", prefix)
	}
}

func TestCopyFileIfNewer_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.json")
	dst := filepath.Join(dir, "dst.json")

	if err := os.WriteFile(src, []byte(testRigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	copyFileIfNewer(src, dst)

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dst: %v", err)
	}
	if string(data) != testRigsJSON {
		t.Error("dst content does not match src")
	}

	// Temp file should not be left behind.
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file was not cleaned up")
	}
}
