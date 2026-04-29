package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStashOrphanCheck_Name(t *testing.T) {
	c := NewStashOrphanCheck()
	if c.Name() != "stash-orphan" {
		t.Errorf("Name = %q, want stash-orphan", c.Name())
	}
}

func TestStashOrphanCheck_NoRigs(t *testing.T) {
	c := NewStashOrphanCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}
	res := c.Run(ctx)
	if res.Status != StatusOK {
		t.Errorf("Status = %v, want OK (no rigs)", res.Status)
	}
}

// initRepoWithStash creates a git repo at dir and produces a stash.
// Sets the stash's reflog timestamp to ageAgo so we can simulate an old stash
// without sleeping the test.
func initRepoWithStash(t *testing.T, dir string, ageAgo time.Duration) {
	t.Helper()
	for _, cmd := range [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "test"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("%v: %v", cmd, err)
		}
	}
	// Initial commit so we have a HEAD
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-q", "-m", "seed"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = dir
		if err := c.Run(); err != nil {
			t.Fatalf("%v: %v", cmd, err)
		}
	}
	// Make working tree dirty + stash with backdated commit timestamps so the
	// stash entry's author date is `ageAgo` in the past.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("git", "add", ".")
	c.Dir = dir
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	stashCmd := exec.Command("git", "stash", "push", "-m", "old-stash")
	stashCmd.Dir = dir
	when := time.Now().Add(-ageAgo).Format(time.RFC3339)
	stashCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+when,
		"GIT_COMMITTER_DATE="+when,
	)
	if err := stashCmd.Run(); err != nil {
		t.Fatalf("git stash: %v", err)
	}
}

func TestCountOrphanStashes_OldStash(t *testing.T) {
	dir := t.TempDir()
	initRepoWithStash(t, dir, 48*time.Hour) // 2 days old

	// 24h threshold — should detect
	if got := countOrphanStashes(dir, 24*time.Hour); got != 1 {
		t.Errorf("countOrphanStashes(48h, threshold=24h) = %d, want 1", got)
	}

	// 7d threshold — should NOT detect
	if got := countOrphanStashes(dir, 7*24*time.Hour); got != 0 {
		t.Errorf("countOrphanStashes(48h, threshold=7d) = %d, want 0", got)
	}

	// No-stash dir — should be 0
	empty := t.TempDir()
	exec.Command("git", "init", "-q", empty).Run()
	if got := countOrphanStashes(empty, 24*time.Hour); got != 0 {
		t.Errorf("countOrphanStashes(empty repo) = %d, want 0", got)
	}
}

func TestCountOrphanStashes_FreshStash(t *testing.T) {
	dir := t.TempDir()
	initRepoWithStash(t, dir, 1*time.Hour) // fresh

	if got := countOrphanStashes(dir, 24*time.Hour); got != 0 {
		t.Errorf("fresh stash flagged as orphan: got %d, want 0", got)
	}
}

func TestDiscoverGitWorkdirs(t *testing.T) {
	rig := t.TempDir()

	// Create rig root as a git repo
	exec.Command("git", "init", "-q", rig).Run()

	// Crew clone
	crewDir := filepath.Join(rig, "crew", "alice")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "init", "-q", crewDir).Run()

	// Polecat worktree (one level deeper: polecats/<name>/<rig>/)
	polecatDir := filepath.Join(rig, "polecats", "fury", "myrig")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "init", "-q", polecatDir).Run()

	// Non-git dir under crew/ should be ignored
	noGitDir := filepath.Join(rig, "crew", "no_git_here")
	os.MkdirAll(noGitDir, 0755)

	got := discoverGitWorkdirs(rig)
	if len(got) != 3 {
		t.Errorf("discoverGitWorkdirs found %d dirs, want 3 (rig root + crew/alice + polecats/fury/myrig): %v", len(got), got)
	}
}
