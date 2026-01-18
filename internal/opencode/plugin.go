// Package opencode provides OpenCode plugin management.
package opencode

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed plugin/gastown.js plugin/package.json
var pluginFS embed.FS

// EnsurePluginAt ensures the Gas Town OpenCode plugin exists.
// If the file already exists, it's left unchanged.
func EnsurePluginAt(workDir, pluginDir, pluginFile string) error {
	if pluginDir == "" || pluginFile == "" {
		return nil
	}

	pluginPath := filepath.Join(workDir, pluginDir, pluginFile)
	if _, err := os.Stat(pluginPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		return fmt.Errorf("reading plugin template: %w", err)
	}

	if err := os.WriteFile(pluginPath, content, 0644); err != nil {
		return fmt.Errorf("writing plugin: %w", err)
	}

	// Create package.json for OpenCode plugin dependencies
	// OpenCode requires this to load local plugins with TypeScript support
	pluginRoot := filepath.Join(workDir, filepath.Dir(pluginDir))
	packageJsonPath := filepath.Join(pluginRoot, "package.json")

	if _, err := os.Stat(packageJsonPath); os.IsNotExist(err) {
		packageJsonContent, err := pluginFS.ReadFile("plugin/package.json")
		if err != nil {
			return fmt.Errorf("reading plugin package.json template: %w", err)
		}

		if err := os.WriteFile(packageJsonPath, packageJsonContent, 0644); err != nil {
			return fmt.Errorf("writing plugin package.json: %w", err)
		}

		// Install dependencies (required for plugin to load)
		if err := installDependencies(pluginRoot); err != nil {
			return fmt.Errorf("installing plugin dependencies: %w", err)
		}
	}

	return nil
}

// installDependencies runs npm/bun install in the given directory.
func installDependencies(dir string) error {
	// Check if node_modules already exists
	nodeModules := filepath.Join(dir, "node_modules")
	if _, err := os.Stat(nodeModules); err == nil {
		return nil // Already installed
	}

	// Try bun first (faster), fall back to npm
	var cmd *exec.Cmd
	if _, err := exec.LookPath("bun"); err == nil {
		cmd = exec.Command("bun", "install")
	} else if _, err := exec.LookPath("npm"); err == nil {
		cmd = exec.Command("npm", "install", "--silent")
	} else {
		return fmt.Errorf("neither bun nor npm found in PATH")
	}

	cmd.Dir = dir
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil
	return cmd.Run()
}
