package cmd

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gastown/internal/agent"
)

// --- Parallel Start Helper Tests ---

func TestStartCrewParallel_AllSucceed(t *testing.T) {
	starter := func(rig, name string) error {
		return nil // Success
	}

	stats := startCrewParallel("testrig", []string{"alice", "bob", "charlie"}, starter)

	if stats.Started != 3 {
		t.Errorf("expected 3 started, got %d", stats.Started)
	}
	if stats.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", stats.Skipped)
	}
	if stats.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", stats.Failed)
	}
	if stats.LastErr != nil {
		t.Errorf("expected no error, got %v", stats.LastErr)
	}
}

func TestStartCrewParallel_SomeAlreadyRunning(t *testing.T) {
	callCount := int32(0)
	starter := func(rig, name string) error {
		count := atomic.AddInt32(&callCount, 1)
		if count == 2 {
			// Second call returns already running
			return agent.ErrAlreadyRunning
		}
		return nil
	}

	stats := startCrewParallel("testrig", []string{"alice", "bob", "charlie"}, starter)

	if stats.Started != 2 {
		t.Errorf("expected 2 started, got %d", stats.Started)
	}
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}
	if stats.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", stats.Failed)
	}
	if stats.LastErr != nil {
		t.Errorf("expected no error (skipped is not an error), got %v", stats.LastErr)
	}
}

func TestStartCrewParallel_OneFails_OthersSucceed(t *testing.T) {
	testErr := errors.New("start failed")
	callCount := int32(0)
	starter := func(rig, name string) error {
		count := atomic.AddInt32(&callCount, 1)
		if count == 2 {
			// Second call fails
			return testErr
		}
		return nil
	}

	stats := startCrewParallel("testrig", []string{"alice", "bob", "charlie"}, starter)

	if stats.Started != 2 {
		t.Errorf("expected 2 started, got %d", stats.Started)
	}
	if stats.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", stats.Skipped)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.LastErr == nil {
		t.Error("expected error, got nil")
	}
}

func TestStartCrewParallel_AllFail(t *testing.T) {
	testErr := errors.New("start failed")
	starter := func(rig, name string) error {
		return testErr
	}

	stats := startCrewParallel("testrig", []string{"alice", "bob"}, starter)

	if stats.Started != 0 {
		t.Errorf("expected 0 started, got %d", stats.Started)
	}
	if stats.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", stats.Failed)
	}
	if stats.LastErr == nil {
		t.Error("expected error, got nil")
	}
}

func TestStartCrewParallel_EmptyList(t *testing.T) {
	starter := func(rig, name string) error {
		t.Error("starter should not be called for empty list")
		return nil
	}

	stats := startCrewParallel("testrig", []string{}, starter)

	if stats.Started != 0 {
		t.Errorf("expected 0 started, got %d", stats.Started)
	}
	if stats.Skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", stats.Skipped)
	}
	if stats.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", stats.Failed)
	}
}

func TestStartCrewParallel_MixedResults(t *testing.T) {
	// Simulate: alice succeeds, bob already running, charlie fails
	testErr := errors.New("charlie failed")
	starter := func(rig, name string) error {
		switch name {
		case "alice":
			return nil
		case "bob":
			return agent.ErrAlreadyRunning
		case "charlie":
			return testErr
		}
		return nil
	}

	stats := startCrewParallel("testrig", []string{"alice", "bob", "charlie"}, starter)

	if stats.Started != 1 {
		t.Errorf("expected 1 started, got %d", stats.Started)
	}
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", stats.Skipped)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.LastErr == nil {
		t.Error("expected error from charlie, got nil")
	}
}
