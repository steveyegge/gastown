package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestWispTypeToCategory(t *testing.T) {
	tests := []struct {
		wispType string
		want     string
	}{
		{"heartbeat", "Heartbeats"},
		{"ping", "Heartbeats"},
		{"patrol", "Patrols"},
		{"gc_report", "Patrols"},
		{"error", "Errors"},
		{"recovery", "Errors"},
		{"escalation", "Errors"},
		{"", "Untyped"},
		{"unknown", "Untyped"},
		{"default", "Untyped"},
	}

	for _, tc := range tests {
		t.Run(tc.wispType, func(t *testing.T) {
			got := wispTypeToCategory(tc.wispType)
			if got != tc.want {
				t.Errorf("wispTypeToCategory(%q) = %q, want %q", tc.wispType, got, tc.want)
			}
		})
	}
}

func TestBuildReport(t *testing.T) {
	result := &compactResult{
		Deleted: []compactAction{
			{ID: "w-1", Title: "Heartbeat 1", WispType: "heartbeat"},
			{ID: "w-2", Title: "Heartbeat 2", WispType: "heartbeat"},
			{ID: "w-3", Title: "Patrol cycle", WispType: "patrol"},
		},
		Promoted: []compactAction{
			{ID: "w-4", Title: "Stuck error", WispType: "error", Reason: "open past TTL"},
		},
		Skipped: 5,
	}

	activeWisps := []*compactIssue{
		{Issue: beads.Issue{ID: "w-10"}, WispType: "heartbeat"},
		{Issue: beads.Issue{ID: "w-11"}, WispType: "patrol"},
		{Issue: beads.Issue{ID: "w-12"}, WispType: "patrol"},
		{Issue: beads.Issue{ID: "w-13"}, WispType: "error"},
	}

	report := buildReport("2026-02-09", result, activeWisps)

	if report.Date != "2026-02-09" {
		t.Errorf("Date = %q, want %q", report.Date, "2026-02-09")
	}

	// Check heartbeats category
	hb := report.Categories["Heartbeats"]
	if hb.Deleted != 2 {
		t.Errorf("Heartbeats.Deleted = %d, want 2", hb.Deleted)
	}
	if hb.Active != 1 {
		t.Errorf("Heartbeats.Active = %d, want 1", hb.Active)
	}

	// Check patrols category
	p := report.Categories["Patrols"]
	if p.Deleted != 1 {
		t.Errorf("Patrols.Deleted = %d, want 1", p.Deleted)
	}
	if p.Active != 2 {
		t.Errorf("Patrols.Active = %d, want 2", p.Active)
	}

	// Check errors category
	e := report.Categories["Errors"]
	if e.Promoted != 1 {
		t.Errorf("Errors.Promoted = %d, want 1", e.Promoted)
	}
	if e.Active != 1 {
		t.Errorf("Errors.Active = %d, want 1", e.Active)
	}

	// Check promotions list
	if len(report.Promotions) != 1 {
		t.Fatalf("len(Promotions) = %d, want 1", len(report.Promotions))
	}
	if report.Promotions[0].ID != "w-4" {
		t.Errorf("Promotions[0].ID = %q, want %q", report.Promotions[0].ID, "w-4")
	}
}

func TestDetectAnomalies(t *testing.T) {
	t.Run("high heartbeat volume", func(t *testing.T) {
		report := &compactReport{
			Categories: map[string]*categoryStats{
				"Heartbeats": {Deleted: 1500},
				"Patrols":    {Active: 5},
				"Errors":     {},
				"Untyped":    {},
			},
		}
		anomalies := detectAnomalies(report)
		found := false
		for _, a := range anomalies {
			if strings.Contains(a, "heartbeat volume") {
				found = true
			}
		}
		if !found {
			t.Error("expected heartbeat volume anomaly, got none")
		}
	})

	t.Run("zero patrols", func(t *testing.T) {
		report := &compactReport{
			Categories: map[string]*categoryStats{
				"Heartbeats": {Deleted: 100},
				"Patrols":    {Active: 0, Deleted: 0, Promoted: 0},
				"Errors":     {},
				"Untyped":    {},
			},
		}
		anomalies := detectAnomalies(report)
		found := false
		for _, a := range anomalies {
			if strings.Contains(a, "0 patrol wisps") {
				found = true
			}
		}
		if !found {
			t.Error("expected zero patrol anomaly, got none")
		}
	})

	t.Run("high promotion rate", func(t *testing.T) {
		report := &compactReport{
			Categories: map[string]*categoryStats{
				"Heartbeats": {Deleted: 3, Promoted: 15},
				"Patrols":    {Active: 5},
				"Errors":     {},
				"Untyped":    {},
			},
		}
		anomalies := detectAnomalies(report)
		found := false
		for _, a := range anomalies {
			if strings.Contains(a, "promotion rate") {
				found = true
			}
		}
		if !found {
			t.Error("expected high promotion rate anomaly, got none")
		}
	})

	t.Run("no anomalies", func(t *testing.T) {
		report := &compactReport{
			Categories: map[string]*categoryStats{
				"Heartbeats": {Deleted: 100, Active: 20},
				"Patrols":    {Active: 5, Deleted: 10},
				"Errors":     {Active: 2},
				"Untyped":    {},
			},
		}
		anomalies := detectAnomalies(report)
		if len(anomalies) != 0 {
			t.Errorf("expected no anomalies, got %v", anomalies)
		}
	})
}

