package mail

import (
	"errors"
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

	t.Run("lexicographic label order still parses correctly", func(t *testing.T) {
		// bd show --json returns labels in lexicographic order.
		state, by, at := ParseDeliveryLabels([]string{
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery-acked-by:gastown/worker",
			"delivery:acked",
			"delivery:pending",
		})
		if state != DeliveryStateAcked {
			t.Fatalf("state = %q, want %q", state, DeliveryStateAcked)
		}
		if by != "gastown/worker" {
			t.Fatalf("ackedBy = %q, want %q", by, "gastown/worker")
		}
		if at == nil {
			t.Fatal("ackedAt should be populated for acked state with lex-ordered labels")
		}
	})
}

func TestDeliveryAckLabelSequenceIdempotent(t *testing.T) {
	t.Run("no existing labels uses new timestamp", func(t *testing.T) {
		at := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
		got := DeliveryAckLabelSequenceIdempotent("gastown/worker", at, nil)
		want := []string{
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T14:00:00Z",
			"delivery:acked",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("existing timestamp is reused on retry", func(t *testing.T) {
		existing := []string{
			"delivery:pending",
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T12:00:00Z",
		}
		// Use a different time — should be ignored in favor of existing.
		at := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
		got := DeliveryAckLabelSequenceIdempotent("gastown/worker", at, existing)
		want := []string{
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery:acked",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("lexicographic label order still reuses timestamp", func(t *testing.T) {
		// bd show --json returns labels in lexicographic order, so acked-at
		// appears before acked-by. The function must be order-independent.
		existing := []string{
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery-acked-by:gastown/worker",
			"delivery:acked",
			"delivery:pending",
		}
		at := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
		got := DeliveryAckLabelSequenceIdempotent("gastown/worker", at, existing)
		want := []string{
			"delivery-acked-by:gastown/worker",
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery:acked",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("different recipient gets fresh timestamp", func(t *testing.T) {
		existing := []string{
			"delivery:pending",
			"delivery-acked-by:gastown/workerA",
			"delivery-acked-at:2026-02-17T12:00:00Z",
		}
		// Different recipient — should NOT reuse workerA's timestamp.
		at := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
		got := DeliveryAckLabelSequenceIdempotent("gastown/workerB", at, existing)
		want := []string{
			"delivery-acked-by:gastown/workerB",
			"delivery-acked-at:2026-02-17T14:00:00Z",
			"delivery:acked",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("mixed labels after crash: B must not reuse A's timestamp", func(t *testing.T) {
		// Scenario: A acked fully, then B started acking but crashed after
		// writing acked-by:B (before acked-at). Labels accumulated:
		existing := []string{
			"delivery:pending",
			"delivery-acked-by:gastown/workerA",
			"delivery-acked-at:2026-02-17T12:00:00Z",
			"delivery:acked",
			"delivery-acked-by:gastown/workerB",
		}
		// B retries — must generate a fresh timestamp, not reuse A's t1.
		at := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
		got := DeliveryAckLabelSequenceIdempotent("gastown/workerB", at, existing)
		want := []string{
			"delivery-acked-by:gastown/workerB",
			"delivery-acked-at:2026-02-17T14:00:00Z",
			"delivery:acked",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestReadBeadLabelsWithFallback(t *testing.T) {
	original := readBeadLabelsFn
	defer func() { readBeadLabelsFn = original }()

	t.Run("falls back from routed rig to home beads on not found", func(t *testing.T) {
		var calls []string
		readBeadLabelsFn = func(_ string, beadsDir string, _ string) ([]string, error) {
			calls = append(calls, beadsDir)
			switch beadsDir {
			case "/tmp/rig/.beads":
				return nil, &bdError{Err: errors.New("missing"), Stderr: "no issue found"}
			case "/tmp/town/.beads":
				return []string{"delivery:pending"}, nil
			default:
				return nil, errors.New("unexpected beads dir")
			}
		}

		got, err := readBeadLabelsWithFallback("/tmp/town", "/tmp/rig/.beads", "/tmp/town/.beads", "gs-wisp-ext")
		if err != nil {
			t.Fatalf("readBeadLabelsWithFallback() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(got, []string{"delivery:pending"}) {
			t.Fatalf("readBeadLabelsWithFallback() = %v, want pending label", got)
		}
		if !reflect.DeepEqual(calls, []string{"/tmp/rig/.beads", "/tmp/town/.beads"}) {
			t.Fatalf("readBeadLabelsWithFallback() called %v, want primary then fallback", calls)
		}
	})

	t.Run("does not fall back on non not-found errors", func(t *testing.T) {
		var calls []string
		wantErr := errors.New("permission denied")
		readBeadLabelsFn = func(_ string, beadsDir string, _ string) ([]string, error) {
			calls = append(calls, beadsDir)
			return nil, wantErr
		}

		_, err := readBeadLabelsWithFallback("/tmp/town", "/tmp/rig/.beads", "/tmp/town/.beads", "gs-wisp-ext")
		if !errors.Is(err, wantErr) {
			t.Fatalf("readBeadLabelsWithFallback() error = %v, want %v", err, wantErr)
		}
		if !reflect.DeepEqual(calls, []string{"/tmp/rig/.beads"}) {
			t.Fatalf("readBeadLabelsWithFallback() called %v, want only primary", calls)
		}
	})
}

func TestAddDeliveryLabelWithFallback(t *testing.T) {
	original := writeDeliveryLabelFn
	defer func() { writeDeliveryLabelFn = original }()

	t.Run("falls back from routed rig to home beads on not found", func(t *testing.T) {
		var calls []string
		writeDeliveryLabelFn = func(_ string, beadsDir string, _ []string) error {
			calls = append(calls, beadsDir)
			if beadsDir == "/tmp/rig/.beads" {
				return &bdError{Err: errors.New("missing"), Stderr: "no issue found"}
			}
			return nil
		}

		err := addDeliveryLabelWithFallback("/tmp/town", "/tmp/rig/.beads", "/tmp/town/.beads", []string{"label", "add", "gs-wisp-ext", DeliveryLabelAcked})
		if err != nil {
			t.Fatalf("addDeliveryLabelWithFallback() error = %v, want nil", err)
		}
		if !reflect.DeepEqual(calls, []string{"/tmp/rig/.beads", "/tmp/town/.beads"}) {
			t.Fatalf("addDeliveryLabelWithFallback() called %v, want primary then fallback", calls)
		}
	})

	t.Run("does not fall back on other errors", func(t *testing.T) {
		var calls []string
		wantErr := errors.New("write failed")
		writeDeliveryLabelFn = func(_ string, beadsDir string, _ []string) error {
			calls = append(calls, beadsDir)
			return wantErr
		}

		err := addDeliveryLabelWithFallback("/tmp/town", "/tmp/rig/.beads", "/tmp/town/.beads", []string{"label", "add", "gs-wisp-ext", DeliveryLabelAcked})
		if !errors.Is(err, wantErr) {
			t.Fatalf("addDeliveryLabelWithFallback() error = %v, want %v", err, wantErr)
		}
		if !reflect.DeepEqual(calls, []string{"/tmp/rig/.beads"}) {
			t.Fatalf("addDeliveryLabelWithFallback() called %v, want only primary", calls)
		}
	})
}
