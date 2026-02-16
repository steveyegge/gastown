package cmd

import (
	"os"
	"testing"
)

func TestTapGuardTaskDispatch_BlocksWhenMayor(t *testing.T) {
	t.Setenv("GT_ROLE", "mayor")

	err := runTapGuardTaskDispatch(nil, nil)
	if err == nil {
		t.Fatal("expected error (exit 2) when GT_ROLE=mayor")
	}

	se, ok := err.(*SilentExitError)
	if !ok {
		t.Fatalf("expected SilentExitError, got %T: %v", err, err)
	}
	if se.Code != 2 {
		t.Errorf("expected exit code 2, got %d", se.Code)
	}
}

func TestTapGuardTaskDispatch_AllowsWhenNotMayor(t *testing.T) {
	os.Unsetenv("GT_ROLE")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error when GT_ROLE is not set, got %v", err)
	}
}

func TestTapGuardTaskDispatch_AllowsForCrew(t *testing.T) {
	t.Setenv("GT_ROLE", "crew")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for crew member, got %v", err)
	}
}

func TestTapGuardTaskDispatch_AllowsForPolecat(t *testing.T) {
	t.Setenv("GT_ROLE", "polecat")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for polecat, got %v", err)
	}
}
