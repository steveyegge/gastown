// Package mq provides merge queue functionality.
package mq

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateMRID generates a merge request ID following the convention: <prefix>-mr-<hash>
//
// The hash is derived from the branch name + current timestamp + random bytes to ensure uniqueness.
// Example: gt-mr-abc123 for a gastown merge request.
//
// Parameters:
//   - prefix: The project prefix (e.g., "gt" for gastown)
//   - branch: The source branch name (e.g., "polecat/Nux/gt-xyz")
//
// Returns a string in the format "<prefix>-mr-<6-char-hash>"
func GenerateMRID(prefix, branch string) string {
	// Generate 8 random bytes for additional uniqueness
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes) // crypto/rand.Read only fails on broken system

	return generateMRIDInternal(prefix, branch, time.Now(), randomBytes)
}

// GenerateMRIDWithTime generates a merge request ID using a specific timestamp.
// This is primarily useful for testing to ensure deterministic output.
// Note: Without randomness, two calls with identical inputs will produce the same ID.
//
// Parameters:
//   - prefix: The project prefix (e.g., "gt" for gastown, "bd" for beads)
//   - branch: The source branch name (e.g., "polecat/Nux/gt-xyz")
//   - timestamp: The time to use for ID generation instead of time.Now()
//
// Returns a string in the format "<prefix>-mr-<6-char-hash>"
func GenerateMRIDWithTime(prefix, branch string, timestamp time.Time) string {
	return generateMRIDInternal(prefix, branch, timestamp, nil)
}

// generateMRIDInternal is the internal implementation that combines all inputs.
func generateMRIDInternal(prefix, branch string, timestamp time.Time, randomBytes []byte) string {
	// Combine branch, timestamp, and optional random bytes for uniqueness
	input := fmt.Sprintf("%s:%d:%x", branch, timestamp.UnixNano(), randomBytes)

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(input))

	// Take first 6 characters of hex-encoded hash
	hashStr := hex.EncodeToString(hash[:])[:6]

	return fmt.Sprintf("%s-mr-%s", prefix, hashStr)
}
