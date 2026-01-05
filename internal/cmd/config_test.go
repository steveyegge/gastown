package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

// setupTestTown creates a minimal Gas Town workspace for testing.
func setupTestTownForConfig(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()

	// Create mayor directory with required files
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	// Create town.json
	townConfig := &config.TownConfig{
		Type:       "town",
		Version:    config.CurrentTownVersion,
		Name:       "test-town",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	townConfigPath := filepath.Join(mayorDir, "town.json")
	if err := config.SaveTownConfig(townConfigPath, townConfig); err != nil {
		t.Fatalf("save town.json: %v", err)
	}

	// Create empty rigs.json
	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs:    make(map[string]config.RigEntry),
	}
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	return townRoot
}

func TestConfigAgentList(t *testing.T) {
	t.Run("lists built-in agents", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Change to town root so workspace.FindFromCwd works
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{}
		err := runConfigAgentList(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentList failed: %v", err)
		}

		// Verify settings file was created (LoadOrCreate creates it)
		if _, err := os.Stat(settingsPath); err != nil {
			// This is OK - list command works without settings file
		}
	})

	t.Run("lists built-in and custom agents", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Create settings with custom agent
		settings := &config.TownSettings{
			Type:         "town-settings",
			Version:      config.CurrentTownSettingsVersion,
			DefaultAgent: "claude",
			Agents: map[string]*config.RuntimeConfig{
				"my-custom": {
					Command: "my-agent",
					Args:    []string{"--flag"},
				},
			},
		}
		if err := config.SaveTownSettings(settingsPath, settings); err != nil {
			t.Fatalf("save settings: %v", err)
		}

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Load agent registry
		registryPath := config.DefaultAgentRegistryPath(townRoot)
		if err := config.LoadAgentRegistry(registryPath); err != nil {
			t.Fatalf("load agent registry: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{}
		err := runConfigAgentList(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentList failed: %v", err)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Set JSON flag
		configAgentListJSON = true
		defer func() { configAgentListJSON = false }()

		// Load agent registry
		registryPath := config.DefaultAgentRegistryPath(townRoot)
		if err := config.LoadAgentRegistry(registryPath); err != nil {
			t.Fatalf("load agent registry: %v", err)
		}

		// Capture output
		// Note: This test verifies the command runs without error
		// Full JSON validation would require capturing stdout
		cmd := &cobra.Command{}
		args := []string{}
		err := runConfigAgentList(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentList failed: %v", err)
		}
	})
}

func TestConfigAgentGet(t *testing.T) {
	t.Run("gets built-in agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Load agent registry
		registryPath := config.DefaultAgentRegistryPath(townRoot)
		if err := config.LoadAgentRegistry(registryPath); err != nil {
			t.Fatalf("load agent registry: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{"claude"}
		err := runConfigAgentGet(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentGet failed: %v", err)
		}
	})

	t.Run("gets custom agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Create settings with custom agent
		settings := &config.TownSettings{
			Type:         "town-settings",
			Version:      config.CurrentTownSettingsVersion,
			DefaultAgent: "claude",
			Agents: map[string]*config.RuntimeConfig{
				"my-custom": {
					Command: "my-agent",
					Args:    []string{"--flag1", "--flag2"},
				},
			},
		}
		if err := config.SaveTownSettings(settingsPath, settings); err != nil {
			t.Fatalf("save settings: %v", err)
		}

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Load agent registry
		registryPath := config.DefaultAgentRegistryPath(townRoot)
		if err := config.LoadAgentRegistry(registryPath); err != nil {
			t.Fatalf("load agent registry: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{"my-custom"}
		err := runConfigAgentGet(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentGet failed: %v", err)
		}
	})

	t.Run("returns error for unknown agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Load agent registry
		registryPath := config.DefaultAgentRegistryPath(townRoot)
		if err := config.LoadAgentRegistry(registryPath); err != nil {
			t.Fatalf("load agent registry: %v", err)
		}

		// Run the command with unknown agent
		cmd := &cobra.Command{}
		args := []string{"unknown-agent"}
		err := runConfigAgentGet(cmd, args)
		if err == nil {
			t.Fatal("expected error for unknown agent")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %v, want 'not found'", err)
		}
	})
}

func TestConfigAgentSet(t *testing.T) {
	t.Run("sets custom agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{"my-agent", "my-agent --arg1 --arg2"}
		err := runConfigAgentSet(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentSet failed: %v", err)
		}

		// Verify settings were saved
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		if loaded.Agents == nil {
			t.Fatal("Agents map is nil")
		}
		agent, ok := loaded.Agents["my-agent"]
		if !ok {
			t.Fatal("custom agent not found in settings")
		}
		if agent.Command != "my-agent" {
			t.Errorf("Command = %q, want 'my-agent'", agent.Command)
		}
		if len(agent.Args) != 2 {
			t.Errorf("Args count = %d, want 2", len(agent.Args))
		}
	})

	t.Run("sets agent with single command (no args)", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{"simple-agent", "simple-agent"}
		err := runConfigAgentSet(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentSet failed: %v", err)
		}

		// Verify settings were saved
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		agent := loaded.Agents["simple-agent"]
		if agent.Command != "simple-agent" {
			t.Errorf("Command = %q, want 'simple-agent'", agent.Command)
		}
		if len(agent.Args) != 0 {
			t.Errorf("Args count = %d, want 0", len(agent.Args))
		}
	})

	t.Run("overrides existing agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Create initial settings
		settings := &config.TownSettings{
			Type:         "town-settings",
			Version:      config.CurrentTownSettingsVersion,
			DefaultAgent: "claude",
			Agents: map[string]*config.RuntimeConfig{
				"my-agent": {
					Command: "old-command",
					Args:    []string{"--old"},
				},
			},
		}
		if err := config.SaveTownSettings(settingsPath, settings); err != nil {
			t.Fatalf("save initial settings: %v", err)
		}

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command to override
		cmd := &cobra.Command{}
		args := []string{"my-agent", "new-command --new"}
		err := runConfigAgentSet(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentSet failed: %v", err)
		}

		// Verify settings were updated
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		agent := loaded.Agents["my-agent"]
		if agent.Command != "new-command" {
			t.Errorf("Command = %q, want 'new-command'", agent.Command)
		}
	})
}

