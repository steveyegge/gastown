package mrqueue

import (
	"testing"
	"time"
)

func TestScoreMR_BaseScore(t *testing.T) {
	now := time.Now()
	config := DefaultScoreConfig()

	input := ScoreInput{
		Priority:    2,  // P2 (medium)
		MRCreatedAt: now,
		RetryCount:  0,
		Now:         now,
	}

	score := ScoreMR(input, config)

	// BaseScore(1000) + Priority(2 gives 4-2=2, so 2*100=200) = 1200
	expected := 1200.0
	if score != expected {
		t.Errorf("expected score %f, got %f", expected, score)
	}
}

func TestScoreMR_PriorityOrdering(t *testing.T) {
	now := time.Now()

	tests := []struct {
		priority int
		expected float64
	}{
		{0, 1400.0}, // P0: base(1000) + (4-0)*100 = 1400
		{1, 1300.0}, // P1: base(1000) + (4-1)*100 = 1300
		{2, 1200.0}, // P2: base(1000) + (4-2)*100 = 1200
		{3, 1100.0}, // P3: base(1000) + (4-3)*100 = 1100
		{4, 1000.0}, // P4: base(1000) + (4-4)*100 = 1000
	}

	for _, tt := range tests {
		t.Run("P"+string(rune('0'+tt.priority)), func(t *testing.T) {
			input := ScoreInput{
				Priority:    tt.priority,
				MRCreatedAt: now,
				Now:         now,
			}
			score := ScoreMRWithDefaults(input)
			if score != tt.expected {
				t.Errorf("P%d: expected %f, got %f", tt.priority, tt.expected, score)
			}
		})
	}

	// Verify ordering: P0 > P1 > P2 > P3 > P4
	for i := 0; i < 4; i++ {
		input1 := ScoreInput{Priority: i, MRCreatedAt: now, Now: now}
		input2 := ScoreInput{Priority: i + 1, MRCreatedAt: now, Now: now}
		score1 := ScoreMRWithDefaults(input1)
		score2 := ScoreMRWithDefaults(input2)
		if score1 <= score2 {
			t.Errorf("P%d (%f) should score higher than P%d (%f)", i, score1, i+1, score2)
		}
	}
}

func TestScoreMR_ConvoyAgeEscalation(t *testing.T) {
	now := time.Now()
	config := DefaultScoreConfig()

	// MR without convoy
	noConvoy := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		Now:         now,
	}
	scoreNoConvoy := ScoreMR(noConvoy, config)

	// MR with 24-hour old convoy
	convoyTime := now.Add(-24 * time.Hour)
	withConvoy := ScoreInput{
		Priority:        2,
		MRCreatedAt:     now,
		ConvoyCreatedAt: &convoyTime,
		Now:             now,
	}
	scoreWithConvoy := ScoreMR(withConvoy, config)

	// 24 hours * 10 pts/hour = 240 extra points
	expectedDiff := 240.0
	actualDiff := scoreWithConvoy - scoreNoConvoy
	if actualDiff != expectedDiff {
		t.Errorf("expected convoy age to add %f pts, got %f", expectedDiff, actualDiff)
	}
}

func TestScoreMR_ConvoyStarvationPrevention(t *testing.T) {
	now := time.Now()

	// P4 issue in 48-hour old convoy vs P0 issue with no convoy
	oldConvoy := now.Add(-48 * time.Hour)
	lowPriorityOldConvoy := ScoreInput{
		Priority:        4, // P4 (lowest)
		MRCreatedAt:     now,
		ConvoyCreatedAt: &oldConvoy,
		Now:             now,
	}

	highPriorityNoConvoy := ScoreInput{
		Priority:    0, // P0 (highest)
		MRCreatedAt: now,
		Now:         now,
	}

	scoreOldConvoy := ScoreMRWithDefaults(lowPriorityOldConvoy)
	scoreHighPriority := ScoreMRWithDefaults(highPriorityNoConvoy)

	// P4 with 48h convoy: 1000 + 0 + 480 = 1480
	// P0 with no convoy: 1000 + 400 + 0 = 1400
	// Old convoy should win (starvation prevention)
	if scoreOldConvoy <= scoreHighPriority {
		t.Errorf("48h old P4 convoy (%f) should beat P0 no convoy (%f) for starvation prevention",
			scoreOldConvoy, scoreHighPriority)
	}
}

func TestScoreMR_RetryPenalty(t *testing.T) {
	now := time.Now()
	config := DefaultScoreConfig()

	// No retries
	noRetry := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  0,
		Now:         now,
	}
	scoreNoRetry := ScoreMR(noRetry, config)

	// 3 retries
	threeRetries := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  3,
		Now:         now,
	}
	scoreThreeRetries := ScoreMR(threeRetries, config)

	// 3 retries * 50 pts penalty = 150 pts less
	expectedDiff := 150.0
	actualDiff := scoreNoRetry - scoreThreeRetries
	if actualDiff != expectedDiff {
		t.Errorf("expected 3 retries to lose %f pts, lost %f", expectedDiff, actualDiff)
	}
}

