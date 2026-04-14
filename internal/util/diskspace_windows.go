//go:build windows

package util

import (
	"fmt"
	"syscall"
	"unsafe"
)

// GetDiskSpace returns filesystem space information for the given path.
func GetDiskSpace(path string) (*DiskSpaceInfo, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path %s: %w", path, err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDiskFreeSpaceExW %s: %w", path, callErr)
	}

	used := totalBytes - totalFreeBytes
	var usedPct float64
	if totalBytes > 0 {
		usedPct = float64(used) / float64(totalBytes) * 100
	}

	return &DiskSpaceInfo{
		AvailableBytes: freeBytesAvailable,
		TotalBytes:     totalBytes,
		UsedBytes:      used,
		UsedPercent:    usedPct,
	}, nil
}
