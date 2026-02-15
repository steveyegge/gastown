package doctor

import (
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
