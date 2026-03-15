package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestMakefileCheckUpToDateUsesCurrentBranch verifies that the Makefile's
// check-up-to-date target uses the current branch (via git symbolic-ref)
// instead of hardcoding origin/main. This ensures the check works on
// forks using non-main branches like popandpeek.
func TestMakefileCheckUpToDateUsesCurrentBranch(t *testing.T) {
	// Read the Makefile from the repo root
	// Walk up from the test directory to find the repo root
	makefilePath := findRepoFile(t, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}

	content := string(data)

	// Should use git symbolic-ref to detect current branch
	if !strings.Contains(content, "git symbolic-ref --short HEAD") {
		t.Error("Makefile check-up-to-date should use 'git symbolic-ref --short HEAD' to detect current branch")
	}

	// Should NOT hardcode origin/main in the check-up-to-date target
	// (Note: origin/main may appear elsewhere in the Makefile, so check specifically
	// within the check-up-to-date section)
	lines := strings.Split(content, "\n")
	inCheckUpToDate := false
	for _, line := range lines {
		if strings.HasPrefix(line, "check-up-to-date:") {
			inCheckUpToDate = true
			continue
		}
		if inCheckUpToDate {
			// End of target: next non-indented line
			if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") && line != "" && !strings.HasPrefix(line, "#") {
				break
			}
			if strings.Contains(line, "origin/main") {
				t.Errorf("check-up-to-date should not hardcode origin/main, found: %s", strings.TrimSpace(line))
			}
		}
	}

	if !inCheckUpToDate {
		t.Error("check-up-to-date target not found in Makefile")
	}
}

// findRepoFile walks up from the current working directory to find a file
// in the repository root.
func findRepoFile(t *testing.T, filename string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		path := dir + "/" + filename
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatalf("could not find %s walking up from cwd", filename)
		}
		dir = parent
	}
}
