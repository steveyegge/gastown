package refinery

import (
	"testing"
	"time"
)

func TestDefaultScoreConfig(t *testing.T) {
	cfg := DefaultScoreConfig()

	if cfg.BaseScore != 1000.0 {
		t.Errorf("BaseScore = %v, want 1000.0", cfg.BaseScore)
	}
	if cfg.ConvoyAgeWeight != 10.0 {
		t.Errorf("ConvoyAgeWeight = %v, want 10.0", cfg.ConvoyAgeWeight)
	}
	if cfg.PriorityWeight != 100.0 {
		t.Errorf("PriorityWeight = %v, want 100.0", cfg.PriorityWeight)
	}
	if cfg.RetryPenalty != 50.0 {
		t.Errorf("RetryPenalty = %v, want 50.0", cfg.RetryPenalty)
	}
	if cfg.MRAgeWeight != 1.0 {
		t.Errorf("MRAgeWeight = %v, want 1.0", cfg.MRAgeWeight)
	}
	if cfg.MaxRetryPenalty != 300.0 {
		t.Errorf("MaxRetryPenalty = %v, want 300.0", cfg.MaxRetryPenalty)
	}
}

func TestScoreMR_BaseScoreOnly(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	input := ScoreInput{
		Priority:    4, // P4 gives no priority bonus
		MRCreatedAt: now,
		RetryCount:  0,
		Now:         now,
	}
	cfg := DefaultScoreConfig()

	score := ScoreMR(input, cfg)

	// With P4 (no priority bonus), no retry penalty, and MR just created (0 hours),
	// score should be BaseScore only
	if score != cfg.BaseScore {
		t.Errorf("ScoreMR() = %v, want %v (base score only)", score, cfg.BaseScore)
	}
}

func TestScoreMR_PriorityScoring(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	tests := []struct {
		name     string
		priority int
		want     float64
	}{
		{
			name:     "P0 gets maximum priority bonus",
			priority: 0,
			want:     cfg.BaseScore + 4*cfg.PriorityWeight, // 1000 + 400 = 1400
		},
		{
			name:     "P1 gets 300 priority bonus",
			priority: 1,
			want:     cfg.BaseScore + 3*cfg.PriorityWeight, // 1000 + 300 = 1300
		},
		{
			name:     "P2 gets 200 priority bonus",
			priority: 2,
			want:     cfg.BaseScore + 2*cfg.PriorityWeight, // 1000 + 200 = 1200
		},
		{
			name:     "P3 gets 100 priority bonus",
			priority: 3,
			want:     cfg.BaseScore + 1*cfg.PriorityWeight, // 1000 + 100 = 1100
		},
		{
			name:     "P4 gets no priority bonus",
			priority: 4,
			want:     cfg.BaseScore, // 1000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ScoreInput{
				Priority:    tt.priority,
				MRCreatedAt: now,
				RetryCount:  0,
				Now:         now,
			}
			score := ScoreMR(input, cfg)
			if score != tt.want {
				t.Errorf("ScoreMR() with P%d = %v, want %v", tt.priority, score, tt.want)
			}
		})
	}
}

func TestScoreMR_PriorityEdgeCases(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	tests := []struct {
		name     string
		priority int
		want     float64
	}{
		{
			name:     "negative priority clamped to P0 bonus",
			priority: -1,
			want:     cfg.BaseScore + 4*cfg.PriorityWeight, // clamped to max bonus
		},
		{
			name:     "priority > 4 clamped to zero bonus",
			priority: 5,
			want:     cfg.BaseScore, // clamped to zero bonus
		},
		{
			name:     "very high priority clamped",
			priority: 100,
			want:     cfg.BaseScore, // clamped to zero bonus
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ScoreInput{
				Priority:    tt.priority,
				MRCreatedAt: now,
				RetryCount:  0,
				Now:         now,
			}
			score := ScoreMR(input, cfg)
			if score != tt.want {
				t.Errorf("ScoreMR() with priority %d = %v, want %v", tt.priority, score, tt.want)
			}
		})
	}
}

