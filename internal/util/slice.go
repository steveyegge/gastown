// Package util provides common utilities for Gas Town.
package util

// RemoveFromSlice removes all occurrences of an item from a string slice.
// Returns a new slice without modifying the original.
func RemoveFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// ContainsString checks if a string slice contains the given item.
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
