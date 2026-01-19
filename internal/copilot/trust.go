package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/util"
)

const (
	configDirName  = ".copilot"
	configFileName = "config.json"
)

// EnsureTrustedFolder adds the path to Copilot's trusted_folders if needed.
// Returns true if the config was updated.
func EnsureTrustedFolder(path string) (bool, error) {
	if path == "" {
		return false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolving path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	configDir, err := copilotConfigDir()
	if err != nil {
		return false, err
	}
	configPath := filepath.Join(configDir, configFileName)

	cfg, err := readConfig(configPath)
	if err != nil {
		return false, err
	}

	trusted := readTrustedFolders(cfg)
	if isPathTrusted(trusted, absPath) {
		return false, nil
	}

	trusted = append(trusted, absPath)
	cfg["trusted_folders"] = trusted

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return false, fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshaling config: %w", err)
	}
	if err := util.AtomicWriteFile(configPath, data, 0600); err != nil {
		return false, fmt.Errorf("writing config: %w", err)
	}
	return true, nil
}

func copilotConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, configDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home dir: %w", err)
	}
	return filepath.Join(home, configDirName), nil
}

func readConfig(path string) (map[string]interface{}, error) {
	cfg := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func readTrustedFolders(cfg map[string]interface{}) []string {
	raw, ok := cfg["trusted_folders"]
	if !ok {
		return nil
	}

	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func isPathTrusted(trusted []string, target string) bool {
	for _, entry := range trusted {
		if entry == "" {
			continue
		}
		normalized := normalizePath(entry)
		if normalized == "" {
			continue
		}
		if isPathWithin(normalized, target) {
			return true
		}
	}
	return false
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func isPathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
