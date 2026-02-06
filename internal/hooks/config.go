// Package hooks provides centralized Claude Code hook management for Gas Town.
//
// It manages a base hook configuration and per-role/per-rig overrides,
// generating .claude/settings.json files for all agents in the workspace.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HookEntry represents a single hook matcher with its associated hooks.
type HookEntry struct {
	Matcher string `json:"matcher"`
	Hooks   []Hook `json:"hooks"`
}

// Hook represents an individual hook command.
type Hook struct {
	Type    string `json:"type"`    // "command"
	Command string `json:"command"`
}

// HooksConfig represents the hooks section of a Claude Code settings.json.
type HooksConfig struct {
	PreToolUse       []HookEntry `json:"PreToolUse,omitempty"`
	PostToolUse      []HookEntry `json:"PostToolUse,omitempty"`
	SessionStart     []HookEntry `json:"SessionStart,omitempty"`
	Stop             []HookEntry `json:"Stop,omitempty"`
	PreCompact       []HookEntry `json:"PreCompact,omitempty"`
	UserPromptSubmit []HookEntry `json:"UserPromptSubmit,omitempty"`
}

// SettingsJSON represents the full Claude Code settings.json structure.
// Unknown fields are preserved during sync via the Extra map.
type SettingsJSON struct {
	EditorMode     string          `json:"-"`
	EnabledPlugins map[string]bool `json:"-"`
	Hooks          HooksConfig     `json:"-"`
	// Extra holds all raw fields for roundtrip preservation.
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalSettings parses a settings.json file, preserving all fields.
func UnmarshalSettings(data []byte) (*SettingsJSON, error) {
	s := &SettingsJSON{
		Extra: make(map[string]json.RawMessage),
	}

	// Capture everything into the raw map
	if err := json.Unmarshal(data, &s.Extra); err != nil {
		return nil, err
	}

	// Extract known fields
	if raw, ok := s.Extra["editorMode"]; ok {
		json.Unmarshal(raw, &s.EditorMode)
	}
	if raw, ok := s.Extra["enabledPlugins"]; ok {
		json.Unmarshal(raw, &s.EnabledPlugins)
	}
	if raw, ok := s.Extra["hooks"]; ok {
		json.Unmarshal(raw, &s.Hooks)
	}

	return s, nil
}

// MarshalSettings serializes a SettingsJSON, preserving unknown fields.
func MarshalSettings(s *SettingsJSON) ([]byte, error) {
	if s.Extra == nil {
		s.Extra = make(map[string]json.RawMessage)
	}

	// Write known fields back into the map
	if s.EditorMode != "" {
		raw, _ := json.Marshal(s.EditorMode)
		s.Extra["editorMode"] = raw
	}
	if s.EnabledPlugins != nil {
		raw, _ := json.Marshal(s.EnabledPlugins)
		s.Extra["enabledPlugins"] = raw
	}

	// Always write hooks (even if empty, it's the managed section)
	raw, err := json.Marshal(s.Hooks)
	if err != nil {
		return nil, err
	}
	s.Extra["hooks"] = raw

	return json.MarshalIndent(s.Extra, "", "  ")
}

// LoadSettings reads and parses a settings.json file, preserving unknown fields.
// Returns a zero-value SettingsJSON if the file doesn't exist.
func LoadSettings(path string) (*SettingsJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SettingsJSON{}, nil
		}
		return nil, err
	}
	return UnmarshalSettings(data)
}

// HooksEqual returns true if two HooksConfigs are structurally equal.
// Compares by serializing to JSON for reliable deep equality.
func HooksEqual(a, b *HooksConfig) bool {
	aj, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aj) == string(bj)
}

// Target represents a managed settings.json location.
type Target struct {
	Path string // Full path to .claude/settings.json
	Key  string // Override key: "gastown/crew", "mayor", etc.
	Rig  string // Rig name or empty for town-level
	Role string // crew, witness, refinery, polecats, mayor, deacon
}

// DisplayKey returns a human-readable label for the target.
// For targets with a rig, shows "rig/role"; for town-level targets, shows the role.
func (t Target) DisplayKey() string {
	if t.Rig != "" {
		return t.Rig + "/" + t.Role
	}
	return t.Role
}

