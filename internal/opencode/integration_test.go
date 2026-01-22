//go:build integration

// Package opencode provides integration tests for OpenCode CLI support.
//
// Run with: go test -tags=integration ./internal/opencode -v
//
// Optional flags:
//   --install-opencode    Install OpenCode CLI if not present (requires npm or go)
//
// Example:
//   go test -tags=integration ./internal/opencode -v --install-opencode
package opencode

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var installOpencode = flag.Bool("install-opencode", false, "Install OpenCode CLI if not present")

// opencodeInstalled tracks whether opencode was installed by the tests
var opencodeInstalled bool

func TestMain(m *testing.M) {
	flag.Parse()

	// Check if opencode is already available
	if _, err := exec.LookPath("opencode"); err != nil {
		if *installOpencode {
			if err := installOpencodeCLI(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to install opencode: %v\n", err)
				os.Exit(1)
			}
			opencodeInstalled = true
		}
	}

	code := m.Run()
	os.Exit(code)
}

// installOpencodeCLI attempts to install the opencode CLI using available package managers.
// It tries npm first, then go install.
func installOpencodeCLI() error {
	// Try npm install first
	if _, err := exec.LookPath("npm"); err == nil {
		fmt.Println("Installing opencode via npm...")
		cmd := exec.Command("npm", "install", "-g", "opencode-ai@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		fmt.Println("npm install failed, trying go install...")
	}

	// Try go install
	if _, err := exec.LookPath("go"); err == nil {
		fmt.Println("Installing opencode via go install...")
		cmd := exec.Command("go", "install", "github.com/sst/opencode@latest")
		cmd.Env = append(os.Environ(), "GOBIN="+filepath.Join(os.Getenv("HOME"), "go", "bin"))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			// Update PATH to include GOBIN
			gobin := filepath.Join(os.Getenv("HOME"), "go", "bin")
			os.Setenv("PATH", gobin+string(os.PathListSeparator)+os.Getenv("PATH"))
			return nil
		}
	}

	return fmt.Errorf("no package manager available to install opencode (tried npm, go)")
}

// requireOpencode skips the test if opencode is not available.
func requireOpencode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode not installed, skipping test (use --install-opencode to auto-install)")
	}
}

// TestOpenCodeAvailable verifies that the opencode CLI is accessible.
func TestOpenCodeAvailable(t *testing.T) {
	requireOpencode(t)

	cmd := exec.Command("opencode", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("opencode --version failed: %v\nOutput: %s", err, output)
	}

	t.Logf("OpenCode version: %s", strings.TrimSpace(string(output)))
}

// TestPluginInstallation verifies that the Gas Town plugin can be installed
// in a directory structure that OpenCode expects.
func TestPluginInstallation(t *testing.T) {
	tmpDir := t.TempDir()

	// Install the plugin
	pluginDir := ".opencode/plugin"
	pluginFile := "gastown.js"

	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt failed: %v", err)
	}

	// Verify the plugin was created
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)
	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Plugin file not created: %v", err)
	}

	if info.IsDir() {
		t.Error("Plugin path should be a file, not a directory")
	}

	// Verify the content is valid JavaScript
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin: %v", err)
	}

	// Check for expected content markers
	contentStr := string(content)
	expectedMarkers := []string{
		"GasTown",
		"export",
		"gt prime",
		"session.created",
	}

	for _, marker := range expectedMarkers {
		if !strings.Contains(contentStr, marker) {
			t.Errorf("Plugin missing expected content: %q", marker)
		}
	}
}

// TestPluginDirectoryStructure verifies that the plugin creates the correct
// directory structure matching OpenCode's expected layout.
func TestPluginDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with nested directory structure
	pluginDir := ".opencode/plugins/gastown"
	pluginFile := "index.js"

	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt failed: %v", err)
	}

	// Verify the entire directory structure was created
	fullDir := filepath.Join(tmpDir, pluginDir)
	info, err := os.Stat(fullDir)
	if err != nil {
		t.Fatalf("Plugin directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Plugin parent path should be a directory")
	}

	// Verify file exists in the nested structure
	pluginPath := filepath.Join(fullDir, pluginFile)
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("Plugin file not created in nested directory: %v", err)
	}
}

// TestPluginIdempotence verifies that running EnsurePluginAt multiple times
// doesn't overwrite an existing plugin.
func TestPluginIdempotence(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := ".opencode/plugin"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	// Create the directory and a custom plugin
	if err := os.MkdirAll(filepath.Join(tmpDir, pluginDir), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	customContent := []byte("// Custom plugin - should not be overwritten")
	if err := os.WriteFile(pluginPath, customContent, 0644); err != nil {
		t.Fatalf("Failed to write custom plugin: %v", err)
	}

	// Run EnsurePluginAt - should not overwrite
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt failed: %v", err)
	}

	// Verify content was NOT changed
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin: %v", err)
	}

	if string(content) != string(customContent) {
		t.Error("EnsurePluginAt overwrote existing plugin file")
	}
}

