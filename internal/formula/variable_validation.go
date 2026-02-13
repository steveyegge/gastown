package formula

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// variablePattern matches {{variable}} template placeholders.
// It captures the variable name (alphanumeric + underscore, starting with letter/underscore).
var variablePattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

// ExtractTemplateVariables finds all {{variable}} patterns in text.
// Returns a deduplicated, sorted list of variable names.
// Handlebars helpers like {{#if}}, {{/each}}, {{else}} are excluded.
func ExtractTemplateVariables(text string) []string {
	matches := variablePattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var vars []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := match[1]

		// Skip Handlebars helpers and keywords
		if isHandlebarsKeyword(name) {
			continue
		}

		if !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}

	sort.Strings(vars)
	return vars
}

// isHandlebarsKeyword returns true for Handlebars control keywords
// that look like variables but aren't (e.g., "else", "this").
func isHandlebarsKeyword(name string) bool {
	switch name {
	case "else", "this", "root", "index", "key", "first", "last",
		"end", "range", "with", "block", "define", "template", "nil":
		return true
	default:
		return false
	}
}

// ValidateTemplateVariables checks that all {{variable}} placeholders used
// in the formula are defined in the [vars] section.
//
// This catches the bug where formulas use computed variables like {{ready_count}}
// in their text but don't define them in [vars], causing bd mol wisp to fail
// with "missing required variables" error.
//
// Variables with any definition in [vars] (even with default="") are considered valid.
func (f *Formula) ValidateTemplateVariables() error {
	// Collect all text that might contain variables
	var allText strings.Builder

	// Description
	allText.WriteString(f.Description)
	allText.WriteString("\n")

	// Steps (workflow)
	for _, step := range f.Steps {
		allText.WriteString(step.Title)
		allText.WriteString("\n")
		allText.WriteString(step.Description)
		allText.WriteString("\n")
	}

	// Legs (convoy)
	for _, leg := range f.Legs {
		allText.WriteString(leg.Title)
		allText.WriteString("\n")
		allText.WriteString(leg.Description)
		allText.WriteString("\n")
		allText.WriteString(leg.Focus)
		allText.WriteString("\n")
	}

	// Synthesis
	if f.Synthesis != nil {
		allText.WriteString(f.Synthesis.Title)
		allText.WriteString("\n")
		allText.WriteString(f.Synthesis.Description)
		allText.WriteString("\n")
	}

	// Template (expansion)
	for _, tmpl := range f.Template {
		allText.WriteString(tmpl.Title)
		allText.WriteString("\n")
		allText.WriteString(tmpl.Description)
		allText.WriteString("\n")
	}

	// Aspects
	for _, aspect := range f.Aspects {
		allText.WriteString(aspect.Title)
		allText.WriteString("\n")
		allText.WriteString(aspect.Description)
		allText.WriteString("\n")
		allText.WriteString(aspect.Focus)
		allText.WriteString("\n")
	}

	// Extract all variables used
	usedVars := ExtractTemplateVariables(allText.String())

	// Check each against defined vars
	var undefined []string
	for _, v := range usedVars {
		if _, defined := f.Vars[v]; !defined {
			undefined = append(undefined, v)
		}
	}

	if len(undefined) > 0 {
		return fmt.Errorf("undefined template variables: %s (add to [vars] section with default=\"\" for computed values)",
			strings.Join(undefined, ", "))
	}

	return nil
}

