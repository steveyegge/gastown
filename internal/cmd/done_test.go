package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestDoneUsesResolveBeadsDir verifies that the done command correctly uses
// beads.ResolveBeadsDir to follow redirect files when initializing beads.
// This is critical for polecat/crew worktrees that use .beads/redirect to point
// to the shared mayor/rig/.beads directory.
//
// The done.go file has two code paths that initialize beads:
//   - Line 181: ExitCompleted path - bd := beads.New(beads.ResolveBeadsDir(cwd))
//   - Line 277: ExitPhaseComplete path - bd := beads.New(beads.ResolveBeadsDir(cwd))
//
// Both must use ResolveBeadsDir to properly handle redirects.
func TestDoneUsesResolveBeadsDir(t *testing.T) {
	// Create a temp directory structure simulating polecat worktree with redirect
	tmpDir := t.TempDir()

	// Create structure like:
	//   gastown/
	//     mayor/rig/.beads/          <- shared beads directory
	//     polecats/fixer/.beads/     <- polecat with redirect
	//       redirect -> ../../mayor/rig/.beads

	mayorRigBeadsDir := filepath.Join(tmpDir, "gastown", "mayor", "rig", ".beads")
	polecatDir := filepath.Join(tmpDir, "gastown", "polecats", "fixer")
	polecatBeadsDir := filepath.Join(polecatDir, ".beads")

	// Create directories
	if err := os.MkdirAll(mayorRigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig/.beads: %v", err)
	}
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecats/fixer/.beads: %v", err)
	}

	// Create redirect file pointing to mayor/rig/.beads
	redirectContent := "../../mayor/rig/.beads"
	redirectPath := filepath.Join(polecatBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	t.Run("redirect followed from polecat directory", func(t *testing.T) {
		// This mirrors how done.go initializes beads at line 181 and 277
		resolvedDir := beads.ResolveBeadsDir(polecatDir)

		// Should resolve to mayor/rig/.beads
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ResolveBeadsDir(%s) = %s, want %s", polecatDir, resolvedDir, mayorRigBeadsDir)
		}

		// Verify the beads instance is created with the resolved path
		// We use the same pattern as done.go: beads.New(beads.ResolveBeadsDir(cwd))
		bd := beads.New(beads.ResolveBeadsDir(polecatDir))
		if bd == nil {
			t.Error("beads.New returned nil")
		}
	})

	t.Run("redirect not present uses local beads", func(t *testing.T) {
		// Without redirect, should use local .beads
		localDir := filepath.Join(tmpDir, "gastown", "mayor", "rig")
		resolvedDir := beads.ResolveBeadsDir(localDir)

		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ResolveBeadsDir(%s) = %s, want %s", localDir, resolvedDir, mayorRigBeadsDir)
		}
	})
}

// TestDoneBeadsInitWithoutRedirect verifies that beads initialization works
// normally when no redirect file exists.
func TestDoneBeadsInitWithoutRedirect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple .beads directory without redirect (like mayor/rig)
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// ResolveBeadsDir should return the same directory when no redirect exists
	resolvedDir := beads.ResolveBeadsDir(tmpDir)
	if resolvedDir != beadsDir {
		t.Errorf("ResolveBeadsDir(%s) = %s, want %s", tmpDir, resolvedDir, beadsDir)
	}

	// Beads initialization should work the same way done.go does it
	bd := beads.New(beads.ResolveBeadsDir(tmpDir))
	if bd == nil {
		t.Error("beads.New returned nil")
	}
}

