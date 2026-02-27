package guardian

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// MaxRecentResults is the maximum number of recent results kept per worker.
	MaxRecentResults = 50

	// stateSubdir is the subdirectory under stateDir for Guardian state.
	stateSubdir = "guardian"

	// stateFileName is the name of the state file.
	stateFileName = "judgment-state.json"
)

// GuardianState holds aggregated judgment data for all workers.
type GuardianState struct {
	// Workers maps worker name to their judgment history.
	Workers map[string]*PolecatJudgment `json:"workers"`

	// LastUpdated is when this state was last written.
	LastUpdated time.Time `json:"last_updated"`
}

// NewGuardianState creates an empty GuardianState.
func NewGuardianState() *GuardianState {
	return &GuardianState{
		Workers: make(map[string]*PolecatJudgment),
	}
}

// PolecatJudgment holds rolling aggregates for a single worker.
type PolecatJudgment struct {
	// Worker is the polecat name.
	Worker string `json:"worker"`

	// TotalReviews is the total number of reviews performed.
	TotalReviews int `json:"total_reviews"`

	// AvgScore is the rolling average quality score.
	AvgScore float64 `json:"avg_score"`

	// RejectionRate is the fraction of reviews with "request_changes".
	RejectionRate float64 `json:"rejection_rate"`

	// RecentResults is a ring buffer of the most recent review results.
	RecentResults []RecentResult `json:"recent_results"`
}

// RecentResult is a condensed record of a single review.
type RecentResult struct {
	BeadID         string    `json:"bead_id"`
	Score          float64   `json:"score"`
	Recommendation string    `json:"recommendation"`
	IssueCount     int       `json:"issue_count"`
	ReviewedAt     time.Time `json:"reviewed_at"`
}

// AddResult records a new review result for the given worker.
func (s *GuardianState) AddResult(worker string, result *GuardianResult) {
	pj, ok := s.Workers[worker]
	if !ok {
		pj = &PolecatJudgment{Worker: worker}
		s.Workers[worker] = pj
	}

	recent := RecentResult{
		BeadID:         result.BeadID,
		Score:          result.Score,
		Recommendation: result.Recommendation,
		IssueCount:     len(result.Issues),
		ReviewedAt:     result.ReviewedAt,
	}

	pj.RecentResults = append(pj.RecentResults, recent)

	// Cap ring buffer
	if len(pj.RecentResults) > MaxRecentResults {
		pj.RecentResults = pj.RecentResults[len(pj.RecentResults)-MaxRecentResults:]
	}

	pj.TotalReviews++
	pj.recomputeAggregates()
	s.LastUpdated = time.Now()
}

// recomputeAggregates recalculates average score and rejection rate from recent results.
func (pj *PolecatJudgment) recomputeAggregates() {
	if len(pj.RecentResults) == 0 {
		pj.AvgScore = 0
		pj.RejectionRate = 0
		return
	}

	var totalScore float64
	var rejections int
	for _, r := range pj.RecentResults {
		totalScore += r.Score
		if r.Recommendation == "request_changes" {
			rejections++
		}
	}

	pj.AvgScore = totalScore / float64(len(pj.RecentResults))
	pj.RejectionRate = float64(rejections) / float64(len(pj.RecentResults))
}

// StateFilePath returns the path to the Guardian state file.
func StateFilePath(stateDir string) string {
	return filepath.Join(stateDir, stateSubdir, stateFileName)
}

// LoadState loads the Guardian state from disk.
// Returns a new empty state if the file doesn't exist.
func LoadState(stateDir string) (*GuardianState, error) {
	path := StateFilePath(stateDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewGuardianState(), nil
		}
		return nil, err
	}

	var state GuardianState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Workers == nil {
		state.Workers = make(map[string]*PolecatJudgment)
	}

	return &state, nil
}

// SaveState writes the Guardian state to disk.
func SaveState(stateDir string, state *GuardianState) error {
	path := StateFilePath(stateDir)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	state.LastUpdated = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	return os.WriteFile(path, data, 0644) //nolint:gosec // G306: state file is non-sensitive operational data
}
