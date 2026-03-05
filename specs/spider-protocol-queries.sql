-- Spider Protocol Fraud Detection Queries
-- Task: w-hop-003
-- Date: 2026-03-04
-- Author: nullpriest (via Claude)
--
-- These queries operate on disclosed stamps data to detect:
-- 1. Collusion rings (mutual high-rating patterns)
-- 2. Rubber-stamping (validators who approve everything)
-- 3. Confidence inflation (suspiciously high averages)
-- 4. Temporal anomalies (burst stamping)
-- 5. Self-dealing through proxies

-- ============================================================================
-- 1. COLLUSION RING DETECTION
-- ============================================================================
-- Detect pairs of rigs that consistently stamp each other highly.
-- A collusion pattern shows mutual benefit without third-party validation.

-- 1a. Mutual Stamping Pairs
-- Find pairs where A stamps B AND B stamps A
CREATE OR REPLACE VIEW spider_mutual_stamps AS
SELECT
    s1.author AS rig_a,
    s1.subject AS rig_b,
    COUNT(DISTINCT s1.id) AS a_stamps_b,
    COUNT(DISTINCT s2.id) AS b_stamps_a,
    AVG(JSON_EXTRACT(s1.valence, '$.quality')) AS avg_a_gives_b,
    AVG(JSON_EXTRACT(s2.valence, '$.quality')) AS avg_b_gives_a
FROM stamps s1
JOIN stamps s2 ON s1.author = s2.subject AND s1.subject = s2.author
WHERE s1.author < s1.subject  -- Avoid duplicate pairs
GROUP BY s1.author, s1.subject
HAVING a_stamps_b >= 2 AND b_stamps_a >= 2;

-- 1b. Collusion Score
-- High score = suspicious mutual back-scratching
CREATE OR REPLACE VIEW spider_collusion_scores AS
SELECT
    rig_a,
    rig_b,
    a_stamps_b,
    b_stamps_a,
    avg_a_gives_b,
    avg_b_gives_a,
    -- Collusion score: high if both give high ratings mutually
    (avg_a_gives_b + avg_b_gives_a) / 2 *
    LEAST(a_stamps_b, b_stamps_a) /
    GREATEST(a_stamps_b, b_stamps_a) AS collusion_score
FROM spider_mutual_stamps
WHERE avg_a_gives_b >= 4.0 AND avg_b_gives_a >= 4.0
ORDER BY collusion_score DESC;

-- 1c. Ring Detection (3+ members)
-- Find cliques where multiple rigs all stamp each other
CREATE OR REPLACE VIEW spider_ring_candidates AS
WITH ring_edges AS (
    SELECT DISTINCT
        LEAST(author, subject) AS node_a,
        GREATEST(author, subject) AS node_b
    FROM stamps
    WHERE JSON_EXTRACT(valence, '$.quality') >= 4
)
SELECT
    e1.node_a AS member_1,
    e1.node_b AS member_2,
    e2.node_b AS member_3,
    COUNT(*) AS edge_count
FROM ring_edges e1
JOIN ring_edges e2 ON e1.node_b = e2.node_a AND e1.node_a != e2.node_b
JOIN ring_edges e3 ON e2.node_b = e3.node_a AND e3.node_b = e1.node_a
GROUP BY member_1, member_2, member_3
HAVING edge_count >= 3;


-- ============================================================================
-- 2. RUBBER-STAMPING DETECTION
-- ============================================================================
-- Validators who approve everything with high ratings are suspicious.
-- Legitimate validators should have variance in their ratings.

-- 2a. Validator Rating Distribution
CREATE OR REPLACE VIEW spider_validator_stats AS
SELECT
    author AS validator,
    COUNT(*) AS stamps_issued,
    AVG(JSON_EXTRACT(valence, '$.quality')) AS avg_quality,
    STDDEV(JSON_EXTRACT(valence, '$.quality')) AS stddev_quality,
    MIN(JSON_EXTRACT(valence, '$.quality')) AS min_quality,
    MAX(JSON_EXTRACT(valence, '$.quality')) AS max_quality,
    SUM(CASE WHEN JSON_EXTRACT(valence, '$.quality') = 5 THEN 1 ELSE 0 END) AS five_star_count,
    SUM(CASE WHEN JSON_EXTRACT(valence, '$.quality') <= 2 THEN 1 ELSE 0 END) AS low_rating_count
