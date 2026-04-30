package cmd

import (
	"fmt"
	"regexp"
	"strings"
)

// validateRatifyCC enforces the 2026-04-27 Approval Routing doctrine: ratify-class
// mail must not include mayor/ in the cc list (Munger is the default approver,
// Mayor is informational only). The doctrine landed in /home/karuna/gt/CLAUDE.md
// "Approval Routing" section + 13 active per-crew CLAUDE.md pointers.
//
// Returns a non-nil error with educational guidance when:
//   - subject contains a ratify-class keyword (EN or KR), AND
//   - cc list contains a mayor/ recipient, AND
//   - body does not contain an explicit Approval Routing waiver.
//
// All checks are case-insensitive. The error message instructs the sender to
// either re-send without mayor/ in cc, or add a waiver line in the body.
//
// Educational, not punitive: the error includes both the rule and the escape
// hatch, matching the user's "강제 + 자연스럽게" pacing.
func validateRatifyCC(subject string, cc []string, body string) error {
	if len(cc) == 0 {
		return nil
	}
	if !subjectIsRatifyClass(subject) {
		return nil
	}
	if bodyHasApprovalRoutingWaiver(body) {
		return nil
	}
	if mayorAddrs := mayorCCMatches(cc); len(mayorAddrs) > 0 {
		return fmt.Errorf(
			"ratify-class mail with mayor/ in cc — 2026-04-27 Approval Routing doctrine\n\n"+
				"  matched cc:     %s\n"+
				"  subject:        %q\n\n"+
				"Munger is the default approver. Mayor cc on ratify mail is the active-orchestrator\n"+
				"pattern that Mayor v2 (passive caretaker) replaces. Re-send without mayor/ in cc.\n\n"+
				"Waiver: if mayor/ cc is intentional (e.g., Mayor IS the dispatcher), add a line\n"+
				"to the body: \"Approval Routing waiver: <reason>\"\n\n"+
				"Reference: ~/gt/CLAUDE.md \"Approval Routing\" section",
			strings.Join(mayorAddrs, ", "), subject)
	}
	return nil
}

// ratifyKeywordRegex matches subject keywords in EN and KR that signal a
// ratify-class mail (approval/sign-off requested or delivered). Word-boundary
// anchoring on the EN side avoids false positives like "approved-disapproval"
// (which would match "approve" mid-token without boundaries).
//
// EN keywords: ratify, approve, approval, ratification, sign-off (with hyphen
// or space), GO/NO-GO. KR keywords: 비준, 승인, 재가.
var ratifyKeywordRegex = regexp.MustCompile(
	`(?i)\b(ratif(?:y|ied|ication)|approv(?:e|ed|al)|sign[- ]off|go/no[- ]go)\b|비준|승인|재가`,
)

func subjectIsRatifyClass(subject string) bool {
	return ratifyKeywordRegex.MatchString(subject)
}

// mayorCCRegex matches addresses that route to the Mayor role:
//
//	mayor                 (bare role)
//	mayor/                (role with trailing slash, gt convention)
//	hq-mayor              (town-scope bead/session prefix)
//	<rig>/mayor           (rig-scoped — a few rigs have a per-rig mayor session)
//	<rig>/mayor/<name>    (rig-scoped with sub-name)
//
// Does NOT match addresses that contain "mayor" as a substring of an unrelated
// name (e.g., a hypothetical crew called "mayor-watcher" — leading/trailing
// boundary required).
var mayorCCRegex = regexp.MustCompile(
	`(?i)^(mayor/?|hq-mayor|[a-z0-9_-]+/mayor(/[a-z0-9_-]+)?)$`,
)

func mayorCCMatches(cc []string) []string {
	var matches []string
	for _, addr := range cc {
		trimmed := strings.TrimSpace(addr)
		if mayorCCRegex.MatchString(trimmed) {
			matches = append(matches, trimmed)
		}
	}
	return matches
}

// approvalRoutingWaiverRegex matches a waiver declaration anywhere in the body.
// Multiple wordings accepted to reduce friction: "Approval Routing waiver:",
// "mayor cc waiver:", and "approval-routing-waiver:" all valid.
var approvalRoutingWaiverRegex = regexp.MustCompile(
	`(?i)\b(approval[- ]routing[- ]waiver|mayor[- ]cc[- ]waiver)\s*:`,
)

func bodyHasApprovalRoutingWaiver(body string) bool {
	return approvalRoutingWaiverRegex.MatchString(body)
}
