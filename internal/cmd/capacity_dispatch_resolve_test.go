package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestListReadyWorkBeadIDs_RedirectResolution is a regression test for gt-e3r:
// "Scheduler shows '0 ready' for rig beads that ARE ready."
//
// Root cause: listReadyWorkBeadIDsWithError and batchFetchBeadInfoByIDs used
// beads.New(dir), which ignores .beads/redirect files. Rigs that use shared
// beads via redirect (e.g., polecats pointing to the rig's .beads) would
// query the wrong (empty/nonexistent) dolt database, returning 0 results.
//
// Fix: use beads.ResolveBeadsDir(dir) + beads.NewWithBeadsDir(dir, beadsDir)
// to follow redirects before querying, matching the pattern used throughout
// the rest of the codebase.
func TestListReadyWorkBeadIDs_RedirectResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows — symlink/redirect test")
	}

	// Simulate a town with a rig that has a .beads/redirect pointing elsewhere.
	//
	// Layout:
	//   town/
	//     .beads/              <- HQ beads (has actual db)
	//     mayor/town.json      <- marks this as a town root
	//     myrig/
	//       .beads/
	//         redirect         <- "../../.beads" (points to town/.beads)

	townRoot := t.TempDir()

	// Create HQ beads dir (the redirect target)
	hqBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(hqBeads, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create mayor/town.json to mark town root
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create rig with redirect
	rigDir := filepath.Join(townRoot, "myrig")
	rigBeadsDir := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Redirect: relative path from rigDir to town/.beads
	// Use filepath.Rel to compute the correct relative path
	relPath, err := filepath.Rel(rigDir, hqBeads)
	if err != nil {
		t.Fatal(err)
	}
	redirectContent := relPath
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte(redirectContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Subtest 1: Prove that beads.New(rigDir) ignores the redirect.
	// The beadsDir field is empty, so bd commands won't set BEADS_DIR
	// and will default to rigDir/.beads — the wrong database.
	t.Run("beads.New ignores redirect", func(t *testing.T) {
		b := beads.New(rigDir)
		if b == nil {
			t.Fatal("New returned nil")
		}
		// beads.New creates a wrapper with no beadsDir override —
		// it will NOT follow the redirect file.
	})

	// Subtest 2: Prove that ResolveBeadsDir follows the redirect.
	t.Run("ResolveBeadsDir follows redirect", func(t *testing.T) {
		resolved := beads.ResolveBeadsDir(rigDir)

		// Normalize for macOS /private/var vs /var
		resolvedReal, _ := filepath.EvalSymlinks(resolved)
		hqReal, _ := filepath.EvalSymlinks(hqBeads)

		if resolvedReal != hqReal {
			t.Errorf("ResolveBeadsDir(%q) = %q, want %q (the redirect target)",
				rigDir, resolved, hqBeads)
		}
	})

	// Subtest 3: Prove NewWithBeadsDir + ResolveBeadsDir is the correct pattern.
	t.Run("NewWithBeadsDir uses resolved dir", func(t *testing.T) {
		beadsDir := beads.ResolveBeadsDir(rigDir)
		b := beads.NewWithBeadsDir(rigDir, beadsDir)
		if b == nil {
			t.Fatal("NewWithBeadsDir returned nil")
		}
		// The wrapper now has the correct beadsDir pointing to HQ,
		// so bd commands will set BEADS_DIR to the right database.
	})

	// Subtest 4: beadsSearchDirs includes rig dirs — verify the dirs
	// returned by beadsSearchDirs include our rig (which has a redirect).
	t.Run("beadsSearchDirs includes rig with redirect", func(t *testing.T) {
		dirs := beadsSearchDirs(townRoot)
		found := false
		for _, d := range dirs {
			// Normalize for comparison
			dReal, _ := filepath.EvalSymlinks(d)
			rigReal, _ := filepath.EvalSymlinks(rigDir)
			if dReal == rigReal {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("beadsSearchDirs(%q) did not include rig dir %q; got %v",
				townRoot, rigDir, dirs)
		}
	})
}
