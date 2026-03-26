//go:build windows

package daemon

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getDiskUsage returns disk usage information for the filesystem containing path.
func getDiskUsage(path string) (*DiskUsageInfo, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("converting path: %w", err)
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("GetDiskFreeSpaceEx: %w", err)
	}

	usedBytes := totalNumberOfBytes - totalNumberOfFreeBytes
	var usedFraction float64
	if totalNumberOfBytes > 0 {
		usedFraction = float64(usedBytes) / float64(totalNumberOfBytes)
	}

	return &DiskUsageInfo{
		TotalBytes:   totalNumberOfBytes,
		FreeBytes:    freeBytesAvailable,
		UsedFraction: usedFraction,
	}, nil
}
