//go:build !windows

package daemon

import (
	"fmt"
	"syscall"
)

// getDiskUsage returns disk usage information for the filesystem containing path.
func getDiskUsage(path string) (*DiskUsageInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs %s: %w", path, err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)         //nolint:unconvert
	freeBytes := uint64(stat.Bavail) * uint64(stat.Bsize) //nolint:unconvert
	usedBytes := totalBytes - freeBytes

	var usedFraction float64
	if totalBytes > 0 {
		usedFraction = float64(usedBytes) / float64(totalBytes)
	}

	return &DiskUsageInfo{
		TotalBytes:   totalBytes,
		FreeBytes:    freeBytes,
		UsedFraction: usedFraction,
	}, nil
}
