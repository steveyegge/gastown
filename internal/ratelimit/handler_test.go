package ratelimit

import (
	"context"
	"testing"
)

// MockSessionController implements SessionController for testing.
type MockSessionController struct {
	// Tracking fields
	IsRunningResult  bool
	StopCalled       bool
	StartCalled      bool
	StopError        error
	StartError       error
	HookedWork       string
	NudgedMessage    string
	StartedOpts      StartedOptions
	GetHookedWorkErr error
}

// StartedOptions tracks the options passed to Start.
type StartedOptions struct {
	RigName     string
	PolecatName string
	Account     string
}

func (m *MockSessionController) IsRunning(rigName, polecatName string) (bool, error) {
	return m.IsRunningResult, nil
}

func (m *MockSessionController) Stop(rigName, polecatName string, force bool) error {
	m.StopCalled = true
	return m.StopError
}

func (m *MockSessionController) Start(rigName, polecatName, profile string) (string, error) {
	m.StartCalled = true
	m.StartedOpts = StartedOptions{
		RigName:     rigName,
		PolecatName: polecatName,
		Account:     profile,
	}
	if m.StartError != nil {
		return "", m.StartError
	}
	return "test-session-id", nil
}

func (m *MockSessionController) GetHookedWork(rigName, polecatName string) (string, error) {
	return m.HookedWork, m.GetHookedWorkErr
}

func (m *MockSessionController) HookWork(rigName, polecatName, beadID string) error {
	m.HookedWork = beadID
	return nil
}

func (m *MockSessionController) Nudge(rigName, polecatName, message string) error {
	m.NudgedMessage = message
	return nil
}

func TestHandler_DetectsRateLimit_TriggersSwap(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		DefaultCooldownMinutes: 5,
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b", "profile-c"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2, // Rate limit exit code
		Stderr:         "",
		CurrentProfile: "profile-a",
		Provider:       "anthropic",
		HookedWork:     "issue-123",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	if !result.WasRateLimit {
		t.Error("expected rate limit to be detected")
	}
	if !result.SwapAttempted {
		t.Error("expected swap to be attempted")
	}
	if result.SwapResult == nil || !result.SwapResult.Success {
		t.Error("expected successful swap")
	}
	if mock.StartedOpts.Account != "profile-b" {
		t.Errorf("expected swap to profile-b, got %s", mock.StartedOpts.Account)
	}
}

func TestHandler_SwapSucceeds_WorkContinues(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2,
		CurrentProfile: "profile-a",
		HookedWork:     "issue-456",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	// Work should be re-hooked
	if mock.HookedWork != "issue-456" {
		t.Errorf("expected work to be re-hooked, got %s", mock.HookedWork)
	}

	// Session should be nudged
	if mock.NudgedMessage == "" {
		t.Error("expected nudge to be sent")
	}

	// Result should indicate success
	if !result.SwapResult.Success {
		t.Error("expected swap to succeed")
	}
}

func TestHandler_AllProfilesCooling_EmitsAlert(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	// First rate limit - should swap to profile-b
	exitInfo1 := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2,
		CurrentProfile: "profile-a",
	}
	h.HandlePolecatExit(context.Background(), exitInfo1)

	// Second rate limit on profile-b - all profiles now cooling
	exitInfo2 := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2,
		CurrentProfile: "profile-b",
	}
	result := h.HandlePolecatExit(context.Background(), exitInfo2)

	if !result.AllProfilesCooling {
		t.Error("expected all profiles to be cooling")
	}
	if result.SwapAttempted {
		t.Error("expected no swap when all profiles cooling")
	}
}

func TestHandler_NormalExit_NoAction(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       0, // Normal exit
		CurrentProfile: "profile-a",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	if result.WasRateLimit {
		t.Error("normal exit should not be detected as rate limit")
	}
	if result.SwapAttempted {
		t.Error("no swap should be attempted for normal exit")
	}
	if mock.StopCalled || mock.StartCalled {
		t.Error("no session operations should occur for normal exit")
	}
}

func TestHandler_StderrRateLimit_TriggersSwap(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       1,                                       // Generic error
		Stderr:         "Error: 429 Too Many Requests from API", // Rate limit in stderr
		CurrentProfile: "profile-a",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	if !result.WasRateLimit {
		t.Error("stderr rate limit should be detected")
	}
	if !result.SwapAttempted {
		t.Error("swap should be attempted for stderr rate limit")
	}
}

func TestHandler_SwapError_ReturnsError(t *testing.T) {
	mock := &MockSessionController{
		IsRunningResult: true,                 // Session is running, so Stop will be called
		StopError:       ErrAllProfilesCooling, // Simulate stop failure
	}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2,
		CurrentProfile: "profile-a",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	if result.Error == nil {
		t.Error("expected error when swap fails")
	}
	if result.SwapResult != nil && result.SwapResult.Success {
		t.Error("swap should not succeed when stop fails")
	}
}

func TestHandler_EventContainsContext(t *testing.T) {
	mock := &MockSessionController{}
	cfg := HandlerConfig{
		RolePolicies: map[string]RolePolicy{
			"polecat": {
				FallbackChain:   []string{"profile-a", "profile-b"},
				CooldownMinutes: 5,
			},
		},
	}

	h := NewHandler(mock, cfg)

	exitInfo := PolecatExitInfo{
		RigName:        "testrig",
		PolecatName:    "polecat-1",
		ExitCode:       2,
		Stderr:         "Rate limit exceeded",
		CurrentProfile: "profile-a",
		Provider:       "anthropic",
	}

	result := h.HandlePolecatExit(context.Background(), exitInfo)

	if result.Event == nil {
		t.Fatal("expected event to be set")
	}
	if result.Event.AgentID != "testrig/polecat-1" {
		t.Errorf("expected AgentID testrig/polecat-1, got %s", result.Event.AgentID)
	}
	if result.Event.Profile != "profile-a" {
		t.Errorf("expected Profile profile-a, got %s", result.Event.Profile)
	}
	if result.Event.Provider != "anthropic" {
		t.Errorf("expected Provider anthropic, got %s", result.Event.Provider)
	}
	if result.Event.ErrorSnippet == "" {
		t.Error("expected error snippet to be populated")
	}
}
