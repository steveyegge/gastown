package dockermon

import (
	"encoding/json"
	"io"
)

// containerStats holds the subset of Docker stats we care about.
type containerStats struct {
	MemoryUsage      uint64
	MemoryLimit      uint64
	ThrottledPeriods uint64
	TotalPeriods     uint64
}

// dockerStatsJSON matches the Docker /containers/{id}/stats response shape.
type dockerStatsJSON struct {
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	CPUStats struct {
		ThrottlingData struct {
			ThrottledPeriods uint64 `json:"throttled_periods"`
			Periods          uint64 `json:"periods"`
		} `json:"throttling_data"`
	} `json:"cpu_stats"`
}

func decodeStats(r io.Reader) (*containerStats, error) {
	var raw dockerStatsJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, err
	}
	return &containerStats{
		MemoryUsage:      raw.MemoryStats.Usage,
		MemoryLimit:      raw.MemoryStats.Limit,
		ThrottledPeriods: raw.CPUStats.ThrottlingData.ThrottledPeriods,
		TotalPeriods:     raw.CPUStats.ThrottlingData.Periods,
	}, nil
}