// Merge merges an override config into a base config using per-matcher merging.
// For each hook type present in the override:
//   - Same matcher: override replaces the base entry entirely
//   - Different matcher: both entries are included (base first, then override)
//   - Empty hooks list on a matcher: removes that entry (explicit disable)
//
// Hook types not present in the override are preserved from the base.
func Merge(base, override *HooksConfig) *HooksConfig {
	result := copyConfig(base)

	for _, eventType := range EventTypes {
		overrideEntries := override.GetEntries(eventType)
		if len(overrideEntries) == 0 {
			continue
		}

		baseEntries := result.GetEntries(eventType)
		if baseEntries == nil {
			baseEntries = []HookEntry{}
		}

		for _, oe := range overrideEntries {
			replaced := false
			for i, be := range baseEntries {
				if be.Matcher == oe.Matcher {
					replaced = true
					if len(oe.Hooks) == 0 {
						// Explicit disable: remove this entry
						baseEntries = append(baseEntries[:i], baseEntries[i+1:]...)
					} else {
						baseEntries[i] = oe
					}
					break
				}
			}
			if !replaced && len(oe.Hooks) > 0 {
				baseEntries = append(baseEntries, oe)
			}
		}

		result.SetEntries(eventType, baseEntries)
	}

	return result
}

// copyConfig creates a deep copy of a HooksConfig.
func copyConfig(c *HooksConfig) *HooksConfig {
	if c == nil {
		return &HooksConfig{}
	}
	result := &HooksConfig{}
	for _, eventType := range EventTypes {
		entries := c.GetEntries(eventType)
		if entries == nil {
			continue
		}
		copied := make([]HookEntry, len(entries))
		for i, e := range entries {
			copied[i] = HookEntry{
				Matcher: e.Matcher,
				Hooks:   make([]Hook, len(e.Hooks)),
			}
			copy(copied[i].Hooks, e.Hooks)
		}
		result.SetEntries(eventType, copied)
	}
	return result
}

// ComputeExpected computes the expected HooksConfig for a target by loading
// the base config and applying all applicable overrides in order of specificity.
// If no base config exists, uses DefaultBase().
func ComputeExpected(target string) (*HooksConfig, error) {
	base, err := LoadBase()
	if err != nil {
		if os.IsNotExist(err) {
			base = DefaultBase()
		} else {
			return nil, fmt.Errorf("loading base config: %w", err)
		}
	}

	result := base
	for _, overrideKey := range GetApplicableOverrides(target) {
		override, err := LoadOverride(overrideKey)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("loading override %q: %w", overrideKey, err)
		}
		result = Merge(result, override)
	}

	return result, nil
}

