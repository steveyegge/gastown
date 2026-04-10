package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBareRepo(t *testing.T) {
	t.Run("prefers .repo.git over repo.git", func(t *testing.T) {
		dir := t.TempDir()
		dotPath := filepath.Join(dir, ".repo.git")
		plainPath := filepath.Join(dir, "repo.git")
		os.MkdirAll(dotPath, 0755)
		os.MkdirAll(plainPath, 0755)

		got := FindBareRepo(dir)
		if got != dotPath {
			t.Errorf("FindBareRepo = %q, want %q (.repo.git should take priority)", got, dotPath)
		}
	})

	t.Run("finds repo.git when .repo.git absent", func(t *testing.T) {
		dir := t.TempDir()
		plainPath := filepath.Join(dir, "repo.git")
		os.MkdirAll(plainPath, 0755)

		got := FindBareRepo(dir)
		if got != plainPath {
			t.Errorf("FindBareRepo = %q, want %q", got, plainPath)
		}
	})

	t.Run("finds .repo.git when repo.git absent", func(t *testing.T) {
		dir := t.TempDir()
		dotPath := filepath.Join(dir, ".repo.git")
		os.MkdirAll(dotPath, 0755)

		got := FindBareRepo(dir)
		if got != dotPath {
			t.Errorf("FindBareRepo = %q, want %q", got, dotPath)
		}
	})

	t.Run("returns empty when neither exists", func(t *testing.T) {
		dir := t.TempDir()

		got := FindBareRepo(dir)
		if got != "" {
			t.Errorf("FindBareRepo = %q, want empty string", got)
		}
	})

	t.Run("ignores regular file named .repo.git", func(t *testing.T) {
		dir := t.TempDir()
		// Create a file, not a directory
		os.WriteFile(filepath.Join(dir, ".repo.git"), []byte("not a dir"), 0644)

		got := FindBareRepo(dir)
		if got != "" {
			t.Errorf("FindBareRepo = %q, want empty (file should be ignored)", got)
		}
	})
}
