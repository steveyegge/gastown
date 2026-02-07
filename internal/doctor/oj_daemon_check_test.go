package doctor

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNewOjDaemonCheck(t *testing.T) {
	check := NewOjDaemonCheck()
	if check.Name() != "oj-daemon" {
		t.Errorf("Name() = %q, want %q", check.Name(), "oj-daemon")
	}
	if check.Description() != "Check if OJ daemon is running and healthy" {
		t.Errorf("Description() = %q, want %q", check.Description(), "Check if OJ daemon is running and healthy")
	}
	if check.CanFix() {
		t.Error("CanFix() should return false")
	}
}

func TestOjDaemonCheck_OjNotEnabled(t *testing.T) {
	old := os.Getenv("GT_SLING_OJ")
	os.Unsetenv("GT_SLING_OJ")
	defer func() {
		if old != "" {
			os.Setenv("GT_SLING_OJ", old)
		}
	}()

	check := NewOjDaemonCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK when OJ not enabled", result.Status)
	}
	if result.Message != "OJ dispatch not enabled (GT_SLING_OJ != 1)" {
		t.Errorf("Message = %q, want OJ dispatch not enabled message", result.Message)
	}
}

func TestOjDaemonCheck_OjEnabledButNoBinary(t *testing.T) {
	old := os.Getenv("GT_SLING_OJ")
	oldPath := os.Getenv("PATH")
	os.Setenv("GT_SLING_OJ", "1")
	os.Setenv("PATH", t.TempDir()) // Empty dir, no oj binary
	defer func() {
		if old != "" {
			os.Setenv("GT_SLING_OJ", old)
		} else {
			os.Unsetenv("GT_SLING_OJ")
		}
		os.Setenv("PATH", oldPath)
	}()

	check := NewOjDaemonCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning when oj binary not found", result.Status)
	}
	if result.FixHint == "" {
		t.Error("FixHint should not be empty when binary not found")
	}
}

func TestOjDaemonCheck_CategoryIsInfrastructure(t *testing.T) {
	check := NewOjDaemonCheck()
	if check.CheckCategory != CategoryInfrastructure {
		t.Errorf("Category = %q, want %q", check.CheckCategory, CategoryInfrastructure)
	}
}

func TestOjDaemonStatus_ParsesRunningJSON(t *testing.T) {
	input := `{
		"status": "running",
		"version": "0.1.0",
		"uptime_secs": 3600,
		"uptime": "1h 0m 0s",
		"jobs_active": 3,
		"sessions_active": 5,
		"orphan_count": 1
	}`

	var status ojDaemonStatus
	if err := json.Unmarshal([]byte(input), &status); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if status.Status != "running" {
		t.Errorf("Status = %q, want %q", status.Status, "running")
	}
	if status.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", status.Version, "0.1.0")
	}
	if status.UptimeSecs != 3600 {
		t.Errorf("UptimeSecs = %d, want %d", status.UptimeSecs, 3600)
	}
	if status.JobsActive != 3 {
		t.Errorf("JobsActive = %d, want %d", status.JobsActive, 3)
	}
	if status.SessionsActive != 5 {
		t.Errorf("SessionsActive = %d, want %d", status.SessionsActive, 5)
	}
	if status.OrphanCount != 1 {
		t.Errorf("OrphanCount = %d, want %d", status.OrphanCount, 1)
	}
}

func TestOjStatusOverview_ParsesJSON(t *testing.T) {
	input := `{
		"uptime_secs": 7200,
		"namespaces": [
			{
				"namespace": "gt11",
				"active_jobs": [
					{"id": "abc123", "name": "sling-rictus", "kind": "gt-sling", "step": "spawn"}
				],
				"workers": [
					{"name": "default", "status": "running", "active": 2, "concurrency": 4}
				],
				"queues": [
					{"name": "default", "pending": 5, "active": 2, "dead": 0}
				]
			}
		]
	}`

	var overview ojStatusOverview
	if err := json.Unmarshal([]byte(input), &overview); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if overview.UptimeSecs != 7200 {
		t.Errorf("UptimeSecs = %d, want %d", overview.UptimeSecs, 7200)
	}
	if len(overview.Namespaces) != 1 {
		t.Fatalf("Namespaces count = %d, want 1", len(overview.Namespaces))
	}

	ns := overview.Namespaces[0]
	if ns.Namespace != "gt11" {
		t.Errorf("Namespace = %q, want %q", ns.Namespace, "gt11")
	}
	if len(ns.ActiveJobs) != 1 {
		t.Errorf("ActiveJobs count = %d, want 1", len(ns.ActiveJobs))
	}
	if len(ns.Workers) != 1 {
		t.Errorf("Workers count = %d, want 1", len(ns.Workers))
	}
	if ns.Workers[0].Concurrency != 4 {
		t.Errorf("Worker concurrency = %d, want 4", ns.Workers[0].Concurrency)
	}
	if len(ns.Queues) != 1 {
		t.Errorf("Queues count = %d, want 1", len(ns.Queues))
	}
	if ns.Queues[0].Pending != 5 {
		t.Errorf("Queue pending = %d, want 5", ns.Queues[0].Pending)
	}
}

func TestOjDaemonStatus_ParsesNotRunningJSON(t *testing.T) {
	input := `{ "status": "not_running" }`

	var status ojDaemonStatus
	if err := json.Unmarshal([]byte(input), &status); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if status.Status != "not_running" {
		t.Errorf("Status = %q, want %q", status.Status, "not_running")
	}
}

func TestOjDaemonStatus_EmptyNamespaces(t *testing.T) {
	input := `{
		"uptime_secs": 100,
		"namespaces": []
	}`

	var overview ojStatusOverview
	if err := json.Unmarshal([]byte(input), &overview); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(overview.Namespaces) != 0 {
		t.Errorf("Namespaces count = %d, want 0", len(overview.Namespaces))
	}
}
