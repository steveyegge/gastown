package cli

import (
	"sync"
	"testing"
)

func TestName_DefaultIsGt(t *testing.T) {
	// Reset singleton for test isolation
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("GT_COMMAND", "")

	got := Name()
	if got != "gt" {
		t.Errorf("Name() = %q, want %q", got, "gt")
	}
}

func TestName_RespectsGT_COMMAND(t *testing.T) {
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("GT_COMMAND", "gastown")

	got := Name()
	if got != "gastown" {
		t.Errorf("Name() = %q, want %q", got, "gastown")
	}
}

func TestName_OnceSemantics(t *testing.T) {
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("GT_COMMAND", "first")

	first := Name()
	if first != "first" {
		t.Fatalf("Name() = %q, want %q", first, "first")
	}

	// Changing env after first call should have no effect (sync.Once)
	t.Setenv("GT_COMMAND", "second")
	second := Name()
	if second != "first" {
		t.Errorf("Name() returned %q after env change, want %q (sync.Once should cache)", second, "first")
	}
}
