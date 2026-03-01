package guardian

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// MaxRecentResults is the ring buffer cap per worker.
	MaxRecentResults = 50

	stateSubdir  = "guardian"
	stateFileName = "judgment-state.json"
)

// GuardianState holds judgment state for all workers.
type GuardianState struct {
	Workers     map[string]*PolecatJudgment `json:"workers"`
	LastUpdated time.Time                   `json:"last_updated"`
}

// PolecatJudgment holds aggregated judgment data for a single worker.
type PolecatJudgment struct {
	Worker        string         `json:"worker"`
	TotalReviews  int            `json:"total_reviews"`
	AvgScore      float64        `json:"avg_score"`
	RejectionRate float64        `json:"rejection_rate"` // fraction of request_changes
	RecentResults []RecentResult `json:"recent_results"`
}

// RecentResult is a summary of a single review, stored in the ring buffer.
type RecentResult struct {
	BeadID         string    `json:"bead_id"`
	Rig            string    `json:"rig,omitempty"`
	Score          float64   `json:"score"`
	Recommendation string    `json:"recommendation"`
	IssueCount     int       `json:"issue_count"`
	ReviewedAt     time.Time `json:"reviewed_at"`
}

// NewGuardianState creates an empty state.
func NewGuardianState() *GuardianState {
	return &GuardianState{
		Workers: make(map[string]*PolecatJudgment),
	}
}

// AddResult records a new review result for a worker.
// It appends to the ring buffer, enforces the cap, and recomputes aggregates.
func (s *GuardianState) AddResult(worker string, result RecentResult) {
	pj, ok := s.Workers[worker]
	if !ok {
		pj = &PolecatJudgment{Worker: worker}
		s.Workers[worker] = pj
	}

	pj.RecentResults = append(pj.RecentResults, result)
	pj.TotalReviews++

	// Enforce ring buffer cap — drop oldest entries.
	if len(pj.RecentResults) > MaxRecentResults {
		pj.RecentResults = pj.RecentResults[len(pj.RecentResults)-MaxRecentResults:]
	}

	recomputeAggregates(pj)
	s.LastUpdated = time.Now().UTC()
}

// recomputeAggregates recalculates AvgScore and RejectionRate from recent results.
func recomputeAggregates(pj *PolecatJudgment) {
	n := len(pj.RecentResults)
	if n == 0 {
		pj.AvgScore = 0
		pj.RejectionRate = 0
		return
	}

	var totalScore float64
	var rejections int
	for _, r := range pj.RecentResults {
		totalScore += r.Score
		if r.Recommendation == RecommendRequestChanges {
			rejections++
		}
	}

	pj.AvgScore = totalScore / float64(n)
	pj.RejectionRate = float64(rejections) / float64(n)
}

// StateFilePath returns the path to the judgment state file.
func StateFilePath(townRoot string) string {
	return filepath.Join(townRoot, stateSubdir, stateFileName)
}

// LoadState reads the judgment state from disk.
// Returns an empty state if the file does not exist.
func LoadState(townRoot string) (*GuardianState, error) {
	path := StateFilePath(townRoot)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewGuardianState(), nil
		}
		return nil, fmt.Errorf("reading guardian state: %w", err)
	}

	var state GuardianState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing guardian state: %w", err)
	}

	if state.Workers == nil {
		state.Workers = make(map[string]*PolecatJudgment)
	}

	return &state, nil
}

// SaveState writes the judgment state to disk atomically (temp file + rename).
func SaveState(townRoot string, state *GuardianState) error {
	path := StateFilePath(townRoot)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating guardian directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding guardian state: %w", err)
	}

	// Atomic write: write to temp file, then rename.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil { //nolint:gosec // G306: judgment state is non-sensitive operational data
		return fmt.Errorf("writing guardian state temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming guardian state file: %w", err)
	}

	return nil
}
