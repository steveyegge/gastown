package beads

import (
	"os/exec"
	"testing"
)

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