func TestScoreMR_RetryPenaltyCapped(t *testing.T) {
	now := time.Now()
	config := DefaultScoreConfig()

	// Max penalty is 300, so 10 retries should be same as 6
	sixRetries := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  6,
		Now:         now,
	}
	tenRetries := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		RetryCount:  10,
		Now:         now,
	}

	scoreSix := ScoreMR(sixRetries, config)
	scoreTen := ScoreMR(tenRetries, config)

	if scoreSix != scoreTen {
		t.Errorf("penalty should be capped: 6 retries (%f) should equal 10 retries (%f)",
			scoreSix, scoreTen)
	}

	// Both should be base(1000) + priority(200) - maxPenalty(300) = 900
	expected := 900.0
	if scoreSix != expected {
		t.Errorf("expected capped score %f, got %f", expected, scoreSix)
	}
}

func TestScoreMR_MRAgeAsTiebreaker(t *testing.T) {
	now := time.Now()

	// Two MRs with same priority, one submitted 10 hours ago
	oldMR := ScoreInput{
		Priority:    2,
		MRCreatedAt: now.Add(-10 * time.Hour),
		Now:         now,
	}
	newMR := ScoreInput{
		Priority:    2,
		MRCreatedAt: now,
		Now:         now,
	}

	scoreOld := ScoreMRWithDefaults(oldMR)
	scoreNew := ScoreMRWithDefaults(newMR)

	// Old MR should have 10 pts more (1 pt/hour)
	expectedDiff := 10.0
	actualDiff := scoreOld - scoreNew
	if actualDiff != expectedDiff {
		t.Errorf("older MR should score %f more, got %f", expectedDiff, actualDiff)
	}
}

func TestScoreMR_Deterministic(t *testing.T) {
	fixedNow := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	convoyTime := time.Date(2024, 12, 31, 12, 0, 0, 0, time.UTC)
	mrTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	input := ScoreInput{
		Priority:        1,
		MRCreatedAt:     mrTime,
		ConvoyCreatedAt: &convoyTime,
		RetryCount:      2,
		Now:             fixedNow,
	}

	// Run 100 times, should always be same
	first := ScoreMRWithDefaults(input)
	for i := 0; i < 100; i++ {
		score := ScoreMRWithDefaults(input)
		if score != first {
			t.Errorf("score not deterministic: iteration %d got %f, expected %f", i, score, first)
		}
	}
}

func TestScoreMR_InvalidPriorityClamped(t *testing.T) {
	now := time.Now()

	// Negative priority should clamp to 0 bonus (priority=4)
	negativePriority := ScoreInput{
		Priority:    -1,
		MRCreatedAt: now,
		Now:         now,
	}
	scoreNegative := ScoreMRWithDefaults(negativePriority)

	// Very high priority should clamp to max bonus (priority=0)
	highPriority := ScoreInput{
		Priority:    10,
		MRCreatedAt: now,
		Now:         now,
	}
	scoreHigh := ScoreMRWithDefaults(highPriority)

	// Negative priority gets clamped to max bonus (4*100=400)
	if scoreNegative != 1400.0 {
		t.Errorf("negative priority should clamp to P0 bonus, got %f", scoreNegative)
	}

	// High priority (10) gives 4-10=-6, clamped to 0
	if scoreHigh != 1000.0 {
		t.Errorf("priority>4 should give 0 bonus, got %f", scoreHigh)
	}
}

func TestMR_Score(t *testing.T) {
	now := time.Now()
	convoyTime := now.Add(-12 * time.Hour)

	mr := &MR{
		Priority:        1,
		CreatedAt:       now.Add(-2 * time.Hour),
		ConvoyCreatedAt: &convoyTime,
		RetryCount:      1,
	}

	score := mr.ScoreAt(now)

	// base(1000) + convoy(12*10=120) + priority(3*100=300) - retry(1*50=50) + mrAge(2*1=2)
	expected := 1000.0 + 120.0 + 300.0 - 50.0 + 2.0
	if score != expected {
		t.Errorf("MR.ScoreAt expected %f, got %f", expected, score)
	}
}

func TestScoreMR_EdgeCases(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name  string
		input ScoreInput
	}{
		{
			name: "zero time MR",
			input: ScoreInput{
				Priority:    2,
				MRCreatedAt: time.Time{},
				Now:         now,
			},
		},
		{
			name: "future MR",
			input: ScoreInput{
				Priority:    2,
				MRCreatedAt: now.Add(24 * time.Hour),
				Now:         now,
			},
		},
		{
			name: "future convoy",
			input: ScoreInput{
				Priority:        2,
				MRCreatedAt:     now,
				ConvoyCreatedAt: func() *time.Time { t := now.Add(24 * time.Hour); return &t }(),
				Now:             now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			score := ScoreMRWithDefaults(tt.input)
			// Score should still be reasonable (>= base - maxPenalty)
			if score < 700 {
				t.Errorf("score %f unexpectedly low for edge case", score)
			}
		})
	}
}
