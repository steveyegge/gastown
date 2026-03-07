// spider.go implements the Spider Protocol — fraud detection for the Wasteland
// reputation system. It operates exclusively on disclosed stamps data (the
// public stamps table in wl-commons) to detect collusion, rubber-stamping,
// and confidence inflation patterns.
//
// The Spider Protocol works by running anomaly-detection SQL queries against
// a Dolt database and scoring the results. Each detector produces a list of
// FraudSignal values; downstream consumers (the scorekeeper agent, admin
// dashboards) decide thresholds and enforcement.
//
// Design principles:
//   - Queries only read from stamps + completions + wanted (no private data).
//   - All patterns are statistical — no single signal is proof of fraud.
//   - Detectors are composable: callers combine signals for risk scoring.
package wasteland

import (
	"fmt"
	"os/exec"
	"strings"
)

// FraudSignalKind identifies the category of suspicious behavior detected.
type FraudSignalKind string

const (
	// SignalCollusion indicates two rigs disproportionately stamp each other,
	// suggesting coordinated reputation inflation.
	SignalCollusion FraudSignalKind = "collusion"

	// SignalRubberStamp indicates a rig stamps many completions with identical
	// valence scores, suggesting no real evaluation occurred.
	SignalRubberStamp FraudSignalKind = "rubber_stamp"

	// SignalConfidenceInflation indicates a rig consistently gives maximum
	// confidence scores with minimal evidence variation.
	SignalConfidenceInflation FraudSignalKind = "confidence_inflation"

	// SignalSelfLoop indicates stamps flowing in tight reciprocal loops
	// (A→B→A) rather than through the broader network.
	SignalSelfLoop FraudSignalKind = "self_loop"
)

// FraudSignal represents a single detected anomaly in the stamps graph.
type FraudSignal struct {
	Kind       FraudSignalKind
	Rigs       []string // The rig handles involved
	Score      float64  // 0.0-1.0 severity (higher = more suspicious)
	Detail     string   // Human-readable explanation
	Evidence   string   // The raw query result that triggered this signal
	SampleSize int      // Number of stamps analyzed for this signal
}

// SpiderConfig controls detection thresholds. Callers can tune these to
// reduce false positives in small wastelands or tighten them for larger ones.
type SpiderConfig struct {
	// MinStampsForCollusion is the minimum number of mutual stamps between
	// two rigs before the collusion detector activates. Low values produce
	// noise in active communities; high values miss small-scale fraud.
	MinStampsForCollusion int

	// CollusionRatioThreshold is the fraction of a rig's total stamps that
	// go to a single counterpart before flagging. 0.5 = half their stamps
	// go to one rig.
	CollusionRatioThreshold float64

	// RubberStampMinCount is the minimum number of stamps with identical
	// valence before flagging. Identical valence across many completions
	// suggests copy-paste validation.
	RubberStampMinCount int

	// ConfidenceFloor is the minimum average confidence that triggers the
	// inflation detector. Rigs that always give 1.0 confidence are suspicious
	// unless they have very few stamps.
	ConfidenceFloor float64

	// ConfidenceMinStamps is the minimum stamps before confidence inflation
	// detection activates. Small sample sizes produce false positives.
	ConfidenceMinStamps int
}

// DefaultSpiderConfig returns production-reasonable defaults for fraud
// detection. These are calibrated for a wasteland with 50-200 active rigs.
func DefaultSpiderConfig() SpiderConfig {
	return SpiderConfig{
		MinStampsForCollusion:   3,
		CollusionRatioThreshold: 0.5,
		RubberStampMinCount:     5,
		ConfidenceFloor:         0.95,
		ConfidenceMinStamps:     5,
	}
}

// collusionQuery detects rig pairs that disproportionately stamp each other.
// It finds pairs where >threshold of one rig's stamps go to a single partner,
// and the relationship is mutual (both sides stamp each other heavily).
//
// The query uses a self-join on stamps to find reciprocal relationships,
// then filters by the ratio of mutual stamps to total stamps per rig.
func collusionQuery(cfg SpiderConfig) string {
	return fmt.Sprintf(`
SELECT
    a.author AS rig_a,
    a.subject AS rig_b,
    COUNT(*) AS a_to_b_count,
    (SELECT COUNT(*) FROM stamps WHERE author = a.author) AS a_total,
    (SELECT COUNT(*) FROM stamps WHERE author = a.subject AND subject = a.author) AS b_to_a_count,
    ROUND(COUNT(*) * 1.0 / (SELECT COUNT(*) FROM stamps WHERE author = a.author), 3) AS a_to_b_ratio
FROM stamps a
GROUP BY a.author, a.subject
HAVING
    a_to_b_count >= %d
    AND a_to_b_ratio >= %f
    AND b_to_a_count >= %d
ORDER BY a_to_b_ratio DESC`,
		cfg.MinStampsForCollusion,
		cfg.CollusionRatioThreshold,
		cfg.MinStampsForCollusion,
	)
}