FROM stamps
GROUP BY author
HAVING stamps_issued >= 5;

-- 2b. Rubber-Stamper Detection
-- Validators with very high average and low variance
CREATE OR REPLACE VIEW spider_rubber_stampers AS
SELECT
    validator,
    stamps_issued,
    avg_quality,
    stddev_quality,
    five_star_count,
    low_rating_count,
    -- Rubber stamp score: high avg + low variance + many 5-stars
    (avg_quality / 5) *
    (1 - COALESCE(stddev_quality, 0) / 2) *
    (five_star_count / stamps_issued) AS rubber_stamp_score
FROM spider_validator_stats
WHERE avg_quality >= 4.5
  AND (stddev_quality IS NULL OR stddev_quality < 0.5)
  AND five_star_count > stamps_issued * 0.8
ORDER BY rubber_stamp_score DESC;


-- ============================================================================
-- 3. CONFIDENCE INFLATION DETECTION
-- ============================================================================
-- Detect rigs whose received stamps are suspiciously high compared to peers.

-- 3a. Per-Rig Received Rating Stats
CREATE OR REPLACE VIEW spider_rig_reputation AS
SELECT
    subject AS rig,
    COUNT(*) AS stamps_received,
    AVG(JSON_EXTRACT(valence, '$.quality')) AS avg_received_quality,
    AVG(JSON_EXTRACT(valence, '$.reliability')) AS avg_received_reliability,
    COUNT(DISTINCT author) AS unique_validators
FROM stamps
GROUP BY subject
HAVING stamps_received >= 3;

-- 3b. Comparison to Network Average
CREATE OR REPLACE VIEW spider_inflation_detection AS
WITH network_avg AS (
    SELECT
        AVG(JSON_EXTRACT(valence, '$.quality')) AS network_avg_quality,
        STDDEV(JSON_EXTRACT(valence, '$.quality')) AS network_stddev_quality
    FROM stamps
)
SELECT
    r.rig,
    r.stamps_received,
    r.avg_received_quality,
    r.unique_validators,
    n.network_avg_quality,
    n.network_stddev_quality,
    -- Z-score: how many stddevs above average
    (r.avg_received_quality - n.network_avg_quality) /
        NULLIF(n.network_stddev_quality, 0) AS quality_z_score
FROM spider_rig_reputation r
CROSS JOIN network_avg n
WHERE r.avg_received_quality > n.network_avg_quality + n.network_stddev_quality
ORDER BY quality_z_score DESC;


-- ============================================================================
-- 4. TEMPORAL ANOMALY DETECTION
-- ============================================================================
-- Detect burst stamping (many stamps in short period) which may indicate gaming.

-- 4a. Stamping Velocity by Validator
CREATE OR REPLACE VIEW spider_stamp_velocity AS
SELECT
    author AS validator,
    DATE(created_at) AS stamp_date,
    COUNT(*) AS stamps_that_day,
    GROUP_CONCAT(DISTINCT subject) AS subjects_stamped
FROM stamps
GROUP BY author, DATE(created_at)
HAVING stamps_that_day >= 5;

-- 4b. Burst Detection
CREATE OR REPLACE VIEW spider_burst_stamping AS
WITH validator_daily AS (
    SELECT
        author,
        DATE(created_at) AS stamp_date,
        COUNT(*) AS daily_count
    FROM stamps
    GROUP BY author, DATE(created_at)
),
validator_avg AS (
    SELECT
        author,
        AVG(daily_count) AS avg_daily,
        STDDEV(daily_count) AS stddev_daily
    FROM validator_daily
    GROUP BY author
)
SELECT
    d.author AS validator,
    d.stamp_date,
    d.daily_count,
    a.avg_daily,
    a.stddev_daily,
    (d.daily_count - a.avg_daily) / NULLIF(a.stddev_daily, 0) AS burst_z_score
