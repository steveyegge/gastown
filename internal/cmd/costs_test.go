package cmd

import (
	"encoding/json"
	"os"
	"testing"
	"time"

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
			saved := make(map[string]string)
			envKeys := []string{"GT_ROLE", "GT_RIG", "GT_POLECAT", "GT_CREW", "GT_TOWN"}
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

	oldSession := recordSession
	recordSession = ""
	defer func() { recordSession = oldSession }()

	err := runCostsRecord(nil, nil)
	if err != nil {
		t.Errorf("runCostsRecord() returned error %v, want nil for non-GT session", err)
	}
}

func TestCostDigestPayload_ExcludesSessions(t *testing.T) {
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

	fullJSON, err := json.Marshal(digest)
	if err != nil {
		t.Fatalf("marshaling full digest: %v", err)
	}

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

	if len(compactJSON) >= len(fullJSON) {
		t.Errorf("compact payload (%d bytes) should be smaller than full digest (%d bytes)",
			len(compactJSON), len(fullJSON))
	}

	if len(compactJSON) > 1024 {
		t.Errorf("compact payload is %d bytes, expected under 1024", len(compactJSON))
	}

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

func TestBuildAgentPath(t *testing.T) {
	tests := []struct {
		name string
		role string
		rig  string
		id   string
		want string
	}{
		{name: "mayor", role: "mayor", want: "mayor"},
		{name: "deacon", role: "deacon", want: "deacon"},
		{name: "witness", role: "witness", rig: "gastown", want: "gastown/witness"},
		{name: "refinery", role: "refinery", rig: "gastown", want: "gastown/refinery"},
		{name: "crew", role: "crew", rig: "gastown", id: "max", want: "gastown/crew/max"},
		{name: "polecat", role: "polecat", rig: "gastown", id: "toast", want: "gastown/polecats/toast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAgentPath(tt.role, tt.rig, tt.id); got != tt.want {
				t.Fatalf("buildAgentPath(%q, %q, %q) = %q, want %q", tt.role, tt.rig, tt.id, got, tt.want)
			}
		})
	}
}
