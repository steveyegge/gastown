package krc

import (
	"math"
	"testing"
	"time"
)

func TestForensicScore_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		age       time.Duration
		ttl       time.Duration
		want      float64
	}{
		{"zero age = full value", "patrol_started", 0, 24 * time.Hour, 1.0},
		{"at TTL = zero value", "patrol_started", 24 * time.Hour, 24 * time.Hour, 0.0},
		{"past TTL = zero value", "patrol_started", 48 * time.Hour, 24 * time.Hour, 0.0},
		{"negative age = full value", "patrol_started", -1 * time.Hour, 24 * time.Hour, 1.0},
		{"zero TTL = full value", "patrol_started", 1 * time.Hour, 0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForensicScore(tt.eventType, tt.age, tt.ttl)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("ForensicScore(%q, %v, %v) = %f, want %f", tt.eventType, tt.age, tt.ttl, got, tt.want)
			}
		})
	}
}

func TestForensicScore_RapidDecay(t *testing.T) {
	ttl := 24 * time.Hour

	// Rapid decay: heartbeats/pings lose value fast
	score25 := ForensicScore("heartbeat", 6*time.Hour, ttl)  // 25% of TTL
	score50 := ForensicScore("heartbeat", 12*time.Hour, ttl) // 50% of TTL
	score75 := ForensicScore("heartbeat", 18*time.Hour, ttl) // 75% of TTL

	// At 25% of TTL, rapid decay should be at half-life (~0.5)
	if math.Abs(score25-0.5) > 0.05 {
		t.Errorf("rapid decay at 25%% TTL = %f, want ~0.5", score25)
	}

	// Should decrease monotonically
	if score50 >= score25 || score75 >= score50 {
		t.Errorf("rapid decay not monotonically decreasing: %f, %f, %f", score25, score50, score75)
	}

	// At 75% of TTL, should be very low
	if score75 > 0.15 {
		t.Errorf("rapid decay at 75%% TTL = %f, want < 0.15", score75)
	}
}

func TestForensicScore_SteadyDecay(t *testing.T) {
	ttl := 24 * time.Hour

	// Steady = linear
	score50 := ForensicScore("session_start", 12*time.Hour, ttl)
	if math.Abs(score50-0.5) > 0.01 {
		t.Errorf("steady decay at 50%% TTL = %f, want 0.5", score50)
	}

	score25 := ForensicScore("session_start", 6*time.Hour, ttl)
	if math.Abs(score25-0.75) > 0.01 {
		t.Errorf("steady decay at 25%% TTL = %f, want 0.75", score25)
	}
}

func TestForensicScore_SlowDecay(t *testing.T) {
	ttl := 14 * 24 * time.Hour

	// Slow decay: errors/escalations retain value longer
	score50 := ForensicScore("error", 7*24*time.Hour, ttl) // 50% of TTL

	// At 50% TTL, slow decay should still have significant value
	if score50 < 0.5 {
		t.Errorf("slow decay at 50%% TTL = %f, want > 0.5", score50)
	}

	// At 75% TTL, should still have some value
	score75 := ForensicScore("error", time.Duration(float64(ttl)*0.75), ttl)
	if score75 < 0.3 {
		t.Errorf("slow decay at 75%% TTL = %f, want > 0.3", score75)
	}
}

func TestForensicScore_FlatDecay(t *testing.T) {
	ttl := 30 * 24 * time.Hour

	// Flat decay: mail retains full value until near TTL
	score50 := ForensicScore("mail", 15*24*time.Hour, ttl)
	if score50 < 0.99 {
		t.Errorf("flat decay at 50%% TTL = %f, want ~1.0", score50)
	}

	score80 := ForensicScore("mail", 24*24*time.Hour, ttl) // 80% TTL
	if score80 < 0.99 {
		t.Errorf("flat decay at 80%% TTL = %f, want ~1.0", score80)
	}

	// At 95% TTL, should be dropping
	score95 := ForensicScore("mail", time.Duration(float64(ttl)*0.95), ttl)
	if score95 > 0.6 {
		t.Errorf("flat decay at 95%% TTL = %f, want < 0.6", score95)
	}
}

func TestForensicScore_GlobMatch(t *testing.T) {
	ttl := 24 * time.Hour
	age := 6 * time.Hour // 25% of TTL

	// patrol_started should match patrol_* -> rapid decay
	patrolScore := ForensicScore("patrol_started", age, ttl)
	heartbeatScore := ForensicScore("heartbeat", age, ttl)

	// Both are rapid decay, should be similar
	if math.Abs(patrolScore-heartbeatScore) > 0.01 {
		t.Errorf("patrol_started (%f) and heartbeat (%f) should have same decay curve", patrolScore, heartbeatScore)
	}

	// merge_started should match merge_* -> flat decay
	mergeScore := ForensicScore("merge_started", age, ttl)
	if mergeScore < 0.99 {
		t.Errorf("merge_started at 25%% TTL = %f, want ~1.0 (flat decay)", mergeScore)
	}
}

func TestForensicScore_UnknownType(t *testing.T) {
	ttl := 24 * time.Hour

	// Unknown types get steady (linear) decay
	score := ForensicScore("unknown_event", 12*time.Hour, ttl)
	if math.Abs(score-0.5) > 0.01 {
		t.Errorf("unknown event at 50%% TTL = %f, want 0.5 (steady/linear)", score)
	}
}

func TestGetDecayCurve(t *testing.T) {
	tests := []struct {
		eventType string
		want      DecayCurve
	}{
		{"heartbeat", DecayRapid},
		{"patrol_started", DecayRapid},
		{"patrol_complete", DecayRapid},
		{"session_start", DecaySteady},
		{"error", DecaySlow},
		{"mail", DecayFlat},
		{"merge_started", DecayFlat},
		{"unknown", DecaySteady}, // default
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			got := getDecayCurve(tt.eventType)
			if got != tt.want {
				t.Errorf("getDecayCurve(%q) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestCurveToString(t *testing.T) {
	tests := []struct {
		curve DecayCurve
		want  string
	}{
		{DecayRapid, "rapid"},
		{DecaySteady, "steady"},
		{DecaySlow, "slow"},
		{DecayFlat, "flat"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := curveToString(tt.curve)
			if got != tt.want {
				t.Errorf("curveToString(%d) = %q, want %q", tt.curve, got, tt.want)
			}
		})
	}
}

func TestGenerateDecayReport(t *testing.T) {
	config := DefaultConfig()
	stats := &Stats{
		TTLBreakdown: map[string]TTLInfo{
			"patrol_started": {
				TTL:       24 * time.Hour,
				Count:     10,
				Expired:   3,
				ExpiresIn: 6 * time.Hour,
			},
			"mail": {
				TTL:       30 * 24 * time.Hour,
				Count:     5,
				Expired:   0,
				ExpiresIn: 20 * 24 * time.Hour,
			},
		},
	}

	report := GenerateDecayReport(stats, config)

	if report.TotalEvents != 15 {
		t.Errorf("TotalEvents = %d, want 15", report.TotalEvents)
	}

	if report.Expired != 3 {
		t.Errorf("Expired = %d, want 3", report.Expired)
	}

	if len(report.Types) != 2 {
		t.Errorf("Types count = %d, want 2", len(report.Types))
	}

	// Report should be sorted by avg score ascending
	if len(report.Types) == 2 {
		if report.Types[0].AvgScore > report.Types[1].AvgScore {
			t.Errorf("Types not sorted by AvgScore ascending: %f > %f",
				report.Types[0].AvgScore, report.Types[1].AvgScore)
		}
	}
}
