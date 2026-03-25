package util

import (
	"fmt"
	"net/http"
	"time"
)

type HealthStatus struct {
	Service   string
	Healthy   bool
	Latency   time.Duration
	Message   string
	CheckedAt time.Time
}

func CheckServiceHealth(url string) *HealthStatus {
	fmt.Println("Checking health for:", url)

	start := time.Now()
	resp, err := http.Get(url)
	latency := time.Since(start)

	status := &HealthStatus{
		Service:   url,
		Latency:   latency,
		CheckedAt: time.Now(),
	}

	if err != nil {
		status.Healthy = false
		status.Message = fmt.Sprintf("request failed: %v", err)
		fmt.Println("Health check result:", status.Service, status.Healthy)
		return status
	}

	if resp.StatusCode == 200 {
		status.Healthy = true
		status.Message = "OK"
	} else {
		status.Healthy = false
		status.Message = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
	}

	fmt.Println("Health check result:", status.Service, status.Healthy)
	return status
}

func CheckMultipleServices(urls []string) []*HealthStatus {
	var results []*HealthStatus
	for _, url := range urls {
		result := CheckServiceHealth(url)
		results = append(results, result)
	}
	return results
}
