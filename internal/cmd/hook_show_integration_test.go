//go:build integration

package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

type hookShowJSON struct {
	Agent  string `json:"agent"`
	BeadID string `json:"bead_id"`
	Status string `json:"status"`
}

// TestHookShowShorthandResolvesToCanonical verifies that hook show accepts
// shorthand polecat targets (rig/name) and resolves them to canonical
// assignee IDs (rig/polecats/name) before querying hooked work.
func TestHookShowShorthandResolvesToCanonical(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping integration test")
	}

	townRoot, polecatDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot

	rigDir := filepath.Join(polecatDir, "..", "..", "mayor", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	issue, err := b.Create(beads.CreateOptions{
		Title:    "Hook show target normalization test",
		Type:     "task",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	hooked := beads.StatusHooked
	assignee := "gastown/polecats/toast"
	if err := b.Update(issue.ID, beads.UpdateOptions{
		Status:   &hooked,
		Assignee: &assignee,
	}); err != nil {
		t.Fatalf("hook issue: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(polecatDir); err != nil {
		t.Fatalf("chdir to polecat dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	prevJSON := moleculeJSON
	moleculeJSON = true
	t.Cleanup(func() {
		moleculeJSON = prevJSON
	})

	runShow := func(target string) hookShowJSON {
		out := captureStdout(t, func() {
			if err := runHookShow(nil, []string{target}); err != nil {
				t.Fatalf("runHookShow(%q): %v", target, err)
			}
		})
		var parsed hookShowJSON
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("parse runHookShow(%q) output %q: %v", target, out, err)
		}
		return parsed
	}

	canonical := runShow("gastown/polecats/toast")
	if canonical.BeadID != issue.ID || canonical.Status != beads.StatusHooked {
		t.Fatalf("canonical target mismatch: got bead=%q status=%q, want bead=%q status=%q",
			canonical.BeadID, canonical.Status, issue.ID, beads.StatusHooked)
	}

	shorthand := runShow("gastown/toast")
	if shorthand.BeadID != issue.ID || shorthand.Status != beads.StatusHooked {
		t.Fatalf("shorthand target mismatch: got bead=%q status=%q, want bead=%q status=%q",
			shorthand.BeadID, shorthand.Status, issue.ID, beads.StatusHooked)
	}
	if shorthand.Agent != "gastown/polecats/toast" {
		t.Fatalf("shorthand target did not normalize: got agent=%q, want %q",
			shorthand.Agent, "gastown/polecats/toast")
	}
}

// TestHookShowCrossRigRouting verifies that gt hook show <rig/refinery> correctly
// queries the rig's beads database even when called from a different rig's directory.
// This covers the bug where runHookShow always used findLocalBeadsDir() (CWD-based),
// causing it to query the wrong database for remote rig-level targets.
func TestHookShowCrossRigRouting(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping integration test")
	}

	townRoot, polecatDir, rigPrefix := setupHookTestTown(t)

	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	issue, err := b.Create(beads.CreateOptions{
		Title:    "Cross-rig hook show test",
		Type:     "task",
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}

	hooked := beads.StatusHooked
	assignee := "gastown/refinery"
	if err := b.Update(issue.ID, beads.UpdateOptions{
		Status:   &hooked,
		Assignee: &assignee,
	}); err != nil {
		t.Fatalf("hook issue: %v", err)
	}

	// Simulate calling gt hook show from a different rig (the polecat's worktree),
	// which is the scenario that was broken: queries the wrong DB via CWD.
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(polecatDir); err != nil {
		t.Fatalf("chdir to polecat dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	prevJSON := moleculeJSON
	moleculeJSON = true
	t.Cleanup(func() {
		moleculeJSON = prevJSON
	})

	// Also simulate calling from an unrelated town root (worst case: mayor's dir)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir to townRoot: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runHookShow(nil, []string{"gastown/refinery"}); err != nil {
			t.Fatalf("runHookShow(gastown/refinery): %v", err)
		}
	})
	var result hookShowJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse output %q: %v", out, err)
	}
	if result.BeadID != issue.ID || result.Status != beads.StatusHooked {
		t.Fatalf("cross-rig hook show returned wrong result: got bead=%q status=%q, want bead=%q status=%q",
			result.BeadID, result.Status, issue.ID, beads.StatusHooked)
	}
}