func TestConfigAgentRemove(t *testing.T) {
	t.Run("removes custom agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Create settings with custom agent
		settings := &config.TownSettings{
			Type:         "town-settings",
			Version:      config.CurrentTownSettingsVersion,
			DefaultAgent: "claude",
			Agents: map[string]*config.RuntimeConfig{
				"my-agent": {
					Command: "my-agent",
					Args:    []string{"--flag"},
				},
			},
		}
		if err := config.SaveTownSettings(settingsPath, settings); err != nil {
			t.Fatalf("save settings: %v", err)
		}

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command
		cmd := &cobra.Command{}
		args := []string{"my-agent"}
		err := runConfigAgentRemove(cmd, args)
		if err != nil {
			t.Fatalf("runConfigAgentRemove failed: %v", err)
		}

		// Verify agent was removed
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		if loaded.Agents != nil {
			if _, ok := loaded.Agents["my-agent"]; ok {
				t.Error("agent still exists after removal")
			}
		}
	})

	t.Run("rejects removing built-in agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Try to remove a built-in agent
		cmd := &cobra.Command{}
		args := []string{"claude"}
		err := runConfigAgentRemove(cmd, args)
		if err == nil {
			t.Fatal("expected error when removing built-in agent")
		}
		if !strings.Contains(err.Error(), "cannot remove built-in") {
			t.Errorf("error = %v, want 'cannot remove built-in'", err)
		}
	})

	t.Run("returns error for non-existent custom agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Try to remove a non-existent agent
		cmd := &cobra.Command{}
		args := []string{"non-existent"}
		err := runConfigAgentRemove(cmd, args)
		if err == nil {
			t.Fatal("expected error for non-existent agent")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %v, want 'not found'", err)
		}
	})
}

func TestConfigDefaultAgent(t *testing.T) {
	t.Run("gets default agent (shows current)", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Run the command with no args (should show current default)
		cmd := &cobra.Command{}
		args := []string{}
		err := runConfigDefaultAgent(cmd, args)
		if err != nil {
			t.Fatalf("runConfigDefaultAgent failed: %v", err)
		}
	})

	t.Run("sets default agent to built-in", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Set default to gemini
		cmd := &cobra.Command{}
		args := []string{"gemini"}
		err := runConfigDefaultAgent(cmd, args)
		if err != nil {
			t.Fatalf("runConfigDefaultAgent failed: %v", err)
		}

		// Verify settings were saved
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		if loaded.DefaultAgent != "gemini" {
			t.Errorf("DefaultAgent = %q, want 'gemini'", loaded.DefaultAgent)
		}
	})

	t.Run("sets default agent to custom", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)
		settingsPath := config.TownSettingsPath(townRoot)

		// Create settings with custom agent
		settings := &config.TownSettings{
			Type:         "town-settings",
			Version:      config.CurrentTownSettingsVersion,
			DefaultAgent: "claude",
			Agents: map[string]*config.RuntimeConfig{
				"my-custom": {
					Command: "my-agent",
					Args:    []string{},
				},
			},
		}
		if err := config.SaveTownSettings(settingsPath, settings); err != nil {
			t.Fatalf("save settings: %v", err)
		}

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Set default to custom agent
		cmd := &cobra.Command{}
		args := []string{"my-custom"}
		err := runConfigDefaultAgent(cmd, args)
		if err != nil {
			t.Fatalf("runConfigDefaultAgent failed: %v", err)
		}

		// Verify settings were saved
		loaded, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("load settings: %v", err)
		}

		if loaded.DefaultAgent != "my-custom" {
			t.Errorf("DefaultAgent = %q, want 'my-custom'", loaded.DefaultAgent)
		}
	})

	t.Run("returns error for unknown agent", func(t *testing.T) {
		townRoot := setupTestTownForConfig(t)

		// Change to town root
		originalWd, _ := os.Getwd()
		defer os.Chdir(originalWd)
		if err := os.Chdir(townRoot); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Try to set default to unknown agent
		cmd := &cobra.Command{}
		args := []string{"unknown-agent"}
		err := runConfigDefaultAgent(cmd, args)
		if err == nil {
			t.Fatal("expected error for unknown agent")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %v, want 'not found'", err)
		}
	})
}
