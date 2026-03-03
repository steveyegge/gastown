package refinery

import (
	"testing"
	"time"
)

func TestScoreMR_NegativePriority_ClampsToLowest(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	config := DefaultScoreConfig()

	// P4 (lowest valid priority) should get 0 bonus
	p4Input := ScoreInput{
		Priority:    4,
		MRCreatedAt: now,
		Now:         now,
	}
	p4Score := ScoreMR(p4Input, config)

	// Negative priority (-1 sentinel) should also get 0 bonus (same as P4)
	negInput := ScoreInput{
		Priority:    -1,
		MRCreatedAt: now,
		Now:         now,
	}
	negScore := ScoreMR(negInput, config)

	if negScore != p4Score {
		t.Errorf("negative priority score = %f, want %f (same as P4)", negScore, p4Score)
	}

	// P0 (highest valid priority) should get maximum bonus
	p0Input := ScoreInput{
		Priority:    0,
		MRCreatedAt: now,
		Now:         now,
	}
	p0Score := ScoreMR(p0Input, config)

	if negScore >= p0Score {
		t.Errorf("negative priority score %f should be less than P0 score %f", negScore, p0Score)
	}
}

func TestScoreMR_PriorityAbove4_ClampsToLowest(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	config := DefaultScoreConfig()

	p4Input := ScoreInput{
		Priority:    4,
		MRCreatedAt: now,
		Now:         now,
	}
	p4Score := ScoreMR(p4Input, config)

	// Priority 10 (out of range high) should clamp to P4
	highInput := ScoreInput{
		Priority:    10,
		MRCreatedAt: now,
		Now:         now,
	}
	highScore := ScoreMR(highInput, config)

	if highScore != p4Score {
		t.Errorf("priority 10 score = %f, want %f (same as P4)", highScore, p4Score)
	}
}

func TestScoreMR_ValidPriorities(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	config := DefaultScoreConfig()

	// Verify P0 > P1 > P2 > P3 > P4
	var prevScore float64
	for p := 0; p <= 4; p++ {
		input := ScoreInput{
			Priority:    p,
			MRCreatedAt: now,
			Now:         now,
		}
		score := ScoreMR(input, config)

		if p > 0 && score >= prevScore {
			t.Errorf("P%d score %f should be less than P%d score %f", p, score, p-1, prevScore)
		}
		prevScore = score
	}
}
