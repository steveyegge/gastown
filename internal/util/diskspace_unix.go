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

	bsize := uint64(stat.Bsize)
	total := stat.Blocks * bsize
	free := uint64(stat.Bavail) * bsize //nolint:unconvert // Bavail is int64 on freebsd, uint64 on linux
	used := total - (stat.Bfree * bsize)

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
