package cmd

// shortHash returns at most 8 characters of a hash for display.
// Prevents panics when git returns abbreviated hashes shorter than 8 chars.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
