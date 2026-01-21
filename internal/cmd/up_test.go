package cmd

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestAgentStartResult_Fields(t *testing.T) {
	result := agentStartResult{
		name:   "Witness (gastown)",
		ok:     true,
		detail: "gt-gastown-witness",
	}

	if result.name != "Witness (gastown)" {
		t.Errorf("name = %q, want %q", result.name, "Witness (gastown)")
	}
	if !result.ok {
		t.Error("ok should be true")
	}
	if result.detail != "gt-gastown-witness" {
		t.Errorf("detail = %q, want %q", result.detail, "gt-gastown-witness")
	}
}

func TestMaxConcurrentAgentStarts_Constant(t *testing.T) {
	// Verify the constant is set to a reasonable value
	if maxConcurrentAgentStarts < 1 {
		t.Errorf("maxConcurrentAgentStarts = %d, should be >= 1", maxConcurrentAgentStarts)
	}
	if maxConcurrentAgentStarts > 100 {
		t.Errorf("maxConcurrentAgentStarts = %d, should be <= 100 to prevent resource exhaustion", maxConcurrentAgentStarts)
	}
}

func TestSemaphoreLimitsConcurrency(t *testing.T) {
	// Test that a semaphore pattern properly limits concurrency
	const maxConcurrent = 3
	const totalTasks = 10

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var maxObserved int32
	var current int32

	for i := 0; i < totalTasks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Track concurrent count
			cur := atomic.AddInt32(&current, 1)
			defer atomic.AddInt32(&current, -1)

			// Update max observed
			for {
				max := atomic.LoadInt32(&maxObserved)
				if cur <= max || atomic.CompareAndSwapInt32(&maxObserved, max, cur) {
					break
				}
			}

			// Simulate work
			time.Sleep(10 * time.Millisecond)
		}()
	}

	wg.Wait()

	if maxObserved > maxConcurrent {
		t.Errorf("max concurrent = %d, should not exceed %d", maxObserved, maxConcurrent)
	}
}

func TestStartRigAgentsParallel_EmptyRigs(t *testing.T) {
	// Test with empty rig list - should return empty maps without error
	witnessResults, refineryResults := startRigAgentsParallel([]string{})

	if len(witnessResults) != 0 {
		t.Errorf("witnessResults should be empty, got %d entries", len(witnessResults))
	}
	if len(refineryResults) != 0 {
		t.Errorf("refineryResults should be empty, got %d entries", len(refineryResults))
	}
}

func TestStartRigAgentsWithPrefetch_EmptyRigs(t *testing.T) {
	// Test with empty inputs
	witnessResults, refineryResults := startRigAgentsWithPrefetch(
		[]string{},
		make(map[string]*rig.Rig),
		make(map[string]error),
	)

	if len(witnessResults) != 0 {
		t.Errorf("witnessResults should be empty, got %d entries", len(witnessResults))
	}
	if len(refineryResults) != 0 {
		t.Errorf("refineryResults should be empty, got %d entries", len(refineryResults))
	}
}

func TestStartRigAgentsWithPrefetch_RecordsErrors(t *testing.T) {
	// Test that rig errors are properly recorded
	rigErrors := map[string]error{
		"badrig": fmt.Errorf("rig not found"),
	}

	witnessResults, refineryResults := startRigAgentsWithPrefetch(
		[]string{"badrig"},
		make(map[string]*rig.Rig),
		rigErrors,
	)

	if len(witnessResults) != 1 {
		t.Errorf("witnessResults should have 1 entry, got %d", len(witnessResults))
	}
	if result, ok := witnessResults["badrig"]; !ok {
		t.Error("witnessResults should have badrig entry")
	} else if result.ok {
		t.Error("badrig witness result should not be ok")
	}

	if len(refineryResults) != 1 {
		t.Errorf("refineryResults should have 1 entry, got %d", len(refineryResults))
	}
	if result, ok := refineryResults["badrig"]; !ok {
		t.Error("refineryResults should have badrig entry")
	} else if result.ok {
		t.Error("badrig refinery result should not be ok")
	}
}

func TestPrefetchRigs_Empty(t *testing.T) {
	// Test with empty rig list
	rigs, errors := prefetchRigs([]string{})

	if len(rigs) != 0 {
		t.Errorf("rigs should be empty, got %d entries", len(rigs))
	}
	if len(errors) != 0 {
		t.Errorf("errors should be empty, got %d entries", len(errors))
	}
}

func TestWorkerPoolLimitsConcurrency(t *testing.T) {
	// Test that a worker pool pattern properly limits concurrency
	const numWorkers = 3
	const numTasks = 15

	tasks := make(chan int, numTasks)
	results := make(chan int, numTasks)

	var maxObserved int32
	var current int32

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range tasks {
				// Track concurrent count
				cur := atomic.AddInt32(&current, 1)

				// Update max observed
				for {
					max := atomic.LoadInt32(&maxObserved)
					if cur <= max || atomic.CompareAndSwapInt32(&maxObserved, max, cur) {
						break
					}
				}

				// Simulate work
				time.Sleep(5 * time.Millisecond)

				atomic.AddInt32(&current, -1)
				results <- 1
			}
		}()
	}

	// Enqueue tasks
	for i := 0; i < numTasks; i++ {
		tasks <- i
	}
	close(tasks)

	// Wait for workers and collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	count := 0
	for range results {
		count++
	}

	if count != numTasks {
		t.Errorf("expected %d results, got %d", numTasks, count)
	}
	if maxObserved > numWorkers {
		t.Errorf("max concurrent = %d, should not exceed %d workers", maxObserved, numWorkers)
	}
}
