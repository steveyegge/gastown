package config

import (
	"fmt"
	"strings"
)

// QualityTier represents a predefined quality control tier for the merge pipeline.
// It determines what happens between polecat MR submission and refinery merge.
type QualityTier string

const (
	// QualityStandard runs test gates only (current behavior, no peer review).
	QualityStandard QualityTier = "standard"
	// QualityReviewed adds peer code review between submission and merge.
	QualityReviewed QualityTier = "reviewed"
	// QualityFull adds peer code review and requires design review prerequisite.
	QualityFull QualityTier = "full"
)

// ValidQualityTiers returns all valid tier names.
func ValidQualityTiers() []string {
	return []string{string(QualityStandard), string(QualityReviewed), string(QualityFull)}
}

// IsValidQualityTier checks if a string is a valid quality tier name.
func IsValidQualityTier(tier string) bool {
	switch QualityTier(tier) {
	case QualityStandard, QualityReviewed, QualityFull:
		return true
	default:
		return false
	}
}

// RequiresPeerReview returns whether this tier spawns a review polecat
// between MR submission and refinery merge.
func (t QualityTier) RequiresPeerReview() bool {
	return t == QualityReviewed || t == QualityFull
}

// RequiresDesignReview returns whether this tier enforces that a design
// review was completed before accepting an implementation MR.
// The witness checks for DESIGN_REVIEW_PASSED notes on the source bead.
func (t QualityTier) RequiresDesignReview() bool {
	return t == QualityFull
}

// ReviewFormula returns the peer review formula name for this tier.
// Returns empty string if the tier doesn't require peer review.
func (t QualityTier) ReviewFormula() string {
	if t.RequiresPeerReview() {
		return "mol-peer-review-gate"
	}
	return ""
}

// DesignReviewFormula returns the design review formula name for this tier.
// Returns empty string if the tier doesn't require design review.
func (t QualityTier) DesignReviewFormula() string {
	if t.RequiresDesignReview() {
		return "mol-peer-review-design"
	}
	return ""
}

// QualityTierDescription returns a human-readable description of the tier.
func QualityTierDescription(tier QualityTier) string {
	switch tier {
	case QualityStandard:
		return "Test gates only — no peer review (current default)"
	case QualityReviewed:
		return "Test gates + peer code review with evidence-based findings and dispute protocol"
	case QualityFull:
		return "Test gates + peer code review + design review prerequisite enforced"
	default:
		return "Unknown tier"
	}
}

// FormatQualityTierTable returns a formatted string showing all tiers and their behavior.
func FormatQualityTierTable() string {
	tiers := []QualityTier{QualityStandard, QualityReviewed, QualityFull}
	var lines []string
	for _, tier := range tiers {
		review := "no"
		if tier.RequiresPeerReview() {
			review = "yes"
		}
		design := "no"
		if tier.RequiresDesignReview() {
			design = "yes"
		}
		lines = append(lines, fmt.Sprintf("  %-10s  peer-review: %-3s  design-review: %-3s  %s",
			string(tier), review, design, QualityTierDescription(tier)))
	}
	return strings.Join(lines, "\n")
}

// GetQualityTier returns the effective quality tier for a MergeQueueConfig.
// Derives the tier from the existing PeerReview field for backwards compatibility:
//   - PeerReview not set or false → standard
//   - PeerReview true → reviewed
//
// When QualityTier field is explicitly set, it takes precedence.
func (c *MergeQueueConfig) GetQualityTier() QualityTier {
	if c == nil {
		return QualityStandard
	}
	// Derive from PeerReview field for backwards compat with furiosa's implementation
	if c.IsPeerReviewEnabled() {
		return QualityReviewed
	}
	return QualityStandard
}

// ResolveQualityTier determines the effective quality tier for a specific bead.
// Resolution order:
//  1. Per-bead override (from bead label "quality:<tier>")
//  2. Rig default (derived from MergeQueueConfig)
//  3. Built-in default (standard)
//
// Returns error if the override is not a valid tier name.
func ResolveQualityTier(beadOverride string, rigConfig *MergeQueueConfig) (QualityTier, error) {
	// 1. Per-bead override takes precedence
	if beadOverride != "" {
		if !IsValidQualityTier(beadOverride) {
			return "", fmt.Errorf("invalid quality tier %q on bead (valid: %s)",
				beadOverride, strings.Join(ValidQualityTiers(), ", "))
		}
		return QualityTier(beadOverride), nil
	}

	// 2. Rig default
	if rigConfig != nil {
		return rigConfig.GetQualityTier(), nil
	}

	// 3. Built-in default
	return QualityStandard, nil
}