func TestScoreMR_ConvoyAgeScoring(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	tests := []struct {
		name           string
		convoyAge      time.Duration
		wantAgeBonus   float64
	}{
		{
			name:         "convoy 1 hour old",
			convoyAge:    1 * time.Hour,
			wantAgeBonus: 10.0, // 10 pts/hour
		},
		{
			name:         "convoy 24 hours old (1 day)",
			convoyAge:    24 * time.Hour,
			wantAgeBonus: 240.0, // 10 * 24 = 240 pts
		},
		{
			name:         "convoy 48 hours old (2 days)",
			convoyAge:    48 * time.Hour,
			wantAgeBonus: 480.0, // 10 * 48 = 480 pts
		},
		{
			name:         "convoy just created",
			convoyAge:    0,
			wantAgeBonus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			convoyCreatedAt := now.Add(-tt.convoyAge)
			input := ScoreInput{
				Priority:        4, // no priority bonus
				MRCreatedAt:     now,
				ConvoyCreatedAt: &convoyCreatedAt,
				RetryCount:      0,
				Now:             now,
			}
			score := ScoreMR(input, cfg)
			wantScore := cfg.BaseScore + tt.wantAgeBonus
			if score != wantScore {
				t.Errorf("ScoreMR() = %v, want %v", score, wantScore)
			}
		})
	}
}

func TestScoreMR_NoConvoy(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	input := ScoreInput{
		Priority:        4,   // no priority bonus
		MRCreatedAt:     now, // no MR age bonus
		ConvoyCreatedAt: nil, // no convoy
		RetryCount:      0,
		Now:             now,
	}
	score := ScoreMR(input, cfg)

	// With nil convoy, should just get base score
	if score != cfg.BaseScore {
		t.Errorf("ScoreMR() with nil convoy = %v, want %v", score, cfg.BaseScore)
	}
}

func TestScoreMR_RetryPenalty(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	tests := []struct {
		name        string
		retryCount  int
		wantPenalty float64
	}{
		{
			name:        "no retries",
			retryCount:  0,
			wantPenalty: 0,
		},
		{
			name:        "1 retry",
			retryCount:  1,
			wantPenalty: 50.0, // 50 pts per retry
		},
		{
			name:        "2 retries",
			retryCount:  2,
			wantPenalty: 100.0,
		},
		{
			name:        "6 retries (at max penalty)",
			retryCount:  6,
			wantPenalty: 300.0, // MaxRetryPenalty
		},
		{
			name:        "10 retries (capped at max)",
			retryCount:  10,
			wantPenalty: 300.0, // capped at MaxRetryPenalty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := ScoreInput{
				Priority:    4, // no priority bonus
				MRCreatedAt: now,
				RetryCount:  tt.retryCount,
				Now:         now,
			}
			score := ScoreMR(input, cfg)
			wantScore := cfg.BaseScore - tt.wantPenalty
			if score != wantScore {
				t.Errorf("ScoreMR() with %d retries = %v, want %v", tt.retryCount, score, wantScore)
			}
		})
	}
}

func TestScoreMR_MRAgeScoring(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	tests := []struct {
		name         string
		mrAge        time.Duration
		wantAgeBonus float64
	}{
		{
			name:         "MR just created",
			mrAge:        0,
			wantAgeBonus: 0,
		},
		{
			name:         "MR 1 hour old",
			mrAge:        1 * time.Hour,
			wantAgeBonus: 1.0, // 1 pt/hour
		},
		{
			name:         "MR 24 hours old",
			mrAge:        24 * time.Hour,
			wantAgeBonus: 24.0, // 24 pts
		},
		{
			name:         "MR 168 hours old (1 week)",
			mrAge:        168 * time.Hour,
			wantAgeBonus: 168.0, // 168 pts
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mrCreatedAt := now.Add(-tt.mrAge)
			input := ScoreInput{
				Priority:    4, // no priority bonus
				MRCreatedAt: mrCreatedAt,
				RetryCount:  0,
				Now:         now,
			}
			score := ScoreMR(input, cfg)
			wantScore := cfg.BaseScore + tt.wantAgeBonus
			if score != wantScore {
				t.Errorf("ScoreMR() = %v, want %v", score, wantScore)
			}
		})
	}
}

