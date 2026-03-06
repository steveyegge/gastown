// sling_idempotency.go — normalizeAgentID trims surrounding whitespace and trailing slash for comparison
package cmd

import "strings"

// normalizeAgentID trims surrounding whitespace and trailing slash for comparison.
func normalizeAgentID(v string) string {
	return strings.TrimSuffix(strings.TrimSpace(v), "/")
}

// matchesSlingTarget returns true when target should be treated as equivalent
// to the existing assignee for idempotent sling behavior.
//
// Only matches unambiguous equivalences. Ambiguous shorthand targets
// (e.g., "rig/name" which could resolve to polecats or crew) and pool
// targets (e.g., "deacon/dogs" which dispatches to an idle dog) are NOT
// matched — these must go through normal resolution to pick the right agent.
func matchesSlingTarget(target, assignee, selfAgent string) bool {
	assigneeNorm := normalizeAgentID(assignee)
	if assigneeNorm == "" {
		return false
	}

	target = strings.TrimSpace(target)
	if target == "" || target == "." {
		selfNorm := normalizeAgentID(selfAgent)
		return selfNorm != "" && selfNorm == assigneeNorm
	}

	targetNorm := normalizeAgentID(target)
	if targetNorm == assigneeNorm {
		return true
	}

	// Rig-only target maps to polecat dispatch within that rig.
	// Intentionally excludes crew/witness/refinery: rig-name targets resolve
	// exclusively to polecats via IsRigName, so "gastown" + "gastown/crew/alex"
	// is NOT a match (different dispatch path).
	parts := strings.Split(targetNorm, "/")
	if len(parts) == 1 && strings.HasPrefix(assigneeNorm, targetNorm+"/polecats/") {
		return true
	}

	// NOTE: Two-segment shorthand targets (e.g., "gastown/alex") and pool
	// targets (e.g., "deacon/dogs") are intentionally NOT matched here.
	// - Shorthand: the real resolver has priority logic (prefers crew when
	//   crew dir exists) that this pure function cannot replicate.
	// - Pool: "deacon/dogs" means "dispatch to an idle dog", not "keep the
	//   current dog". Matching would prevent reassignment to idle workers.
	// Users can use full paths (e.g., "gastown/polecats/toast") for
	// unambiguous idempotent behavior with these targets.

	return false
}
