package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v2"

	"github.com/steveyegge/gastown/internal/workspace"
)

// writeBeadsConfig persists the remote daemon connection config.
func writeBeadsConfig(daemonURL, token string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding workspace root: %w", err)
	}

	beadsDir := filepath.Join(townRoot, ".beads")
	configPath := filepath.Join(beadsDir, "config.yaml")

	// Ensure .beads directory exists.
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating .beads directory: %w", err)
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

	fmt.Printf("Wrote daemon config to %s\n", configPath)
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

// runDisconnect removes the remote daemon config and reverts to local.
func runDisconnect(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding workspace root: %w", err)
	}

	configPath := filepath.Join(townRoot, ".beads", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Disconnected from remote daemon")
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}

	var config yaml.MapSlice
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	config = removeMapSliceKey(config, "daemon-host")
	config = removeMapSliceKey(config, "daemon-token")

	if len(config) == 0 {
		// No remaining keys; remove the file.
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("removing config file: %w", err)
		}
	} else {
		out, err := yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("marshaling config: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0644); err != nil {
			return fmt.Errorf("writing config file: %w", err)
		}
	}

	fmt.Println("Disconnected from remote daemon")
	return nil
}
