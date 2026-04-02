package dbmon

import (
	"database/sql"
	"testing"
	"time"
)

func TestIsOrphanDatabase(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"testdb_abc", true},
		{"beads_t123", true},
		{"beads_pt456", true},
		{"doctest_xyz", true},
		{"production", false},
		{"faultline", false},
		{"beads", false},
		{"information_schema", false},
		{"test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOrphanDatabase(tt.name)
			if got != tt.want {
				t.Errorf("isOrphanDatabase(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestParseDoltThresholds_Default(t *testing.T) {
	th := parseDoltThresholds(sql.NullString{})
	if th.commitLagWarning() != 5*time.Minute {
		t.Errorf("expected default 5m, got %v", th.commitLagWarning())
	}
}

func TestParseDoltThresholds_Custom(t *testing.T) {
	th := parseDoltThresholds(sql.NullString{
		String: `{"commit_lag_warning_s": 120}`,
		Valid:  true,
	})
	if th.commitLagWarning() != 2*time.Minute {
		t.Errorf("expected 2m, got %v", th.commitLagWarning())
	}
}

func TestParseDoltThresholds_InvalidJSON(t *testing.T) {
	th := parseDoltThresholds(sql.NullString{
		String: `{invalid`,
		Valid:  true,
	})
	// Should fall back to defaults.
	if th.commitLagWarning() != 5*time.Minute {
		t.Errorf("expected default 5m on invalid JSON, got %v", th.commitLagWarning())
	}
}

func TestCheckDolt_BadConnectionString(t *testing.T) {
	target := DatabaseTarget{
		ID:               "test-dolt-1",
		Name:             "test-dolt",
		DBType:           "dolt",
		ConnectionString: "invalid://not-a-dsn",
		Enabled:          true,
		CheckIntervalS:   60,
	}

	// sql.Open with mysql driver won't fail on open, but the individual
	// checks will fail on ping/query. We verify that CheckDolt returns
	// results with critical status for connection failure.
	results := CheckDolt(t.Context(), target)
	if len(results) == 0 {
		t.Fatal("expected at least one result from CheckDolt")
	}

	// The first result should be the connection check.
	found := false
	for _, r := range results {
		if r.CheckType == "connection" && r.Status == CheckCritical {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a critical connection check result for bad DSN")
	}
}

func TestCheckDolt_IntegrationWithMonitor(t *testing.T) {
	// Verify CheckDolt has the correct signature for RegisterChecker.
	provider := newMockProvider([]DatabaseTarget{
		{ID: "dolt-1", Name: "test-dolt", DBType: "dolt", Enabled: true, CheckIntervalS: 1,
			ConnectionString: "invalid:3307"},
	})

	m := New(provider, testLogger(), 5*time.Second)
	m.RegisterChecker("dolt", CheckDolt)

	// Just verify the checker is registered and callable (will fail on connect, that's fine).
	m.tick(t.Context())

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("expected check results after tick with registered dolt checker")
	}

	// All results should reference the correct database.
	for _, c := range checks {
		if c.DatabaseID != "dolt-1" {
			t.Errorf("expected DatabaseID dolt-1, got %s", c.DatabaseID)
		}
	}
}
