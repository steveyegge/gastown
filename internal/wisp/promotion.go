// Package wisp provides utilities for working with the .beads directory.
// This file implements promotion criteria checks for wisp compaction.
// A wisp that meets any promotion criterion should be promoted to Level 1
// (permanent bead) rather than being compacted away.
package wisp

import "time"

// KeepLabel is the label that explicitly marks a wisp for preservation.
const KeepLabel = "gt:keep"

// WispCandidate holds the data needed to evaluate promotion criteria for a wisp.
// Populate this from a beads.Issue before calling the promotion helpers.
type WispCandidate struct {
	// CommentCount is the number of comments on this wisp.
	CommentCount int

	// NonWispRefCount is the number of non-wisp (permanent) beads that reference this wisp.
	NonWispRefCount int

	// Labels are the labels attached to this wisp.
	Labels []string

	// Age is how long the wisp has been open.
	Age time.Duration

	// TTL is the configured TTL for this wisp's type.
	TTL time.Duration
}

// HasComments returns true if someone has discussed this wisp.
// A wisp with comments has proven value and should be promoted.
func HasComments(c *WispCandidate) bool {
	return c.CommentCount > 0
}

// IsReferenced returns true if this wisp is linked from a non-wisp bead.
// Being referenced by permanent work means this wisp has context worth preserving.
func IsReferenced(c *WispCandidate) bool {
	return c.NonWispRefCount > 0
}

// HasKeepLabel returns true if this wisp has been explicitly flagged for preservation.
func HasKeepLabel(c *WispCandidate) bool {
	for _, label := range c.Labels {
		if label == KeepLabel {
			return true
		}
	}
	return false
}

// ShouldPromote returns true if any promotion criterion is met.
// Criteria (any one triggers promotion):
//  1. Has comments — someone discussed it
//  2. Is referenced — linked from a non-wisp bead
//  3. Has keep label — explicitly flagged
//  4. Open past TTL — something is stuck and needs attention
func ShouldPromote(c *WispCandidate) bool {
	return HasComments(c) || IsReferenced(c) || HasKeepLabel(c) || isPastTTL(c)
}

// isPastTTL returns true if the wisp has been open longer than its TTL.
// A zero TTL means "never expires" so it won't trigger promotion via this path.
func isPastTTL(c *WispCandidate) bool {
	return c.TTL > 0 && c.Age > c.TTL
}
