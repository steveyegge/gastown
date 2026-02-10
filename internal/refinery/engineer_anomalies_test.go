package refinery

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestDetectQueueAnomalies_StaleClaimSeverity(t *testing.T) {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	issues := []*beads.Issue{
		{
			ID:        "gt-warn",
			Status:    "open",
			Assignee:  "rig/refinery-1",
			UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339),
			Description: `branch: polecat/warn
target: main
worker: nux`,
		},
		{
			ID:        "gt-critical",
			Status:    "open",
			Assignee:  "rig/refinery-2",
			UpdatedAt: now.Add(-7 * time.Hour).Format(time.RFC3339),
			Description: `branch: polecat/critical
target: main
worker: nux`,
		},
	}

	anomalies := detectQueueAnomalies(issues, now, func(branch string) (bool, bool, error) {
		return true, false, nil
	})

	if len(anomalies) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(anomalies))
	}
	if anomalies[0].Type != "stale-claim" || anomalies[1].Type != "stale-claim" {
		t.Fatalf("expected stale-claim anomalies, got %+v", anomalies)
	}

	got := map[string]string{}
	for _, a := range anomalies {
		got[a.ID] = a.Severity
	}
	if got["gt-warn"] != "warning" {
		t.Fatalf("gt-warn severity = %q, want warning", got["gt-warn"])
	}
	if got["gt-critical"] != "critical" {
		t.Fatalf("gt-critical severity = %q, want critical", got["gt-critical"])
	}
}

func TestDetectQueueAnomalies_OrphanedBranch(t *testing.T) {
	now := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	issues := []*beads.Issue{
		{
			ID:        "gt-orphan",
			Status:    "open",
			UpdatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			Description: `branch: polecat/orphan
target: main
worker: nux`,
		},
		{
			ID:        "gt-ok",
			Status:    "open",
			UpdatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			Description: `branch: polecat/ok
target: main
worker: nux`,
		},
	}

	anomalies := detectQueueAnomalies(issues, now, func(branch string) (bool, bool, error) {
		if branch == "polecat/orphan" {
			return false, false, nil
		}
		return false, true, nil
	})

	if len(anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d (%+v)", len(anomalies), anomalies)
	}
	if anomalies[0].Type != "orphaned-branch" {
		t.Fatalf("anomaly type = %q, want orphaned-branch", anomalies[0].Type)
	}
	if anomalies[0].Severity != "critical" {
		t.Fatalf("anomaly severity = %q, want critical", anomalies[0].Severity)
	}
	if anomalies[0].ID != "gt-orphan" {
		t.Fatalf("anomaly ID = %q, want gt-orphan", anomalies[0].ID)
	}
}
