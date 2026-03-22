package autopilot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTakeSnapshot(t *testing.T) {
	snap := TakeSnapshot()

	assert.False(t, snap.Timestamp.IsZero(), "Timestamp should be set")
	assert.True(t, snap.Timestamp.Before(time.Now().Add(time.Second)), "Timestamp should be recent")
	assert.Greater(t, snap.NumCores, 0, "NumCores should be positive")
	assert.Greater(t, snap.MemoryTotalBytes, uint64(0), "MemoryTotalBytes should be positive")
	// MemoryAvailableBytes can be 0 in some CI environments, so just check it's not greater than total
	assert.LessOrEqual(t, snap.MemoryAvailableBytes, snap.MemoryTotalBytes,
		"Available memory should not exceed total")
}

func TestMaxSafePolecats_8GB_8Core(t *testing.T) {
	// Simulate an 8GB M1 Mac with 8 cores
	snap := ResourceSnapshot{
		MemoryTotalBytes:     8 * 1024 * 1024 * 1024, // 8 GB
		MemoryAvailableBytes: 4 * 1024 * 1024 * 1024, // 4 GB available
		NumCores:             8,
	}

	result := MaxSafePolecats(snap)

	// available = 8GB - 2GB = 6GB = 6442450944 bytes
	// mem_based = 6442450944 / 157286400 = 40 (lots of headroom)
	// cpu_based = 8 / 2 = 4
	// hard_cap = 3
	// result = min(40, 4, 3) = 3
	assert.Equal(t, 3, result, "8GB/8-core should yield 3 (hard cap)")
}

func TestMaxSafePolecats_16GB_10Core(t *testing.T) {
	snap := ResourceSnapshot{
		MemoryTotalBytes:     16 * 1024 * 1024 * 1024,
		MemoryAvailableBytes: 10 * 1024 * 1024 * 1024,
		NumCores:             10,
	}

	result := MaxSafePolecats(snap)

	// available = 16GB - 2GB = 14GB
	// mem_based = 14GB / 150MB = 93+
	// cpu_based = 10 / 2 = 5
	// hard_cap = 3
	// result = 3
	assert.Equal(t, 3, result, "16GB/10-core should hit hard cap of 3")
}

func TestMaxSafePolecats_4GB_4Core(t *testing.T) {
	snap := ResourceSnapshot{
		MemoryTotalBytes:     4 * 1024 * 1024 * 1024,
		MemoryAvailableBytes: 2 * 1024 * 1024 * 1024,
		NumCores:             4,
	}

	result := MaxSafePolecats(snap)

	// available = 4GB - 2GB = 2GB
	// mem_based = 2GB / 150MB = 13
	// cpu_based = 4 / 2 = 2
	// hard_cap = 3
	// result = min(13, 2, 3) = 2
	assert.Equal(t, 2, result, "4GB/4-core should yield 2 (CPU limited)")
}

func TestMaxSafePolecats_2GB_2Core(t *testing.T) {
	// Very constrained machine — only 2GB total, 2 cores
	snap := ResourceSnapshot{
		MemoryTotalBytes:     2 * 1024 * 1024 * 1024,
		MemoryAvailableBytes: 512 * 1024 * 1024,
		NumCores:             2,
	}

	result := MaxSafePolecats(snap)

	// available = 2GB - 2GB = 0
	// mem_based = 0
	// cpu_based = 2 / 2 = 1
	// hard_cap = 3
	// min(0, 1, 3) = 0 → but floor at 1 since resources exist
	assert.Equal(t, 1, result, "Very low memory should floor at 1")
}

func TestMaxSafePolecats_1Core(t *testing.T) {
	snap := ResourceSnapshot{
		MemoryTotalBytes:     8 * 1024 * 1024 * 1024,
		MemoryAvailableBytes: 4 * 1024 * 1024 * 1024,
		NumCores:             1,
	}

	result := MaxSafePolecats(snap)

	// cpu_based = 1 / 2 = 0
	// Floor at 1 since machine has resources
	assert.Equal(t, 1, result, "Single core should floor at 1")
}

func TestMaxSafePolecats_ZeroResources(t *testing.T) {
	snap := ResourceSnapshot{
		MemoryTotalBytes: 0,
		NumCores:         0,
	}

	result := MaxSafePolecats(snap)
	assert.Equal(t, 0, result, "No resources should yield 0")
}

func TestEstimateCPUUsage(t *testing.T) {
	tests := []struct {
		name     string
		loadAvg  float64
		numCores int
		expected float64
	}{
		{"idle system", 0.5, 8, 6.25},
		{"moderate load", 4.0, 8, 50.0},
		{"fully loaded", 8.0, 8, 100.0},
		{"overloaded", 16.0, 8, 100.0}, // clamped to 100
		{"zero cores", 1.0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateCPUUsage(tt.loadAvg, tt.numCores)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestResourceSnapshot_MemoryHelpers(t *testing.T) {
	snap := ResourceSnapshot{
		MemoryTotalBytes:     8 * 1024 * 1024 * 1024,
		MemoryAvailableBytes: 4 * 1024 * 1024 * 1024,
	}

	assert.InDelta(t, 8.0, snap.MemoryTotalGB(), 0.01)
	assert.InDelta(t, 4.0, snap.MemoryAvailableGB(), 0.01)
}

func TestTakeSnapshot_Integration(t *testing.T) {
	// Integration test: verify TakeSnapshot returns sensible values on this machine
	snap := TakeSnapshot()

	require.Greater(t, snap.MemoryTotalBytes, uint64(0), "Must detect total memory")
	require.Greater(t, snap.NumCores, 0, "Must detect CPU cores")

	// Verify MaxSafePolecats works with a real snapshot
	result := MaxSafePolecats(snap)
	assert.GreaterOrEqual(t, result, 1, "Real machine should allow at least 1 polecat")
	assert.LessOrEqual(t, result, hardCapPolecats, "Should not exceed hard cap")
}
