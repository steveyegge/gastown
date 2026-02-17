package mail

import (
	"strings"
	"time"
)

const (
	// DeliveryStatePending indicates a message has been durably written but not
	// yet acknowledged by a worker/recipient.
	DeliveryStatePending = "pending"
	// DeliveryStateAcked indicates receipt has been acknowledged.
	DeliveryStateAcked = "acked"

	// Label keys used for two-phase delivery tracking.
	DeliveryLabelPending       = "delivery:pending"
	DeliveryLabelAcked         = "delivery:acked"
	DeliveryLabelAckedByPrefix = "delivery-acked-by:"
	DeliveryLabelAckedAtPrefix = "delivery-acked-at:"
)

// DeliverySendLabels returns labels written during phase-1 (send).
func DeliverySendLabels() []string {
	return []string{DeliveryLabelPending}
}

// DeliveryAckLabelSequence returns labels for phase-2 (ack). The ordering is
// intentional for crash safety: state remains pending until the final ack label
// write succeeds.
func DeliveryAckLabelSequence(recipientIdentity string, at time.Time) []string {
	ackedAt := at.UTC().Format(time.RFC3339)
	return []string{
		DeliveryLabelAckedByPrefix + recipientIdentity,
		DeliveryLabelAckedAtPrefix + ackedAt,
		DeliveryLabelAcked,
	}
}

// ParseDeliveryLabels derives delivery state and ack metadata from labels.
// The state is append-only:
// - `delivery:pending` means pending
// - once `delivery:acked` appears, state is acked (even if pending remains)
func ParseDeliveryLabels(labels []string) (state, ackedBy string, ackedAt *time.Time) {
	hasPending := false
	hasAcked := false

	for _, label := range labels {
		switch {
		case label == DeliveryLabelPending:
			hasPending = true
		case label == DeliveryLabelAcked:
			hasAcked = true
		case strings.HasPrefix(label, DeliveryLabelAckedByPrefix):
			ackedBy = strings.TrimPrefix(label, DeliveryLabelAckedByPrefix)
		case strings.HasPrefix(label, DeliveryLabelAckedAtPrefix):
			ts := strings.TrimPrefix(label, DeliveryLabelAckedAtPrefix)
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				ackedAt = &t
			}
		}
	}

	if hasAcked {
		return DeliveryStateAcked, ackedBy, ackedAt
	}
	if hasPending {
		return DeliveryStatePending, "", nil
	}
	return "", "", nil
}