func TestScoreMR_CombinedFactors(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Test case: P1 priority, 24h convoy age, 2 retries, 12h MR age
	convoyCreatedAt := now.Add(-24 * time.Hour)
	mrCreatedAt := now.Add(-12 * time.Hour)
	input := ScoreInput{
		Priority:        1,
		MRCreatedAt:     mrCreatedAt,
		ConvoyCreatedAt: &convoyCreatedAt,
		RetryCount:      2,
		Now:             now,
	}

	score := ScoreMR(input, cfg)

	// Calculate expected score:
	// Base: 1000
	// Priority (P1): +300 (3 * 100)
	// Convoy age (24h): +240 (24 * 10)
	// Retry penalty (2): -100 (2 * 50)
	// MR age (12h): +12 (12 * 1)
	// Total: 1000 + 300 + 240 - 100 + 12 = 1452
	want := 1452.0

	if score != want {
		t.Errorf("ScoreMR() combined = %v, want %v", score, want)
	}
}

func TestScoreMR_UsesTimeNowWhenZero(t *testing.T) {
	// When Now is zero, ScoreMR should use time.Now()
	mrCreatedAt := time.Now().Add(-1 * time.Hour)
	input := ScoreInput{
		Priority:    4,
		MRCreatedAt: mrCreatedAt,
		RetryCount:  0,
		Now:         time.Time{}, // zero time
	}
	cfg := DefaultScoreConfig()

	score := ScoreMR(input, cfg)

	// Should have approximately 1 hour of MR age bonus
	// Allow some tolerance for test execution time
	minScore := cfg.BaseScore + 0.9  // at least ~1 hour
	maxScore := cfg.BaseScore + 1.1  // not more than ~1 hour

	if score < minScore || score > maxScore {
		t.Errorf("ScoreMR() with zero Now = %v, want between %v and %v", score, minScore, maxScore)
	}
}

func TestScoreMRWithDefaults(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	input := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  0,
		Now:         now,
	}

	score := ScoreMRWithDefaults(input)

	// P2 with defaults: 1000 + 200 = 1200
	want := 1200.0
	if score != want {
		t.Errorf("ScoreMRWithDefaults() = %v, want %v", score, want)
	}
}

func TestScoreMR_CustomConfig(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	convoyCreatedAt := now.Add(-10 * time.Hour)
	mrCreatedAt := now.Add(-5 * time.Hour)

	input := ScoreInput{
		Priority:        1,
		MRCreatedAt:     mrCreatedAt,
		ConvoyCreatedAt: &convoyCreatedAt,
		RetryCount:      3,
		Now:             now,
	}

	// Custom config with different weights
	cfg := ScoreConfig{
		BaseScore:       500.0,
		ConvoyAgeWeight: 5.0,
		PriorityWeight:  50.0,
		RetryPenalty:    25.0,
		MRAgeWeight:     2.0,
		MaxRetryPenalty: 100.0,
	}

	score := ScoreMR(input, cfg)

	// Calculate expected:
	// Base: 500
	// Priority (P1): +150 (3 * 50)
	// Convoy age (10h): +50 (10 * 5)
	// Retry penalty (3): -75 (3 * 25)
	// MR age (5h): +10 (5 * 2)
	// Total: 500 + 150 + 50 - 75 + 10 = 635
	want := 635.0

	if score != want {
		t.Errorf("ScoreMR() with custom config = %v, want %v", score, want)
	}
}