FROM validator_daily d
JOIN validator_avg a ON d.author = a.author
WHERE d.daily_count > a.avg_daily + 2 * COALESCE(a.stddev_daily, 1)
ORDER BY burst_z_score DESC;


-- ============================================================================
-- 5. PROXY SELF-DEALING DETECTION
-- ============================================================================
-- Detect patterns where validators primarily stamp a small set of subjects.

-- 5a. Validator Subject Concentration
CREATE OR REPLACE VIEW spider_subject_concentration AS
SELECT
    author AS validator,
    COUNT(*) AS total_stamps,
    COUNT(DISTINCT subject) AS unique_subjects,
    COUNT(*) * 1.0 / COUNT(DISTINCT subject) AS concentration_ratio,
    -- Top subject stats
    (
        SELECT subject
        FROM stamps s2
        WHERE s2.author = stamps.author
        GROUP BY subject
        ORDER BY COUNT(*) DESC
        LIMIT 1
    ) AS top_subject,
    (
        SELECT COUNT(*)
        FROM stamps s2
        WHERE s2.author = stamps.author
        GROUP BY subject
        ORDER BY COUNT(*) DESC
        LIMIT 1
    ) AS top_subject_count
FROM stamps
GROUP BY author
HAVING total_stamps >= 5;

-- 5b. Suspicious Concentration
CREATE OR REPLACE VIEW spider_suspicious_concentration AS
SELECT
    validator,
    total_stamps,
    unique_subjects,
    concentration_ratio,
    top_subject,
    top_subject_count,
    top_subject_count * 1.0 / total_stamps AS top_subject_pct
FROM spider_subject_concentration
WHERE concentration_ratio >= 3  -- Avg 3+ stamps per subject
   OR top_subject_count * 1.0 / total_stamps >= 0.5  -- 50%+ to one subject
ORDER BY top_subject_pct DESC;


-- ============================================================================
-- 6. COMBINED FRAUD SCORE
-- ============================================================================
-- Aggregate all signals into a single fraud risk score per rig.

CREATE OR REPLACE VIEW spider_fraud_scores AS
WITH collusion_flags AS (
    SELECT rig_a AS rig, MAX(collusion_score) AS collusion_score FROM spider_collusion_scores GROUP BY rig_a
    UNION ALL
    SELECT rig_b AS rig, MAX(collusion_score) AS collusion_score FROM spider_collusion_scores GROUP BY rig_b
),
rubber_stamp_flags AS (
    SELECT validator AS rig, rubber_stamp_score FROM spider_rubber_stampers
),
inflation_flags AS (
    SELECT rig, quality_z_score AS inflation_score FROM spider_inflation_detection
),
concentration_flags AS (
    SELECT validator AS rig, top_subject_pct AS concentration_score FROM spider_suspicious_concentration
)
SELECT
    r.handle AS rig,
    COALESCE(c.collusion_score, 0) AS collusion_risk,
    COALESCE(rs.rubber_stamp_score, 0) AS rubber_stamp_risk,
    COALESCE(i.inflation_score, 0) AS inflation_risk,
    COALESCE(con.concentration_score, 0) AS concentration_risk,
    -- Combined score (weighted)
    (
        COALESCE(c.collusion_score, 0) * 0.3 +
        COALESCE(rs.rubber_stamp_score, 0) * 0.25 +
        COALESCE(i.inflation_score, 0) * 0.25 +
        COALESCE(con.concentration_score, 0) * 0.2
    ) AS combined_fraud_score
FROM rigs r
LEFT JOIN collusion_flags c ON r.handle = c.rig
LEFT JOIN rubber_stamp_flags rs ON r.handle = rs.rig
LEFT JOIN inflation_flags i ON r.handle = i.rig
LEFT JOIN concentration_flags con ON r.handle = con.rig
WHERE COALESCE(c.collusion_score, 0) > 0
   OR COALESCE(rs.rubber_stamp_score, 0) > 0
   OR COALESCE(i.inflation_score, 0) > 0
   OR COALESCE(con.concentration_score, 0) > 0
ORDER BY combined_fraud_score DESC;


-- ============================================================================
-- 7. INVESTIGATION QUERIES
-- ============================================================================
-- Helper queries for investigating flagged rigs.

