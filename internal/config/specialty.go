package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// SpecialtyConfig represents per-rig specialty definitions.
// Loaded from <rig>/conductor/specialties.toml with built-in defaults.
type SpecialtyConfig struct {
	Specialties []Specialty `toml:"specialty"`
}

// Specialty defines an artisan specialty domain.
type Specialty struct {
	// Name is the specialty identifier (e.g., "frontend", "backend").
	Name string `toml:"name"`

	// Description explains what this specialty covers.
	Description string `toml:"description"`

	// PromptTemplate is the template file for artisans of this specialty.
	PromptTemplate string `toml:"prompt_template,omitempty"`

	// FilePatterns are glob patterns for files this specialty owns.
	FilePatterns []string `toml:"file_patterns,omitempty"`

	// Labels are bead labels that route to this specialty.
	Labels []string `toml:"labels,omitempty"`
}

// BuiltinSpecialties returns the default set of specialties.
func BuiltinSpecialties() *SpecialtyConfig {
	return &SpecialtyConfig{
		Specialties: []Specialty{
			{
				Name:         "frontend",
				Description:  "UI components, styling, browser interactions, accessibility",
				FilePatterns: []string{"src/components/**", "src/pages/**", "*.css", "*.tsx"},
				Labels:       []string{"frontend", "ui", "css"},
			},
			{
				Name:         "backend",
				Description:  "APIs, database, auth, server-side logic",
				FilePatterns: []string{"internal/**", "api/**", "*.go", "models/**"},
				Labels:       []string{"backend", "api", "database"},
			},
			{
				Name:         "tests",
				Description:  "Test writing, coverage, fixtures, mocking",
				FilePatterns: []string{"*_test.go", "**/*.test.ts", "tests/**"},
				Labels:       []string{"tests", "testing", "coverage"},
			},
			{
				Name:         "security",
				Description:  "Security review, vulnerability scanning, auth/authz, input validation",
				FilePatterns: []string{"internal/auth/**", "internal/middleware/**", "**/*auth*", "**/*token*"},
				Labels:       []string{"security", "auth", "vulnerability"},
			},
			{
				Name:         "docs",
				Description:  "Documentation, README files, API docs, architecture docs",
				FilePatterns: []string{"docs/**", "*.md", "**/*.md"},
				Labels:       []string{"docs", "documentation", "readme"},
			},
		},
	}
}

// LoadSpecialties loads specialty configuration for a rig.
// Resolution: built-in defaults, then merged with <rig>/conductor/specialties.toml if present.
func LoadSpecialties(rigPath string) (*SpecialtyConfig, error) {
	base := BuiltinSpecialties()

	if rigPath == "" {
		return base, nil
	}

	overridePath := filepath.Join(rigPath, "conductor", "specialties.toml")
	data, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, fmt.Errorf("reading specialties config: %w", err)
	}

	var override SpecialtyConfig
	if err := toml.Unmarshal(data, &override); err != nil {
		return nil, fmt.Errorf("parsing specialties config %s: %w", overridePath, err)
	}

	return mergeSpecialties(base, &override), nil
}

// mergeSpecialties merges override into base.
// Override specialties replace base specialties with the same name;
// new specialties are appended.
func mergeSpecialties(base, override *SpecialtyConfig) *SpecialtyConfig {
	if override == nil || len(override.Specialties) == 0 {
		return base
	}

	result := &SpecialtyConfig{}
	seen := make(map[string]bool)

	// Build index of overrides
	overrideMap := make(map[string]Specialty)
	for _, s := range override.Specialties {
		overrideMap[s.Name] = s
	}

	// Apply overrides to matching base entries
	for _, s := range base.Specialties {
		if o, ok := overrideMap[s.Name]; ok {
			result.Specialties = append(result.Specialties, mergeSpecialty(s, o))
			seen[s.Name] = true
		} else {
			result.Specialties = append(result.Specialties, s)
		}
	}

	// Append new specialties from override
	for _, s := range override.Specialties {
		if !seen[s.Name] {
			result.Specialties = append(result.Specialties, s)
		}
	}

	return result
}

// mergeSpecialty merges a single specialty override into a base.
func mergeSpecialty(base, override Specialty) Specialty {
	result := base
	if override.Description != "" {
		result.Description = override.Description
	}
	if override.PromptTemplate != "" {
		result.PromptTemplate = override.PromptTemplate
	}
	if len(override.FilePatterns) > 0 {
		result.FilePatterns = override.FilePatterns
	}
	if len(override.Labels) > 0 {
		result.Labels = override.Labels
	}
	return result
}

// GetSpecialty returns the named specialty, or nil if not found.
func (c *SpecialtyConfig) GetSpecialty(name string) *Specialty {
	for i := range c.Specialties {
		if c.Specialties[i].Name == name {
			return &c.Specialties[i]
		}
	}
	return nil
}

// Names returns all specialty names.
func (c *SpecialtyConfig) Names() []string {
	names := make([]string, len(c.Specialties))
	for i, s := range c.Specialties {
		names[i] = s.Name
	}
	return names
}

// MatchLabels returns specialties that match any of the given labels.
func (c *SpecialtyConfig) MatchLabels(labels []string) []Specialty {
	labelSet := make(map[string]bool, len(labels))
	for _, l := range labels {
		labelSet[l] = true
	}

	var matches []Specialty
	for _, s := range c.Specialties {
		for _, l := range s.Labels {
			if labelSet[l] {
				matches = append(matches, s)
				break
			}
		}
	}
	return matches
}
