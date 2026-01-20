// Package parallel provides generic parallel execution utilities.
package parallel

import (
	"sync"
)

// Result represents the outcome of processing a single item.
type Result[T any] struct {
	Index   int   // Original index in input slice
	Input   T     // The input item
	Success bool  // Whether processing succeeded
	Error   error // Error if processing failed
}

// WorkFunc is the function type for processing items.
type WorkFunc[T any] func(item T) error

// Execute processes items in parallel with the given concurrency.
// Returns results in the same order as input items.
func Execute[T any](items []T, parallelism int, work WorkFunc[T]) []Result[T] {
	if len(items) == 0 {
		return nil
	}

	if parallelism < 1 {
		parallelism = 1
	}

	results := make([]Result[T], len(items))

	// Channel for jobs (indices into items slice)
	jobs := make(chan int, len(items))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				item := items[idx]
				err := work(item)
				results[idx] = Result[T]{
					Index:   idx,
					Input:   item,
					Success: err == nil,
					Error:   err,
				}
			}
		}()
	}

	// Send jobs
	for i := range items {
		jobs <- i
	}
	close(jobs)

	// Wait for completion
	wg.Wait()

	return results
}

// ExecuteWithCallback processes items in parallel and calls callback for each result.
// Results may arrive out of order.
func ExecuteWithCallback[T any](items []T, parallelism int, work WorkFunc[T], callback func(Result[T])) []Result[T] {
	if len(items) == 0 {
		return nil
	}

	if parallelism < 1 {
		parallelism = 1
	}

	results := make([]Result[T], len(items))
	var mu sync.Mutex

	// Channel for jobs
	jobs := make(chan int, len(items))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				item := items[idx]
				err := work(item)
				r := Result[T]{
					Index:   idx,
					Input:   item,
					Success: err == nil,
					Error:   err,
				}

				mu.Lock()
				results[idx] = r
				mu.Unlock()

				if callback != nil {
					callback(r)
				}
			}
		}()
	}

	// Send jobs
	for i := range items {
		jobs <- i
	}
	close(jobs)

	// Wait for completion
	wg.Wait()

	return results
}

// CountSuccesses returns the number of successful results.
func CountSuccesses[T any](results []Result[T]) int {
	count := 0
	for _, r := range results {
		if r.Success {
			count++
		}
	}
	return count
}

// Errors returns all errors from the results.
func Errors[T any](results []Result[T]) []error {
	var errs []error
	for _, r := range results {
		if r.Error != nil {
			errs = append(errs, r.Error)
		}
	}
	return errs
}