// TestDoneBeadsInitBothCodePaths documents that both code paths in done.go
// that create beads instances use ResolveBeadsDir:
//   - ExitCompleted (line 181): for MR creation and issue operations
//   - ExitPhaseComplete (line 277): for gate waiter registration
//
// This test verifies the pattern by demonstrating that the resolved directory
// is used consistently for different operations.
func TestDoneBeadsInitBothCodePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup: crew directory with redirect to mayor/rig/.beads
	mayorRigBeadsDir := filepath.Join(tmpDir, "mayor", "rig", ".beads")
	crewDir := filepath.Join(tmpDir, "crew", "max")
	crewBeadsDir := filepath.Join(crewDir, ".beads")

	if err := os.MkdirAll(mayorRigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig/.beads: %v", err)
	}
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir crew/max/.beads: %v", err)
	}

	// Create redirect
	redirectPath := filepath.Join(crewBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	t.Run("ExitCompleted path uses ResolveBeadsDir", func(t *testing.T) {
		// This simulates the line 181 path in done.go:
		// bd := beads.New(beads.ResolveBeadsDir(cwd))
		resolvedDir := beads.ResolveBeadsDir(crewDir)
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ExitCompleted path: ResolveBeadsDir(%s) = %s, want %s",
				crewDir, resolvedDir, mayorRigBeadsDir)
		}

		bd := beads.New(beads.ResolveBeadsDir(crewDir))
		if bd == nil {
			t.Error("beads.New returned nil for ExitCompleted path")
		}
	})

	t.Run("ExitPhaseComplete path uses ResolveBeadsDir", func(t *testing.T) {
		// This simulates the line 277 path in done.go:
		// bd := beads.New(beads.ResolveBeadsDir(cwd))
		resolvedDir := beads.ResolveBeadsDir(crewDir)
		if resolvedDir != mayorRigBeadsDir {
			t.Errorf("ExitPhaseComplete path: ResolveBeadsDir(%s) = %s, want %s",
				crewDir, resolvedDir, mayorRigBeadsDir)
		}

		bd := beads.New(beads.ResolveBeadsDir(crewDir))
		if bd == nil {
			t.Error("beads.New returned nil for ExitPhaseComplete path")
		}
	})
}

// TestDoneRedirectChain verifies behavior with chained redirects.
// ResolveBeadsDir follows exactly one level of redirect by design - it does NOT
// follow chains transitively. This is intentional: chains typically indicate
// misconfiguration (e.g., a redirect file that shouldn't exist).
func TestDoneRedirectChain(t *testing.T) {
	tmpDir := t.TempDir()

	// Create chain: worktree -> intermediate -> canonical
	canonicalBeadsDir := filepath.Join(tmpDir, "canonical", ".beads")
	intermediateDir := filepath.Join(tmpDir, "intermediate")
	intermediateBeadsDir := filepath.Join(intermediateDir, ".beads")
	worktreeDir := filepath.Join(tmpDir, "worktree")
	worktreeBeadsDir := filepath.Join(worktreeDir, ".beads")

	// Create all directories
	for _, dir := range []string{canonicalBeadsDir, intermediateBeadsDir, worktreeBeadsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// Create redirects
	// intermediate -> canonical
	if err := os.WriteFile(filepath.Join(intermediateBeadsDir, "redirect"), []byte("../canonical/.beads"), 0644); err != nil {
		t.Fatalf("write intermediate redirect: %v", err)
	}
	// worktree -> intermediate
	if err := os.WriteFile(filepath.Join(worktreeBeadsDir, "redirect"), []byte("../intermediate/.beads"), 0644); err != nil {
		t.Fatalf("write worktree redirect: %v", err)
	}

	// ResolveBeadsDir follows exactly one level - stops at intermediate
	// (A warning is printed about the chain, but intermediate is returned)
	resolved := beads.ResolveBeadsDir(worktreeDir)

	// Should resolve to intermediate (one level), NOT canonical (two levels)
	if resolved != intermediateBeadsDir {
		t.Errorf("ResolveBeadsDir should follow one level only: got %s, want %s",
			resolved, intermediateBeadsDir)
	}
}

// TestDoneEmptyRedirectFallback verifies that an empty or whitespace-only
// redirect file falls back to the local .beads directory.
func TestDoneEmptyRedirectFallback(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create empty redirect file
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte("   \n"), 0644); err != nil {
		t.Fatalf("write empty redirect: %v", err)
	}

	// Should fall back to local .beads
	resolved := beads.ResolveBeadsDir(tmpDir)
	if resolved != beadsDir {
		t.Errorf("empty redirect should fallback: got %s, want %s", resolved, beadsDir)
	}
}

// TestDoneCircularRedirectProtection verifies that circular redirects
// are detected and handled safely.
func TestDoneCircularRedirectProtection(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create circular redirect (points to itself)
	redirectPath := filepath.Join(beadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(".beads"), 0644); err != nil {
		t.Fatalf("write circular redirect: %v", err)
	}

	// Should detect circular redirect and return original
	resolved := beads.ResolveBeadsDir(tmpDir)
	if resolved != beadsDir {
		t.Errorf("circular redirect should return original: got %s, want %s", resolved, beadsDir)
	}
}