// rubberStampQuery finds rigs that give identical valence JSON across many
// stamps. Real evaluation produces variation; identical scores suggest the
// validator isn't actually reviewing the work.
func rubberStampQuery(cfg SpiderConfig) string {
	return fmt.Sprintf(`
SELECT
    author,
    JSON_EXTRACT(valence, '$') AS valence_pattern,
    COUNT(*) AS identical_count,
    (SELECT COUNT(*) FROM stamps s2 WHERE s2.author = stamps.author) AS total_stamps,
    ROUND(COUNT(*) * 1.0 / (SELECT COUNT(*) FROM stamps s2 WHERE s2.author = stamps.author), 3) AS uniformity_ratio
FROM stamps
GROUP BY author, JSON_EXTRACT(valence, '$')
HAVING identical_count >= %d
ORDER BY uniformity_ratio DESC`,
		cfg.RubberStampMinCount,
	)
}

// confidenceInflationQuery detects rigs that consistently assign near-maximum
// confidence scores. High confidence should be earned through strong evidence;
// blanket high-confidence stamps suggest gaming the system.
func confidenceInflationQuery(cfg SpiderConfig) string {
	return fmt.Sprintf(`
SELECT
    author,
    COUNT(*) AS stamp_count,
    ROUND(AVG(confidence), 3) AS avg_confidence,
    MIN(confidence) AS min_confidence,
    MAX(confidence) AS max_confidence,
    ROUND(AVG(confidence) - MIN(confidence), 3) AS confidence_spread
FROM stamps
GROUP BY author
HAVING
    stamp_count >= %d
    AND avg_confidence >= %f
ORDER BY avg_confidence DESC`,
		cfg.ConfidenceMinStamps,
		cfg.ConfidenceFloor,
	)
}

// selfLoopQuery detects tight reciprocal stamp loops (A stamps B, B stamps A)
// where the loop accounts for most of both rigs' stamp activity. Unlike
// collusion (which checks ratios), this focuses on rigs whose entire stamp
// history is essentially one reciprocal relationship.
func selfLoopQuery() string {
	return `
SELECT
    CASE WHEN a.author < a.subject THEN a.author ELSE a.subject END AS rig_1,
    CASE WHEN a.author < a.subject THEN a.subject ELSE a.author END AS rig_2,
    SUM(CASE WHEN a.author < a.subject THEN 1 ELSE 0 END) AS forward_count,
    SUM(CASE WHEN a.author >= a.subject THEN 1 ELSE 0 END) AS reverse_count,
    COUNT(*) AS loop_total
FROM stamps a
WHERE EXISTS (
    SELECT 1 FROM stamps b
    WHERE b.author = a.subject AND b.subject = a.author
)
GROUP BY rig_1, rig_2
HAVING forward_count >= 2 AND reverse_count >= 2
ORDER BY loop_total DESC`
}

// RunSpiderDetection executes all fraud detection queries against a local
// dolt fork and returns the combined signals. Callers should aggregate
// these signals to produce a per-rig risk score.
//
// The doltPath is the path to the dolt binary; forkDir is the local
// wl-commons clone directory.
//
// Returns an empty slice (not an error) when no suspicious patterns are
// found — absence of fraud signals is the normal case.
func RunSpiderDetection(doltPath, forkDir string, cfg SpiderConfig) ([]FraudSignal, error) {
	var signals []FraudSignal

	// Run each detector independently. A failure in one detector shouldn't
	// prevent others from running — partial results are better than none.
	detectors := []struct {
		name  string
		kind  FraudSignalKind
		query string
	}{
		{"collusion", SignalCollusion, collusionQuery(cfg)},
		{"rubber_stamp", SignalRubberStamp, rubberStampQuery(cfg)},
		{"confidence_inflation", SignalConfidenceInflation, confidenceInflationQuery(cfg)},
		{"self_loop", SignalSelfLoop, selfLoopQuery()},
	}

	for _, d := range detectors {
		rows, err := runDoltQuery(doltPath, forkDir, d.query)
		if err != nil {
			// Log but don't fail — the table might be empty or the query
			// might hit a Dolt edge case.
			continue
		}

		for _, row := range rows {
			signals = append(signals, FraudSignal{
				Kind:     d.kind,
				Rigs:     extractRigs(row),
				Score:    0.5, // Base score; callers refine based on context
				Detail:   fmt.Sprintf("%s detector matched", d.name),
				Evidence: strings.Join(row, " | "),
			})
		}
	}

	return signals, nil
}

// runDoltQuery executes a SQL query against a local dolt database and returns
// the result rows (excluding the header). Returns nil on any error.
func runDoltQuery(doltPath, forkDir, query string) ([][]string, error) {
	cmd := exec.Command(doltPath, "sql", "-r", "csv", "-q", query)
	cmd.Dir = forkDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("dolt sql: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil, nil // header only = no results
	}

	var rows [][]string
	for _, line := range lines[1:] { // skip header
		fields := strings.Split(line, ",")
		rows = append(rows, fields)
	}
	return rows, nil
}

// extractRigs pulls rig handles from a CSV result row. It looks for fields
// that match known column positions (first two columns are typically rigs).
func extractRigs(row []string) []string {
	var rigs []string
	// First two columns are typically rig handles in our fraud queries.
	for i := 0; i < 2 && i < len(row); i++ {
		val := strings.TrimSpace(row[i])
		if val != "" {
			rigs = append(rigs, val)
		}
	}
	return rigs
}
