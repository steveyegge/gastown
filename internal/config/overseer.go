package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OverseerConfig represents the human operator's identity (mayor/overseer.json).
// The overseer is the human who controls Gas Town, distinct from AI agents.
type OverseerConfig struct {
	Type     string `json:"type"`               // "overseer"
	Version  int    `json:"version"`            // schema version
	Name     string `json:"name"`               // display name
	Email    string `json:"email,omitempty"`    // email address
	Username string `json:"username,omitempty"` // username/handle
	Source   string `json:"source"`             // how identity was detected
}

// CurrentOverseerVersion is the current schema version for OverseerConfig.
const CurrentOverseerVersion = 1

// OverseerConfigPath returns the standard path for overseer config in a town.
func OverseerConfigPath(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "overseer.json")
}

// LoadOverseerConfig loads and validates an overseer configuration file.
func LoadOverseerConfig(path string) (*OverseerConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading overseer config: %w", err)
	}

	var config OverseerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing overseer config: %w", err)
	}

	if err := validateOverseerConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveOverseerConfig saves an overseer configuration to a file.
func SaveOverseerConfig(path string, config *OverseerConfig) error {
	if err := validateOverseerConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding overseer config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: overseer config doesn't contain secrets
		return fmt.Errorf("writing overseer config: %w", err)
	}

	return nil
}

// validateOverseerConfig validates an OverseerConfig.
func validateOverseerConfig(c *OverseerConfig) error {
	// Type must be "overseer" (allow empty for backwards compat on load, set on save)
	if c.Type != "overseer" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'overseer', got '%s'", ErrInvalidType, c.Type)
	}
	// Ensure type is set for saving
	if c.Type == "" {
		c.Type = "overseer"
	}
	if c.Version > CurrentOverseerVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentOverseerVersion)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingField)
	}
	return nil
}

// DetectOverseer attempts to detect the overseer's identity from available sources.
// Priority order:
//  1. Existing config file (if path provided and exists)
//  2. Git config (user.name + user.email)
//  3. GitHub CLI (gh api user)
//  4. Environment ($USER or whoami)
func DetectOverseer(townRoot string) (*OverseerConfig, error) {
	configPath := OverseerConfigPath(townRoot)

	// Priority 1: Check existing config
	if existing, err := LoadOverseerConfig(configPath); err == nil {
		return existing, nil
	}

	// Priority 2: Try git config
	if config := detectFromGitConfig(townRoot); config != nil {
		return config, nil
	}

	// Priority 3: Try GitHub CLI
	if config := detectFromGitHub(); config != nil {
		return config, nil
	}

	// Priority 4: Fall back to environment
	return detectFromEnvironment(), nil
}

// detectFromGitConfig attempts to get identity from git config.
func detectFromGitConfig(dir string) *OverseerConfig {
	// Try to get user.name
	nameCmd := exec.Command("git", "config", "user.name")
	nameCmd.Dir = dir
	nameOut, err := nameCmd.Output()
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(nameOut))
	if name == "" {
		return nil
	}

	config := &OverseerConfig{
		Type:    "overseer",
		Version: CurrentOverseerVersion,
		Name:    name,
		Source:  "git-config",
	}

	// Try to get user.email (optional)
	emailCmd := exec.Command("git", "config", "user.email")
	emailCmd.Dir = dir
	if emailOut, err := emailCmd.Output(); err == nil {
		config.Email = strings.TrimSpace(string(emailOut))
	}

	// Extract username from email if available
	if config.Email != "" {
		if idx := strings.Index(config.Email, "@"); idx > 0 {
			config.Username = config.Email[:idx]
		}
	}

	return config
}

// detectFromGitHub attempts to get identity from GitHub CLI.
func detectFromGitHub() *OverseerConfig {
	cmd := exec.Command("gh", "api", "user", "--jq", ".login + \"|\" + .name + \"|\" + .email")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) < 1 || parts[0] == "" {
		return nil
	}

	config := &OverseerConfig{
		Type:     "overseer",
		Version:  CurrentOverseerVersion,
		Username: parts[0],
		Source:   "github-cli",
	}

	// Use name if available, otherwise username
	if len(parts) >= 2 && parts[1] != "" {
		config.Name = parts[1]
	} else {
		config.Name = parts[0]
	}

	// Add email if available
	if len(parts) >= 3 && parts[2] != "" {
		config.Email = parts[2]
	}

	return config
}

// detectFromEnvironment falls back to environment variables.
func detectFromEnvironment() *OverseerConfig {
	username := os.Getenv("USER")
	if username == "" {
		// Try whoami as last resort
		if out, err := exec.Command("whoami").Output(); err == nil {
			username = strings.TrimSpace(string(out))
		}
	}
	if username == "" {
		username = "overseer"
	}

	return &OverseerConfig{
		Type:     "overseer",
		Version:  CurrentOverseerVersion,
		Name:     username,
		Username: username,
		Source:   "environment",
	}
}

// LoadOrDetectOverseer loads existing config or detects and saves a new one.
func LoadOrDetectOverseer(townRoot string) (*OverseerConfig, error) {
	configPath := OverseerConfigPath(townRoot)

	// Try loading existing
	if config, err := LoadOverseerConfig(configPath); err == nil {
		return config, nil
	}

	// Detect new
	config, err := DetectOverseer(townRoot)
	if err != nil {
		return nil, err
	}

	// Save for next time
	if err := SaveOverseerConfig(configPath, config); err != nil {
		// Non-fatal - we can still use the detected config
		fmt.Fprintf(os.Stderr, "warning: could not save overseer config: %v\n", err)
	}

	return config, nil
}

// FormatOverseerIdentity returns a formatted string for display.
// Example: "Steve Yegge <stevey@example.com>"
func (c *OverseerConfig) FormatOverseerIdentity() string {
	if c.Email != "" {
		return fmt.Sprintf("%s <%s>", c.Name, c.Email)
	}
	if c.Username != "" && c.Username != c.Name {
		return fmt.Sprintf("%s (@%s)", c.Name, c.Username)
	}
	return c.Name
}
