// Package witness provides the polecat monitoring agent.
package witness

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
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

	// MERGE_READY <polecat-name> - witness notifying refinery that work is ready
	PatternMergeReady = regexp.MustCompile(`^MERGE_READY\s+(\S+)`)

	// HANDOFF - session continuity message
	PatternHandoff = regexp.MustCompile(`^🤝\s*HANDOFF`)

	// SWARM_START - mayor initiating batch work
	PatternSwarmStart = regexp.MustCompile(`^SWARM_START`)
)

// ProtocolType identifies the type of protocol message.
type ProtocolType string

const (
	ProtoPolecatDone       ProtocolType = "polecat_done"
	ProtoLifecycleShutdown ProtocolType = "lifecycle_shutdown"
	ProtoHelp              ProtocolType = "help"
	ProtoMerged            ProtocolType = "merged"
	ProtoMergeFailed       ProtocolType = "merge_failed"
	ProtoMergeReady        ProtocolType = "merge_ready"
	ProtoHandoff           ProtocolType = "handoff"
	ProtoSwarmStart        ProtocolType = "swarm_start"
	ProtoUnknown           ProtocolType = "unknown"
)

// AgentState is an alias for beads.AgentState. Agent state constants are
// defined in the beads package (the canonical source) and re-exported here
// for backward compatibility. See beads/status.go and gt-4d7p.
type AgentState = beads.AgentState

const (
	AgentStateRunning   = beads.AgentStateRunning
	AgentStateIdle      = beads.AgentStateIdle
	AgentStateDone      = beads.AgentStateDone
	AgentStateStuck     = beads.AgentStateStuck
	AgentStateEscalated = beads.AgentStateEscalated
	AgentStateSpawning  = beads.AgentStateSpawning
	AgentStateWorking   = beads.AgentStateWorking
	AgentStateNuked     = beads.AgentStateNuked
)

// ExitType constants define the completion outcome for polecat work.
// These match the exit statuses used by `gt done` and are stored on the
// agent bead's exit_type field so the witness can discover completion
// outcomes from beads instead of POLECAT_DONE mail.
type ExitType string

const (
	ExitTypeCompleted     ExitType = "COMPLETED"
	ExitTypeEscalated     ExitType = "ESCALATED"
	ExitTypeDeferred      ExitType = "DEFERRED"
	ExitTypePhaseComplete ExitType = "PHASE_COMPLETE"
)

// PolecatDonePayload contains parsed data from a POLECAT_DONE message.
type PolecatDonePayload struct {
	PolecatName string
	Exit        string // COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE
	IssueID     string
	MRID        string
	Branch      string
	Gate        string // Gate ID when Exit is PHASE_COMPLETE
	MRFailed    bool   // True when MR bead creation was attempted but failed
}

// HelpCategory classifies the nature of a help request for routing.
type HelpCategory string

const (
	HelpCategoryDecision  HelpCategory = "decision"  // Multiple valid paths, need choice
	HelpCategoryHelp      HelpCategory = "help"       // Need guidance or expertise
	HelpCategoryBlocked   HelpCategory = "blocked"    // Waiting on unresolvable dependency
	HelpCategoryFailed    HelpCategory = "failed"     // Unexpected error, can't proceed
	HelpCategoryEmergency HelpCategory = "emergency"  // Security or data integrity issue
	HelpCategoryLifecycle HelpCategory = "lifecycle"   // Worker stuck or needs recycle
	HelpCategoryUnknown   HelpCategory = "help"        // Default to general help
)

// HelpSeverity indicates the assessed urgency of a help request.
type HelpSeverity string

const (
	HelpSeverityCritical HelpSeverity = "critical" // P0: immediate attention
	HelpSeverityHigh     HelpSeverity = "high"     // P1: urgent blocker
	HelpSeverityMedium   HelpSeverity = "medium"   // P2: standard help request
)

// HelpAssessment contains the assessed category, severity, and routing suggestion.
type HelpAssessment struct {
	Category   HelpCategory
	Severity   HelpSeverity
	SuggestTo  string // Suggested escalation target (e.g., "deacon", "mayor", "overseer")
	Rationale  string // Brief explanation of why this classification was chosen
}

// HelpPayload contains parsed data from a HELP message.
type HelpPayload struct {
	Topic       string
	Agent       string
	IssueID     string
	Problem     string
	Tried       string
	RequestedAt time.Time
	Assessment  *HelpAssessment // Populated by AssessHelp()
}

// MergedPayload contains parsed data from a MERGED message.
type MergedPayload struct {
	PolecatName string
	Branch      string
	IssueID     string
	MergedAt    time.Time
}

// MergeReadyPayload contains parsed data from a MERGE_READY message.
// This is sent by Witness to Refinery when a polecat completes work with a pending MR.
type MergeReadyPayload struct {
	PolecatName string
	Branch      string
	IssueID     string
	MRID        string
	ReadyAt     time.Time
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
	case PatternMergeReady.MatchString(subject):
		return ProtoMergeReady
	case PatternHandoff.MatchString(subject):
		return ProtoHandoff
	case PatternSwarmStart.MatchString(subject):
		return ProtoSwarmStart
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
		} else if strings.HasPrefix(line, "MRFailed:") {
			payload.MRFailed = strings.TrimSpace(strings.TrimPrefix(line, "MRFailed:")) == "true"
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

// ParseMergeReady extracts payload from a MERGE_READY message.
// Subject format: MERGE_READY <polecat-name>
// Body format:
//
//	Branch: <branch>
//	Issue: <issue-id>
//	MR: <mr-id>
//	Verified: clean git state
func ParseMergeReady(subject, body string) (*MergeReadyPayload, error) {
	matches := PatternMergeReady.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid MERGE_READY subject: %s", subject)
	}

	payload := &MergeReadyPayload{
		PolecatName: matches[1],
		ReadyAt:     time.Now(),
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Branch:"):
			payload.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		case strings.HasPrefix(line, "Issue:"):
			payload.IssueID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		case strings.HasPrefix(line, "MR:"):
			payload.MRID = strings.TrimSpace(strings.TrimPrefix(line, "MR:"))
		}
	}

	return payload, nil
}

