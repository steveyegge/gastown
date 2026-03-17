// Tests for spider.go — verifies fraud detection query generation and
// signal extraction. Uses table-driven tests for query shape validation
// and unit tests for the extraction helpers.
package wasteland

import (
	"math"
	"strings"
	"testing"
)

func TestDefaultSpiderConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultSpiderConfig()

	if cfg.MinStampsForCollusion <= 0 {
		t.Error("MinStampsForCollusion should be positive")
	}
	if cfg.CollusionRatioThreshold <= 0 || cfg.CollusionRatioThreshold > 1 {
		t.Errorf("CollusionRatioThreshold should be (0,1], got %f", cfg.CollusionRatioThreshold)
	}
	if cfg.RubberStampMinCount <= 0 {
		t.Error("RubberStampMinCount should be positive")
	}
	if cfg.ConfidenceFloor <= 0 || cfg.ConfidenceFloor > 1 {
		t.Errorf("ConfidenceFloor should be (0,1], got %f", cfg.ConfidenceFloor)
	}
}

// TestQueryGeneration verifies that each fraud detector produces valid SQL
// containing the expected clauses. We don't execute against Dolt here —
// these are shape tests to catch query construction bugs early.
func TestQueryGeneration(t *testing.T) {
	t.Parallel()
	cfg := DefaultSpiderConfig()

	tests := []struct {
		name     string
		query    string
		mustHave []string // substrings that must appear in the query
	}{
		{
			name:  "collusion",
			query: collusionQuery(cfg),
			mustHave: []string{
				"stamps",
				"author",
				"subject",
				"GROUP BY",
				"HAVING",
				"a_to_b_ratio",
			},
		},
		{
			name:  "rubber_stamp",
			query: rubberStampQuery(cfg),
			mustHave: []string{
				"stamps",
				"JSON_EXTRACT",
				"valence",
				"GROUP BY",
				"identical_count",
			},
		},
		{
			name:  "confidence_inflation",
			query: confidenceInflationQuery(cfg),
			mustHave: []string{
				"stamps",
				"AVG(confidence)",
				"GROUP BY",
				"HAVING",
			},
		},
		{
			name:  "self_loop",
			query: selfLoopQuery(),
			mustHave: []string{
				"stamps",
				"EXISTS",
				"forward_count",
				"reverse_count",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, sub := range tt.mustHave {
				if !strings.Contains(tt.query, sub) {
					t.Errorf("%s query missing expected substring %q", tt.name, sub)
				}
			}
		})
	}
}

func TestExtractRigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		row  []string
		want []string
	}{
		{"two_rigs", []string{"alice", "bob", "5", "0.8"}, []string{"alice", "bob"}},
		{"one_rig", []string{"alice"}, []string{"alice"}},
		{"empty_row", []string{}, nil},
		{"with_spaces", []string{" alice ", " bob "}, []string{"alice", "bob"}},
		{"empty_fields", []string{"", ""}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractRigs(tt.row)
			if len(got) != len(tt.want) {
				t.Errorf("extractRigs(%v) = %v, want %v", tt.row, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractRigs(%v)[%d] = %q, want %q", tt.row, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFraudSignalKindConstants(t *testing.T) {
	t.Parallel()

	// Verify all signal kinds are distinct and non-empty.
	kinds := []FraudSignalKind{
		SignalCollusion,
		SignalRubberStamp,
		SignalConfidenceInflation,
		SignalSelfLoop,
	}

	seen := make(map[FraudSignalKind]bool)
	for _, k := range kinds {
		if k == "" {
			t.Error("FraudSignalKind should not be empty")
		}
		if seen[k] {
			t.Errorf("duplicate FraudSignalKind: %s", k)
		}
		seen[k] = true
	}
}

// TestCollusionQueryThresholds verifies that changing config thresholds
// is reflected in the generated query (prevents hardcoding bugs).
func TestCollusionQueryThresholds(t *testing.T) {
	t.Parallel()

	cfg := SpiderConfig{
		MinStampsForCollusion:   10,
		CollusionRatioThreshold: 0.75,
	}
	q := collusionQuery(cfg)

	if !strings.Contains(q, "10") {
		t.Error("collusion query should contain the min stamps threshold")
	}
	if !strings.Contains(q, "0.75") {
		t.Error("collusion query should contain the ratio threshold")
	}
}

// --- Scoring function tests ---

func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestScoreCollusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		row        []string
		wantScore  float64
		wantSample int
	}{
		{
			name:       "high_ratio",
			row:        []string{"alice", "bob", "8", "10", "7", "0.800"},
			wantScore:  0.8,
			wantSample: 10,
		},
		{
			name:       "threshold_ratio",
			row:        []string{"alice", "bob", "5", "10", "3", "0.500"},
			wantScore:  0.5,
			wantSample: 10,
		},
		{
			name:       "max_ratio",
			row:        []string{"alice", "bob", "10", "10", "10", "1.000"},
			wantScore:  1.0,
			wantSample: 10,
		},
		{
			name:       "short_row_fallback",
			row:        []string{"alice", "bob"},
			wantScore:  0.5,
			wantSample: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score, sample, detail := scoreCollusion(tt.row)
			if !approxEqual(score, tt.wantScore, 0.01) {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
			if sample != tt.wantSample {
				t.Errorf("sample = %d, want %d", sample, tt.wantSample)
			}
			if detail == "" {
				t.Error("detail should not be empty")
			}
		})
	}
}

func TestScoreRubberStamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		row        []string
		wantScore  float64
		wantSample int
	}{
		{
			name:       "all_identical",
			row:        []string{"alice", `{"quality":5}`, "10", "10", "1.000"},
			wantScore:  1.0,
			wantSample: 10,
		},
		{
			name:       "half_identical",
			row:        []string{"alice", `{"quality":5}`, "5", "10", "0.500"},
			wantScore:  0.5,
			wantSample: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score, sample, detail := scoreRubberStamp(tt.row)
			if !approxEqual(score, tt.wantScore, 0.01) {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
			if sample != tt.wantSample {
				t.Errorf("sample = %d, want %d", sample, tt.wantSample)
			}
			if detail == "" {
				t.Error("detail should not be empty")
			}
		})
	}
}

