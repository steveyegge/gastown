package beads

import (
	"os/exec"
	"testing"
)

func TestParseBdDaemonCount_Array(t *testing.T) {
	input := []byte(`[{"pid":1234},{"pid":5678}]`)
	count := parseBdDaemonCount(input)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestParseBdDaemonCount_ObjectWithCount(t *testing.T) {
	input := []byte(`{"count":3,"daemons":[{},{},{}]}`)
	count := parseBdDaemonCount(input)
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestParseBdDaemonCount_ObjectWithDaemons(t *testing.T) {
	input := []byte(`{"daemons":[{},{}]}`)
	count := parseBdDaemonCount(input)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestParseBdDaemonCount_Empty(t *testing.T) {
	input := []byte(``)
	count := parseBdDaemonCount(input)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestParseBdDaemonCount_Invalid(t *testing.T) {
	input := []byte(`not json`)
	count := parseBdDaemonCount(input)
	if count != 0 {
		t.Errorf("expected 0 for invalid JSON, got %d", count)
	}
}

func TestCountBdActivityProcesses(t *testing.T) {
	count := CountBdActivityProcesses()
	if count < 0 {
		t.Errorf("count should be non-negative, got %d", count)
	}
}

func TestCountBdDaemons(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed")
	}
	count := CountBdDaemons()
	if count < 0 {
		t.Errorf("count should be non-negative, got %d", count)
	}
}

func TestStopAllBdProcesses_DryRun(t *testing.T) {
	daemonsKilled, activityKilled, err := StopAllBdProcesses(true, false)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if daemonsKilled < 0 || activityKilled < 0 {
		t.Errorf("counts should be non-negative: daemons=%d, activity=%d", daemonsKilled, activityKilled)
	}
}
