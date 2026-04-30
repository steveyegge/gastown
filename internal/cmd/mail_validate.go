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

// validateRecipientAddressFormat enforces ka-n85 (P1, 2026-04-27): syntactic
// pre-flight check on a typed-by-hand mail address. Catches malformations
// that the downstream Resolver would also reject — but surfaces the failure
// loudly at the caller's syscall, so a script-level batch invocation cannot
// silently lose deliveries (Munger's hq-wisp-... broadcast: 12 sends, 2
// silent fails on `karuna/fe_crew` + `karuna/ad` were the trigger evidence).
//
// Scope is SYNTACTIC ONLY:
//   - allowed chars: lowercase a-z, digits 0-9, hyphen `-`, underscore `_`
//     (canonical for some crews — e.g., `karuna/backend_auth`), slash `/`,
//     colon `:` (for channel:/queue:/group: prefixes), dot `.` (for hq-*
//     and occasional rig names), and `@` (for `@town` etc.)
//   - no whitespace
//   - segment count after prefix-strip: 1-3
//   - each segment non-empty (no `karuna//ad` or trailing slash with
//     additional content)
//
// Semantically-valid-but-unknown addresses (e.g., `karuna/ad` where the
// rig directory exists but no crew named `ad` is registered) are NOT caught
// here — those are the Resolver's job. validateRecipientAddressFormat is
// the first line of defense; the Resolver is the second. The two layers
// together prevent both the typo case (caught by Resolver) and the
// shape-malformation case (caught here).
//
// 2026-04-28 hotfix: original implementation excluded underscore as a
// "common typo" hint, but Munger's ratified spec allows underscore (and
// `karuna/backend_auth` is a canonical crew). Validator stays purely
// syntactic; Resolver handles canonical-name vs typo discrimination.
//
// Returns nil for empty input — empty `to` is caught upstream by mail_send.go's
// "address required" check and shouldn't reach this function.
func validateRecipientAddressFormat(addr string) error {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return nil
	}

	// Special prefixes for routing types — strip the prefix before structural
	// checks. The resolver still semantically validates the rest.
	for _, prefix := range []string{"group:", "queue:", "channel:", "list:", "announce:"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			break
		}
	}

	// `@town`, `@crew`, `@rig/X`, `@role/X` — strip the leading `@` for the
	// per-segment char check; the leading `@` itself is intentional.
	if strings.HasPrefix(trimmed, "@") {
		trimmed = strings.TrimPrefix(trimmed, "@")
	}

	if trimmed == "" {
		return fmt.Errorf("address %q has no body after prefix strip", addr)
	}

	// Char-set check. The set is intentionally narrow: gas town conventions
	// use lowercase + hyphen segment names. Underscores, uppercase, and
	// whitespace are common typos and should fail loud.
	if !addressCharsRegex.MatchString(trimmed) {
		return fmt.Errorf(
			"address %q contains invalid characters\n\n"+
				"  expected: lowercase a-z, 0-9, hyphen `-`, underscore `_`, slash `/`, dot `.`\n"+
				"  common mistakes:\n"+
				"    - uppercase (addresses are lowercase: `Mayor` → `mayor`)\n"+
				"    - whitespace (no spaces in addresses)\n\n"+
				"  reference: ~/gt/CLAUDE.md \"Mail addressing\" + `gt mail send --help`",
			addr)
	}

	// Trailing-slash form is allowed only on bare role addresses (e.g.,
	// `mayor/`, `witness/`). Strip for segment counting.
	stripped := strings.TrimSuffix(trimmed, "/")
	if stripped == "" {
		return fmt.Errorf("address %q is just a slash", addr)
	}

	// Segment count check. After prefix and trailing-slash stripping, allow
	// 1-3 segments: `mayor`, `karuna/witness`, `karuna/crew/munger`.
	segments := strings.Split(stripped, "/")
	if len(segments) > 3 {
		return fmt.Errorf("address %q has %d segments; gas town addresses use 1-3 segments (e.g., `mayor`, `karuna/witness`, `karuna/crew/munger`)", addr, len(segments))
	}
	for i, s := range segments {
		if s == "" {
			return fmt.Errorf("address %q has empty segment at position %d (consecutive slashes or trailing-content slash)", addr, i+1)
		}
	}

	return nil
}

// addressCharsRegex matches the allowed character set for a normalized
// gas town address (after prefix and `@` stripping). See
// validateRecipientAddressFormat docstring for rationale.
var addressCharsRegex = regexp.MustCompile(`^[a-z0-9._/-]+$`)

// validateRecipientAddresses runs validateRecipientAddressFormat against the
// `to` field and every entry in `cc`, aggregating ALL failures into a single
// error so the caller fixes them in one round-trip rather than discovering
// bad addresses one at a time.
//
// Returns nil on success; a single multi-line error listing every malformed
// address on failure.
func validateRecipientAddresses(to string, cc []string) error {
	var errs []string
	if err := validateRecipientAddressFormat(to); err != nil {
		errs = append(errs, "  to: "+err.Error())
	}
	for _, c := range cc {
		if err := validateRecipientAddressFormat(c); err != nil {
			errs = append(errs, "  cc: "+err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("recipient address(es) malformed (ka-n85 syntactic pre-flight):\n\n%s", strings.Join(errs, "\n\n"))
}
