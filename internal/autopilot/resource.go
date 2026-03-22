package autopilot

import (
	"math"
	"runtime"
	"time"
)

const (
	// osReserveBytes is memory reserved for OS, Dolt, Ollama, etc.
	// 2 GB as specified in the autopilot plan.
	osReserveBytes = 2 * 1024 * 1024 * 1024

	// memPerPolecatBytes is the estimated memory per Claude agent instance.
	// 0.15 GB (≈150 MB) as specified in the plan.
	memPerPolecatBytes = 150 * 1024 * 1024

	// coresPerPolecat is the CPU core divisor — each polecat needs ~2 cores.
	coresPerPolecat = 2

	// hardCapPolecats is the absolute maximum polecats regardless of resources.
	// Practical limit on 8GB M1 machines.
	hardCapPolecats = 3
)

// TakeSnapshot captures the current system resource state.
// It reads memory, CPU, and load information from the OS.
func TakeSnapshot() ResourceSnapshot {
	snap := ResourceSnapshot{
		Timestamp: time.Now(),
		NumCores:  runtime.NumCPU(),
	}

	snap.MemoryTotalBytes = totalMemoryBytes()
	snap.MemoryAvailableBytes = availableMemoryBytes()
	snap.LoadAvg1 = loadAverage1()
	snap.CPUUsagePercent = estimateCPUUsage(snap.LoadAvg1, snap.NumCores)
	snap.ActiveSessions = countActiveSessions()

	return snap
}

// MaxSafePolecats computes the maximum number of polecats that can safely
// run given the current resource snapshot. The algorithm:
//
//	available = total - 2GB (OS/Dolt/Ollama reserve)
//	mem_based = available / 0.15GB per claude instance
//	cpu_based = cores / 2
//	hard_cap  = 3
//	result    = min(mem_based, cpu_based, hard_cap)
//
// Returns at least 1 if any resources are available at all.
func MaxSafePolecats(snap ResourceSnapshot) int {
	// Memory-based calculation
	var available int64
	if snap.MemoryTotalBytes > osReserveBytes {
		available = int64(snap.MemoryTotalBytes) - osReserveBytes
	}
	memBased := int(available / memPerPolecatBytes)

	// CPU-based calculation
	cpuBased := snap.NumCores / coresPerPolecat

	// Take the minimum of all constraints
	result := min(memBased, cpuBased, hardCapPolecats)

	// Floor at 1 if there are any resources at all
	if result < 1 && snap.MemoryTotalBytes > 0 && snap.NumCores > 0 {
		return 1
	}
	if result < 0 {
		return 0
	}
	return result
}

// estimateCPUUsage approximates CPU usage percentage from load average.
// load_avg / num_cores * 100, clamped to 0-100.
func estimateCPUUsage(loadAvg float64, numCores int) float64 {
	if numCores <= 0 {
		return 0
	}
	pct := (loadAvg / float64(numCores)) * 100
	return math.Min(pct, 100)
}
