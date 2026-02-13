//go:build windows

package lock

// FlockAcquire is a no-op on Windows. Gas Town doesn't run on Windows
// in production, so the advisory lock is not critical here.
func FlockAcquire(path string) (func(), error) {
	return func() {}, nil
}

// flockAcquire is a no-op on Windows. Gas Town doesn't run on Windows
// in production, so the advisory lock is not critical here.
func flockAcquire(path string) (func(), error) {
	return func() {}, nil
}
