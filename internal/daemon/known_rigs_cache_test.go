package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestGetKnownRigs_CachedBetweenInvalidations verifies that d.getKnownRigs()
// memoizes rigs.json reads and only re-reads after invalidation. This is the
// regression test for #3463 — without the cache the ~10 per-tick callers each
// read and parse the file independently.
func TestGetKnownRigs_CachedBetweenInvalidations(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	if err := os.WriteFile(rigsPath, []byte(`{"rigs":{"alpha":{},"beta":{}}}`), 0o644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	d := &Daemon{config: &Config{TownRoot: townRoot}}

	// First call populates the cache.
	first := d.getKnownRigs()
	slices.Sort(first)
	if !slices.Equal(first, []string{"alpha", "beta"}) {
		t.Fatalf("first call: got %v, want [alpha beta]", first)
	}

	// Delete the file on disk. A cached call must still return the old list.
	if err := os.Remove(rigsPath); err != nil {
		t.Fatalf("remove rigs.json: %v", err)
	}
	cached := d.getKnownRigs()
	slices.Sort(cached)
	if !slices.Equal(cached, []string{"alpha", "beta"}) {
		t.Fatalf("cached call after delete: got %v, want [alpha beta] (cache bypassed?)", cached)
	}

	// Invalidate — next call must re-read from disk (now empty).
	d.invalidateKnownRigsCache()
	if got := d.getKnownRigs(); len(got) != 0 {
		t.Fatalf("post-invalidate call: got %v, want empty", got)
	}

	// A subsequent write should still not surface until the next invalidation.
	if err := os.WriteFile(rigsPath, []byte(`{"rigs":{"gamma":{}}}`), 0o644); err != nil {
		t.Fatalf("rewrite rigs.json: %v", err)
	}
	if got := d.getKnownRigs(); len(got) != 0 {
		t.Fatalf("cached-empty call after rewrite: got %v, want empty (cache bypassed?)", got)
	}
	d.invalidateKnownRigsCache()
	if got := d.getKnownRigs(); !slices.Equal(got, []string{"gamma"}) {
		t.Fatalf("post-invalidate call after rewrite: got %v, want [gamma]", got)
	}
}
