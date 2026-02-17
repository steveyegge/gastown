package mail

import (
	"reflect"
	"testing"
	"time"
)

func TestDeliveryAckLabelSequenceOrder(t *testing.T) {
	at := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	got := DeliveryAckLabelSequence("gastown/worker", at)
	want := []string{
		"delivery-acked-by:gastown/worker",
		"delivery-acked-at:2026-02-17T12:00:00Z",
		"delivery:acked",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeliveryAckLabelSequence() = %v, want %v", got, want)
	}
}

func TestParseDeliveryLabels_CrashAndRetryStates(t *testing.T) {
	t.Run("pending only", func(t *testing.T) {
		state, by, at := ParseDeliveryLabels([]string{
			DeliveryLabelPending,
		})
		if state != DeliveryStatePending {
			t.Fatalf("state = %q, want %q", state, DeliveryStatePending)
		}
		if by != "" || at != nil {
			t.Fatalf("pending state should not include ack metadata, got by=%q at=%v", by, at)
		}
	})

	t.Run("partial ack write keeps pending", func(t *testing.T) {
		state, by, at := ParseDeliveryLabels([]string{
			DeliveryLabelPending,
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T12:00:00Z",
		})
		if state != DeliveryStatePending {
			t.Fatalf("state = %q, want %q", state, DeliveryStatePending)
		}
		if by != "" || at != nil {
			t.Fatalf("partial ack should not flip state, got by=%q at=%v", by, at)
		}
	})

	t.Run("acked label flips state", func(t *testing.T) {
		state, by, at := ParseDeliveryLabels([]string{
			DeliveryLabelPending,
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T12:00:00Z",
			DeliveryLabelAcked,
		})
		if state != DeliveryStateAcked {
			t.Fatalf("state = %q, want %q", state, DeliveryStateAcked)
		}
		if by != "gastown/worker" {
			t.Fatalf("ackedBy = %q, want %q", by, "gastown/worker")
		}
		if at == nil {
			t.Fatal("ackedAt should be populated for acked state")
		}
	})
}
