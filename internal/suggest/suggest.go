// Package suggest provides fuzzy matching and "did you mean" suggestions.
package suggest

import (
	"sort"
	"strings"
	"unicode"
)

// Scoring weights for similarity calculation.
// Higher values indicate stronger signal for a match.
const (
	// ScoreExactMatch is awarded when two strings are identical.
	ScoreExactMatch = 1000

	// ScorePrefixWeight is the per-character bonus for matching prefixes.
	// Prefix matches are weighted highly as users often type the start of commands.
	ScorePrefixWeight = 20

	// ScoreContainsFullWeight is awarded per character when the search term
	// is fully contained within the candidate.
	ScoreContainsFullWeight = 15

	// ScoreSuffixWeight is the per-character bonus for matching suffixes.
	ScoreSuffixWeight = 10

	// ScoreContainsPartialWeight is awarded per character when the candidate
	// is contained within the search term.
	ScoreContainsPartialWeight = 10

	// ScoreDistanceWeight is the per-character bonus for close Levenshtein distance.
	// Applied when edit distance is at most half the longer string's length.
	ScoreDistanceWeight = 5

	// ScoreCommonCharsWeight is the per-character bonus for shared characters.
	ScoreCommonCharsWeight = 2

	// LengthDiffThreshold is the length difference above which a penalty applies.
	LengthDiffThreshold = 5

	// LengthDiffPenalty is the per-character penalty for length differences
	// exceeding LengthDiffThreshold.
	LengthDiffPenalty = 2
)

// Match represents a potential match with its score.
type Match struct {
	Value string
	Score int
}

// FindSimilar finds similar strings from candidates that are close to target.
// Returns up to maxResults matches, sorted by similarity (best first).
func FindSimilar(target string, candidates []string, maxResults int) []string {
	if len(candidates) == 0 || maxResults <= 0 {
		return nil
	}

	target = strings.ToLower(target)

	var matches []Match
	for _, c := range candidates {
		score := similarity(target, strings.ToLower(c))
		if score > 0 {
			matches = append(matches, Match{Value: c, Score: score})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Take top results
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m.Value
	}
	return result
}

// similarity calculates a similarity score between two strings.
// Higher is more similar. Uses a combination of techniques:
// - Prefix matching (high weight)
// - Contains matching (medium weight)
// - Levenshtein distance (for close matches)
// - Common substring matching
func similarity(a, b string) int {
	if a == b {
		return ScoreExactMatch
	}

	score := 0

	// Prefix matching - high value
	prefixLen := commonPrefixLength(a, b)
	if prefixLen > 0 {
		score += prefixLen * ScorePrefixWeight
	}

	// Suffix matching
	suffixLen := commonSuffixLength(a, b)
	if suffixLen > 0 {
		score += suffixLen * ScoreSuffixWeight
	}

	// Contains matching
	if strings.Contains(b, a) {
		score += len(a) * ScoreContainsFullWeight
	} else if strings.Contains(a, b) {
		score += len(b) * ScoreContainsPartialWeight
	}

	// Levenshtein distance for close matches
	dist := levenshteinDistance(a, b)
	maxLen := max(len(a), len(b))
	if maxLen > 0 && dist <= maxLen/2 {
		// Closer distance = higher score
		score += (maxLen - dist) * ScoreDistanceWeight
	}

	// Common characters bonus (order-independent)
	common := commonChars(a, b)
	if common > 0 {
		score += common * ScoreCommonCharsWeight
	}

	// Penalize very different lengths
	lenDiff := abs(len(a) - len(b))
	if lenDiff > LengthDiffThreshold {
		score -= lenDiff * LengthDiffPenalty
	}

	return score
}

// commonPrefixLength returns the length of the common prefix.
func commonPrefixLength(a, b string) int {
	minLen := min(len(a), len(b))
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return minLen
}

// commonSuffixLength returns the length of the common suffix.
func commonSuffixLength(a, b string) int {
	minLen := min(len(a), len(b))
	for i := 0; i < minLen; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return minLen
}

// commonChars counts characters that appear in both strings.
func commonChars(a, b string) int {
	aChars := make(map[rune]int)
	for _, r := range a {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			aChars[r]++
		}
	}

	common := 0
	for _, r := range b {
		if count, ok := aChars[r]; ok && count > 0 {
			common++
			aChars[r]--
		}
	}
	return common
}

// levenshteinDistance calculates the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create distance matrix
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	// Fill the matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min3(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}

	return d[len(a)][len(b)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min3(a, b, c int) int {
	return min(min(a, b), c)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// FormatSuggestion formats an error message with suggestions.
func FormatSuggestion(entity, name string, suggestions []string, createHint string) string {
	var sb strings.Builder

	sb.WriteString(entity)
	sb.WriteString(" '")
	sb.WriteString(name)
	sb.WriteString("' not found")

	if len(suggestions) > 0 {
		sb.WriteString("\n\n  Did you mean?\n")
		for _, s := range suggestions {
			sb.WriteString("    • ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}

	if createHint != "" {
		sb.WriteString("\n  ")
		sb.WriteString(createHint)
	}

	return sb.String()
}
