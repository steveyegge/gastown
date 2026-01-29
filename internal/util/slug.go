// Package util provides shared utility functions.
package util

import (
	"fmt"
	"strings"
)

// GenerateSemanticSlug creates a human-readable semantic slug from a bead.
// Format: prefix-type-title_slug+random (e.g., gt-dec-cache_strategyzfyl8)
//
// Parameters:
//   - id: The canonical bead ID (e.g., "gt-zfyl8")
//   - typeCode: The type abbreviation (e.g., "dec", "esc", "bug", "tsk")
//   - title: The title or question text to slugify
func GenerateSemanticSlug(id, typeCode, title string) string {
	// Extract prefix and random from ID (e.g., "gt-zfyl8" -> prefix="gt", random="zfyl8")
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return id // Can't parse, return original
	}
	prefix := parts[0]
	random := parts[1]

	// Strip child suffix if present (e.g., "zfyl8.1" -> "zfyl8")
	if dotIdx := strings.Index(random, "."); dotIdx > 0 {
		random = random[:dotIdx]
	}

	// Generate slug from title
	slug := GenerateSlug(title)
	if slug == "" {
		slug = "untitled"
	}

	return fmt.Sprintf("%s-%s-%s%s", prefix, typeCode, slug, random)
}

// GenerateDecisionSlug creates a semantic slug for a decision bead.
// Convenience wrapper for GenerateSemanticSlug with type code "dec".
func GenerateDecisionSlug(id, question string) string {
	return GenerateSemanticSlug(id, "dec", question)
}

// GenerateEscalationSlug creates a semantic slug for an escalation bead.
// Convenience wrapper for GenerateSemanticSlug with type code "esc".
func GenerateEscalationSlug(id, title string) string {
	return GenerateSemanticSlug(id, "esc", title)
}

// GenerateSlug converts a title/question to a slug.
// Removes stop words, lowercases, replaces non-alphanumeric with underscores.
func GenerateSlug(title string) string {
	if title == "" {
		return "untitled"
	}

	// Lowercase
	slug := strings.ToLower(title)

	// Stop words to remove
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "as": true,
		"and": true, "or": true, "but": true, "nor": true,
		"is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true,
		"should": true, "would": true, "could": true,
		"how": true, "what": true, "which": true, "who": true,
		"we": true, "i": true, "you": true, "they": true,
	}

	// Replace non-alphanumeric with spaces
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		} else {
			result = append(result, ' ')
		}
	}
	slug = string(result)

	// Split and filter stop words
	words := strings.Fields(slug)
	var filtered []string
	for _, word := range words {
		if !stopWords[word] && len(word) > 0 {
			filtered = append(filtered, word)
		}
	}

	// Fallback if all words were filtered
	if len(filtered) == 0 && len(words) > 0 {
		filtered = []string{words[0]}
	}

	// Join with underscores
	slug = strings.Join(filtered, "_")

	// Ensure starts with letter
	if len(slug) > 0 && (slug[0] >= '0' && slug[0] <= '9') {
		slug = "n" + slug
	}

	// Truncate to 40 chars at word boundary
	if len(slug) > 40 {
		truncated := slug[:40]
		if lastUnderscore := strings.LastIndex(truncated, "_"); lastUnderscore > 20 {
			truncated = truncated[:lastUnderscore]
		}
		slug = truncated
	}

	// Ensure minimum length
	if len(slug) < 3 {
		slug = slug + strings.Repeat("x", 3-len(slug))
	}

	// Clean up
	slug = strings.Trim(slug, "_")

	return slug
}
