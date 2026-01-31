// Package witness provides the polecat monitoring agent.
package witness

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Protocol message patterns for Witness inbox routing.
var (
	// POLECAT_DONE <name> - polecat signaling work completion
	PatternPolecatDone = regexp.MustCompile(`^POLECAT_DONE\s+(\S+)`)

	// LIFECYCLE:Shutdown <name> - daemon-triggered polecat shutdown
	PatternLifecycleShutdown = regexp.MustCompile(`^LIFECYCLE:Shutdown\s+(\S+)`)

	// HELP: <topic> - polecat requesting intervention
	PatternHelp = regexp.MustCompile(`^HELP:\s+(.+)`)

	// MERGED <name> - refinery confirms branch merged
	PatternMerged = regexp.MustCompile(`^MERGED\s+(\S+)`)

	// MERGE_FAILED <name> - refinery reporting merge failure
	PatternMergeFailed = regexp.MustCompile(`^MERGE_FAILED\s+(\S+)`)

	// HANDOFF - session continuity message
	PatternHandoff = regexp.MustCompile(`^🤝\s*HANDOFF`)

	// SWARM_START - mayor initiating batch work
	PatternSwarmStart = regexp.MustCompile(`^SWARM_START`)

	// RATE_LIMITED <source> - rate limit detected
	PatternRateLimited = regexp.MustCompile(`^RATE_LIMITED\s+(\S+)`)
)

// ProtocolType identifies the type of protocol message.
type ProtocolType string

const (
	ProtoPolecatDone       ProtocolType = "polecat_done"
	ProtoLifecycleShutdown ProtocolType = "lifecycle_shutdown"
	ProtoHelp              ProtocolType = "help"
	ProtoMerged            ProtocolType = "merged"
	ProtoMergeFailed       ProtocolType = "merge_failed"
	ProtoHandoff           ProtocolType = "handoff"
	ProtoSwarmStart        ProtocolType = "swarm_start"
	ProtoRateLimited       ProtocolType = "rate_limited"
	ProtoUnknown           ProtocolType = "unknown"
)

// PolecatDonePayload contains parsed data from a POLECAT_DONE message.
type PolecatDonePayload struct {
	PolecatName string
	Exit        string // COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE
	IssueID     string
	MRID        string
	Branch      string
	Gate        string // Gate ID when Exit is PHASE_COMPLETE
}

// HelpPayload contains parsed data from a HELP message.
type HelpPayload struct {
	Topic       string
	Agent       string
	IssueID     string
	Problem     string
	Tried       string
	RequestedAt time.Time
}

// MergedPayload contains parsed data from a MERGED message.
type MergedPayload struct {
	PolecatName string
	Branch      string
	IssueID     string
	MergedAt    time.Time
}

// MergeFailedPayload contains parsed data from a MERGE_FAILED message.
type MergeFailedPayload struct {
	PolecatName string
	Branch      string
	IssueID     string
	FailureType string // "build", "test", "lint", etc.
	Error       string
	FailedAt    time.Time
}

// SwarmStartPayload contains parsed data from a SWARM_START message.
type SwarmStartPayload struct {
	SwarmID   string
	BeadIDs   []string
	Total     int
	StartedAt time.Time
}

// RateLimitedPayload contains parsed data from a RATE_LIMITED message.
type RateLimitedPayload struct {
	Source          string        // Where rate limit was detected (e.g., "polecat:nux", "spawn")
	Account         string        // Account that hit the limit (if known)
	BackoffDuration time.Duration // Recommended backoff duration
	ConsecutiveHits int           // Number of consecutive rate limit hits
	DetectedAt      time.Time     // When the rate limit was detected
}

// ClassifyMessage determines the protocol type from a message subject.
func ClassifyMessage(subject string) ProtocolType {
	switch {
	case PatternPolecatDone.MatchString(subject):
		return ProtoPolecatDone
	case PatternLifecycleShutdown.MatchString(subject):
		return ProtoLifecycleShutdown
	case PatternHelp.MatchString(subject):
		return ProtoHelp
	case PatternMerged.MatchString(subject):
		return ProtoMerged
	case PatternMergeFailed.MatchString(subject):
		return ProtoMergeFailed
	case PatternHandoff.MatchString(subject):
		return ProtoHandoff
	case PatternSwarmStart.MatchString(subject):
		return ProtoSwarmStart
	case PatternRateLimited.MatchString(subject):
		return ProtoRateLimited
	default:
		return ProtoUnknown
	}
}

