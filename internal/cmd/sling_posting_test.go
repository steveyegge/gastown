package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/posting"
)

// ---------------------------------------------------------------------------
// gt sling --posting: posting flows through to polecat .runtime/posting
// Spec: ~/postings-test-spec.md section 10 (tests 10.1–10.6)
// ---------------------------------------------------------------------------

// 10.1: --posting flag appears in sling help output
func TestSlingPosting_HelpShowsPostingFlag(t *testing.T) {
	t.Parallel()

	usage := slingCmd.UsageString()
	if !strings.Contains(usage, "--posting") {
		t.Errorf("sling --help output should contain --posting flag, got:\n%s", usage)
	}
	// Verify the description is present too
	if !strings.Contains(usage, "posting") {
		t.Error("sling help should describe the --posting flag")
	}
}

// 10.5: nonexistent posting name passes through without validation — the file
// is written and the polecat gets a warning at gt prime time, not at sling time.
func TestSlingPosting_NonexistentPostingPassesThrough(t *testing.T) {
	t.Parallel()
	clonePath := t.TempDir()

	// A posting name that doesn't correspond to any template
	nonexistent := "totally-made-up-posting"
	if err := posting.Write(clonePath, nonexistent); err != nil {
		t.Fatalf("posting.Write with nonexistent posting should succeed, got: %v", err)
	}

	got := posting.Read(clonePath)
	if got != nonexistent {
		t.Errorf("nonexistent posting should be written as-is, got %q, want %q", got, nonexistent)
	}

	// File should exist on disk
	filePath := filepath.Join(clonePath, ".runtime", "posting")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error(".runtime/posting should exist even for nonexistent posting names")
	}
}

// 10.6: posting.Write with empty string does not create a posting file.
// This also tests the Write function's own guard (it calls Clear on empty).
func TestSlingPosting_EmptyStringWriteNoFile(t *testing.T) {
	t.Parallel()
	clonePath := t.TempDir()

	// Calling Write with "" should be a no-op (calls Clear internally)
	if err := posting.Write(clonePath, ""); err != nil {
		t.Fatalf("posting.Write with empty string should not error, got: %v", err)
	}

	// No .runtime dir should exist
	runtimeDir := filepath.Join(clonePath, ".runtime")
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Error(".runtime dir should not exist when posting is empty string")
	}

	got := posting.Read(clonePath)
	if got != "" {
		t.Errorf("Read() should return empty after Write with empty string, got %q", got)
	}
}

// TestSlingPosting_WritesToPolecatRuntime verifies that when --posting is set
// during sling, the posting value is written to the polecat's .runtime/posting
// file. This simulates what executeSling and runSling do at lines 362-368 and
// 975-979 respectively.
func TestSlingPosting_WritesToPolecatRuntime(t *testing.T) {
	t.Parallel()

	// Simulate polecat clone path (the worktree directory)
	clonePath := t.TempDir()

	// Simulate the sling --posting flow: posting.Write(clonePath, postingName)
	postingName := "dispatcher"
	if err := posting.Write(clonePath, postingName); err != nil {
		t.Fatalf("posting.Write(%q, %q) failed: %v", clonePath, postingName, err)
	}

	// Verify the posting is readable by the polecat on startup
	got := posting.Read(clonePath)
	if got != postingName {
		t.Errorf("after sling --posting, Read() = %q, want %q", got, postingName)
	}

	// Verify file path is .runtime/posting
	filePath := filepath.Join(clonePath, ".runtime", "posting")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error(".runtime/posting file should exist after sling --posting write")
	}
}

// TestSlingPosting_EmptyPostingSkipsWrite verifies that when --posting is not
// set (empty string), no .runtime/posting file is created.
func TestSlingPosting_EmptyPostingSkipsWrite(t *testing.T) {
	t.Parallel()
	clonePath := t.TempDir()

	// Simulate sling without --posting: the CLI checks `if slingPosting != ""`
	// before calling posting.Write, so no file is created.
	slingPosting := ""
	if slingPosting != "" {
		_ = posting.Write(clonePath, slingPosting)
	}

	// No .runtime dir should exist
	runtimeDir := filepath.Join(clonePath, ".runtime")
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Error(".runtime dir should not exist when --posting is not set")
	}

	// Read returns empty
	got := posting.Read(clonePath)
	if got != "" {
		t.Errorf("Read() should return empty when no posting set, got %q", got)
	}
}

// TestSlingPosting_SlingParamsCarriesPosting verifies the SlingParams struct
// has the Posting field and it flows correctly through the struct.
func TestSlingPosting_SlingParamsCarriesPosting(t *testing.T) {
	t.Parallel()

	params := SlingParams{
		BeadID:  "gt-abc",
		RigName: "testrig",
		Posting: "scout",
	}

	if params.Posting != "scout" {
		t.Errorf("SlingParams.Posting = %q, want %q", params.Posting, "scout")
	}
}

// TestSlingPosting_ReuseClearsStalePosting verifies that when a polecat is
// reused without --posting, any stale .runtime/posting from the prior session
// is cleared. This is the scenario described in gt-puj: a polecat previously
// posted as "dispatcher" gets reused for a coding task and should NOT inherit
// the old posting.
func TestSlingPosting_ReuseClearsStalePosting(t *testing.T) {
	t.Parallel()
	clonePath := t.TempDir()

	// Simulate prior session's posting (stale state from previous work)
	if err := posting.Write(clonePath, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify stale posting exists
	if got := posting.Read(clonePath); got != "dispatcher" {
		t.Fatalf("setup: expected stale posting %q, got %q", "dispatcher", got)
	}

	// Simulate reuse without --posting: sling clears stale posting (gt-puj fix)
	slingPostingVal := ""
	if slingPostingVal != "" {
		_ = posting.Write(clonePath, slingPostingVal)
	} else {
		// This is the fix: clear stale posting when no --posting flag
		_ = posting.Clear(clonePath)
	}

	// Verify posting is cleared
	got := posting.Read(clonePath)
	if got != "" {
		t.Errorf("after reuse without --posting, Read() = %q, want empty", got)
	}

	// Verify .runtime/posting file is gone
	filePath := filepath.Join(clonePath, ".runtime", "posting")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error(".runtime/posting should not exist after reuse without --posting")
	}
}

// TestSlingPosting_OverwritesExistingPosting verifies that sling --posting
// overwrites any pre-existing .runtime/posting in the polecat directory.
func TestSlingPosting_OverwritesExistingPosting(t *testing.T) {
	t.Parallel()
	clonePath := t.TempDir()

	// Pre-existing posting (e.g., from a previous session)
	if err := posting.Write(clonePath, "old-posting"); err != nil {
		t.Fatal(err)
	}

	// Sling writes new posting
	if err := posting.Write(clonePath, "new-posting"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(clonePath)
	if got != "new-posting" {
		t.Errorf("sling --posting should overwrite, got %q, want %q", got, "new-posting")
	}
}

// TestSlingPosting_PrimeSeesPostingAfterSling verifies that after sling
// writes the posting, resolvePostingName picks it up as session-level posting.
func TestSlingPosting_PrimeSeesPostingAfterSling(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "alpha"

	// Create polecat work dir
	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate sling --posting writing to the polecat's worktree
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify resolvePostingName picks it up
	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}
}
