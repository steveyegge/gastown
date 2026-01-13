package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePluginAt_EmptyParameters(t *testing.T) {
	// Test that empty pluginDir or pluginFile returns nil
	t.Run("empty pluginDir", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "", "plugin.js")
		if err != nil {
			t.Errorf("EnsurePluginAt() with empty pluginDir should return nil, got %v", err)
		}
	})

	t.Run("empty pluginFile", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "plugins", "")
		if err != nil {
			t.Errorf("EnsurePluginAt() with empty pluginFile should return nil, got %v", err)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "", "")
		if err != nil {
			t.Errorf("EnsurePluginAt() with both empty should return nil, got %v", err)
		}
	})
}

func TestEnsurePluginAt_FileExists(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create the plugin file first
	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Write a placeholder file
	existingContent := []byte("// existing plugin")
	if err := os.WriteFile(pluginPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// EnsurePluginAt should not overwrite existing file
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify file content is unchanged
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	if string(content) != string(existingContent) {
		t.Error("EnsurePluginAt() should not overwrite existing file")
	}
}

func TestEnsurePluginAt_CreatesFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	// Ensure plugin doesn't exist
	if _, err := os.Stat(pluginPath); err == nil {
		t.Fatal("Plugin file should not exist yet")
	}

	// Create the plugin
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify file was created
	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Plugin file was not created: %v", err)
	}
	if info.IsDir() {
		t.Error("Plugin path should be a file, not a directory")
	}

	// Verify file has content
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Plugin file should have content")
	}
}

func TestEnsurePluginAt_CreatesDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "nested/plugins/dir"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	// Create the plugin
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify directory was created
	dirInfo, err := os.Stat(filepath.Dir(pluginPath))
	if err != nil {
		t.Fatalf("Plugin directory was not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Plugin parent path should be a directory")
	}
}

func TestEnsurePluginAt_FilePermissions(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Failed to stat plugin file: %v", err)
	}

	// Check file mode is 0644 (rw-r--r--)
	expectedMode := os.FileMode(0644)
	if info.Mode() != expectedMode {
		t.Errorf("Plugin file mode = %v, want %v", info.Mode(), expectedMode)
	}
}
