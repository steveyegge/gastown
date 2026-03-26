package verify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func TestGatesForPhase(t *testing.T) {
	t.Parallel()

	mq := &config.MergeQueueConfig{
		Gates: map[string]*config.VerificationGateConfig{
			"post":   {Cmd: "echo post", Phase: config.MergeQueueGatePhasePostSquash},
			"verify": {Cmd: "./scripts/ci/verify.sh", Timeout: "2s"},
			"lint":   {Cmd: "make lint"},
		},
	}

	gates, err := GatesForPhase(mq, PhasePreMerge)
	if err != nil {
		t.Fatalf("GatesForPhase: %v", err)
	}
	if len(gates) != 2 {
		t.Fatalf("expected 2 pre-merge gates, got %d", len(gates))
	}
	if gates[0].Name != "lint" || gates[1].Name != "verify" {
		t.Fatalf("unexpected gate order: %+v", gates)
	}
	if gates[1].Timeout != 2*time.Second {
		t.Fatalf("verify timeout = %v, want 2s", gates[1].Timeout)
	}
}

func TestRunSequentialStopsOnFailure(t *testing.T) {
	t.Parallel()

	summary := Run(context.Background(), t.TempDir(), []Gate{
		{Name: "first", Cmd: "printf first"},
		{Name: "fail", Cmd: "echo boom >&2; exit 7"},
		{Name: "after", Cmd: "printf after"},
	}, false, nil)

	if summary.Success {
		t.Fatal("expected failure summary")
	}
	if len(summary.Results) != 2 {
		t.Fatalf("expected 2 executed gates, got %d", len(summary.Results))
	}
	if summary.Results[1].Name != "fail" {
		t.Fatalf("second result = %q, want fail", summary.Results[1].Name)
	}
	if !strings.Contains(summary.Error, "fail") {
		t.Fatalf("summary error = %q, want failing gate name", summary.Error)
	}
}

func TestRunTimeout(t *testing.T) {
	t.Parallel()

	summary := Run(context.Background(), t.TempDir(), []Gate{
		{Name: "slow", Cmd: "sleep 1", Timeout: 10 * time.Millisecond},
	}, false, nil)

	if summary.Success {
		t.Fatal("expected timeout failure")
	}
	if got := summary.Results[0].Error; !strings.Contains(got, "timed out") {
		t.Fatalf("gate error = %q, want timeout message", got)
	}
}