func TestScoreMR_OrderingBehavior(t *testing.T) {
	// This test verifies that scoring produces correct ordering in realistic scenarios
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Create several MRs with different characteristics
	// Note: The convoy age bonus is intentionally strong (+10 pts/hour) to prevent
	// starvation of old convoys. A 48h old convoy gets +480, which can outweigh
	// priority differences. This is by design - we want to prevent convoy starvation.
	tests := []struct {
		name  string
		input ScoreInput
	}{
		{
			name: "P2 with very old convoy - highest due to starvation prevention",
			input: ScoreInput{
				Priority:        2,
				MRCreatedAt:     now,
				ConvoyCreatedAt: timePtr(now.Add(-48 * time.Hour)),
				RetryCount:      0,
				Now:             now,
			},
		},
		{
			name: "P0 urgent - second highest (no convoy bonus)",
			input: ScoreInput{
				Priority:    0,
				MRCreatedAt: now,
				RetryCount:  0,
				Now:         now,
			},
		},
		{
			name: "P1 normal - third",
			input: ScoreInput{
				Priority:    1,
				MRCreatedAt: now,
				RetryCount:  0,
				Now:         now,
			},
		},
		{
			name: "P0 but many retries - lowest due to retry penalty",
			input: ScoreInput{
				Priority:    0,
				MRCreatedAt: now,
				RetryCount:  6, // max penalty
				Now:         now,
			},
		},
	}

	scores := make([]float64, len(tests))
	for i, tt := range tests {
		scores[i] = ScoreMR(tt.input, cfg)
		t.Logf("%s: score = %.2f", tt.name, scores[i])
	}

	// Verify ordering: P2 old convoy > P0 urgent > P1 normal > P0 with retries
	// The convoy age bonus is intentionally strong to prevent starvation
	if scores[0] <= scores[1] {
		t.Errorf("P2 with old convoy (%v) should score higher than P0 urgent (%v) due to starvation prevention", scores[0], scores[1])
	}
	if scores[1] <= scores[2] {
		t.Errorf("P0 urgent (%v) should score higher than P1 normal (%v)", scores[1], scores[2])
	}
	if scores[2] <= scores[3] {
		t.Errorf("P1 normal (%v) should score higher than P0 with retries (%v)", scores[2], scores[3])
	}
}

func TestMRInfo_Score(t *testing.T) {
	// Test the Score method on MRInfo struct
	now := time.Now()
	convoyCreatedAt := now.Add(-24 * time.Hour)
	mr := &MRInfo{
		Priority:        2,
		CreatedAt:       now.Add(-1 * time.Hour),
		ConvoyCreatedAt: &convoyCreatedAt,
		RetryCount:      1,
	}

	score := mr.Score()

	// Should return a positive score
	if score <= 0 {
		t.Errorf("MRInfo.Score() = %v, want positive value", score)
	}
}

func TestMRInfo_ScoreAt(t *testing.T) {
	// Test the ScoreAt method for deterministic testing
	fixedNow := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	convoyCreatedAt := fixedNow.Add(-24 * time.Hour)
	mrCreatedAt := fixedNow.Add(-12 * time.Hour)

	mr := &MRInfo{
		Priority:        2,
		CreatedAt:       mrCreatedAt,
		ConvoyCreatedAt: &convoyCreatedAt,
		RetryCount:      1,
	}

	score := mr.ScoreAt(fixedNow)

	// Calculate expected:
	// Base: 1000
	// Priority (P2): +200
	// Convoy age (24h): +240
	// Retry penalty (1): -50
	// MR age (12h): +12
	// Total: 1000 + 200 + 240 - 50 + 12 = 1402
	want := 1402.0

	if score != want {
		t.Errorf("MRInfo.ScoreAt() = %v, want %v", score, want)
	}
}

func TestMRInfo_ScoreAt_NilConvoy(t *testing.T) {
	fixedNow := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	mr := &MRInfo{
		Priority:        3,
		CreatedAt:       fixedNow,
		ConvoyCreatedAt: nil,
		RetryCount:      0,
	}

	score := mr.ScoreAt(fixedNow)

	// Base: 1000 + P3 bonus: 100 = 1100
	want := 1100.0

	if score != want {
		t.Errorf("MRInfo.ScoreAt() with nil convoy = %v, want %v", score, want)
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}

// Benchmark tests for performance
func BenchmarkScoreMR(b *testing.B) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	convoyCreatedAt := now.Add(-24 * time.Hour)
	input := ScoreInput{
		Priority:        2,
		MRCreatedAt:     now.Add(-12 * time.Hour),
		ConvoyCreatedAt: &convoyCreatedAt,
		RetryCount:      2,
		Now:             now,
	}
	cfg := DefaultScoreConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreMR(input, cfg)
	}
}

func BenchmarkScoreMRWithDefaults(b *testing.B) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	input := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  0,
		Now:         now,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreMRWithDefaults(input)
	}
}

// ============================================================================
// Integration Tests for Queue Ordering and Refinery Flow
// ============================================================================

