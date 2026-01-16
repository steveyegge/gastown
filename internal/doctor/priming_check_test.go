package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimingCheck_PolecatNewStructure(t *testing.T) {
	// This test verifies that priming check correctly handles the new polecat path structure.
	// Bug: priming_check.go looks at polecats/<name>/ but the actual worktree is at
	// polecats/<name>/<rigname>/ which is where the .beads/redirect file lives.
	//
	// Expected behavior: If polecats/<name>/<rigname>/.beads/redirect points to rig's beads
	// which has PRIME.md, the priming check should report no issues.
	//
	// Actual behavior with bug: Check looks at polecats/<name>/ which has no .beads/redirect,
	// so it incorrectly reports PRIME.MD as missing.

	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatName := "testpc"

	// Set up rig structure with .beads and PRIME.md
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	primeMdPath := filepath.Join(rigBeadsDir, "PRIME.md")
	if err := os.WriteFile(primeMdPath, []byte("# Test PRIME.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up polecat with NEW structure: polecats/<name>/<rigname>/
	polecatWorktree := filepath.Join(tmpDir, rigName, "polecats", polecatName, rigName)
	polecatWorktreeBeads := filepath.Join(polecatWorktree, ".beads")
	if err := os.MkdirAll(polecatWorktreeBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create redirect file pointing to rig's beads (from worktree perspective)
	// From polecats/<name>/<rigname>/, we go up 3 levels to rig root: ../../../.beads
	redirectFile := filepath.Join(polecatWorktreeBeads, "redirect")
	if err := os.WriteFile(redirectFile, []byte("../../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Look for any polecat-related issues
	polecatIssueFound := false
	for _, detail := range result.Details {
		// The bug would report: "testrig/polecats/testpc: Missing PRIME.md..."
		if filepath.Base(filepath.Dir(detail)) == "polecats" ||
			(len(detail) > 0 && detail[0:len(rigName)+len("/polecats/")] == rigName+"/polecats/") {
			polecatIssueFound = true
			t.Logf("Found polecat issue: %s", detail)
		}
	}

	// With the bug fixed, there should be NO polecat issues because:
	// - The worktree at polecats/<name>/<rigname>/ has .beads/redirect
	// - The redirect points to rig's .beads which has PRIME.md
	//
	// With the bug present, the check looks at polecats/<name>/ which has no redirect,
	// so it reports missing PRIME.md
	if polecatIssueFound {
		t.Errorf("priming check incorrectly reported polecat issues; result: %+v", result)
	}
}

func TestPrimingCheck_PolecatDirLevel_NoPrimeMD(t *testing.T) {
	// This test verifies that NO PRIME.md should exist at the polecatDir level
	// (polecats/<name>/.beads/PRIME.md). PRIME.md should only exist at:
	// 1. Rig level: <rig>/.beads/PRIME.md
	// 2. Accessed via redirect from worktree: polecats/<name>/<rigname>/.beads/redirect -> rig's beads

	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatName := "testpc"

	// Set up rig with PRIME.md
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the intermediate polecatDir level (polecats/<name>/)
	// This directory should NOT have .beads/PRIME.md
	polecatDir := filepath.Join(tmpDir, rigName, "polecats", polecatName)
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create the actual worktree with redirect
	polecatWorktree := filepath.Join(polecatDir, rigName)
	polecatWorktreeBeads := filepath.Join(polecatWorktree, ".beads")
	if err := os.MkdirAll(polecatWorktreeBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatWorktreeBeads, "redirect"), []byte("../../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify: polecatDir level should NOT have .beads/PRIME.md
	polecatDirPrimeMd := filepath.Join(polecatDir, ".beads", "PRIME.md")
	if _, err := os.Stat(polecatDirPrimeMd); err == nil {
		t.Errorf("PRIME.md should NOT exist at polecatDir level: %s", polecatDirPrimeMd)
	}

	// Also verify the doctor doesn't create one at the wrong level when fixing
	// (This would happen if doctor --fix incorrectly provisions PRIME.md at polecats/<name>/)
}

func TestPrimingCheck_FixRemovesBadPolecatBeads(t *testing.T) {
	// This test verifies that doctor --fix removes spurious .beads directories
	// that were incorrectly created at the polecatDir level (polecats/<name>/.beads).
	//
	// Background: A bug in priming_check.go caused it to look at polecats/<name>/
	// instead of polecats/<name>/<rigname>/. When it didn't find a redirect, it
	// would create .beads/PRIME.md at the wrong level. These orphaned .beads
	// directories should be cleaned up.

	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatName := "testpc"

	// Set up rig with .beads and PRIME.md
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up correct polecat worktree structure: polecats/<name>/<rigname>/
	polecatDir := filepath.Join(tmpDir, rigName, "polecats", polecatName)
	polecatWorktree := filepath.Join(polecatDir, rigName)
	polecatWorktreeBeads := filepath.Join(polecatWorktree, ".beads")
	if err := os.MkdirAll(polecatWorktreeBeads, 0755); err != nil {
		t.Fatal(err)
	}
	// Create redirect pointing to rig's beads
	if err := os.WriteFile(filepath.Join(polecatWorktreeBeads, "redirect"), []byte("../../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create BAD .beads at polecatDir level (this is what the bug created)
	badBeadsDir := filepath.Join(polecatDir, ".beads")
	if err := os.MkdirAll(badBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	badPrimeMd := filepath.Join(badBeadsDir, "PRIME.md")
	if err := os.WriteFile(badPrimeMd, []byte("# BAD PRIME - should be removed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify bad .beads exists before fix
	if _, err := os.Stat(badBeadsDir); os.IsNotExist(err) {
		t.Fatal("test setup failed: bad .beads directory should exist")
	}

	// Run priming check and fix
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	_ = check.Run(ctx) // Populate issues

	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify bad .beads was removed
	if _, err := os.Stat(badBeadsDir); err == nil {
		t.Errorf("bad .beads directory at polecatDir level should have been removed: %s", badBeadsDir)
		// List contents for debugging
		entries, _ := os.ReadDir(badBeadsDir)
		for _, e := range entries {
			t.Logf("  - %s", e.Name())
		}
	}

	// Verify correct structure still exists
	if _, err := os.Stat(polecatWorktreeBeads); os.IsNotExist(err) {
		t.Errorf("correct .beads at worktree level should still exist: %s", polecatWorktreeBeads)
	}
	if _, err := os.Stat(filepath.Join(polecatWorktreeBeads, "redirect")); os.IsNotExist(err) {
		t.Errorf("redirect file should still exist")
	}

	// Verify rig's PRIME.md still exists
	if _, err := os.Stat(filepath.Join(rigBeadsDir, "PRIME.md")); os.IsNotExist(err) {
		t.Errorf("rig's PRIME.md should still exist")
	}
}

// TestPrimingCheck_DetectsUnexpectedClaudeMdInMayorRig verifies that CLAUDE.md
// inside mayor/rig/ (the source repo) is detected as an issue.
func TestPrimingCheck_DetectsUnexpectedClaudeMdInMayorRig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor/rig/ structure (the source repo clone)
	mayorRigPath := filepath.Join(tmpDir, rigName, "mayor", "rig")
	if err := os.MkdirAll(mayorRigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md inside mayor/rig/ (WRONG location - inside source repo)
	badClaudeMd := filepath.Join(mayorRigPath, "CLAUDE.md")
	if err := os.WriteFile(badClaudeMd, []byte("# Bad CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find the unexpected_claude_md issue
	foundIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "mayor/rig") && strings.Contains(detail, "CLAUDE.md") {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Errorf("expected to find unexpected CLAUDE.md issue for mayor/rig, got details: %v", result.Details)
	}

	if result.Status != StatusError {
		t.Errorf("expected StatusError, got %v", result.Status)
	}
}

// TestPrimingCheck_DetectsUnexpectedClaudeMdInRefineryRig verifies that CLAUDE.md
// inside refinery/rig/ (the source repo worktree) is detected as an issue.
func TestPrimingCheck_DetectsUnexpectedClaudeMdInRefineryRig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create refinery/rig/ structure (the source repo worktree)
	refineryRigPath := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(refineryRigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md inside refinery/rig/ (WRONG location - inside source repo)
	badClaudeMd := filepath.Join(refineryRigPath, "CLAUDE.md")
	if err := os.WriteFile(badClaudeMd, []byte("# Bad CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find the unexpected_claude_md issue
	foundIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "refinery/rig") && strings.Contains(detail, "CLAUDE.md") {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Errorf("expected to find unexpected CLAUDE.md issue for refinery/rig, got details: %v", result.Details)
	}
}

// TestPrimingCheck_DetectsUnexpectedClaudeMdInCrewWorktree verifies that CLAUDE.md
// inside crew/<name>/ (the worktree itself) is detected as an issue.
func TestPrimingCheck_DetectsUnexpectedClaudeMdInCrewWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	crewName := "alice"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create crew/<name>/ structure with beads redirect
	crewWorktree := filepath.Join(tmpDir, rigName, "crew", crewName)
	crewBeadsDir := filepath.Join(crewWorktree, ".beads")
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crewBeadsDir, "redirect"), []byte("../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md inside crew worktree (WRONG - inside source repo)
	badClaudeMd := filepath.Join(crewWorktree, "CLAUDE.md")
	if err := os.WriteFile(badClaudeMd, []byte("# Bad CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find the unexpected_claude_md issue
	foundIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "crew/"+crewName) && strings.Contains(detail, "CLAUDE.md") {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Errorf("expected to find unexpected CLAUDE.md issue for crew/%s, got details: %v", crewName, result.Details)
	}
}

// TestPrimingCheck_DetectsUnexpectedClaudeMdInPolecatWorktree verifies that CLAUDE.md
// inside polecats/<name>/<rigname>/ (the worktree) is detected as an issue.
func TestPrimingCheck_DetectsUnexpectedClaudeMdInPolecatWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatName := "testpc"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create polecat worktree structure: polecats/<name>/<rigname>/
	polecatWorktree := filepath.Join(tmpDir, rigName, "polecats", polecatName, rigName)
	polecatBeadsDir := filepath.Join(polecatWorktree, ".beads")
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatBeadsDir, "redirect"), []byte("../../../.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md inside polecat worktree (WRONG - inside source repo)
	badClaudeMd := filepath.Join(polecatWorktree, "CLAUDE.md")
	if err := os.WriteFile(badClaudeMd, []byte("# Bad CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find the unexpected_claude_md issue
	foundIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "polecats/"+polecatName) && strings.Contains(detail, "CLAUDE.md") {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Errorf("expected to find unexpected CLAUDE.md issue for polecats/%s, got details: %v", polecatName, result.Details)
	}
}

// TestPrimingCheck_FixRemovesUnexpectedClaudeMd verifies that doctor --fix
// removes CLAUDE.md files from inside source repos.
func TestPrimingCheck_FixRemovesUnexpectedClaudeMd(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mayor/rig/ with bad CLAUDE.md
	mayorRigPath := filepath.Join(tmpDir, rigName, "mayor", "rig")
	if err := os.MkdirAll(mayorRigPath, 0755); err != nil {
		t.Fatal(err)
	}
	badClaudeMd := filepath.Join(mayorRigPath, "CLAUDE.md")
	if err := os.WriteFile(badClaudeMd, []byte("# Bad CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify bad file exists before fix
	if _, err := os.Stat(badClaudeMd); os.IsNotExist(err) {
		t.Fatal("test setup failed: bad CLAUDE.md should exist")
	}

	// Run priming check and fix
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	_ = check.Run(ctx)

	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify bad CLAUDE.md was removed
	if _, err := os.Stat(badClaudeMd); err == nil {
		t.Errorf("bad CLAUDE.md should have been removed: %s", badClaudeMd)
	}
}

// TestPrimingCheck_DoesNotFlagAgentLevelClaudeMd verifies that CLAUDE.md
// at agent level (e.g., refinery/CLAUDE.md) is NOT flagged as an issue.
// Note: Per-rig mayor (<rig>/mayor/) does NOT get bootstrap files - it's just a source clone.
// Only refinery and witness get bootstrap files at rig level.
func TestPrimingCheck_DoesNotFlagAgentLevelClaudeMd(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create refinery/ directory structure
	refineryPath := filepath.Join(tmpDir, rigName, "refinery")
	refineryRigPath := filepath.Join(refineryPath, "rig")
	if err := os.MkdirAll(refineryRigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md and AGENTS.md at agent level (CORRECT location - outside source repo)
	goodClaudeMd := filepath.Join(refineryPath, "CLAUDE.md")
	if err := os.WriteFile(goodClaudeMd, []byte("# Good CLAUDE.md at agent level\n"), 0644); err != nil {
		t.Fatal(err)
	}
	goodAgentsMd := filepath.Join(refineryPath, "AGENTS.md")
	if err := os.WriteFile(goodAgentsMd, []byte("# Good AGENTS.md at agent level\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should NOT find any unexpected_claude_md issue for agent-level file
	for _, detail := range result.Details {
		// The agent-level path is refinery/CLAUDE.md, not refinery/rig/CLAUDE.md
		// We should only flag refinery/rig/ paths
		if strings.Contains(detail, "CLAUDE.md") && !strings.Contains(detail, "/rig") {
			t.Errorf("should NOT flag agent-level CLAUDE.md, but got: %s", detail)
		}
	}
}

// TestPrimingCheck_NoIssuesWhenCorrectlyConfigured verifies that a correctly
// configured rig reports no priming issues.
// Note: Per-rig mayor does NOT get bootstrap files - it's just a source clone.
// Refinery, witness, crew, and polecats get bootstrap files at rig level.
func TestPrimingCheck_NoIssuesWhenCorrectlyConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads and PRIME.md
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create refinery structure with agent-level CLAUDE.md and AGENTS.md
	refineryPath := filepath.Join(tmpDir, rigName, "refinery")
	refineryRigPath := filepath.Join(refineryPath, "rig")
	if err := os.MkdirAll(refineryRigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refineryPath, "CLAUDE.md"), []byte("# Refinery\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refineryPath, "AGENTS.md"), []byte("# Refinery\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create witness structure with agent-level CLAUDE.md and AGENTS.md
	witnessPath := filepath.Join(tmpDir, rigName, "witness")
	witnessRigPath := filepath.Join(witnessPath, "rig")
	if err := os.MkdirAll(witnessRigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(witnessPath, "CLAUDE.md"), []byte("# Witness\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(witnessPath, "AGENTS.md"), []byte("# Witness\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create crew directory with CLAUDE.md and AGENTS.md
	crewPath := filepath.Join(tmpDir, rigName, "crew")
	if err := os.MkdirAll(crewPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crewPath, "CLAUDE.md"), []byte("# Crew\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crewPath, "AGENTS.md"), []byte("# Crew\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create polecats directory with CLAUDE.md and AGENTS.md
	polecatsPath := filepath.Join(tmpDir, rigName, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatsPath, "CLAUDE.md"), []byte("# Polecats\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatsPath, "AGENTS.md"), []byte("# Polecats\nRun gt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Filter out gt_not_in_path which depends on system PATH
	var relevantDetails []string
	for _, d := range result.Details {
		if !strings.Contains(d, "gt binary not found") {
			relevantDetails = append(relevantDetails, d)
		}
	}

	if len(relevantDetails) > 0 {
		t.Errorf("expected no priming issues for correctly configured rig, got: %v", relevantDetails)
	}
}

// TestPrimingCheck_DetectsLargeClaudeMd verifies that CLAUDE.md files
// exceeding 30 lines are flagged.
func TestPrimingCheck_DetectsLargeClaudeMd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level mayor directory with large CLAUDE.md (>30 lines)
	// The priming check looks at townRoot/mayor/CLAUDE.md for town-level mayor
	mayorPath := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorPath, 0755); err != nil {
		t.Fatal(err)
	}
	var largeContent strings.Builder
	for i := 0; i < 50; i++ {
		largeContent.WriteString("# Line " + string(rune('0'+i%10)) + "\n")
	}
	if err := os.WriteFile(filepath.Join(mayorPath, "CLAUDE.md"), []byte(largeContent.String()), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find large_claude_md issue
	foundIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "CLAUDE.md has") && strings.Contains(detail, "lines") {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Errorf("expected to find large CLAUDE.md issue, got details: %v", result.Details)
	}
}

// TestPrimingCheck_DetectsMissingAgentBootstrapFiles verifies that missing CLAUDE.md
// and/or AGENTS.md at agent level (e.g., refinery/) is detected as an issue.
// Note: Per-rig mayor (<rig>/mayor/) does NOT get bootstrap files - it's just a source clone.
// Refinery, witness, crew, and polecats get bootstrap files at rig level.
func TestPrimingCheck_DetectsMissingAgentBootstrapFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create refinery with only CLAUDE.md (missing AGENTS.md)
	refineryPath := filepath.Join(tmpDir, rigName, "refinery")
	refineryRigPath := filepath.Join(refineryPath, "rig")
	if err := os.MkdirAll(refineryRigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refineryPath, "CLAUDE.md"), []byte("# Refinery\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create witness WITHOUT CLAUDE.md or AGENTS.md
	witnessPath := filepath.Join(tmpDir, rigName, "witness")
	witnessRigPath := filepath.Join(witnessPath, "rig")
	if err := os.MkdirAll(witnessRigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create crew WITHOUT CLAUDE.md or AGENTS.md
	crewPath := filepath.Join(tmpDir, rigName, "crew")
	if err := os.MkdirAll(crewPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create polecats with only AGENTS.md (missing CLAUDE.md)
	polecatsPath := filepath.Join(tmpDir, rigName, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(polecatsPath, "AGENTS.md"), []byte("# Polecats\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run priming check
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	result := check.Run(ctx)

	// Should find missing bootstrap issues for all
	foundRefineryIssue := false
	foundWitnessIssue := false
	foundCrewIssue := false
	foundPolecatsIssue := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "refinery") && strings.Contains(detail, "bootstrap") {
			foundRefineryIssue = true
		}
		if strings.Contains(detail, "witness") && strings.Contains(detail, "bootstrap") {
			foundWitnessIssue = true
		}
		if strings.Contains(detail, "crew") && strings.Contains(detail, "bootstrap") {
			foundCrewIssue = true
		}
		if strings.Contains(detail, "polecats") && strings.Contains(detail, "bootstrap") {
			foundPolecatsIssue = true
		}
	}

	if !foundRefineryIssue {
		t.Errorf("expected to find missing bootstrap issue for refinery, got details: %v", result.Details)
	}
	if !foundWitnessIssue {
		t.Errorf("expected to find missing bootstrap issue for witness, got details: %v", result.Details)
	}
	if !foundCrewIssue {
		t.Errorf("expected to find missing bootstrap issue for crew, got details: %v", result.Details)
	}
	if !foundPolecatsIssue {
		t.Errorf("expected to find missing bootstrap issue for polecats, got details: %v", result.Details)
	}
}

// TestPrimingCheck_FixCreatesMissingBootstrapFiles verifies that doctor --fix
// creates missing CLAUDE.md and AGENTS.md at agent level with correct bootstrap content.
// Note: Per-rig mayor (<rig>/mayor/) does NOT get bootstrap files - it's just a source clone.
// Refinery, witness, crew, and polecats get bootstrap files at rig level.
func TestPrimingCheck_FixCreatesMissingBootstrapFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Set up rig with .beads
	rigBeadsDir := filepath.Join(tmpDir, rigName, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "PRIME.md"), []byte("# PRIME\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create refinery with only CLAUDE.md (missing AGENTS.md)
	refineryPath := filepath.Join(tmpDir, rigName, "refinery")
	refineryRigPath := filepath.Join(refineryPath, "rig")
	if err := os.MkdirAll(refineryRigPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refineryPath, "CLAUDE.md"), []byte("# Refinery\ngt prime\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create witness WITHOUT bootstrap files
	witnessPath := filepath.Join(tmpDir, rigName, "witness")
	witnessRigPath := filepath.Join(witnessPath, "rig")
	if err := os.MkdirAll(witnessRigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create crew WITHOUT bootstrap files
	crewPath := filepath.Join(tmpDir, rigName, "crew")
	if err := os.MkdirAll(crewPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create polecats WITHOUT bootstrap files
	polecatsPath := filepath.Join(tmpDir, rigName, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Run priming check and fix
	check := NewPrimingCheck()
	ctx := &CheckContext{TownRoot: tmpDir}
	_ = check.Run(ctx)

	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify witness files were created
	witnessClaudeMd := filepath.Join(witnessPath, "CLAUDE.md")
	witnessAgentsMd := filepath.Join(witnessPath, "AGENTS.md")
	if _, err := os.Stat(witnessClaudeMd); os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should have been created at: %s", witnessClaudeMd)
	}
	if _, err := os.Stat(witnessAgentsMd); os.IsNotExist(err) {
		t.Errorf("AGENTS.md should have been created at: %s", witnessAgentsMd)
	}

	// Verify witness content
	content, _ := os.ReadFile(witnessClaudeMd)
	if !strings.Contains(string(content), "Witness Context") {
		t.Errorf("created CLAUDE.md should contain 'Witness Context', got: %s", string(content))
	}

	// Verify refinery AGENTS.md was created (CLAUDE.md already existed)
	refineryAgentsMd := filepath.Join(refineryPath, "AGENTS.md")
	if _, err := os.Stat(refineryAgentsMd); os.IsNotExist(err) {
		t.Errorf("AGENTS.md should have been created at: %s", refineryAgentsMd)
	}

	// Verify refinery AGENTS.md content
	content, _ = os.ReadFile(refineryAgentsMd)
	if !strings.Contains(string(content), "Refinery Context") {
		t.Errorf("created AGENTS.md should contain 'Refinery Context', got: %s", string(content))
	}

	// Verify crew files were created
	crewClaudeMd := filepath.Join(crewPath, "CLAUDE.md")
	crewAgentsMd := filepath.Join(crewPath, "AGENTS.md")
	if _, err := os.Stat(crewClaudeMd); os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should have been created at: %s", crewClaudeMd)
	}
	if _, err := os.Stat(crewAgentsMd); os.IsNotExist(err) {
		t.Errorf("AGENTS.md should have been created at: %s", crewAgentsMd)
	}

	// Verify crew content
	content, _ = os.ReadFile(crewClaudeMd)
	if !strings.Contains(string(content), "Crew Context") {
		t.Errorf("created CLAUDE.md should contain 'Crew Context', got: %s", string(content))
	}

	// Verify polecats files were created
	polecatsClaudeMd := filepath.Join(polecatsPath, "CLAUDE.md")
	polecatsAgentsMd := filepath.Join(polecatsPath, "AGENTS.md")
	if _, err := os.Stat(polecatsClaudeMd); os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should have been created at: %s", polecatsClaudeMd)
	}
	if _, err := os.Stat(polecatsAgentsMd); os.IsNotExist(err) {
		t.Errorf("AGENTS.md should have been created at: %s", polecatsAgentsMd)
	}

	// Verify polecats content
	content, _ = os.ReadFile(polecatsClaudeMd)
	if !strings.Contains(string(content), "Polecat Context") {
		t.Errorf("created CLAUDE.md should contain 'Polecat Context', got: %s", string(content))
	}
}

