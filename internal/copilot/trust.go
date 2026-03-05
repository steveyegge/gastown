// Package copilot provides GitHub Copilot CLI integration for Gas Town.
package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnsureTownTrusted adds townRoot to ~/.copilot/config.json trusted_folders
// if it (or a parent directory) is not already present. This prevents the
// Copilot CLI from prompting for workspace trust during automated sessions.
func EnsureTownTrusted(townRoot string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return ensureTownTrustedAt(filepath.Join(homeDir, ".copilot", "config.json"), townRoot)
}

// ensureTownTrustedAt is the testable core that operates on an explicit config path.
func ensureTownTrustedAt(configPath, townRoot string) error {
	townRoot, err := filepath.Abs(townRoot)
	if err != nil {
		return fmt.Errorf("resolving town root: %w", err)
	}

	config, err := readConfigJSON(configPath)
	if err != nil {
		return err
	}

	folders := extractTrustedFolders(config)

	// Check if townRoot or any parent is already trusted.
	if isCoveredByExisting(townRoot, folders) {
		return nil
	}

	// Append townRoot and write back.
	folders = append(folders, townRoot)
	config["trusted_folders"] = folders

	fmt.Fprintf(os.Stderr, "copilot: added %s to trusted_folders in %s\n", townRoot, configPath)
	return writeConfigJSON(configPath, config)
}

// readConfigJSON reads and parses the config file, returning an empty map if
// the file does not exist.
func readConfigJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("reading copilot config: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing copilot config: %w", err)
	}
	return config, nil
}

// extractTrustedFolders pulls the trusted_folders string slice from config.
// Returns an empty slice if the key is missing or not an array.
func extractTrustedFolders(config map[string]interface{}) []string {
	raw, ok := config["trusted_folders"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// isCoveredByExisting returns true if townRoot or any of its parents appears
// in the existing trusted folders list.
func isCoveredByExisting(townRoot string, folders []string) bool {
	for _, f := range folders {
		if isEqualOrParent(f, townRoot) {
			return true
		}
	}
	return false
}

// isEqualOrParent reports whether candidate equals target or is a parent of target.
func isEqualOrParent(candidate, target string) bool {
	if candidate == "" || target == "" {
		return false
	}
	candidate = filepath.Clean(candidate)
	target = filepath.Clean(target)
	if candidate == target {
		return true
	}
	// Ensure candidate ends with separator for prefix check.
	prefix := candidate
	if !os.IsPathSeparator(candidate[len(candidate)-1]) {
		prefix += string(filepath.Separator)
	}
	return len(target) > len(prefix) && target[:len(prefix)] == prefix
}

// writeConfigJSON marshals config to JSON and writes it to path, creating
// parent directories as needed.
func writeConfigJSON(path string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling copilot config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating copilot config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing copilot config: %w", err)
	}
	return nil
}