func TestFormatDailyDigest(t *testing.T) {
	report := &compactReport{
		Date: "2026-02-09",
		Categories: map[string]*categoryStats{
			"Heartbeats": {Deleted: 2847, Promoted: 0, Active: 23},
			"Patrols":    {Deleted: 42, Promoted: 1, Active: 48},
			"Errors":     {Deleted: 2, Promoted: 3, Active: 7},
			"Untyped":    {Deleted: 15, Promoted: 0, Active: 4},
		},
		Promotions: []compactAction{
			{ID: "gt-wisp-abc", Title: "Polecat crash during convoy", Reason: "has comments"},
		},
		Anomalies: []string{"gastown: 3x normal heartbeat volume (possible restart loop)"},
	}

	md := formatDailyDigest(report)

	// Check structure
	if !strings.Contains(md, "## Wisp Compaction: 2026-02-09") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "### Summary") {
		t.Error("missing summary section")
	}
	if !strings.Contains(md, "| Heartbeats | 2847 | 0 | 23 |") {
		t.Error("missing heartbeats row")
	}
	if !strings.Contains(md, "### Promotions") {
		t.Error("missing promotions section")
	}
	if !strings.Contains(md, "gt-wisp-abc") {
		t.Error("missing promotion entry")
	}
	if !strings.Contains(md, "### Anomalies") {
		t.Error("missing anomalies section")
	}
	if !strings.Contains(md, "heartbeat volume") {
		t.Error("missing anomaly entry")
	}
}

func TestFormatDailyDigestEmpty(t *testing.T) {
	report := &compactReport{
		Date: "2026-02-09",
		Categories: map[string]*categoryStats{
			"Heartbeats": {},
			"Patrols":    {},
			"Errors":     {},
			"Untyped":    {},
		},
	}

	md := formatDailyDigest(report)

	// Should have header and summary but no promotions/anomalies sections
	if !strings.Contains(md, "## Wisp Compaction: 2026-02-09") {
		t.Error("missing header")
	}
	if strings.Contains(md, "### Promotions") {
		t.Error("should not have promotions section when empty")
	}
	if strings.Contains(md, "### Anomalies") {
		t.Error("should not have anomalies section when empty")
	}
}

func TestFormatWeeklyRollup(t *testing.T) {
	rollup := &weeklyRollup{
		WeekStart: "2026-02-02",
		WeekEnd:   "2026-02-09",
		Days:      7,
		Totals: map[string]*categoryStats{
			"Heartbeats": {Deleted: 15000, Promoted: 0, Active: 25},
			"Patrols":    {Deleted: 280, Promoted: 5, Active: 50},
			"Errors":     {Deleted: 10, Promoted: 8, Active: 3},
			"Untyped":    {Deleted: 90, Promoted: 2, Active: 6},
		},
		Promotions: 15,
		Anomalies:  []string{"high heartbeat volume on 2026-02-05"},
	}

	md := formatWeeklyRollup(rollup)

	if !strings.Contains(md, "## Weekly Wisp Compaction: 2026-02-02 to 2026-02-09") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "**Days reported:** 7") {
		t.Error("missing days count")
	}
	if !strings.Contains(md, "### Totals") {
		t.Error("missing totals section")
	}
	if !strings.Contains(md, "### Rates") {
		t.Error("missing rates section")
	}
	if !strings.Contains(md, "Promotion rate") {
		t.Error("missing promotion rate")
	}
	if !strings.Contains(md, "### Anomalies This Week") {
		t.Error("missing anomalies section")
	}
}
