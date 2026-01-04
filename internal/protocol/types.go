// Package protocol provides inter-agent protocol message handling.
//
// This package defines protocol message types for Witness-Refinery communication
// and provides handlers for processing these messages.
//
// Protocol Message Types:
//   - MERGE_READY: Witness → Refinery (branch ready for merge)
//   - MERGED: Refinery → Witness (merge succeeded, cleanup ok)
//   - MERGE_FAILED: Refinery → Witness (merge failed, needs rework)
//   - REWORK_REQUEST: Refinery → Witness (rebase needed)
package protocol

import (
	"strings"
	"time"
)

// MessageType identifies the protocol message type.
type MessageType string

const (
	// TypeMergeReady is sent from Witness to Refinery when a polecat's work
	// is verified and ready for merge queue processing.
	// Subject format: "MERGE_READY <polecat-name>"
	TypeMergeReady MessageType = "MERGE_READY"

	// TypeMerged is sent from Refinery to Witness when a branch has been
	// successfully merged to the target branch.
	// Subject format: "MERGED <polecat-name>"
	TypeMerged MessageType = "MERGED"

	// TypeMergeFailed is sent from Refinery to Witness when a merge attempt
	// failed (tests, build, or other non-conflict error).
	// Subject format: "MERGE_FAILED <polecat-name>"
	TypeMergeFailed MessageType = "MERGE_FAILED"

	// TypeReworkRequest is sent from Refinery to Witness when a polecat's
	// branch needs rebasing due to conflicts with the target branch.
	// Subject format: "REWORK_REQUEST <polecat-name>"
	TypeReworkRequest MessageType = "REWORK_REQUEST"

	// TypeSemanticConflictEscalated is sent from Refinery to Mayor when
	// semantic conflicts are detected that require decision-making.
	// Subject format: "SEMANTIC_CONFLICT_ESCALATED <mr-id>"
	TypeSemanticConflictEscalated MessageType = "SEMANTIC_CONFLICT_ESCALATED"

	// TypeSemanticConflictResolved is sent from Mayor to Witness/Refinery
	// after a semantic conflict has been resolved with a decision.
	// Subject format: "SEMANTIC_CONFLICT_RESOLVED <mr-id>"
	TypeSemanticConflictResolved MessageType = "SEMANTIC_CONFLICT_RESOLVED"
)

// ParseMessageType extracts the protocol message type from a mail subject.
// Returns empty string if subject doesn't match a known protocol type.
func ParseMessageType(subject string) MessageType {
	subject = strings.TrimSpace(subject)

	// Check each known prefix
	prefixes := []MessageType{
		TypeMergeReady,
		TypeMerged,
		TypeMergeFailed,
		TypeReworkRequest,
		TypeSemanticConflictEscalated,
		TypeSemanticConflictResolved,
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(subject, string(prefix)) {
			return prefix
		}
	}

	return ""
}

// MergeReadyPayload contains the data for a MERGE_READY message.
// Sent by Witness after verifying polecat work is complete.
type MergeReadyPayload struct {
	// Branch is the polecat's work branch (e.g., "polecat/Toast/gt-abc").
	Branch string `json:"branch"`

	// Issue is the beads issue ID the polecat completed.
	Issue string `json:"issue"`

	// Polecat is the worker name.
	Polecat string `json:"polecat"`

	// Rig is the rig name containing the polecat.
	Rig string `json:"rig"`

	// Verified contains verification notes.
	Verified string `json:"verified,omitempty"`

	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
}

// MergedPayload contains the data for a MERGED message.
// Sent by Refinery after successful merge to target branch.
type MergedPayload struct {
	// Branch is the source branch that was merged.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Polecat is the worker name.
	Polecat string `json:"polecat"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// MergedAt is when the merge completed.
	MergedAt time.Time `json:"merged_at"`

	// MergeCommit is the SHA of the merge commit.
	MergeCommit string `json:"merge_commit,omitempty"`

	// TargetBranch is the branch merged into (e.g., "main").
	TargetBranch string `json:"target_branch"`
}

// MergeFailedPayload contains the data for a MERGE_FAILED message.
// Sent by Refinery when merge fails due to tests, build, or other errors.
type MergeFailedPayload struct {
	// Branch is the source branch that failed to merge.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Polecat is the worker name.
	Polecat string `json:"polecat"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// FailedAt is when the failure occurred.
	FailedAt time.Time `json:"failed_at"`

	// FailureType categorizes the failure (tests, build, push, etc.).
	FailureType string `json:"failure_type"`

	// Error is the error message.
	Error string `json:"error"`

	// TargetBranch is the branch we tried to merge into.
	TargetBranch string `json:"target_branch"`
}