// ParseSwarmStart extracts payload from a SWARM_START message.
// Subject format: SWARM_START
// Body format:
//
//	SwarmID: <swarm-id>
//	Beads: <bead-a>, <bead-b>, ...
//	Total: <count>
func ParseSwarmStart(body string) (*SwarmStartPayload, error) {
	payload := &SwarmStartPayload{
		StartedAt: time.Now(),
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SwarmID:") {
			payload.SwarmID = strings.TrimSpace(strings.TrimPrefix(line, "SwarmID:"))
		} else if strings.HasPrefix(line, "Beads:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "Beads:"))
			if raw != "" {
				for _, b := range strings.Split(raw, ",") {
					b = strings.TrimSpace(b)
					if b != "" {
						payload.BeadIDs = append(payload.BeadIDs, b)
					}
				}
			}
		} else if strings.HasPrefix(line, "Total:") {
			_, _ = fmt.Sscanf(line, "Total: %d", &payload.Total)
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

// FormatHelpSummary formats a parsed HelpPayload into a human-readable summary
// for the witness agent to triage. Includes assessment if available.
func FormatHelpSummary(payload *HelpPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "HELP REQUEST from %s", payload.Agent)
	if payload.IssueID != "" {
		fmt.Fprintf(&b, " (issue: %s)", payload.IssueID)
	}
	b.WriteString("\n")
	if payload.Assessment != nil {
		fmt.Fprintf(&b, "Assessment: [%s] severity=%s → suggest escalate to %s\n",
			payload.Assessment.Category, payload.Assessment.Severity, payload.Assessment.SuggestTo)
		if payload.Assessment.Rationale != "" {
			fmt.Fprintf(&b, "Rationale: %s\n", payload.Assessment.Rationale)
		}
	}
	if payload.Topic != "" {
		fmt.Fprintf(&b, "Topic: %s\n", payload.Topic)
	}
	if payload.Problem != "" {
		fmt.Fprintf(&b, "Problem: %s\n", payload.Problem)
	}
	if payload.Tried != "" {
		fmt.Fprintf(&b, "Tried: %s\n", payload.Tried)
	}
	if !payload.RequestedAt.IsZero() {
		fmt.Fprintf(&b, "Requested: %s\n", payload.RequestedAt.Format(time.RFC3339))
	}
	return b.String()
}

// helpKeywords maps keyword patterns to their category and severity.
// Checked in priority order — first match wins.
var helpKeywords = []struct {
	patterns []string
	category HelpCategory
	severity HelpSeverity
}{
	// Emergency: security, data corruption, system down
	{
		patterns: []string{"security", "vulnerability", "breach", "unauthorized", "credential", "exposed secret", "data corruption", "data loss", "system down"},
		category: HelpCategoryEmergency,
		severity: HelpSeverityCritical,
	},
	// Failed: errors, crashes, unexpected failures
	{
		patterns: []string{"crash", "panic", "fatal", "segfault", "oom", "out of memory", "disk full", "connection refused", "database error", "dolt", "server unreachable"},
		category: HelpCategoryFailed,
		severity: HelpSeverityHigh,
	},
	// Blocked: dependencies, waiting, merge conflicts
	{
		patterns: []string{"blocked", "waiting on", "depends on", "merge conflict", "conflict", "deadlock", "stuck", "cannot proceed", "can't proceed"},
		category: HelpCategoryBlocked,
		severity: HelpSeverityHigh,
	},
	// Decision: architecture, design choices, ambiguity
	{
		patterns: []string{"which approach", "decision", "ambiguous", "unclear", "multiple options", "design choice", "architecture", "how should", "which way"},
		category: HelpCategoryDecision,
		severity: HelpSeverityMedium,
	},
	// Lifecycle: worker state, session issues
	{
		patterns: []string{"session", "respawn", "restart", "zombie", "hung", "timeout", "idle", "no progress"},
		category: HelpCategoryLifecycle,
		severity: HelpSeverityMedium,
	},
}

// categoryRoutes maps categories to their default escalation target.
var categoryRoutes = map[HelpCategory]string{
	HelpCategoryEmergency: "overseer",
	HelpCategoryFailed:    "deacon",
	HelpCategoryBlocked:   "mayor",
	HelpCategoryDecision:  "deacon",
	HelpCategoryLifecycle: "witness",
	HelpCategoryHelp:      "deacon",
}

// AssessHelp classifies a help request's category and severity based on
// the topic and problem fields, using keyword matching against the
// escalation categories defined in the escalation protocol.
func AssessHelp(payload *HelpPayload) *HelpAssessment {
	combined := strings.ToLower(payload.Topic + " " + payload.Problem)

	for _, entry := range helpKeywords {
		for _, pattern := range entry.patterns {
			if strings.Contains(combined, pattern) {
				target := categoryRoutes[entry.category]
				return &HelpAssessment{
					Category:  entry.category,
					Severity:  entry.severity,
					SuggestTo: target,
					Rationale: fmt.Sprintf("matched keyword %q", pattern),
				}
			}
		}
	}

	// Default: general help, medium severity, route to deacon
	return &HelpAssessment{
		Category:  HelpCategoryHelp,
		Severity:  HelpSeverityMedium,
		SuggestTo: "deacon",
		Rationale: "no specific keywords matched, defaulting to general help",
	}
}
