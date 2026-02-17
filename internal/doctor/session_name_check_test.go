package doctor

import (
	"testing"
)

func TestNewMalformedSessionNameCheck(t *testing.T) {
	check := NewMalformedSessionNameCheck()

	if check.Name() != "session-name-format" {
		t.Errorf("expected name 'session-name-format', got %q", check.Name())
	}

	if check.Description() != "Detect sessions with outdated Gas Town naming format" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}

	if check.Category() != CategoryCleanup {
		t.Errorf("expected category %q, got %q", CategoryCleanup, check.Category())
	}
}

func TestMalformedSessionNameCheck_Run_NoSessions(t *testing.T) {
	check := NewMalformedSessionNameCheck()
	check.sessionListerForTest = &mockSessionLister{sessions: []string{}}

	ctx := &CheckContext{TownRoot: t.TempDir()}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected OK with no sessions, got %v: %s", result.Status, result.Message)
	}
}

func TestMalformedSessionNameCheck_Run_AllCorrect(t *testing.T) {
	// Sessions already in canonical format should produce OK
	check := NewMalformedSessionNameCheck()
	check.sessionListerForTest = &mockSessionLister{sessions: []string{
		"hq-mayor",
		"hq-deacon",
		"hq-boot",
		"wa-crew-batista",
	}}

	ctx := &CheckContext{TownRoot: t.TempDir()}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected OK for correctly-named sessions, got %v: %s\nDetails: %v",
			result.Status, result.Message, result.Details)
	}
}

func TestMalformedSessionNameCheck_Run_NonGasTownSessions(t *testing.T) {
	// Non-Gas Town sessions should be ignored
	check := NewMalformedSessionNameCheck()
	check.sessionListerForTest = &mockSessionLister{sessions: []string{
		"my-personal-session",
		"vim",
		"jupyter",
	}}

	ctx := &CheckContext{TownRoot: t.TempDir()}
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected OK for non-Gas Town sessions, got %v", result.Status)
	}
}