// ReworkRequestPayload contains the data for a REWORK_REQUEST message.
// Sent by Refinery when a polecat's branch has conflicts requiring rebase.
type ReworkRequestPayload struct {
	// Branch is the source branch that needs rebasing.
	Branch string `json:"branch"`

	// Issue is the beads issue ID.
	Issue string `json:"issue"`

	// Polecat is the worker name.
	Polecat string `json:"polecat"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// RequestedAt is when the rework was requested.
	RequestedAt time.Time `json:"requested_at"`

	// TargetBranch is the branch to rebase onto.
	TargetBranch string `json:"target_branch"`

	// ConflictFiles lists files with conflicts (if known).
	ConflictFiles []string `json:"conflict_files,omitempty"`

	// Instructions provides specific rebase instructions.
	Instructions string `json:"instructions,omitempty"`
}

// SemanticConflictEscalatedPayload contains the data for a SEMANTIC_CONFLICT_ESCALATED message.
// Sent by Refinery when semantic conflicts are detected that require Mayor decision.
type SemanticConflictEscalatedPayload struct {
	// MRID is the merge request ID containing the conflicts.
	MRID string `json:"mr_id"`

	// Branch is the source branch with conflicts.
	Branch string `json:"branch"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// EscalatedAt is when the escalation occurred.
	EscalatedAt time.Time `json:"escalated_at"`

	// Conflicts lists all detected semantic conflicts.
	Conflicts []SemanticConflictData `json:"conflicts"`

	// MailID is the ID of the escalation mail sent to Mayor.
	MailID string `json:"mail_id"`
}

// SemanticConflictData represents a single semantic conflict.
type SemanticConflictData struct {
	// BeadID is the bead with conflicting changes.
	BeadID string `json:"bead_id"`

	// Field is the field with conflicting values.
	Field string `json:"field"`

	// Changes lists all conflicting changes to this field.
	Changes []BeadFieldChangeData `json:"changes"`
}

// BeadFieldChangeData represents a modification to a bead field.
type BeadFieldChangeData struct {
	// Polecat is the agent that made this change.
	Polecat string `json:"polecat"`

	// OldValue is the previous value.
	OldValue string `json:"old_value"`

	// NewValue is the new value.
	NewValue string `json:"new_value"`

	// CommitSHA is the git commit that made this change.
	CommitSHA string `json:"commit_sha"`

	// Timestamp is when the change was made.
	Timestamp time.Time `json:"timestamp"`

	// Confidence is the confidence score (0.0-1.0, optional).
	Confidence float64 `json:"confidence,omitempty"`

	// Reasoning explains why this change was made (optional).
	Reasoning string `json:"reasoning,omitempty"`
}

// SemanticConflictResolvedPayload contains the data for a SEMANTIC_CONFLICT_RESOLVED message.
// Sent by Mayor after deciding how to resolve semantic conflicts.
type SemanticConflictResolvedPayload struct {
	// MRID is the merge request ID.
	MRID string `json:"mr_id"`

	// Rig is the rig name.
	Rig string `json:"rig"`

	// ResolvedAt is when the resolution was made.
	ResolvedAt time.Time `json:"resolved_at"`

	// Resolutions maps "beadID:field" to the resolved value.
	Resolutions map[string]string `json:"resolutions"`

	// DecisionReasoning explains the Mayor's decision.
	DecisionReasoning string `json:"decision_reasoning"`

	// DecisionMaker identifies who made the decision (mayor or human).
	DecisionMaker string `json:"decision_maker"`
}

// IsProtocolMessage returns true if the subject matches a known protocol type.
func IsProtocolMessage(subject string) bool {
	return ParseMessageType(subject) != ""
}

// ExtractPolecat extracts the polecat name from a protocol message subject.
// Subject format: "TYPE <polecat-name>"
func ExtractPolecat(subject string) string {
	subject = strings.TrimSpace(subject)
	parts := strings.SplitN(subject, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