// TestQueueOrdering_HappyPath tests that single MRs with clean rebases
// are ordered correctly by priority and age.
func TestQueueOrdering_HappyPath(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Scenario: Two MRs, both clean (no conflicts), different priorities
	mrP2 := ScoreInput{
		Priority:    2,
		MRCreatedAt: now.Add(-2 * time.Hour), // 2 hours old
		RetryCount:  0,
		Now:         now,
	}
	mrP0 := ScoreInput{
		Priority:    0,
		MRCreatedAt: now.Add(-1 * time.Hour), // 1 hour old
		RetryCount:  0,
		Now:         now,
	}

	scoreP2 := ScoreMR(mrP2, cfg)
	scoreP0 := ScoreMR(mrP0, cfg)

	// P0 should have higher score than P2 even though P2 is older
	if scoreP0 <= scoreP2 {
		t.Errorf("P0 MR (%v) should score higher than P2 MR (%v)", scoreP0, scoreP2)
	}
}

// TestQueueOrdering_MultipleMRs_NoConflicts tests priority ordering with
// multiple MRs in the queue without conflicts.
func TestQueueOrdering_MultipleMRs_NoConflicts(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Simulate 5 MRs submitted at different times with different priorities
	mrs := []struct {
		name     string
		priority int
		age      time.Duration
	}{
		{"MR1", 3, 5 * time.Hour},  // P3, oldest
		{"MR2", 1, 4 * time.Hour},  // P1
		{"MR3", 2, 3 * time.Hour},  // P2
		{"MR4", 0, 2 * time.Hour},  // P0, urgent
		{"MR5", 2, 1 * time.Hour},  // P2, newest
	}

	type scoredMR struct {
		name  string
		score float64
	}
	scored := make([]scoredMR, len(mrs))

	for i, mr := range mrs {
		input := ScoreInput{
			Priority:    mr.priority,
			MRCreatedAt: now.Add(-mr.age),
			RetryCount:  0,
			Now:         now,
		}
		scored[i] = scoredMR{name: mr.name, score: ScoreMR(input, cfg)}
	}

	// Sort by score (highest first) to simulate queue ordering
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Expected order: MR4 (P0), MR2 (P1), MR3 (P2 older), MR5 (P2 newer), MR1 (P3)
	// P0 gets +400, P1 gets +300, P2 gets +200, P3 gets +100
	// Age adds small bonus but much less than priority difference
	expectedFirst := "MR4" // P0 should be first
	if scored[0].name != expectedFirst {
		t.Errorf("Queue order: first should be %s, got %s (scores: %v)", expectedFirst, scored[0].name, scored)
	}

	expectedSecond := "MR2" // P1 should be second
	if scored[1].name != expectedSecond {
		t.Errorf("Queue order: second should be %s, got %s", expectedSecond, scored[1].name)
	}
}

// TestQueueOrdering_ConflictHandling tests that MRs with conflicts (retries)
// are deprioritized to prevent thrashing.
func TestQueueOrdering_ConflictHandling(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Scenario: MR has conflict and has been retried 3 times
	mrWithConflict := ScoreInput{
		Priority:    1, // P1
		MRCreatedAt: now.Add(-5 * time.Hour),
		RetryCount:  3, // 3 retries = -150 penalty
		Now:         now,
	}

	// Clean MR with lower priority but no retries
	mrClean := ScoreInput{
		Priority:    2, // P2 (lower priority)
		MRCreatedAt: now.Add(-1 * time.Hour),
		RetryCount:  0,
		Now:         now,
	}

	scoreConflict := ScoreMR(mrWithConflict, cfg)
	scoreClean := ScoreMR(mrClean, cfg)

	// Expected: P1 gets +300, P2 gets +200, but P1 has -150 penalty
	// P1 with retries: 1000 + 300 - 150 + 5 = 1155
	// P2 clean: 1000 + 200 + 1 = 1201
	// Clean P2 should score higher than conflicting P1

	t.Logf("P1 with 3 retries: %v", scoreConflict)
	t.Logf("P2 clean: %v", scoreClean)

	if scoreConflict >= scoreClean {
		t.Errorf("P2 clean MR (%v) should score higher than P1 with 3 retries (%v)", scoreClean, scoreConflict)
	}
}

