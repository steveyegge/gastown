package parallel

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecute_Sequential(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	var sum int32

	results := Execute(items, 1, func(item int) error {
		atomic.AddInt32(&sum, int32(item))
		return nil
	})

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	if sum != 15 {
		t.Errorf("Expected sum=15, got %d", sum)
	}

	for i, r := range results {
		if !r.Success {
			t.Errorf("Result %d should be success", i)
		}
		if r.Input != items[i] {
			t.Errorf("Result %d input=%d, want %d", i, r.Input, items[i])
		}
	}
}

func TestExecute_Parallel(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	var sum int32

	results := Execute(items, 4, func(item int) error {
		atomic.AddInt32(&sum, int32(item))
		return nil
	})

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}

	if sum != 55 {
		t.Errorf("Expected sum=55, got %d", sum)
	}

	// Results should be in order by index
	for i, r := range results {
		if r.Index != i {
			t.Errorf("Result %d has Index=%d", i, r.Index)
		}
	}
}

func TestExecute_WithErrors(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	errOdd := errors.New("odd number")

	results := Execute(items, 2, func(item int) error {
		if item%2 == 1 {
			return errOdd
		}
		return nil
	})

	successCount := CountSuccesses(results)
	if successCount != 2 {
		t.Errorf("Expected 2 successes, got %d", successCount)
	}

	errs := Errors(results)
	if len(errs) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(errs))
	}
}

func TestExecute_Empty(t *testing.T) {
	var items []int

	results := Execute(items, 4, func(item int) error {
		return nil
	})

	if results != nil {
		t.Errorf("Expected nil results for empty input, got %v", results)
	}
}

func TestExecute_ParallelismActuallyParallel(t *testing.T) {
	items := []int{1, 2, 3, 4}
	var maxConcurrent int32
	var current int32

	results := Execute(items, 4, func(item int) error {
		c := atomic.AddInt32(&current, 1)
		// Track max concurrent
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if c <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, c) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // Simulate work
		atomic.AddInt32(&current, -1)
		return nil
	})

	if len(results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(results))
	}

	// With parallelism=4 and 4 items, we should see >1 concurrent
	if maxConcurrent < 2 {
		t.Errorf("Expected parallel execution, maxConcurrent=%d", maxConcurrent)
	}
}

func TestExecuteWithCallback(t *testing.T) {
	items := []string{"a", "b", "c"}
	var callbackCount int32

	results := ExecuteWithCallback(items, 2, func(item string) error {
		return nil
	}, func(r Result[string]) {
		atomic.AddInt32(&callbackCount, 1)
	})

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	if callbackCount != 3 {
		t.Errorf("Expected 3 callbacks, got %d", callbackCount)
	}
}