-- 7a. Full stamp history for a rig (as author)
-- Usage: Replace 'suspect_handle' with actual handle
-- SELECT * FROM spider_rig_stamps_given WHERE validator = 'suspect_handle';
CREATE OR REPLACE VIEW spider_rig_stamps_given AS
SELECT
    author AS validator,
    subject,
    JSON_EXTRACT(valence, '$.quality') AS quality,
    JSON_EXTRACT(valence, '$.reliability') AS reliability,
    context_id,
    context_type,
    created_at
FROM stamps
ORDER BY author, created_at DESC;

-- 7b. Full stamp history for a rig (as subject)
CREATE OR REPLACE VIEW spider_rig_stamps_received AS
SELECT
    subject AS rig,
    author AS validator,
    JSON_EXTRACT(valence, '$.quality') AS quality,
    JSON_EXTRACT(valence, '$.reliability') AS reliability,
    context_id,
    context_type,
    created_at
FROM stamps
ORDER BY subject, created_at DESC;

-- 7c. Cross-reference completions with stamps
CREATE OR REPLACE VIEW spider_completion_stamps AS
SELECT
    c.id AS completion_id,
    c.wanted_id,
    c.completed_by,
    c.completed_at,
    s.id AS stamp_id,
    s.author AS validator,
    JSON_EXTRACT(s.valence, '$.quality') AS quality,
    s.created_at AS stamp_time,
    TIMESTAMPDIFF(HOUR, c.completed_at, s.created_at) AS hours_to_stamp
FROM completions c
LEFT JOIN stamps s ON s.context_id = c.id AND s.context_type = 'completion'
ORDER BY c.completed_at DESC;


-- ============================================================================
-- 8. ALERTS AND THRESHOLDS
-- ============================================================================
-- Queries that return actionable alerts based on thresholds.

-- 8a. High-Priority Fraud Alerts
CREATE OR REPLACE VIEW spider_alerts AS
SELECT
    'COLLUSION' AS alert_type,
    CONCAT(rig_a, ' <-> ', rig_b) AS subject,
    collusion_score AS score,
    'High mutual rating pattern detected' AS description
FROM spider_collusion_scores
WHERE collusion_score >= 0.8

UNION ALL

SELECT
    'RUBBER_STAMP' AS alert_type,
    validator AS subject,
    rubber_stamp_score AS score,
    CONCAT('Issued ', stamps_issued, ' stamps with avg ', ROUND(avg_quality, 2)) AS description
FROM spider_rubber_stampers
WHERE rubber_stamp_score >= 0.8

UNION ALL

SELECT
    'INFLATION' AS alert_type,
    rig AS subject,
    quality_z_score AS score,
    CONCAT('Rating ', ROUND(quality_z_score, 1), ' stddevs above network avg') AS description
FROM spider_inflation_detection
WHERE quality_z_score >= 2.0

UNION ALL

SELECT
    'CONCENTRATION' AS alert_type,
    validator AS subject,
    top_subject_pct AS score,
    CONCAT(ROUND(top_subject_pct * 100, 0), '% of stamps to ', top_subject) AS description
FROM spider_suspicious_concentration
WHERE top_subject_pct >= 0.6

ORDER BY score DESC;


-- ============================================================================
-- USAGE NOTES
-- ============================================================================
--
-- To run fraud detection:
--   1. Create the views: dolt sql < spider-protocol-queries.sql
--   2. Check alerts: SELECT * FROM spider_alerts;
--   3. Get fraud scores: SELECT * FROM spider_fraud_scores WHERE combined_fraud_score > 0.5;
--   4. Investigate specific rig:
--      SELECT * FROM spider_rig_stamps_given WHERE validator = 'suspect';
--      SELECT * FROM spider_rig_stamps_received WHERE rig = 'suspect';
--
-- Recommended thresholds (adjust based on network size):
--   - Collusion score >= 0.8: High risk
--   - Rubber stamp score >= 0.8: High risk
--   - Inflation z-score >= 2.0: Suspicious
--   - Concentration >= 60%: Suspicious
--   - Combined fraud score >= 0.6: Requires investigation