// ParsePolecatDone extracts payload from a POLECAT_DONE message.
// Subject format: POLECAT_DONE <polecat-name>
// Body format:
//
//	Exit: COMPLETED|ESCALATED|DEFERRED|PHASE_COMPLETE
//	Issue: <issue-id>
//	MR: <mr-id>
//	Gate: <gate-id>
//	Branch: <branch>
func ParsePolecatDone(subject, body string) (*PolecatDonePayload, error) {
	matches := PatternPolecatDone.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid POLECAT_DONE subject: %s", subject)
	}

	payload := &PolecatDonePayload{
		PolecatName: matches[1],
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Exit:") {
			payload.Exit = strings.TrimSpace(strings.TrimPrefix(line, "Exit:"))
		} else if strings.HasPrefix(line, "Issue:") {
			payload.IssueID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		} else if strings.HasPrefix(line, "MR:") {
			payload.MRID = strings.TrimSpace(strings.TrimPrefix(line, "MR:"))
		} else if strings.HasPrefix(line, "Gate:") {
			payload.Gate = strings.TrimSpace(strings.TrimPrefix(line, "Gate:"))
		} else if strings.HasPrefix(line, "Branch:") {
			payload.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		}
	}

	return payload, nil
}

// ParseHelp extracts payload from a HELP message.
// Subject format: HELP: <topic>
// Body format:
//
//	Agent: <agent-id>
//	Issue: <issue-id>
//	Problem: <description>
//	Tried: <what was attempted>
func ParseHelp(subject, body string) (*HelpPayload, error) {
	matches := PatternHelp.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid HELP subject: %s", subject)
	}

	payload := &HelpPayload{
		Topic:       matches[1],
		RequestedAt: time.Now(),
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Agent:") {
			payload.Agent = strings.TrimSpace(strings.TrimPrefix(line, "Agent:"))
		} else if strings.HasPrefix(line, "Issue:") {
			payload.IssueID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		} else if strings.HasPrefix(line, "Problem:") {
			payload.Problem = strings.TrimSpace(strings.TrimPrefix(line, "Problem:"))
		} else if strings.HasPrefix(line, "Tried:") {
			payload.Tried = strings.TrimSpace(strings.TrimPrefix(line, "Tried:"))
		}
	}

	return payload, nil
}

// ParseMerged extracts payload from a MERGED message.
// Subject format: MERGED <polecat-name>
// Body format:
//
//	Branch: <branch>
//	Issue: <issue-id>
//	Merged-At: <timestamp>
func ParseMerged(subject, body string) (*MergedPayload, error) {
	matches := PatternMerged.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid MERGED subject: %s", subject)
	}

	payload := &MergedPayload{
		PolecatName: matches[1],
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Branch:") {
			payload.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		} else if strings.HasPrefix(line, "Issue:") {
			payload.IssueID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		} else if strings.HasPrefix(line, "Merged-At:") {
			ts := strings.TrimSpace(strings.TrimPrefix(line, "Merged-At:"))
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				payload.MergedAt = t
			}
		}
	}

	return payload, nil
}

// ParseMergeFailed extracts payload from a MERGE_FAILED message.
// Subject format: MERGE_FAILED <polecat-name>
// Body format:
//
//	Branch: <branch>
//	Issue: <issue-id>
//	FailureType: <type>
//	Error: <error-message>
func ParseMergeFailed(subject, body string) (*MergeFailedPayload, error) {
	matches := PatternMergeFailed.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid MERGE_FAILED subject: %s", subject)
	}

	payload := &MergeFailedPayload{
		PolecatName: matches[1],
		FailedAt:    time.Now(),
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Branch:"):
			payload.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		case strings.HasPrefix(line, "Issue:"):
			payload.IssueID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		case strings.HasPrefix(line, "FailureType:"):
			payload.FailureType = strings.TrimSpace(strings.TrimPrefix(line, "FailureType:"))
		case strings.HasPrefix(line, "Error:"):
			payload.Error = strings.TrimSpace(strings.TrimPrefix(line, "Error:"))
		}
	}

	return payload, nil
}

// ParseSwarmStart extracts payload from a SWARM_START message.
// Body format is JSON: {"swarm_id": "batch-123", "beads": ["bd-a", "bd-b"]}
func ParseSwarmStart(body string) (*SwarmStartPayload, error) {
	payload := &SwarmStartPayload{
		StartedAt: time.Now(),
	}

	// Parse the JSON-like body (simplified parsing for key-value extraction)
	// Full JSON parsing would require encoding/json import
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SwarmID:") || strings.HasPrefix(line, "swarm_id:") {
			payload.SwarmID = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "SwarmID:"), "swarm_id:"))
		} else if strings.HasPrefix(line, "Total:") {
			_, _ = fmt.Sscanf(line, "Total: %d", &payload.Total)
		}
	}

	return payload, nil
}

