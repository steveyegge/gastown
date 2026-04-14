//go:build !windows

package util

import (
	"fmt"
	"syscall"
)

// GetDiskSpace returns filesystem space information for the given path.
func GetDiskSpace(path string) (*DiskSpaceInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, fmt.Errorf("statfs %s: %w", path, err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize) // Bavail = available to non-root
	used := total - (stat.Bfree * uint64(stat.Bsize))

	var usedPct float64
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}

	return &DiskSpaceInfo{
		AvailableBytes: free,
		TotalBytes:     total,
		UsedBytes:      used,
		UsedPercent:    usedPct,
	}, nil
}