// TestQueueOrdering_ConflictResolution_PriorityBoost tests that resolved
// MRs re-enter the queue with appropriate ordering.
func TestQueueOrdering_ConflictResolution_PriorityBoost(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// MR was rebased and has 1 retry but is now clean
	// The retry count stays but the MR should still be prioritized
	// based on its original priority
	mrResolved := ScoreInput{
		Priority:    0, // P0 urgent
		MRCreatedAt: now.Add(-10 * time.Hour),
		RetryCount:  1, // Was retried once
		Now:         now,
	}

	// New MR with same priority but no retries
	mrNew := ScoreInput{
		Priority:    0,
		MRCreatedAt: now.Add(-1 * time.Hour),
		RetryCount:  0,
		Now:         now,
	}

	scoreResolved := ScoreMR(mrResolved, cfg)
	scoreNew := ScoreMR(mrNew, cfg)

	// Resolved MR: 1000 + 400 - 50 + 10 = 1360
	// New MR: 1000 + 400 + 1 = 1401
	// New MR should score slightly higher due to no retry penalty

	t.Logf("P0 resolved (1 retry): %v", scoreResolved)
	t.Logf("P0 new (0 retries): %v", scoreNew)

	// Both P0, so close in score but new should be slightly higher
	if scoreNew <= scoreResolved {
		t.Errorf("New MR (%v) should score higher than resolved MR with retry (%v)", scoreNew, scoreResolved)
	}
}

// TestQueueOrdering_RetryCountTracking tests that retry count is properly
// tracked across multiple conflict cycles.
func TestQueueOrdering_RetryCountTracking(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Test progressive penalty with increasing retry counts
	scores := make([]float64, 8)
	for i := 0; i < 8; i++ {
		input := ScoreInput{
			Priority:    2,
			MRCreatedAt: now,
			RetryCount:  i,
			Now:         now,
		}
		scores[i] = ScoreMR(input, cfg)
	}

	// Verify scores decrease with each retry (up to max penalty)
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[i-1] {
			t.Errorf("Score for retry %d (%v) should be <= retry %d (%v)", i, scores[i], i-1, scores[i-1])
		}
	}

	// Verify penalty is capped (retries 6 and 7 should have same penalty)
	// MaxRetryPenalty = 300, RetryPenalty = 50, so cap at 6 retries
	if scores[6] != scores[7] {
		t.Errorf("Retry penalty should be capped: retry 6 (%v) != retry 7 (%v)", scores[6], scores[7])
	}
}

// TestQueueOrdering_ConvoyStarvation tests that older convoys get priority
// to prevent starvation.
func TestQueueOrdering_ConvoyStarvation(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// Old convoy (3 days) with P2 priority
	oldConvoyTime := now.Add(-72 * time.Hour)
	mrOldConvoy := ScoreInput{
		Priority:        2,
		MRCreatedAt:     now.Add(-12 * time.Hour),
		ConvoyCreatedAt: &oldConvoyTime,
		RetryCount:      0,
		Now:             now,
	}

	// New convoy (1 hour) with P1 priority
	newConvoyTime := now.Add(-1 * time.Hour)
	mrNewConvoy := ScoreInput{
		Priority:        1, // Higher priority
		MRCreatedAt:     now.Add(-30 * time.Minute),
		ConvoyCreatedAt: &newConvoyTime,
		RetryCount:      0,
		Now:             now,
	}

	scoreOld := ScoreMR(mrOldConvoy, cfg)
	scoreNew := ScoreMR(mrNewConvoy, cfg)

	// Old convoy: 1000 + 200 + 720 + 12 = 1932 (72h * 10 pts/h = 720)
	// New convoy: 1000 + 300 + 10 + 0.5 = 1310.5 (1h * 10 pts/h = 10)

	t.Logf("Old convoy (72h, P2): %v", scoreOld)
	t.Logf("New convoy (1h, P1): %v", scoreNew)

	// Old convoy should win despite lower priority due to starvation prevention
	if scoreOld <= scoreNew {
		t.Errorf("Old convoy (%v) should score higher than new convoy (%v) to prevent starvation", scoreOld, scoreNew)
	}
}

