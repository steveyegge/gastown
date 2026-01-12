// Package profile provides provider profile configuration for Gas Town.
// Profiles define API provider settings including authentication references
// and model selections for multi-provider LLM support.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
)

// Profile represents a provider profile configuration.
// Each profile defines an API provider with authentication and model settings.
type Profile struct {
	// Provider is the API provider name (e.g., "anthropic", "openai").
	Provider string `json:"provider"`

	// AuthRef is the environment variable name containing the API key.
	// This is a reference, not the actual key, for security.
	AuthRef string `json:"auth_ref"`

	// ModelMain is the primary model to use for most tasks.
	ModelMain string `json:"model_main"`

	// ModelFast is the faster/cheaper model for simpler tasks.
	ModelFast string `json:"model_fast"`
}

// townConfigWithProfiles is a partial town.json structure for profile loading.
// We only parse the profiles field to avoid coupling with the full TownConfig.
type townConfigWithProfiles struct {
	Profiles map[string]Profile `json:"profiles"`
}

// LoadProfiles loads provider profiles from a town.json file.
// Returns an empty map (not nil) if no profiles are defined.
// Returns an error if the file cannot be read or parsed.
func LoadProfiles(path string) (map[string]Profile, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from trusted config location
	if err != nil {
		return nil, fmt.Errorf("reading profiles config: %w", err)
	}

	var config townConfigWithProfiles
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing profiles config: %w", err)
	}

	// Return empty map if no profiles defined
	if config.Profiles == nil {
		return make(map[string]Profile), nil
	}

	return config.Profiles, nil
}
