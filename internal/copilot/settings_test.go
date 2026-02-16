package copilot

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureSettingsAt_EmptyParameters(t *testing.T) {
	t.Run("empty hooksDir", func(t *testing.T) {
		err := EnsureSettingsAt("/tmp/work", "", "copilot-instructions.md")
		if err != nil {
			t.Errorf("EnsureSettingsAt() with empty hooksDir should return nil, got %v", err)
		}
	})

	t.Run("empty hooksFile", func(t *testing.T) {
		err := EnsureSettingsAt("/tmp/work", ".copilot", "")
		if err != nil {
			t.Errorf("EnsureSettingsAt() with empty hooksFile should return nil, got %v", err)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		err := EnsureSettingsAt("/tmp/work", "", "")
		if err != nil {
			t.Errorf("EnsureSettingsAt() with both empty should return nil, got %v", err)
		}
	})
}

func TestEnsureSettingsAt_FileExists(t *testing.T) {
	tmpDir := t.TempDir()

	hooksDir := ".copilot"
	hooksFile := "copilot-instructions.md"
	settingsPath := filepath.Join(tmpDir, hooksDir, hooksFile)

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	existingContent := []byte("# existing instructions")
	if err := os.WriteFile(settingsPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// EnsureSettingsAt should not overwrite existing file
	err := EnsureSettingsAt(tmpDir, hooksDir, hooksFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}
	if string(content) != string(existingContent) {
		t.Error("EnsureSettingsAt() should not overwrite existing file")
	}
}

func TestEnsureSettingsAt_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()

	hooksDir := ".copilot"
	hooksFile := "copilot-instructions.md"
	settingsPath := filepath.Join(tmpDir, hooksDir, hooksFile)

	if _, err := os.Stat(settingsPath); err == nil {
		t.Fatal("Settings file should not exist yet")
	}

	err := EnsureSettingsAt(tmpDir, hooksDir, hooksFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Settings file was not created: %v", err)
	}
	if info.IsDir() {
		t.Error("Settings path should be a file, not a directory")
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Settings file should have content")
	}
}

func TestEnsureSettingsAt_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	hooksDir := "nested/copilot/dir"
	hooksFile := "copilot-instructions.md"
	settingsPath := filepath.Join(tmpDir, hooksDir, hooksFile)

	err := EnsureSettingsAt(tmpDir, hooksDir, hooksFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(settingsPath))
	if err != nil {
		t.Fatalf("Settings directory was not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Settings parent path should be a directory")
	}
}

func TestEnsureSettingsAt_UnwritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission checks are not reliable on Windows")
	}

	tmpDir := t.TempDir()
	// Create a read-only directory to prevent file creation
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chmod(readOnlyDir, 0755) // restore so cleanup works
	})

	err := EnsureSettingsAt(readOnlyDir, ".copilot", "copilot-instructions.md")
	if err == nil {
		t.Error("EnsureSettingsAt() should return error for unwritable directory")
	}
}

func TestEnsureSettingsAt_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on Windows")
	}

	tmpDir := t.TempDir()

	hooksDir := ".copilot"
	hooksFile := "copilot-instructions.md"
	settingsPath := filepath.Join(tmpDir, hooksDir, hooksFile)

	err := EnsureSettingsAt(tmpDir, hooksDir, hooksFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Failed to stat settings file: %v", err)
	}

	expectedMode := os.FileMode(0644)
	if info.Mode() != expectedMode {
		t.Errorf("Settings file mode = %v, want %v", info.Mode(), expectedMode)
	}
}
