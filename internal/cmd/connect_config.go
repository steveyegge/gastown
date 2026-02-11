package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v2"

	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/workspace"
)

// writeBeadsConfig persists the remote daemon connection config.
// Writes to both workspace-level (.beads/config.yaml) and global
// (~/.config/gastown/daemon.yaml) locations. The global config allows
// gt connect to work without being in a workspace directory.
func writeBeadsConfig(daemonURL, token string) error {
	// Determine town name if we're in a workspace.
	var townName string
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		townName, _ = workspace.GetTownName(townRoot)
	}

	// Always write to global config so commands work outside workspace.
	if err := writeGlobalDaemonConfig(daemonURL, token, townName); err != nil {
		return fmt.Errorf("writing global daemon config: %w", err)
	}
	fmt.Printf("Wrote daemon config to %s\n", state.DaemonConfigPath())

	// Also write to workspace-level config if we're in a workspace.
	if townRoot != "" {
		beadsDir := filepath.Join(townRoot, ".beads")
		configPath := filepath.Join(beadsDir, "config.yaml")

		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			return fmt.Errorf("creating .beads directory: %w", err)
		}

		if err := writeDaemonConfigToPath(configPath, daemonURL, token); err != nil {
			return fmt.Errorf("writing workspace daemon config: %w", err)
		}
		fmt.Printf("Wrote daemon config to %s\n", configPath)
	}

	return nil
}

// writeGlobalDaemonConfig writes daemon-host, daemon-token, and town-name
// to the global daemon config file.
func writeGlobalDaemonConfig(daemonURL, token, townName string) error {
	configPath := state.DaemonConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	config := make(yaml.MapSlice, 0)

	data, err := os.ReadFile(configPath)
	if err == nil && len(data) > 0 {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parsing existing config: %w", err)
		}
	}

	config = setMapSliceKey(config, "daemon-host", daemonURL)
	config = setMapSliceKey(config, "daemon-token", token)
	if townName != "" {
		config = setMapSliceKey(config, "town-name", townName)
	}

	out, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	return os.WriteFile(configPath, out, 0644)
}

// writeDaemonConfigToPath writes daemon-host and daemon-token to the given YAML file.
func writeDaemonConfigToPath(configPath, daemonURL, token string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Use ordered map to preserve existing keys.
	config := make(yaml.MapSlice, 0)

	// Read existing config if present.
	data, err := os.ReadFile(configPath)
	if err == nil && len(data) > 0 {
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parsing existing config: %w", err)
		}
	}

	// Update or add daemon-host and daemon-token.
	config = setMapSliceKey(config, "daemon-host", daemonURL)
	config = setMapSliceKey(config, "daemon-token", token)

	out, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// setMapSliceKey updates a key in a yaml.MapSlice, or appends it if not found.
func setMapSliceKey(ms yaml.MapSlice, key string, value interface{}) yaml.MapSlice {
	for i, item := range ms {
		if item.Key == key {
			ms[i].Value = value
			return ms
		}
	}
	return append(ms, yaml.MapItem{Key: key, Value: value})
}

// removeMapSliceKey removes a key from a yaml.MapSlice.
func removeMapSliceKey(ms yaml.MapSlice, key string) yaml.MapSlice {
	for i, item := range ms {
		if item.Key == key {
			return append(ms[:i], ms[i+1:]...)
		}
	}
	return ms
}

// removeDaemonKeysFromPath removes daemon-host and daemon-token from a YAML config file.
// Returns true if the file was modified, false if it didn't exist or had no daemon keys.
func removeDaemonKeysFromPath(configPath string) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading config: %w", err)
	}

	var config yaml.MapSlice
	if err := yaml.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("parsing config: %w", err)
	}

	config = removeMapSliceKey(config, "daemon-host")
	config = removeMapSliceKey(config, "daemon-token")

	if len(config) == 0 {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("removing config file: %w", err)
		}
	} else {
		out, err := yaml.Marshal(config)
		if err != nil {
			return false, fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0644); err != nil {
			return false, fmt.Errorf("writing config file: %w", err)
		}
	}

	return true, nil
}

// runDisconnect removes the remote daemon config and reverts to local.
// Cleans up both global and workspace-level daemon configs.
func runDisconnect(cmd *cobra.Command, args []string) error {
	// Remove from global config.
	globalPath := state.DaemonConfigPath()
	if removed, err := removeDaemonKeysFromPath(globalPath); err != nil {
		return err
	} else if removed {
		fmt.Printf("Removed daemon config from %s\n", globalPath)
	}

	// Remove from workspace config if in a workspace.
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		wsPath := filepath.Join(townRoot, ".beads", "config.yaml")
		if removed, err := removeDaemonKeysFromPath(wsPath); err != nil {
			return err
		} else if removed {
			fmt.Printf("Removed daemon config from %s\n", wsPath)
		}
	}

	fmt.Println("Disconnected from remote daemon")
	return nil
}
