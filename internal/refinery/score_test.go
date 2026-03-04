package refinery

import (
	"testing"
	"time"
)

func TestScoreMR_PriorityClamping(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	config := DefaultScoreConfig()

	baseInput := func(priority int) ScoreInput {
		return ScoreInput{
			Priority:    priority,
			MRCreatedAt: now,
			Now:         now,
		}
	}

	t.Run("P0 gets maximum priority bonus", func(t *testing.T) {
		score := ScoreMR(baseInput(0), config)
		expected := config.BaseScore + config.PriorityWeight*4
		if score != expected {
			t.Errorf("P0 score = %f, want %f", score, expected)
		}
	})

	t.Run("P4 gets zero priority bonus", func(t *testing.T) {
		score := ScoreMR(baseInput(4), config)
		expected := config.BaseScore
		if score != expected {
			t.Errorf("P4 score = %f, want %f", score, expected)
		}
	})

	t.Run("negative priority treated as lowest not highest", func(t *testing.T) {
		negScore := ScoreMR(baseInput(-1), config)
		p4Score := ScoreMR(baseInput(4), config)
		p0Score := ScoreMR(baseInput(0), config)

		// Negative priority should get same score as P4 (lowest), not P0 (highest)
		if negScore != p4Score {
			t.Errorf("negative priority score = %f, want %f (same as P4)", negScore, p4Score)
		}
		if negScore == p0Score {
			t.Errorf("negative priority score = %f, should NOT equal P0 score %f", negScore, p0Score)
		}
	})

	t.Run("large negative priority treated as lowest", func(t *testing.T) {
		negScore := ScoreMR(baseInput(-100), config)
		p4Score := ScoreMR(baseInput(4), config)

		if negScore != p4Score {
			t.Errorf("priority -100 score = %f, want %f (same as P4)", negScore, p4Score)
		}
	})

	t.Run("priority above 4 treated as P4", func(t *testing.T) {
		score := ScoreMR(baseInput(10), config)
		p4Score := ScoreMR(baseInput(4), config)

		if score != p4Score {
			t.Errorf("priority 10 score = %f, want %f (same as P4)", score, p4Score)
		}
	})

	t.Run("priority ordering P0 > P1 > P2 > P3 > P4", func(t *testing.T) {
		scores := make([]float64, 5)
		for i := 0; i <= 4; i++ {
			scores[i] = ScoreMR(baseInput(i), config)
		}
		for i := 0; i < 4; i++ {
			if scores[i] <= scores[i+1] {
				t.Errorf("P%d score (%f) should be > P%d score (%f)", i, scores[i], i+1, scores[i+1])
			}
		}
	})
}
