package daemon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestRigWorkerPoolConcurrencyLimit verifies that the pool never runs more than
// the configured number of rigs simultaneously.
func TestRigWorkerPoolConcurrencyLimit(t *testing.T) {
	const (
		numRigs    = 20
		maxWorkers = 5
	)

	pool := newRigWorkerPool(maxWorkers, 10*time.Second, nil)

	var active atomic.Int64
	var peak atomic.Int64

	rigs := make([]string, numRigs)
	for i := range rigs {
		rigs[i] = "rig"
	}

	pool.runPerRig(context.Background(), rigs, func(ctx context.Context, rigName string) error {
		cur := active.Add(1)
		// Record peak concurrency.
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond) // hold the slot briefly
		active.Add(-1)
		return nil
	})

	got := peak.Load()
	if got > maxWorkers {
		t.Errorf("peak concurrency %d exceeded limit %d", got, maxWorkers)
	}
	if got == 0 {
		t.Error("no rigs were processed")
	}
}

// TestRigWorkerPoolContextTimeout verifies that per-rig context timeouts fire and
// allow the remaining rigs to proceed unblocked.
func TestRigWorkerPoolContextTimeout(t *testing.T) {
	const (
		numRigs    = 5
		rigTimeout = 50 * time.Millisecond
		slowDelay  = 500 * time.Millisecond // much longer than timeout
	)

	pool := newRigWorkerPool(numRigs, rigTimeout, nil)

	var cancelled atomic.Int64
	var completed atomic.Int64

	rigs := make([]string, numRigs)
	for i := range rigs {
		rigs[i] = "rig"
	}

	pool.runPerRig(context.Background(), rigs, func(ctx context.Context, _ string) error {
		select {
		case <-time.After(slowDelay):
			completed.Add(1)
			return nil
		case <-ctx.Done():
			cancelled.Add(1)
			return ctx.Err()
		}
	})

	if cancelled.Load() == 0 {
		t.Error("expected at least one rig to be cancelled by timeout, got 0")
	}
	// All rigs should have responded (either completed or cancelled), not hung.
	total := cancelled.Load() + completed.Load()
	if total != numRigs {
		t.Errorf("expected %d rigs total, got %d (cancelled=%d completed=%d)",
			numRigs, total, cancelled.Load(), completed.Load())
	}
}

// TestRigWorkerPoolSlowRigDoesNotBlockOthers verifies that one slow rig does not
// prevent the remaining rigs from completing within a reasonable wall-clock window.
func TestRigWorkerPoolSlowRigDoesNotBlockOthers(t *testing.T) {
	const (
		slowRig    = "slow-rig"
		rigTimeout = 200 * time.Millisecond
		fastDelay  = 10 * time.Millisecond
	)

	pool := newRigWorkerPool(10, rigTimeout, nil)

	var fastDone atomic.Int64

	rigs := []string{slowRig, "fast-1", "fast-2", "fast-3", "fast-4"}

	start := time.Now()
	pool.runPerRig(context.Background(), rigs, func(ctx context.Context, rigName string) error {
		if rigName == slowRig {
			// Slow rig blocks until its context times out.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Second): // never fires in practice
				return nil
			}
		}
		time.Sleep(fastDelay)
		fastDone.Add(1)
		return nil
	})
	elapsed := time.Since(start)

	// All fast rigs must have completed.
	if got := fastDone.Load(); got != 4 {
		t.Errorf("expected 4 fast rigs to complete, got %d", got)
	}

	// Total elapsed should be dominated by rigTimeout (≈200ms), not by a serial
	// execution of the slow rig (which would exceed rigTimeout without the pool).
	// Allow 3× to account for test environment jitter.
	limit := 3 * rigTimeout
	if elapsed > limit {
		t.Errorf("runPerRig took %v, expected < %v (slow rig should not block overall)", elapsed, limit)
	}
}

// BenchmarkRigWorkerPool100RigsOneSlow measures the wall-clock time of a simulated
// heartbeat tick with 100 rigs, where one rig is slow (100ms).
//
// Run with: go test ./internal/daemon/ -bench=BenchmarkRigWorkerPool100RigsOneSlow -benchtime=5s
func BenchmarkRigWorkerPool100RigsOneSlow(b *testing.B) {
	const (
		numRigs   = 100
		slowIndex = 7
		slowDelay = 100 * time.Millisecond
		fastDelay = 1 * time.Millisecond
		rigTimeout = 5 * time.Second
	)

	pool := newRigWorkerPool(defaultRigConcurrency, rigTimeout, nil)

	rigs := make([]string, numRigs)
	for i := range rigs {
		rigs[i] = "rig"
	}

	b.ResetTimer()
	for range b.N {
		i := 0
		pool.runPerRig(context.Background(), rigs, func(ctx context.Context, _ string) error {
			delay := fastDelay
			if i == slowIndex {
				delay = slowDelay
			}
			i++
			time.Sleep(delay)
			return nil
		})
	}
}