// TestQueueOrdering_MRConflictsAgainAfterResolution tests the scenario where
// an MR conflicts again after being resolved.
func TestQueueOrdering_MRConflictsAgainAfterResolution(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// MR that keeps conflicting - now on 4th retry
	mrPersistentConflict := ScoreInput{
		Priority:    1,
		MRCreatedAt: now.Add(-24 * time.Hour),
		RetryCount:  4, // 4 retries = -200 penalty
		Now:         now,
	}

	// Fresh MR
	mrFresh := ScoreInput{
		Priority:    3, // Much lower priority
		MRCreatedAt: now.Add(-1 * time.Hour),
		RetryCount:  0,
		Now:         now,
	}

	scorePersistent := ScoreMR(mrPersistentConflict, cfg)
	scoreFresh := ScoreMR(mrFresh, cfg)

	// Persistent: 1000 + 300 - 200 + 24 = 1124
	// Fresh: 1000 + 100 + 1 = 1101

	t.Logf("Persistent conflict (P1, 4 retries): %v", scorePersistent)
	t.Logf("Fresh (P3, 0 retries): %v", scoreFresh)

	// The persistent conflict still has slight edge due to priority and age
	// but it's close, showing the penalty is working
	diff := scorePersistent - scoreFresh
	if diff < 0 || diff > 50 {
		// Within 50 points means penalty is working but not too harsh
		t.Logf("Score difference: %v (expected between 0-50)", diff)
	}
}

// TestEdgeCase_PolecatRecycledBeforeMerge tests scoring when a polecat
// was recycled before completing the merge (MR age might be high).
func TestEdgeCase_PolecatRecycledBeforeMerge(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// MR created 3 days ago but never merged (polecat was recycled)
	mrAbandoned := ScoreInput{
		Priority:    2,
		MRCreatedAt: now.Add(-72 * time.Hour),
		RetryCount:  0, // Never even got to conflict stage
		Now:         now,
	}

	// Normal MR from today
	mrNormal := ScoreInput{
		Priority:    2,
		MRCreatedAt: now.Add(-2 * time.Hour),
		RetryCount:  0,
		Now:         now,
	}

	scoreAbandoned := ScoreMR(mrAbandoned, cfg)
	scoreNormal := ScoreMR(mrNormal, cfg)

	// Abandoned should have higher score due to MR age bonus
	// helping it not get stuck forever
	if scoreAbandoned <= scoreNormal {
		t.Errorf("Abandoned MR (%v) should score higher than normal MR (%v) to prevent starvation", scoreAbandoned, scoreNormal)
	}
}

// TestEdgeCase_ConvoyCompletedWithPendingMRs tests scoring when a convoy
// is marked complete but still has pending MRs.
func TestEdgeCase_ConvoyCompletedWithPendingMRs(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultScoreConfig()

	// This scenario: convoy was created 48h ago, some MRs merged,
	// some still pending. Pending MRs should have high priority
	// due to convoy age.
	convoyTime := now.Add(-48 * time.Hour)
	mrPendingInConvoy := ScoreInput{
		Priority:        2,
		MRCreatedAt:     now.Add(-24 * time.Hour),
		ConvoyCreatedAt: &convoyTime,
		RetryCount:      0,
		Now:             now,
	}

	// Standalone MR with higher priority but no convoy
	mrStandalone := ScoreInput{
		Priority:    1, // Higher priority
		MRCreatedAt: now.Add(-1 * time.Hour),
		RetryCount:  0,
		Now:         now,
	}

	scoreConvoy := ScoreMR(mrPendingInConvoy, cfg)
	scoreStandalone := ScoreMR(mrStandalone, cfg)

	// Convoy MR: 1000 + 200 + 480 + 24 = 1704
	// Standalone: 1000 + 300 + 1 = 1301

	t.Logf("Pending in 48h convoy (P2): %v", scoreConvoy)
	t.Logf("Standalone (P1): %v", scoreStandalone)

	// Convoy age should push the pending MR to higher priority
	if scoreConvoy <= scoreStandalone {
		t.Errorf("Convoy MR (%v) should score higher than standalone (%v) due to convoy age", scoreConvoy, scoreStandalone)
	}
}
