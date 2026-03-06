package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnsureConfigYAML ensures config.yaml has both prefix keys set for the given
// beads namespace. Existing non-prefix settings are preserved.
func EnsureConfigYAML(beadsDir, prefix string) error {
	return ensureConfigYAML(beadsDir, prefix, false)
}

// EnsureConfigYAMLIfMissing creates config.yaml with the required defaults when
// it is missing. Existing files are left untouched.
func EnsureConfigYAMLIfMissing(beadsDir, prefix string) error {
	return ensureConfigYAML(beadsDir, prefix, true)
}

// EnsureConfigYAMLFromMetadataIfMissing creates config.yaml when missing using
// metadata-derived defaults for prefix when available.
func EnsureConfigYAMLFromMetadataIfMissing(beadsDir, fallbackPrefix string) error {
	prefix := ConfigDefaultsFromMetadata(beadsDir, fallbackPrefix)
	return ensureConfigYAML(beadsDir, prefix, true)
}

// ConfigDefaultsFromMetadata derives config.yaml defaults from metadata.json.
// Falls back to fallbackPrefix when fields are absent.
func ConfigDefaultsFromMetadata(beadsDir, fallbackPrefix string) string {
	prefix := strings.TrimSpace(strings.TrimSuffix(fallbackPrefix, "-"))
	if prefix == "" {
		prefix = fallbackPrefix
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		return prefix
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return prefix
	}

	if derived := firstString(meta, "issue_prefix", "issue-prefix", "prefix"); derived != "" {
		prefix = strings.TrimSpace(strings.TrimSuffix(derived, "-"))
	} else if doltDB := firstString(meta, "dolt_database"); doltDB != "" {
		prefix = normalizeDoltDatabasePrefix(doltDB)
	}

	return prefix
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func normalizeDoltDatabasePrefix(dbName string) string {
	name := strings.TrimSpace(strings.TrimSuffix(dbName, "-"))
	if strings.HasPrefix(name, "beads_") {
		trimmed := strings.TrimPrefix(name, "beads_")
		if trimmed != "" {
			return trimmed
		}
	}
	return name
}

func ensureConfigYAML(beadsDir, prefix string, onlyIfMissing bool) error {
	configPath := filepath.Join(beadsDir, "config.yaml")
	wantPrefix := "prefix: " + prefix
	wantIssuePrefix := "issue-prefix: " + prefix

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		content := wantPrefix + "\n" + wantIssuePrefix + "\n"
		return os.WriteFile(configPath, []byte(content), 0644)
	}
	if err != nil {
		return err
	}
	if onlyIfMissing {
		return nil
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	foundPrefix := false
	foundIssuePrefix := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "prefix:") {
			lines[i] = wantPrefix
			foundPrefix = true
			continue
		}
		if strings.HasPrefix(trimmed, "issue-prefix:") {
			lines[i] = wantIssuePrefix
			foundIssuePrefix = true
			continue
		}
	}

	if !foundPrefix {
		lines = append(lines, wantPrefix)
	}
	if !foundIssuePrefix {
		lines = append(lines, wantIssuePrefix)
	}

	newContent := strings.Join(lines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	if newContent == content {
		return nil
	}

	return os.WriteFile(configPath, []byte(newContent), 0644)
}
