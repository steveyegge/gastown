package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirSizeHuman(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	got := dirSizeHuman(dir)
	if got != "0 B" {
		t.Errorf("empty dir: got %q, want %q", got, "0 B")
	}

	// Write a 1024-byte file
	data := make([]byte, 1024)
	if err := os.WriteFile(filepath.Join(dir, "file.dat"), data, 0644); err != nil {
		t.Fatal(err)
	}
	got = dirSizeHuman(dir)
	if got != "1.0 KB" {
		t.Errorf("1KB file: got %q, want %q", got, "1.0 KB")
	}

	// Add a subdirectory with another file
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	data2 := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(subDir, "nested.dat"), data2, 0644); err != nil {
		t.Fatal(err)
	}
	got = dirSizeHuman(dir)
	if got != "1.5 KB" {
		t.Errorf("1.5KB total: got %q, want %q", got, "1.5 KB")
	}
}

func TestDirSizeHuman_NonexistentDir(t *testing.T) {
	got := dirSizeHuman("/nonexistent/path/that/does/not/exist")
	if got != "0 B" {
		t.Errorf("nonexistent dir: got %q, want %q", got, "0 B")
	}
}
