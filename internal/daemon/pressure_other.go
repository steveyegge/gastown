//go:build !darwin && !linux && !windows

package daemon

// loadAverage1Sysctl is a no-op on unsupported platforms.
func loadAverage1Sysctl() float64 {
	return 0
}

// availableMemoryGB is a no-op on unsupported platforms.
func availableMemoryGB() float64 {
	return 0
}