// ParseRateLimited extracts payload from a RATE_LIMITED message.
// Subject format: RATE_LIMITED <source>
// Body format:
//
//	Account: <account>
//	BackoffSeconds: <duration>
//	ConsecutiveHits: <count>
//	DetectedAt: <timestamp>
func ParseRateLimited(subject, body string) (*RateLimitedPayload, error) {
	matches := PatternRateLimited.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid RATE_LIMITED subject: %s", subject)
	}

	payload := &RateLimitedPayload{
		Source:     matches[1],
		DetectedAt: time.Now(),
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Account:"):
			payload.Account = strings.TrimSpace(strings.TrimPrefix(line, "Account:"))
		case strings.HasPrefix(line, "BackoffSeconds:"):
			var seconds int
			if _, err := fmt.Sscanf(line, "BackoffSeconds: %d", &seconds); err == nil {
				payload.BackoffDuration = time.Duration(seconds) * time.Second
			}
		case strings.HasPrefix(line, "ConsecutiveHits:"):
			_, _ = fmt.Sscanf(line, "ConsecutiveHits: %d", &payload.ConsecutiveHits)
		case strings.HasPrefix(line, "DetectedAt:"):
			ts := strings.TrimSpace(strings.TrimPrefix(line, "DetectedAt:"))
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				payload.DetectedAt = t
			}
		}
	}

	return payload, nil
}

// CleanupWispLabels generates labels for a cleanup wisp.
func CleanupWispLabels(polecatName, state string) []string {
	return []string{
		"cleanup",
		fmt.Sprintf("polecat:%s", polecatName),
		fmt.Sprintf("state:%s", state),
	}
}

// SwarmWispLabels generates labels for a swarm tracking wisp.
func SwarmWispLabels(swarmID string, total, completed int, startTime time.Time) []string {
	return []string{
		"swarm",
		fmt.Sprintf("swarm_id:%s", swarmID),
		fmt.Sprintf("total:%d", total),
		fmt.Sprintf("completed:%d", completed),
		fmt.Sprintf("start:%s", startTime.Format(time.RFC3339)),
	}
}

// HelpAssessment represents the Witness's assessment of a help request.
type HelpAssessment struct {
	CanHelp     bool
	HelpAction  string // What the Witness can do to help
	NeedsEscalation bool
	EscalationReason string
}

// AssessHelpRequest provides guidance for the Witness to assess a help request.
// This is a template/guide - actual assessment is done by the Claude agent.
func AssessHelpRequest(payload *HelpPayload) *HelpAssessment {
	assessment := &HelpAssessment{}

	// Heuristics for common help requests that Witness can handle
	topic := strings.ToLower(payload.Topic)
	problem := strings.ToLower(payload.Problem)

	// Git issues - Witness can often help
	if strings.Contains(topic, "git") || strings.Contains(problem, "git") {
		if strings.Contains(problem, "conflict") {
			assessment.CanHelp = false
			assessment.NeedsEscalation = true
			assessment.EscalationReason = "Git conflicts require human review"
		} else if strings.Contains(problem, "push") || strings.Contains(problem, "fetch") {
			assessment.CanHelp = true
			assessment.HelpAction = "Check git remote status and network connectivity"
		}
	}

	// Test failures - usually need escalation
	if strings.Contains(topic, "test") || strings.Contains(problem, "test fail") {
		assessment.CanHelp = false
		assessment.NeedsEscalation = true
		assessment.EscalationReason = "Test failures require investigation"
	}

	// Build issues - Witness can check basics
	if strings.Contains(topic, "build") || strings.Contains(problem, "compile") {
		assessment.CanHelp = true
		assessment.HelpAction = "Verify dependencies and build configuration"
	}

	// Requirements unclear - always escalate
	if strings.Contains(topic, "unclear") || strings.Contains(problem, "requirement") ||
		strings.Contains(problem, "don't understand") {
		assessment.CanHelp = false
		assessment.NeedsEscalation = true
		assessment.EscalationReason = "Requirements clarification needed from Mayor"
	}

	// Default: escalate if we don't recognize the pattern
	if !assessment.CanHelp && !assessment.NeedsEscalation {
		assessment.NeedsEscalation = true
		assessment.EscalationReason = "Unknown help request type"
	}

	return assessment
}
