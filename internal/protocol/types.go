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

	// TypeConvoyNeedsFeeding is sent from Refinery to Deacon after a
	// convoy-eligible merge completes. This triggers immediate convoy
	// feeding instead of waiting for the next deacon patrol cycle.
	// Subject format: "CONVOY_NEEDS_FEEDING <convoy-id>"
	TypeConvoyNeedsFeeding MessageType = "CONVOY_NEEDS_FEEDING"
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
		TypeConvoyNeedsFeeding,
	}

	for _, prefix := range prefixes {
		p := string(prefix)
		if subject == p || strings.HasPrefix(subject, p+" ") {
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

// PolecatDonePayload contains the data from a POLECAT_DONE notification.
// This is not a formal protocol message (it's a mail convention), but the
// payload is structured for programmatic parsing by witness handlers.
type PolecatDonePayload struct {
	// Polecat is the worker name.
	Polecat string `json:"polecat"`

	// ExitType is the exit status (COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE).
	ExitType string `json:"exit_type"`

	// Issue is the beads issue ID the polecat worked on.
	Issue string `json:"issue,omitempty"`

	// Branch is the polecat's work branch.
	Branch string `json:"branch"`

	// MR is the merge-request bead ID (empty for owned+direct convoys).
	MR string `json:"mr,omitempty"`

	// ConvoyID is the tracking convoy ID (if any).
	ConvoyID string `json:"convoy_id,omitempty"`

	// ConvoyOwned indicates the convoy has caller-managed lifecycle.
	ConvoyOwned bool `json:"convoy_owned,omitempty"`

	// MergeStrategy is the convoy's merge strategy (direct, mr, local).
	MergeStrategy string `json:"merge_strategy,omitempty"`

	// Errors contains any non-fatal errors encountered during gt done.
	Errors string `json:"errors,omitempty"`
}

// SkipMergeFlow returns true if this polecat's work should bypass the
// standard witness/refinery merge pipeline (owned convoy + direct merge).
func (p *PolecatDonePayload) SkipMergeFlow() bool {
	return p.ConvoyOwned && p.MergeStrategy == "direct"
}

// ConvoyNeedsFeedingPayload contains the data for a CONVOY_NEEDS_FEEDING message.
// Sent by Refinery to Deacon after a convoy-eligible merge completes.
type ConvoyNeedsFeedingPayload struct {
	// ConvoyID is the convoy that may have newly-ready issues.
	ConvoyID string `json:"convoy_id"`

	// SourceIssue is the issue that was just merged and closed.
	SourceIssue string `json:"source_issue"`

	// Rig is the rig where the merge happened.
	Rig string `json:"rig"`

	// MergedAt is when the merge completed.
	MergedAt time.Time `json:"merged_at"`
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
