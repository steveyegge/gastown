// Package validator implements decision validator discovery and execution.
package validator

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Validator represents a discovered validator script.
type Validator struct {
	Path   string // Full path to the validator
	Name   string // Validator name (from filename)
	When   string // When to run: "create", "stop", "resolve"
	Scope  string // Scope: "decision", "any", or specific type
	Source string // Where it was found: "user", "project", "beads"
}

// DiscoverValidators finds all validators for a given event.
// It searches in order: beads-specific, project-level, user-global.
// Returns validators sorted by source priority (beads > project > user).
func DiscoverValidators(townRoot, when string) ([]Validator, error) {
	var validators []Validator

	// Define search paths in priority order (later = higher priority)
	searchPaths := []struct {
		path   string
		source string
	}{
		{filepath.Join(os.Getenv("HOME"), ".config", "gt", "validators"), "user"},
		{filepath.Join(townRoot, ".gt", "validators"), "project"},
		{filepath.Join(townRoot, ".beads", "validators"), "beads"},
	}

	for _, sp := range searchPaths {
		found, err := discoverInDir(sp.path, sp.source, when)
		if err != nil {
			continue // Directory might not exist
		}
		validators = append(validators, found...)
	}

	// Sort by source priority (beads first, then project, then user)
	sort.SliceStable(validators, func(i, j int) bool {
		return sourcePriority(validators[i].Source) > sourcePriority(validators[j].Source)
	})

	return validators, nil
}

// discoverInDir finds validators in a single directory.
func discoverInDir(dir, source, when string) ([]Validator, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var validators []Validator
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		v, ok := parseValidatorName(name)
		if !ok {
			continue
		}

		// Check if this validator matches the event
		if v.When != when && v.When != "any" {
			continue
		}

		v.Path = filepath.Join(dir, name)
		v.Source = source

		// Check if executable
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0111 == 0 {
			continue // Not executable
		}

		validators = append(validators, v)
	}

	return validators, nil
}

// parseValidatorName parses a validator filename into its components.
// Expected format: {when}-{scope}-{name}.{ext}
// Examples:
//   - create-decision-require-schema.sh
//   - stop-decision-check-artifacts.py
//   - create-any-log-to-audit
func parseValidatorName(filename string) (Validator, bool) {
	// Remove extension for parsing
	name := filename
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}

	// Pattern: when-scope-name (at least 3 parts)
	pattern := regexp.MustCompile(`^(create|stop|resolve|any)-([a-z]+)-(.+)$`)
	matches := pattern.FindStringSubmatch(name)
	if matches == nil {
		return Validator{}, false
	}

	return Validator{
		Name:  matches[3],
		When:  matches[1],
		Scope: matches[2],
	}, true
}

// sourcePriority returns priority for sorting (higher = runs first).
func sourcePriority(source string) int {
	switch source {
	case "beads":
		return 3
	case "project":
		return 2
	case "user":
		return 1
	default:
		return 0
	}
}

// DiscoverForScope returns validators matching both event and scope.
func DiscoverForScope(townRoot, when, scope string) ([]Validator, error) {
	all, err := DiscoverValidators(townRoot, when)
	if err != nil {
		return nil, err
	}

	var filtered []Validator
	for _, v := range all {
		if v.Scope == scope || v.Scope == "any" {
			filtered = append(filtered, v)
		}
	}

	return filtered, nil
}
