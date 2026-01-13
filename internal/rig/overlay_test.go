package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyOverlay_NoOverlayDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := t.TempDir()

	// No overlay directory exists
	err := CopyOverlay(tmpDir, destDir)
	if err != nil {
		t.Errorf("CopyOverlay() with no overlay directory should return nil, got %v", err)
	}
}

func TestCopyOverlay_CopiesFiles(t *testing.T) {
	rigDir := t.TempDir()
	destDir := t.TempDir()

	// Create overlay directory with test files
	overlayDir := filepath.Join(rigDir, ".runtime", "overlay")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	// Create test files
	testFile1 := filepath.Join(overlayDir, "test1.txt")
	testFile2 := filepath.Join(overlayDir, "test2.txt")

	if err := os.WriteFile(testFile1, []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Copy overlay
	err := CopyOverlay(rigDir, destDir)
	if err != nil {
		t.Fatalf("CopyOverlay() error = %v", err)
	}

	// Verify files were copied
	destFile1 := filepath.Join(destDir, "test1.txt")
	destFile2 := filepath.Join(destDir, "test2.txt")

	content1, err := os.ReadFile(destFile1)
	if err != nil {
		t.Errorf("File test1.txt was not copied: %v", err)
	}
	if string(content1) != "content1" {
		t.Errorf("test1.txt content = %q, want %q", string(content1), "content1")
	}

	content2, err := os.ReadFile(destFile2)
	if err != nil {
		t.Errorf("File test2.txt was not copied: %v", err)
	}
	if string(content2) != "content2" {
		t.Errorf("test2.txt content = %q, want %q", string(content2), "content2")
	}
}

func TestCopyOverlay_PreservesPermissions(t *testing.T) {
	rigDir := t.TempDir()
	destDir := t.TempDir()

	// Create overlay directory with a file
	overlayDir := filepath.Join(rigDir, ".runtime", "overlay")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	testFile := filepath.Join(overlayDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0755); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Copy overlay
	err := CopyOverlay(rigDir, destDir)
	if err != nil {
		t.Fatalf("CopyOverlay() error = %v", err)
	}

	// Verify permissions were preserved
	srcInfo, _ := os.Stat(testFile)
	destInfo, err := os.Stat(filepath.Join(destDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	if srcInfo.Mode().Perm() != destInfo.Mode().Perm() {
		t.Errorf("Permissions not preserved: src=%v, dest=%v", srcInfo.Mode(), destInfo.Mode())
	}
}

func TestCopyOverlay_SkipsSubdirectories(t *testing.T) {
	rigDir := t.TempDir()
	destDir := t.TempDir()

	// Create overlay directory with a subdirectory
	overlayDir := filepath.Join(rigDir, ".runtime", "overlay")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	// Create a subdirectory
	subDir := filepath.Join(overlayDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create a file in the overlay root
	testFile := filepath.Join(overlayDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a file in the subdirectory
	subFile := filepath.Join(subDir, "sub.txt")
	if err := os.WriteFile(subFile, []byte("subcontent"), 0644); err != nil {
		t.Fatalf("Failed to create sub file: %v", err)
	}

	// Copy overlay
	err := CopyOverlay(rigDir, destDir)
	if err != nil {
		t.Fatalf("CopyOverlay() error = %v", err)
	}

	// Verify root file was copied
	if _, err := os.Stat(filepath.Join(destDir, "test.txt")); err != nil {
		t.Error("Root file should be copied")
	}

	// Verify subdirectory was NOT copied
	if _, err := os.Stat(filepath.Join(destDir, "subdir")); err == nil {
		t.Error("Subdirectory should not be copied")
	}
	if _, err := os.Stat(filepath.Join(destDir, "subdir", "sub.txt")); err == nil {
		t.Error("File in subdirectory should not be copied")
	}
}

func TestCopyOverlay_EmptyOverlay(t *testing.T) {
	rigDir := t.TempDir()
	destDir := t.TempDir()

	// Create empty overlay directory
	overlayDir := filepath.Join(rigDir, ".runtime", "overlay")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	// Copy overlay
	err := CopyOverlay(rigDir, destDir)
	if err != nil {
		t.Fatalf("CopyOverlay() error = %v", err)
	}

	// Should succeed without errors
}

func TestCopyOverlay_OverwritesExisting(t *testing.T) {
	rigDir := t.TempDir()
	destDir := t.TempDir()

	// Create overlay directory with test file
	overlayDir := filepath.Join(rigDir, ".runtime", "overlay")
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		t.Fatalf("Failed to create overlay dir: %v", err)
	}

	testFile := filepath.Join(overlayDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create existing file in destination with different content
	destFile := filepath.Join(destDir, "test.txt")
	if err := os.WriteFile(destFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("Failed to create dest file: %v", err)
	}

	// Copy overlay
	err := CopyOverlay(rigDir, destDir)
	if err != nil {
		t.Fatalf("CopyOverlay() error = %v", err)
	}

	// Verify file was overwritten
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("Failed to read dest file: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("File content = %q, want %q", string(content), "new content")
	}
}

func TestCopyFilePreserveMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source file
	srcFile := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(srcFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create src file: %v", err)
	}

	// Copy file
	dstFile := filepath.Join(tmpDir, "dst.txt")
	err := copyFilePreserveMode(srcFile, dstFile)
	if err != nil {
		t.Fatalf("copyFilePreserveMode() error = %v", err)
	}

	// Verify content
	content, err := os.ReadFile(dstFile)
	if err != nil {
		t.Errorf("Failed to read dst file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Content = %q, want %q", string(content), "test content")
	}

	// Verify permissions
	srcInfo, _ := os.Stat(srcFile)
	dstInfo, err := os.Stat(dstFile)
	if err != nil {
		t.Fatalf("Failed to stat dst file: %v", err)
	}
	if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
		t.Errorf("Permissions not preserved: src=%v, dest=%v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestCopyFilePreserveMode_NonexistentSource(t *testing.T) {
	tmpDir := t.TempDir()

	srcFile := filepath.Join(tmpDir, "nonexistent.txt")
	dstFile := filepath.Join(tmpDir, "dst.txt")

	err := copyFilePreserveMode(srcFile, dstFile)
	if err == nil {
		t.Error("copyFilePreserveMode() with nonexistent source should return error")
	}
}
