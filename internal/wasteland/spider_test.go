// Tests for spider.go — verifies fraud detection query generation and
// signal extraction. Uses table-driven tests for query shape validation
// and unit tests for the extraction helpers.
package wasteland

import (
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
