package doctor

import (
	"fmt"
	"sort"
	"strings"
)

// FilterResult holds the outcome of filtering checks by name or category.
type FilterResult struct {
	Matched   []Check  // Checks that matched the filter
	Unmatched []string // Input names that didn't match any check or category
}

// FilterChecks filters registered checks by name or category.
// Resolution order per arg:
//  1. Exact check name match (after normalization)
//  2. Category name match (case-insensitive)
//  3. Unmatched
//
// If args is empty, all checks are returned.
func FilterChecks(checks []Check, args []string) *FilterResult {
	if len(args) == 0 {
		return &FilterResult{Matched: checks}
	}

	result := &FilterResult{}
	seen := make(map[string]bool) // deduplicate by check name

	for _, arg := range args {
		normalized := NormalizeName(arg)
		matched := false

		// 1. Try exact check name match (after normalization)
		for _, check := range checks {
			if NormalizeName(check.Name()) == normalized {
				if !seen[check.Name()] {
					result.Matched = append(result.Matched, check)
					seen[check.Name()] = true
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// 2. Try category match (case-insensitive)
		for _, check := range checks {
			if strings.EqualFold(check.Category(), arg) {
				if !seen[check.Name()] {
					result.Matched = append(result.Matched, check)
					seen[check.Name()] = true
				}
				matched = true
			}
		}
		if !matched {
			result.Unmatched = append(result.Unmatched, arg)
		}
	}

	return result
}

// FilterErrorKind indicates what went wrong in category-first filtering.
type FilterErrorKind int

const (
	FilterErrorNone            FilterErrorKind = iota
	FilterErrorUnknownCategory                 // First arg didn't match any category
	FilterErrorUnknownCheck                    // Second arg didn't match any check in the category
)

// FilterCategoryResult holds the outcome of category-first filtering.
type FilterCategoryResult struct {
	Matched       []Check         // Checks that matched
	CategoryName  string          // Resolved canonical category name
	CategoryInput string          // Raw user input for category
	CheckInput    string          // Raw user input for check name
	Error         error           // Non-nil if category or check name is invalid
	ErrorKind     FilterErrorKind // Type of error for caller to format messages
}

// FilterByCategory implements category-first argument resolution.
//   - Empty category: returns all checks.
//   - Category only: returns all checks in that category (case-insensitive).
//   - Category + checkName: returns the single matching check within that category.
func FilterByCategory(checks []Check, category, checkName string) *FilterCategoryResult {
	result := &FilterCategoryResult{
		CategoryInput: category,
		CheckInput:    checkName,
	}

	if category == "" {
		result.Matched = checks
		return result
	}

	// Resolve canonical category name
	canonical := ResolveCategory(category)
	if canonical == "" {
		result.ErrorKind = FilterErrorUnknownCategory
		result.Error = fmt.Errorf("unknown category %q", category)
		return result
	}
	result.CategoryName = canonical

	// Filter checks by category
	categoryChecks := ChecksInCategory(checks, canonical)

	if checkName == "" {
		result.Matched = categoryChecks
		return result
	}

	// Find specific check within category
	normalized := NormalizeName(checkName)
	for _, check := range categoryChecks {
		if NormalizeName(check.Name()) == normalized {
			result.Matched = []Check{check}
			return result
		}
	}

	result.ErrorKind = FilterErrorUnknownCheck
	result.Error = fmt.Errorf("unknown check %q in category %q", checkName, canonical)
	return result
}

// ResolveCategory returns the canonical category name for a case-insensitive input,
// or empty string if not found.
func ResolveCategory(input string) string {
	for _, cat := range CategoryOrder {
		if strings.EqualFold(cat, input) {
			return cat
		}
	}
	return ""
}

// ChecksInCategory returns all checks belonging to a category (case-insensitive).
func ChecksInCategory(checks []Check, category string) []Check {
	var result []Check
	for _, check := range checks {
		if strings.EqualFold(check.Category(), category) {
			result = append(result, check)
		}
	}
	return result
}

// SuggestCategory returns category names within Levenshtein distance <= 2 of the input.
func SuggestCategory(input string) []string {
	normalized := strings.ToLower(input)
	var suggestions []string
	for _, cat := range CategoryOrder {
		dist := levenshtein(normalized, strings.ToLower(cat))
		if dist > 0 && dist <= 2 {
			suggestions = append(suggestions, cat)
		}
	}
	return suggestions
}

// NormalizeName converts input to canonical kebab-case form.
//   - Case-insensitive: Orphan-Sessions → orphan-sessions
//   - Underscore/hyphen equivalence: orphan_sessions → orphan-sessions
func NormalizeName(input string) string {
	return strings.ReplaceAll(strings.ToLower(input), "_", "-")
}

// SuggestCheck returns up to 3 closest check names within Levenshtein distance ≤ 2.
func SuggestCheck(checks []Check, input string) []string {
	normalized := NormalizeName(input)

	type candidate struct {
		name string
		dist int
	}
	var candidates []candidate

	for _, check := range checks {
		checkNorm := NormalizeName(check.Name())
		dist := levenshtein(normalized, checkNorm)
		if dist > 0 && dist <= 2 {
			candidates = append(candidates, candidate{check.Name(), dist})
		}
	}

	// Sort by distance, then alphabetically for stability
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].dist != candidates[j].dist {
			return candidates[i].dist < candidates[j].dist
		}
		return candidates[i].name < candidates[j].name
	})

	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].name
	}
	return result
}

// levenshtein calculates the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row optimization
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev = curr
	}

	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