// DiscoverTargets finds all managed .claude/settings.json locations in the workspace.
// Returns Target structs with path, override key, rig, and role information.
func DiscoverTargets(townRoot string) ([]Target, error) {
	var targets []Target

	// Town-level targets
	targets = append(targets, Target{
		Path: filepath.Join(townRoot, "mayor", ".claude", "settings.json"),
		Key:  "mayor",
		Role: "mayor",
	})
	targets = append(targets, Target{
		Path: filepath.Join(townRoot, "deacon", ".claude", "settings.json"),
		Key:  "deacon",
		Role: "deacon",
	})

	// Scan rigs
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "mayor" || entry.Name() == "deacon" ||
			entry.Name() == ".beads" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip directories that aren't rigs (no crew/ or witness/ or polecats/ subdirs)
		if !isRig(rigPath) {
			continue
		}

		// Rig-level
		targets = append(targets, Target{
			Path: filepath.Join(rigPath, ".claude", "settings.json"),
			Key:  rigName + "/rig",
			Rig:  rigName,
			Role: "rig",
		})

		// Crew-level
		crewDir := filepath.Join(rigPath, "crew")
		if info, err := os.Stat(crewDir); err == nil && info.IsDir() {
			targets = append(targets, Target{
				Path: filepath.Join(crewDir, ".claude", "settings.json"),
				Key:  rigName + "/crew",
				Rig:  rigName,
				Role: "crew",
			})

			// Individual crew members
			if members, err := os.ReadDir(crewDir); err == nil {
				for _, m := range members {
					if m.IsDir() && !strings.HasPrefix(m.Name(), ".") {
						targets = append(targets, Target{
							Path: filepath.Join(crewDir, m.Name(), ".claude", "settings.json"),
							Key:  rigName + "/crew",
							Rig:  rigName,
							Role: "crew",
						})
					}
				}
			}
		}

		// Polecats-level
		polecatsDir := filepath.Join(rigPath, "polecats")
		if info, err := os.Stat(polecatsDir); err == nil && info.IsDir() {
			targets = append(targets, Target{
				Path: filepath.Join(polecatsDir, ".claude", "settings.json"),
				Key:  rigName + "/polecats",
				Rig:  rigName,
				Role: "polecats",
			})

			// Individual polecats
			if polecats, err := os.ReadDir(polecatsDir); err == nil {
				for _, p := range polecats {
					if p.IsDir() && !strings.HasPrefix(p.Name(), ".") {
						targets = append(targets, Target{
							Path: filepath.Join(polecatsDir, p.Name(), ".claude", "settings.json"),
							Key:  rigName + "/polecats",
							Rig:  rigName,
							Role: "polecats",
						})
					}
				}
			}
		}

		// Witness
		witnessDir := filepath.Join(rigPath, "witness")
		if info, err := os.Stat(witnessDir); err == nil && info.IsDir() {
			targets = append(targets, Target{
				Path: filepath.Join(witnessDir, ".claude", "settings.json"),
				Key:  rigName + "/witness",
				Rig:  rigName,
				Role: "witness",
			})
		}

		// Refinery
		refineryDir := filepath.Join(rigPath, "refinery")
		if info, err := os.Stat(refineryDir); err == nil && info.IsDir() {
			targets = append(targets, Target{
				Path: filepath.Join(refineryDir, ".claude", "settings.json"),
				Key:  rigName + "/refinery",
				Rig:  rigName,
				Role: "refinery",
			})
		}
	}

	return targets, nil
}

// isRig checks if a directory looks like a rig (has crew/, witness/, or polecats/ subdirectory).
func isRig(path string) bool {
	for _, sub := range []string{"crew", "witness", "polecats", "refinery"} {
		info, err := os.Stat(filepath.Join(path, sub))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// EventTypes returns the known hook event type names in display order.
var EventTypes = []string{"PreToolUse", "PostToolUse", "SessionStart", "Stop", "PreCompact", "UserPromptSubmit"}

// GetEntries returns the hook entries for a given event type.
func (c *HooksConfig) GetEntries(eventType string) []HookEntry {
	switch eventType {
	case "PreToolUse":
		return c.PreToolUse
	case "PostToolUse":
		return c.PostToolUse
	case "SessionStart":
		return c.SessionStart
	case "Stop":
		return c.Stop
	case "PreCompact":
		return c.PreCompact
	case "UserPromptSubmit":
		return c.UserPromptSubmit
	default:
		return nil
	}
}

// SetEntries sets the hook entries for a given event type.
func (c *HooksConfig) SetEntries(eventType string, entries []HookEntry) {
	switch eventType {
	case "PreToolUse":
		c.PreToolUse = entries
	case "PostToolUse":
		c.PostToolUse = entries
	case "SessionStart":
		c.SessionStart = entries
	case "Stop":
		c.Stop = entries
	case "PreCompact":
		c.PreCompact = entries
	case "UserPromptSubmit":
		c.UserPromptSubmit = entries
	}
}

// ToMap converts HooksConfig to a map for iteration over non-empty event types.
func (c *HooksConfig) ToMap() map[string][]HookEntry {
	m := make(map[string][]HookEntry)
	for _, et := range EventTypes {
		entries := c.GetEntries(et)
		if len(entries) > 0 {
			m[et] = entries
		}
	}
	return m
}

// AddEntry appends a hook entry to the given event type if the matcher doesn't already exist.
// Returns true if the entry was added.
func (c *HooksConfig) AddEntry(eventType string, entry HookEntry) bool {
	entries := c.GetEntries(eventType)
	for _, e := range entries {
		if e.Matcher == entry.Matcher {
			return false
		}
	}
	c.SetEntries(eventType, append(entries, entry))
	return true
}

// gtDir returns the ~/.gt directory path.
func gtDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".gt")
	}
	return filepath.Join(home, ".gt")
}

