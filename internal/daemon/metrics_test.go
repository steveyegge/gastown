package daemon

import (
	"context"
	"testing"
)

func TestNewDaemonMetrics(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatalf("newDaemonMetrics() error: %v", err)
	}
	if dm == nil {
		t.Fatal("expected non-nil *daemonMetrics")
	}
}

func TestDaemonMetrics_NilReceiver(t *testing.T) {
	var dm *daemonMetrics
	ctx := context.Background()

	// All methods must be nil-safe — no panic expected.
	dm.recordHeartbeat(ctx)
	dm.recordRestart(ctx, "deacon")
	dm.updateDoltHealth(5, 100, 2.5, 1024, true)
}

func TestDaemonMetrics_RecordHeartbeat(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	dm.recordHeartbeat(ctx)
	dm.recordHeartbeat(ctx)
}

func TestDaemonMetrics_RecordRestart(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, agentType := range []string{"deacon", "witness", "refinery", "polecat"} {
		dm.recordRestart(ctx, agentType)
	}
}

func TestDaemonMetrics_UpdateDoltHealth_Healthy(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatal(err)
	}

	dm.updateDoltHealth(5, 100, 2.5, 1048576, true)

	dm.doltMu.RLock()
	defer dm.doltMu.RUnlock()

	if dm.doltConnections != 5 {
		t.Errorf("doltConnections = %d, want 5", dm.doltConnections)
	}
	if dm.doltMaxConnections != 100 {
		t.Errorf("doltMaxConnections = %d, want 100", dm.doltMaxConnections)
	}
	if dm.doltLatencyMs != 2.5 {
		t.Errorf("doltLatencyMs = %f, want 2.5", dm.doltLatencyMs)
	}
	if dm.doltDiskBytes != 1048576 {
		t.Errorf("doltDiskBytes = %d, want 1048576", dm.doltDiskBytes)
	}
	if dm.doltHealthy != 1 {
		t.Errorf("doltHealthy = %d, want 1", dm.doltHealthy)
	}
}

func TestDaemonMetrics_UpdateDoltHealth_Unhealthy(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatal(err)
	}

	dm.updateDoltHealth(0, 0, 0, 0, false)

	dm.doltMu.RLock()
	defer dm.doltMu.RUnlock()

	if dm.doltHealthy != 0 {
		t.Errorf("doltHealthy = %d, want 0 (unhealthy)", dm.doltHealthy)
	}
}

func TestDaemonMetrics_UpdateDoltHealth_Idempotent(t *testing.T) {
	dm, err := newDaemonMetrics()
	if err != nil {
		t.Fatal(err)
	}

	dm.updateDoltHealth(10, 200, 5.0, 2048, true)
	dm.updateDoltHealth(3, 200, 1.0, 2048, false)

	dm.doltMu.RLock()
	defer dm.doltMu.RUnlock()

	if dm.doltConnections != 3 {
		t.Errorf("doltConnections = %d, want 3 (last write wins)", dm.doltConnections)
	}
	if dm.doltHealthy != 0 {
		t.Errorf("doltHealthy = %d, want 0 (unhealthy from last write)", dm.doltHealthy)
	}
}
