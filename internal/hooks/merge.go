package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MergeHooks merges a base config with applicable overrides for a target.
//
// Merge rules:
//  1. Start with base hooks
//  2. Apply role override (crew, witness, refinery, polecats, mayor, deacon)
//  3. Apply rig+role override if exists (gastown/crew, beads/witness, etc.)
//
// For each hook type (SessionStart, PreToolUse, etc.):
//   - Hooks with same matcher: override replaces base entirely
//   - Hooks with different matcher: both are included
//   - Override with empty hook list for a matcher: removes that hook (explicit disable)
func MergeHooks(base *HooksConfig, overrides map[string]*HooksConfig, target string) *HooksConfig {
	if base == nil {
		base = &HooksConfig{}
	}

	result := cloneConfig(base)

	// Apply overrides in order of specificity
	for _, key := range GetApplicableOverrides(target) {
		override, ok := overrides[key]
		if !ok || override == nil {
			continue
		}
		result = applyOverride(result, override)
	}

	return result
}

// LoadAllOverrides loads all override files from the overrides directory.
func LoadAllOverrides() (map[string]*HooksConfig, error) {
	overrides := make(map[string]*HooksConfig)

	dir := OverridesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return overrides, nil // No overrides dir is fine
		}
		return nil, fmt.Errorf("reading overrides directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		// Convert filename back to target key (gastown__crew.json -> gastown/crew)
		key := strings.TrimSuffix(name, ".json")
		key = strings.ReplaceAll(key, "__", "/")

		cfg, err := loadConfig(filepath.Join(dir, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping invalid hooks override %s: %v\n", name, err)
			continue
		}
		overrides[key] = cfg
	}

	return overrides, nil
}

// applyOverride merges an override onto a result config.
func applyOverride(result, override *HooksConfig) *HooksConfig {
	result.PreToolUse = mergeEntries(result.PreToolUse, override.PreToolUse)
	result.PostToolUse = mergeEntries(result.PostToolUse, override.PostToolUse)
	result.SessionStart = mergeEntries(result.SessionStart, override.SessionStart)
	result.Stop = mergeEntries(result.Stop, override.Stop)
	result.PreCompact = mergeEntries(result.PreCompact, override.PreCompact)
	result.UserPromptSubmit = mergeEntries(result.UserPromptSubmit, override.UserPromptSubmit)
	return result
}

// mergeEntries merges override entries into base entries.
// Same-matcher entries from override replace base entries.
// Override entries with empty Hooks list remove that matcher (explicit disable).
// Different-matcher entries are appended.
func mergeEntries(base, override []HookEntry) []HookEntry {
	if len(override) == 0 {
		return base
	}

	// Build a map of matchers to override entries for quick lookup
	overrideByMatcher := make(map[string]HookEntry)
	for _, entry := range override {
		overrideByMatcher[entry.Matcher] = entry
	}

	// Process base entries: replace or keep
	var result []HookEntry
	replacedMatchers := make(map[string]bool)

	for _, baseEntry := range base {
		if ovEntry, found := overrideByMatcher[baseEntry.Matcher]; found {
			replacedMatchers[baseEntry.Matcher] = true
			// Empty hooks list means explicit disable (remove)
			if len(ovEntry.Hooks) > 0 {
				result = append(result, ovEntry)
			}
		} else {
			result = append(result, baseEntry)
		}
	}

	// Add override entries with new matchers (not in base)
	for _, ovEntry := range override {
		if !replacedMatchers[ovEntry.Matcher] {
			if len(ovEntry.Hooks) > 0 {
				result = append(result, ovEntry)
			}
		}
	}

	return result
}

// cloneConfig creates a deep copy of a HooksConfig.
func cloneConfig(cfg *HooksConfig) *HooksConfig {
	return &HooksConfig{
		PreToolUse:       cloneEntries(cfg.PreToolUse),
		PostToolUse:      cloneEntries(cfg.PostToolUse),
		SessionStart:     cloneEntries(cfg.SessionStart),
		Stop:             cloneEntries(cfg.Stop),
		PreCompact:       cloneEntries(cfg.PreCompact),
		UserPromptSubmit: cloneEntries(cfg.UserPromptSubmit),
	}
}

func cloneEntries(entries []HookEntry) []HookEntry {
	if entries == nil {
		return nil
	}
	result := make([]HookEntry, len(entries))
	for i, e := range entries {
		result[i] = HookEntry{
			Matcher: e.Matcher,
			Hooks:   make([]Hook, len(e.Hooks)),
		}
		copy(result[i].Hooks, e.Hooks)
	}
	return result
}
