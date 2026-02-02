package beads

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestSemaphoreLimitsConcurrency(t *testing.T) {
	var peak int32
	var current int32
	var wg sync.WaitGroup

	// Spawn 20 goroutines that all try to acquire the semaphore.
	// Track the peak number of concurrent holders.
	const n = 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			AcquireBd()
			c := atomic.AddInt32(&current, 1)
			// Update peak via CAS loop
			for {
				p := atomic.LoadInt32(&peak)
				if c <= p || atomic.CompareAndSwapInt32(&peak, p, c) {
					break
				}
			}
			// Simulate work so goroutines overlap
			for j := 0; j < 1000; j++ {
				_ = j
			}
			atomic.AddInt32(&current, -1)
			ReleaseBd()
		}()
	}

	wg.Wait()

	if peak > MaxConcurrentBd {
		t.Errorf("peak concurrency %d exceeded limit %d", peak, MaxConcurrentBd)
	}
	if peak == 0 {
		t.Error("peak concurrency was 0; semaphore may not have been used")
	}
}

func TestAcquireRelease(t *testing.T) {
	// Drain any state from other tests by creating a fresh channel.
	// The global semaphore is shared, so just verify acquire/release pairs work.
	AcquireBd()
	AcquireBd()
	AcquireBd()
	// All 3 slots taken; release them.
	ReleaseBd()
	ReleaseBd()
	ReleaseBd()
}
