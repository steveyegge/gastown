package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/session"
)

func setupCostsTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("bd", "beads")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func setupCostsFlagState(t *testing.T) *cobra.Command {
	t.Helper()

	oldJSON := costsJSON
	oldToday := costsToday
	oldWeek := costsWeek
	oldSince := costsSince
	oldHours := costsHours
	oldByRole := costsByRole
	oldByRig := costsByRig
	oldVerbose := costsVerbose

	costsJSON = false
	costsToday = false
	costsWeek = false
	costsSince = ""
	costsHours = 0
	costsByRole = false
	costsByRig = false
	costsVerbose = false

	t.Cleanup(func() {
		costsJSON = oldJSON
		costsToday = oldToday
		costsWeek = oldWeek
		costsSince = oldSince
		costsHours = oldHours
		costsByRole = oldByRole
		costsByRig = oldByRig
		costsVerbose = oldVerbose
	})

	cmd := &cobra.Command{Use: "costs-test"}
	cmd.Flags().IntVar(&costsHours, "hours", 0, "")
	return cmd
}

func TestDeriveSessionName(t *testing.T) {
	setupCostsTestRegistry(t)
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "polecat",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-toast",
		},
		{
			name: "crew session",
			envVars: map[string]string{
				"GT_ROLE": "crew",
				"GT_RIG":  "gastown",
				"GT_CREW": "max",
			},
			expected: "gt-crew-max",
		},
		{
			name: "witness session",
			envVars: map[string]string{
				"GT_ROLE": "witness",
				"GT_RIG":  "gastown",
			},
			expected: "gt-witness",
		},
		{
			name: "refinery session",
			envVars: map[string]string{
				"GT_ROLE": "refinery",
				"GT_RIG":  "gastown",
			},
			expected: "gt-refinery",
		},
		{
			name: "mayor session",
			envVars: map[string]string{
				"GT_ROLE": "mayor",
				"GT_TOWN": "ai",
			},
			expected: "hq-mayor",
		},
		{
			name: "deacon session",
			envVars: map[string]string{
				"GT_ROLE": "deacon",
				"GT_TOWN": "ai",
			},
			expected: "hq-deacon",
		},
		{
			name: "mayor session without GT_TOWN",
			envVars: map[string]string{
				"GT_ROLE": "mayor",
			},
			expected: "hq-mayor",
		},
		{
			name: "deacon session without GT_TOWN",
			envVars: map[string]string{
				"GT_ROLE": "deacon",
			},
			expected: "hq-deacon",
		},
		{
			name: "mayor with stale GT_POLECAT is NOT polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "mayor",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
				"GT_TOWN":    "ai",
			},
			expected: "hq-mayor",
		},
		{
			name: "compound witness with stale GT_POLECAT is NOT polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "gastown/witness",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-witness",
		},
		{
			name: "compound refinery with stale GT_POLECAT is NOT polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "gastown/refinery",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-refinery",
		},
		{
			name: "compound crew with stale GT_POLECAT is NOT polecat session",
			envVars: map[string]string{
				"GT_ROLE":    "gastown/crew/alice",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-crew-alice",
		},
		{
			name: "compound polecat role uses GT_POLECAT for session name",
			envVars: map[string]string{
				"GT_ROLE":    "gastown/polecats/toast",
				"GT_RIG":     "gastown",
				"GT_POLECAT": "toast",
			},
			expected: "gt-toast",
		},
		{
			name:     "no env vars",
			envVars:  map[string]string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear relevant env vars
			saved := make(map[string]string)
			envKeys := []string{"GT_ROLE", "GT_RIG", "GT_POLECAT", "GT_CREW", "GT_TOWN"}
			for _, key := range envKeys {
				saved[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			defer func() {
				// Restore env vars
				for key, val := range saved {
					if val != "" {
						os.Setenv(key, val)
					}
				}
			}()

			// Set test env vars
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			result := deriveSessionName()
			if result != tt.expected {
				t.Errorf("deriveSessionName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRunCostsRecord_NoSession_ReturnsNil(t *testing.T) {
	// Clear all session-related env vars so no session can be derived.
	envKeys := []string{"GT_SESSION", "GT_ROLE", "GT_RIG", "GT_POLECAT", "GT_CREW", "GT_TOWN"}
	saved := make(map[string]string)
	for _, key := range envKeys {
		saved[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for key, val := range saved {
			if val != "" {
				os.Setenv(key, val)
			}
		}
	}()

	// Clear the flag-based session too
	oldSession := recordSession
	recordSession = ""
	defer func() { recordSession = oldSession }()

	// runCostsRecord should return nil (silent skip) when no session is resolvable
	err := runCostsRecord(nil, nil)
	if err != nil {
		t.Errorf("runCostsRecord() returned error %v, want nil for non-GT session", err)
	}
}

func TestCostDigestPayload_ExcludesSessions(t *testing.T) {
	// Build a digest with many sessions (simulating the 2885-session case)
	digest := CostDigest{
		Date:         "2026-02-14",
		TotalUSD:     694.25,
		SessionCount: 2885,
		Sessions:     make([]CostEntry, 2885),
		ByRole: map[string]float64{
			"polecat": 500.0,
			"witness": 100.0,
			"mayor":   94.25,
		},
		ByRig: map[string]float64{
			"gastown": 600.0,
			"beads":   94.25,
		},
	}

	// Fill sessions with realistic data
	for i := range digest.Sessions {
		digest.Sessions[i] = CostEntry{
			SessionID: "gt-session-" + time.Now().Format("150405"),
			Role:      "polecat",
			Rig:       "gastown",
			Worker:    "toast",
			CostUSD:   0.24,
			EndedAt:   time.Now(),
		}
	}

	// Marshal full digest (old format) - should be very large
	fullJSON, err := json.Marshal(digest)
	if err != nil {
		t.Fatalf("marshaling full digest: %v", err)
	}

	// Marshal compact payload (new format) - should be small
	compact := CostDigestPayload{
		Date:         digest.Date,
		TotalUSD:     digest.TotalUSD,
		SessionCount: digest.SessionCount,
		ByRole:       digest.ByRole,
		ByRig:        digest.ByRig,
	}
	compactJSON, err := json.Marshal(compact)
	if err != nil {
		t.Fatalf("marshaling compact payload: %v", err)
	}

	// Compact payload should be dramatically smaller
	if len(compactJSON) >= len(fullJSON) {
		t.Errorf("compact payload (%d bytes) should be smaller than full digest (%d bytes)",
			len(compactJSON), len(fullJSON))
	}

	// Compact payload should be under 1KB (well within Dolt limits)
	if len(compactJSON) > 1024 {
		t.Errorf("compact payload is %d bytes, expected under 1024", len(compactJSON))
	}

	// Verify compact payload round-trips correctly
	var decoded CostDigestPayload
	if err := json.Unmarshal(compactJSON, &decoded); err != nil {
		t.Fatalf("unmarshaling compact payload: %v", err)
	}
	if decoded.Date != digest.Date {
		t.Errorf("date = %q, want %q", decoded.Date, digest.Date)
	}
	if decoded.TotalUSD != digest.TotalUSD {
		t.Errorf("total = %.2f, want %.2f", decoded.TotalUSD, digest.TotalUSD)
	}
	if decoded.SessionCount != digest.SessionCount {
		t.Errorf("session_count = %d, want %d", decoded.SessionCount, digest.SessionCount)
	}

	// Verify compact payload can be decoded as a CostDigest (backwards compat)
	var asDigest CostDigest
	if err := json.Unmarshal(compactJSON, &asDigest); err != nil {
		t.Fatalf("unmarshaling compact payload as CostDigest: %v", err)
	}
	if len(asDigest.Sessions) != 0 {
		t.Errorf("compact payload decoded as CostDigest should have 0 sessions, got %d", len(asDigest.Sessions))
	}
	if asDigest.TotalUSD != digest.TotalUSD {
		t.Errorf("total = %.2f, want %.2f", asDigest.TotalUSD, digest.TotalUSD)
	}
	if len(asDigest.ByRole) != 3 {
		t.Errorf("by_role should have 3 entries, got %d", len(asDigest.ByRole))
	}
}

func TestResolveCostsLookback_Since(t *testing.T) {
	cmd := setupCostsFlagState(t)
	costsSince = "30m"

	now := time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC)
	cutoff, period, hasLookback, err := resolveCostsLookback(cmd, now)
	if err != nil {
		t.Fatalf("resolveCostsLookback returned error: %v", err)
	}
	if !hasLookback {
		t.Fatalf("expected hasLookback=true")
	}
	if !cutoff.Equal(now.Add(-30 * time.Minute)) {
		t.Fatalf("cutoff = %s, want %s", cutoff, now.Add(-30*time.Minute))
	}
	if period != "last 30m" {
		t.Fatalf("period = %q, want %q", period, "last 30m")
	}
}

func TestResolveCostsLookback_Hours(t *testing.T) {
	cmd := setupCostsFlagState(t)
	if err := cmd.Flags().Set("hours", "4"); err != nil {
		t.Fatalf("setting --hours: %v", err)
	}

	now := time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC)
	cutoff, period, hasLookback, err := resolveCostsLookback(cmd, now)
	if err != nil {
		t.Fatalf("resolveCostsLookback returned error: %v", err)
	}
	if !hasLookback {
		t.Fatalf("expected hasLookback=true")
	}
	if !cutoff.Equal(now.Add(-4 * time.Hour)) {
		t.Fatalf("cutoff = %s, want %s", cutoff, now.Add(-4*time.Hour))
	}
	if period != "last 4h" {
		t.Fatalf("period = %q, want %q", period, "last 4h")
	}
}

func TestResolveCostsLookback_MutualExclusion(t *testing.T) {
	cmd := setupCostsFlagState(t)
	costsSince = "1h"
	if err := cmd.Flags().Set("hours", "2"); err != nil {
		t.Fatalf("setting --hours: %v", err)
	}

	_, _, _, err := resolveCostsLookback(cmd, time.Now())
	if err == nil {
		t.Fatal("expected error for --since with --hours")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %q, expected mutually exclusive", err.Error())
	}
}

func TestQuerySessionCostEntriesSince(t *testing.T) {
	setupCostsFlagState(t)

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	logDir := filepath.Join(homeDir, ".gt")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("creating log dir: %v", err)
	}

	now := time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-1 * time.Hour)
	logEntries := []CostLogEntry{
		{SessionID: "s-old", Role: "crew", CostUSD: 1.0, EndedAt: now.Add(-90 * time.Minute)},
		{SessionID: "s-boundary", Role: "witness", CostUSD: 2.0, EndedAt: cutoff},
		{SessionID: "s-new", Role: "polecat", CostUSD: 3.0, EndedAt: now.Add(-5 * time.Minute)},
	}

	var lines []string
	for _, entry := range logEntries {
		b, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("marshaling entry: %v", err)
		}
		lines = append(lines, string(b))
	}
	lines = append(lines, "{not-json")

	logPath := filepath.Join(logDir, "costs.jsonl")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing log file: %v", err)
	}

	entries, err := querySessionCostEntriesSince(cutoff)
	if err != nil {
		t.Fatalf("querySessionCostEntriesSince returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	got := map[string]bool{}
	for _, entry := range entries {
		got[entry.SessionID] = true
	}
	if !got["s-boundary"] || !got["s-new"] {
		t.Fatalf("expected s-boundary and s-new, got %+v", got)
	}
	if got["s-old"] {
		t.Fatalf("did not expect s-old in results")
	}
}
