// Package refinery provides the merge queue processing agent.
// This file contains priority scoring logic for merge requests.

package refinery

import (
	"time"
)

// ScoreConfig contains tunable weights for MR priority scoring.
// All weights are designed so higher scores = higher priority (process first).
type ScoreConfig struct {
	// BaseScore is the starting score before applying factors.
	// Default: 1000 (keeps all scores positive)
	BaseScore float64

	// ConvoyAgeWeight is points added per hour of convoy age.
	// Older convoys get priority to prevent starvation.
	// Default: 10.0 (10 pts/hour = 240 pts/day)
	ConvoyAgeWeight float64

	// PriorityWeight is multiplied by (4 - priority) so P0 gets most points.
	// P0 adds 4*weight, P1 adds 3*weight, ..., P4 adds 0*weight.
	// Default: 100.0 (P0 gets +400, P4 gets +0)
	PriorityWeight float64

	// RetryPenalty is subtracted per retry attempt to prevent thrashing.
	// MRs that keep failing get deprioritized, giving repo state time to stabilize.
	// Default: 50.0 (each retry loses 50 pts)
	RetryPenalty float64

	// MRAgeWeight is points added per hour since MR submission.
	// Minor factor for FIFO ordering within same priority/convoy.
	// Default: 1.0 (1 pt/hour)
	MRAgeWeight float64

	// MaxRetryPenalty caps the total retry penalty to prevent permanent deprioritization.
	// Default: 300.0 (after 6 retries, penalty is capped)
	MaxRetryPenalty float64
}

// DefaultScoreConfig returns sensible defaults for MR scoring.
func DefaultScoreConfig() ScoreConfig {
	return ScoreConfig{
		BaseScore:       1000.0,
		ConvoyAgeWeight: 10.0,
		PriorityWeight:  100.0,
		RetryPenalty:    50.0,
		MRAgeWeight:     1.0,
		MaxRetryPenalty: 300.0,
	}
}

// ScoreInput contains the data needed to score an MR.
// This struct decouples scoring from the MR struct, allowing the
// caller to provide convoy age from external lookups.
type ScoreInput struct {
	// Priority is the issue priority (0=P0/critical, 4=P4/backlog).
	Priority int

	// MRCreatedAt is when the MR was submitted to the queue.
	MRCreatedAt time.Time

	// ConvoyCreatedAt is when the convoy was created.
	// Nil if MR is not part of a convoy (standalone work).
	ConvoyCreatedAt *time.Time

	// RetryCount is how many times this MR has been retried after conflicts.
	// 0 = first attempt.
	RetryCount int

	// Now is the current time (for deterministic testing).
	// If zero, time.Now() is used.
	Now time.Time
}

// ScoreMR calculates the priority score for a merge request.
// Higher scores mean higher priority (process first).
//
// The scoring formula:
//
//	score = BaseScore
//	      + ConvoyAgeWeight * hoursOld(convoy)       // Prevent convoy starvation
//	      + PriorityWeight * (4 - priority)          // P0=+400, P4=+0
//	      - min(RetryPenalty * retryCount, MaxRetryPenalty)  // Prevent thrashing
//	      + MRAgeWeight * hoursOld(MR)               // FIFO tiebreaker
func ScoreMR(input ScoreInput, config ScoreConfig) float64 {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	score := config.BaseScore

	// Convoy age factor: prevent starvation of old convoys
	if input.ConvoyCreatedAt != nil {
		convoyAge := now.Sub(*input.ConvoyCreatedAt)
		convoyHours := convoyAge.Hours()
		if convoyHours > 0 {
			score += config.ConvoyAgeWeight * convoyHours
		}
	}

	// Priority factor: P0 (0) gets +400, P4 (4) gets +0
	priorityBonus := 4 - input.Priority
	if priorityBonus < 0 {
		priorityBonus = 0 // Clamp for invalid priorities > 4
	}
	if priorityBonus > 4 {
		priorityBonus = 4 // Clamp for invalid priorities < 0
	}
	score += config.PriorityWeight * float64(priorityBonus)

	// Retry penalty: prevent thrashing on repeatedly failing MRs
	retryPenalty := config.RetryPenalty * float64(input.RetryCount)
	if retryPenalty > config.MaxRetryPenalty {
		retryPenalty = config.MaxRetryPenalty
	}
	score -= retryPenalty

	// MR age factor: FIFO ordering as tiebreaker
	mrAge := now.Sub(input.MRCreatedAt)
	mrHours := mrAge.Hours()
	if mrHours > 0 {
		score += config.MRAgeWeight * mrHours
	}

	return score
}

// ScoreMRWithDefaults is a convenience wrapper using default config.
func ScoreMRWithDefaults(input ScoreInput) float64 {
	return ScoreMR(input, DefaultScoreConfig())
}

// Score calculates the priority score for this MR using default config.
// Higher scores mean higher priority (process first).
func (mr *MRInfo) Score() float64 {
	return mr.ScoreAt(time.Now())
}

// ScoreAt calculates the priority score at a specific time (for deterministic testing).
func (mr *MRInfo) ScoreAt(now time.Time) float64 {
	input := ScoreInput{
		Priority:        mr.Priority,
		MRCreatedAt:     mr.CreatedAt,
		ConvoyCreatedAt: mr.ConvoyCreatedAt,
		RetryCount:      mr.RetryCount,
		Now:             now,
	}
	return ScoreMRWithDefaults(input)
}