func TestScoreConfidenceInflation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		row       []string
		wantScore float64
	}{
		{
			name:      "perfect_confidence",
			row:       []string{"alice", "20", "1.000", "1.000", "1.000", "0.000"},
			wantScore: 1.0,
		},
		{
			name:      "threshold_confidence",
			row:       []string{"alice", "10", "0.950", "0.900", "1.000", "0.100"},
			wantScore: 0.5,
		},
		{
			name:      "mid_range",
			row:       []string{"alice", "15", "0.975", "0.950", "1.000", "0.050"},
			wantScore: 0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score, _, _ := scoreConfidenceInflation(tt.row)
			if !approxEqual(score, tt.wantScore, 0.01) {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
		})
	}
}

func TestScoreSelfLoop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		row       []string
		wantScore float64
	}{
		{
			name:      "perfectly_symmetric",
			row:       []string{"alice", "bob", "5", "5", "10"},
			wantScore: 1.0,
		},
		{
			name:      "asymmetric",
			row:       []string{"alice", "bob", "2", "8", "10"},
			wantScore: 0.625, // 0.5 + (2/8)*0.5 = 0.625
		},
		{
			name:      "empty_total",
			row:       []string{"alice", "bob", "0", "0", "0"},
			wantScore: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score, _, _ := scoreSelfLoop(tt.row)
			if !approxEqual(score, tt.wantScore, 0.01) {
				t.Errorf("score = %f, want %f", score, tt.wantScore)
			}
		})
	}
}

func TestParseColumnHelpers(t *testing.T) {
	t.Parallel()

	row := []string{"alice", "42", "0.875"}

	if v := parseFloatColumn(row, 2, 0); !approxEqual(v, 0.875, 0.001) {
		t.Errorf("parseFloatColumn = %f, want 0.875", v)
	}
	if v := parseFloatColumn(row, 99, 0.5); !approxEqual(v, 0.5, 0.001) {
		t.Errorf("parseFloatColumn out-of-range should return fallback, got %f", v)
	}
	if v := parseFloatColumn([]string{"not-a-number"}, 0, 0.5); !approxEqual(v, 0.5, 0.001) {
		t.Errorf("parseFloatColumn bad parse should return fallback, got %f", v)
	}

	if v := parseIntColumn(row, 1); v != 42 {
		t.Errorf("parseIntColumn = %d, want 42", v)
	}
	if v := parseIntColumn(row, 99); v != 0 {
		t.Errorf("parseIntColumn out-of-range should return 0, got %d", v)
	}
}
