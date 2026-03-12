package daemon

import (
	"fmt"
	"os"
	"strings"
)

// loadAverage1Sysctl is a no-op on Linux — /proc/loadavg is used directly.
func loadAverage1Sysctl() float64 {
	return 0
}

// availableMemoryGB returns available memory in GB on Linux.
// Reads MemAvailable from /proc/meminfo (kernel 3.14+).
func availableMemoryGB() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			var kb uint64
			_, err := fmt.Sscanf(line, "MemAvailable: %d kB", &kb)
			if err != nil {
				return 0
			}
			return float64(kb) / (1024 * 1024)
		}
	}
	return 0
}
