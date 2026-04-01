package daemon

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	defaultRigConcurrency = 10
	defaultRigTimeout     = 30 * time.Second
)

// RigWorkerPool runs per-rig heartbeat operations with bounded concurrency
// and per-rig context timeouts. This prevents a slow or hung rig from
// blocking heartbeat operations on all other rigs.
//
// With N rigs and a serial loop, the heartbeat takes O(N × max_op_time).
// With the pool, it takes O(max_op_time) — one slow rig no longer gates all others.
type RigWorkerPool struct {
	concurrency int
	timeout     time.Duration
	logger      *log.Logger
}

// newRigWorkerPool creates a RigWorkerPool.
// Zero or negative values for concurrency and timeout fall back to package defaults.
func newRigWorkerPool(concurrency int, timeout time.Duration, logger *log.Logger) *RigWorkerPool {
	if concurrency <= 0 {
		concurrency = defaultRigConcurrency
	}
	if timeout <= 0 {
		timeout = defaultRigTimeout
	}
	return &RigWorkerPool{
		concurrency: concurrency,
		timeout:     timeout,
		logger:      logger,
	}
}

// runPerRig executes fn once for each rig, with bounded concurrency and per-rig timeouts.
//
// Each invocation of fn receives a child context derived from parent with the pool's
// per-rig timeout applied. If fn respects its context (checks ctx.Done()), it will
// be canceled when the timeout fires.
//
// runPerRig blocks until all goroutines complete. Errors are counted and a single
// summary line is logged rather than per-rig noise.
func (p *RigWorkerPool) runPerRig(
	parent context.Context,
	rigs []string,
	fn func(ctx context.Context, rigName string) error,
) {
	if len(rigs) == 0 {
		return
	}

	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errCount int

	for _, r := range rigs {
		wg.Add(1)
		go func(rigName string) {
			defer wg.Done()

			// Acquire a worker slot; block until one is available.
			sem <- struct{}{}
			defer func() { <-sem }()

			// Each rig gets its own timeout-bounded context so a slow rig
			// can be signaled to stop without affecting other rigs.
			ctx, cancel := context.WithTimeout(parent, p.timeout)
			defer cancel()

			if err := fn(ctx, rigName); err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
				if p.logger != nil {
					p.logger.Printf("rig_worker: %s: %v", rigName, err)
				}
			}
		}(r)
	}

	wg.Wait()

	mu.Lock()
	count := errCount
	mu.Unlock()

	if count > 0 && p.logger != nil {
		p.logger.Printf("rig_worker: %d/%d rig(s) had errors", count, len(rigs))
	}
}