// BasePath returns the path to the base hooks config file.
func BasePath() string {
	return filepath.Join(gtDir(), "hooks-base.json")
}

// OverridePath returns the path to the override config for a given target.
func OverridePath(target string) string {
	// Replace "/" with "__" for filesystem safety (e.g., "gastown/crew" -> "gastown__crew")
	safe := strings.ReplaceAll(target, "/", "__")
	return filepath.Join(gtDir(), "hooks-overrides", safe+".json")
}

// OverridesDir returns the path to the overrides directory.
func OverridesDir() string {
	return filepath.Join(gtDir(), "hooks-overrides")
}

// LoadBase loads the base hooks configuration from ~/.gt/hooks-base.json.
// Returns an error if the file doesn't exist or can't be parsed.
func LoadBase() (*HooksConfig, error) {
	return loadConfig(BasePath())
}

// LoadOverride loads an override configuration for the given target.
// Returns an error if the file doesn't exist or can't be parsed.
func LoadOverride(target string) (*HooksConfig, error) {
	return loadConfig(OverridePath(target))
}

// SaveBase writes the base hooks configuration to ~/.gt/hooks-base.json.
func SaveBase(cfg *HooksConfig) error {
	return saveConfig(BasePath(), cfg)
}

// SaveOverride writes an override configuration for the given target.
func SaveOverride(target string, cfg *HooksConfig) error {
	return saveConfig(OverridePath(target), cfg)
}

// MarshalConfig serializes a HooksConfig to pretty-printed JSON.
func MarshalConfig(cfg *HooksConfig) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

// ValidTarget returns true if the target string is a valid override target.
// Valid targets are roles (crew, witness, etc.) or rig/role combinations.
func ValidTarget(target string) bool {
	validRoles := map[string]bool{
		"crew": true, "witness": true, "refinery": true,
		"polecats": true, "mayor": true, "deacon": true,
		"rig": true,
	}

	// Simple role target
	if validRoles[target] {
		return true
	}

	// Rig/role target (e.g., "gastown/crew")
	parts := strings.SplitN(target, "/", 2)
	if len(parts) == 2 && parts[0] != "" && validRoles[parts[1]] {
		return true
	}

	return false
}

// DefaultBase returns a sensible default base configuration.
// This includes PATH setup and gt prime hooks that all agents need.
func DefaultBase() *HooksConfig {
	pathSetup := `export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"`

	return &HooksConfig{
		SessionStart: []HookEntry{
			{
				Matcher: "",
				Hooks: []Hook{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s && gt prime --hook", pathSetup),
					},
				},
			},
		},
		PreCompact: []HookEntry{
			{
				Matcher: "",
				Hooks: []Hook{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s && gt prime --hook", pathSetup),
					},
				},
			},
		},
		UserPromptSubmit: []HookEntry{
			{
				Matcher: "",
				Hooks: []Hook{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s && gt mail check --inject", pathSetup),
					},
				},
			},
		},
		Stop: []HookEntry{
			{
				Matcher: "",
				Hooks: []Hook{
					{
						Type:    "command",
						Command: fmt.Sprintf("%s && gt costs record", pathSetup),
					},
				},
			},
		},
	}
}

// GetApplicableOverrides returns the override keys in order of specificity
// for a given target. More specific overrides are applied later (and win).
//
// Examples:
//
//	"gastown/crew" -> ["crew", "gastown/crew"]
//	"mayor"        -> ["mayor"]
//	"beads/witness" -> ["witness", "beads/witness"]
func GetApplicableOverrides(target string) []string {
	parts := strings.SplitN(target, "/", 2)
	if len(parts) == 2 {
		// Rig/role target: apply role override first, then rig+role
		return []string{parts[1], target}
	}
	// Simple role target
	return []string{target}
}

// loadConfig loads a HooksConfig from a JSON file.
func loadConfig(path string) (*HooksConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg HooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &cfg, nil
}

// saveConfig writes a HooksConfig to a JSON file, creating directories as needed.
func saveConfig(path string, cfg *HooksConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Add trailing newline for human editing
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
