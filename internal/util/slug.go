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

// ValidTypeAbbreviations maps type abbreviations to full type names.
// Used to validate and identify semantic slugs.
var ValidTypeAbbreviations = map[string]string{
	"dec":  "decision",
	"esc":  "escalation",
	"tsk":  "task",
	"bug":  "bug",
	"feat": "feature",
	"epc":  "epic",
	"chr":  "chore",
	"mr":   "merge-request",
	"mol":  "molecule",
	"wsp":  "wisp",
	"agt":  "agent",
	"rol":  "role",
	"cvy":  "convoy",
	"evt":  "event",
	"msg":  "message",
}

// IsSemanticSlug checks if an ID looks like a semantic slug (vs a canonical ID).
// Semantic slugs have format: prefix-type-slug+random (e.g., gt-dec-cache_strategyabc123)
// Canonical IDs have format: prefix-random (e.g., gt-abc123)
func IsSemanticSlug(id string) bool {
	parts := strings.SplitN(id, "-", 3)
	if len(parts) != 3 {
		return false // Not enough components for semantic slug
	}

	// Check if second component is a valid type abbreviation
	_, isValidType := ValidTypeAbbreviations[parts[1]]
	if !isValidType {
		return false
	}

	// Semantic slugs have a slug component (third part) with at least 7 chars
	// (minimum 3 for slug + minimum 4 for random)
	if len(parts[2]) < 7 {
		return false
	}

	return true
}

// ResolveSemanticSlug converts a semantic slug to its canonical ID.
// If the input is already a canonical ID, it returns it unchanged.
//
// Examples:
//   - "gt-dec-cache_strategyabc123" -> "gt-abc123"
//   - "gt-esc-server_downy7m2k" -> "gt-y7m2k"
//   - "gt-abc123" -> "gt-abc123" (already canonical)
func ResolveSemanticSlug(id string) string {
	if !IsSemanticSlug(id) {
		return id // Already canonical or unrecognized format
	}

	parts := strings.SplitN(id, "-", 3)
	if len(parts) != 3 {
		return id
	}

	prefix := parts[0]
	slugWithRandom := parts[2]

	// Handle child segments (e.g., gt-dec-slugabc123.child_name)
	childSuffix := ""
	if dotIdx := strings.Index(slugWithRandom, "."); dotIdx > 0 {
		childSuffix = slugWithRandom[dotIdx:]
		slugWithRandom = slugWithRandom[:dotIdx]
	}

	// Extract random from end of slug (4-8 alphanumeric chars)
	// Strategy: Look for underscore boundary first (reliable), then fall back to length heuristics.
	// Bug fix: gt-3vqgi4 - previously only tried 6,5,4 which failed for 7-char randoms.
	//
	// The slug format is: title_slug + random (no separator between them)
	// e.g., "cache_strategy" + "abc123" = "cache_strategyabc123"
	// But "refinery_patrol_complete_merged_" + "1syec3r" has underscore before random.

	// First pass: Look for underscore boundaries (handles cases like "merged_1syec3r")
	for randLen := 8; randLen >= 4; randLen-- {
		if len(slugWithRandom) >= 3+randLen {
			potentialRandom := slugWithRandom[len(slugWithRandom)-randLen:]
			if isAlphanumeric(potentialRandom) {
				// Check if there's an underscore right before the random
				charBeforeRandom := len(slugWithRandom) - randLen - 1
				if charBeforeRandom >= 0 && slugWithRandom[charBeforeRandom] == '_' {
					// Underscore boundary - this is a reliable delimiter
					return prefix + "-" + potentialRandom + childSuffix
				}
			}
		}
	}

	// Second pass: Fall back to standard length heuristics (6, 5, 4)
	// This handles the common case where random is directly appended to slug word
	for randLen := 6; randLen >= 4; randLen-- {
		if len(slugWithRandom) >= 3+randLen {
			potentialRandom := slugWithRandom[len(slugWithRandom)-randLen:]
			if isAlphanumeric(potentialRandom) {
				// Reconstruct canonical ID: prefix-random[.child]
				return prefix + "-" + potentialRandom + childSuffix
			}
		}
	}

	// Could not extract random, return original
	return id
}

// isAlphanumeric checks if a string contains only lowercase letters and digits.
func isAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// DeriveChannelSlug derives a Slack channel-safe slug from a title string.
// Used for creating epic-based decision channel names.
//
// Rules:
//   - Lowercase
//   - Replace spaces/punctuation with hyphens
//   - Remove consecutive hyphens
//   - Truncate to maxLen chars (default 30)
//   - Strip leading/trailing hyphens
//
// Examples:
//
//	"Ephemeral Polecat Merge Workflow: Rebase-as-Work Architecture" -> "ephemeral-polecat-merge"
//	"Fix bug #123 in the parser" -> "fix-bug-123-in-the-parser"
func DeriveChannelSlug(title string) string {
	return DeriveChannelSlugWithMaxLen(title, 30)
}

// DeriveChannelSlugWithMaxLen derives a channel slug with a custom max length.
func DeriveChannelSlugWithMaxLen(title string, maxLen int) string {
	if title == "" {
		return ""
	}

	// Lowercase
	slug := strings.ToLower(title)

	// Replace non-alphanumeric characters with hyphens
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		} else {
			result = append(result, '-')
		}
	}
	slug = string(result)

	// Remove consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}

	// Strip leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Truncate to maxLen at word boundary if possible
	if len(slug) > maxLen {
		truncated := slug[:maxLen]
		// Find last hyphen to truncate at word boundary
		if lastHyphen := strings.LastIndex(truncated, "-"); lastHyphen > maxLen/2 {
			truncated = truncated[:lastHyphen]
		}
		slug = truncated
	}

	// Final cleanup - strip trailing hyphens after truncation
	slug = strings.TrimRight(slug, "-")

	return slug
}