// TestPluginWithOpenCodeDirectory verifies plugin installation in an actual
// OpenCode configuration directory structure.
func TestPluginWithOpenCodeDirectory(t *testing.T) {
	requireOpencode(t)

	tmpDir := t.TempDir()

	// Create a mock project directory with OpenCode config
	projectDir := filepath.Join(tmpDir, "my-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project directory: %v", err)
	}

	// Standard OpenCode plugin location
	pluginDir := ".opencode/plugin"
	pluginFile := "gastown.js"

	err := EnsurePluginAt(projectDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt failed: %v", err)
	}

	// Verify plugin is in the expected OpenCode location
	expectedPath := filepath.Join(projectDir, ".opencode", "plugin", "gastown.js")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("Plugin not at expected OpenCode path: %v", err)
	}

	// Verify the plugin content is syntactically valid by checking key exports
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read plugin: %v", err)
	}

	// The plugin should export GasTown
	if !strings.Contains(string(content), "export const GasTown") {
		t.Error("Plugin should export GasTown constant")
	}
}

// TestEmbeddedPluginContent verifies the embedded plugin file contains
// the expected functionality.
func TestEmbeddedPluginContent(t *testing.T) {
	// Read the embedded plugin directly
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read embedded plugin: %v", err)
	}

	contentStr := string(content)

	// Verify essential functionality
	checks := []struct {
		name     string
		contains string
	}{
		{"exports GasTown", "export const GasTown"},
		{"handles session events", "event?.type"},
		{"runs gt prime", "gt prime"},
		{"checks GT_ROLE", "GT_ROLE"},
		{"supports autonomous roles", "autonomousRoles"},
		{"handles session.created", `"session.created"`},
	}

	for _, check := range checks {
		if !strings.Contains(contentStr, check.contains) {
			t.Errorf("Embedded plugin missing %s: expected %q", check.name, check.contains)
		}
	}
}

// TestMultiplePluginInstallations verifies that plugins can be installed
// in multiple directories without interference.
func TestMultiplePluginInstallations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple project directories
	projects := []string{"project-a", "project-b", "project-c"}

	for _, project := range projects {
		projectDir := filepath.Join(tmpDir, project)
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", project, err)
		}

		err := EnsurePluginAt(projectDir, ".opencode/plugin", "gastown.js")
		if err != nil {
			t.Fatalf("EnsurePluginAt failed for %s: %v", project, err)
		}
	}

	// Verify all plugins were created independently
	for _, project := range projects {
		pluginPath := filepath.Join(tmpDir, project, ".opencode", "plugin", "gastown.js")
		if _, err := os.Stat(pluginPath); err != nil {
			t.Errorf("Plugin not created for %s: %v", project, err)
		}
	}
}

// TestPluginFilePermissions verifies the plugin file has appropriate permissions.
func TestPluginFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsurePluginAt(tmpDir, ".opencode/plugin", "gastown.js")
	if err != nil {
		t.Fatalf("EnsurePluginAt failed: %v", err)
	}

	pluginPath := filepath.Join(tmpDir, ".opencode", "plugin", "gastown.js")
	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Failed to stat plugin: %v", err)
	}

	// Plugin should be readable but not executable (it's a JS file loaded by opencode)
	mode := info.Mode()
	if mode&0400 == 0 {
		t.Error("Plugin should be owner-readable")
	}
	if mode&0040 == 0 {
		t.Error("Plugin should be group-readable")
	}
	if mode&0004 == 0 {
		t.Error("Plugin should be world-readable")
	}
}

// TestPluginPathVariations tests various path formats are handled correctly.
func TestPluginPathVariations(t *testing.T) {
	tests := []struct {
		name       string
		pluginDir  string
		pluginFile string
	}{
		{"standard path", ".opencode/plugin", "gastown.js"},
		{"nested path", ".opencode/plugins/gastown", "index.js"},
		{"simple path", "plugins", "gt.js"},
		{"deep nesting", "a/b/c/d/plugins", "plugin.js"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			err := EnsurePluginAt(tmpDir, tt.pluginDir, tt.pluginFile)
			if err != nil {
				t.Fatalf("EnsurePluginAt failed: %v", err)
			}

			pluginPath := filepath.Join(tmpDir, tt.pluginDir, tt.pluginFile)
			if _, err := os.Stat(pluginPath); err != nil {
				t.Fatalf("Plugin not created at expected path: %v", err)
			}
		})
	}
}
