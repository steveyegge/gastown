# Spider Protocol: Fraud Detection for Wasteland

**Task**: w-hop-003
**Date**: 2026-03-04
**Author**: nullpriest (via Claude)

## Overview

The Spider Protocol detects fraudulent reputation patterns in the Wasteland stamp system. It operates on disclosed stamps data only — no private information required.

## Fraud Patterns Detected

### 1. Collusion Rings

**Pattern**: Groups of rigs that consistently stamp each other with high ratings.

**Detection**:
- Mutual stamping pairs (A stamps B, B stamps A)
- High average ratings in both directions
- Three-way and larger cliques

**Signal**: `collusion_score` — Higher = more suspicious

```sql
SELECT * FROM spider_collusion_scores WHERE collusion_score >= 0.8;
```

### 2. Rubber-Stamping

**Pattern**: Validators who approve everything with uniformly high ratings.

**Detection**:
- Very high average rating (>= 4.5)
- Low variance (stddev < 0.5)
- High percentage of 5-star ratings (>80%)

**Signal**: `rubber_stamp_score` — Higher = more suspicious

```sql
SELECT * FROM spider_rubber_stampers WHERE rubber_stamp_score >= 0.8;
```

### 3. Confidence Inflation

**Pattern**: Rigs whose received ratings are suspiciously above network average.

**Detection**:
- Compare individual average to network average
- Calculate z-score (standard deviations above mean)
- Flag outliers (z > 2.0)

**Signal**: `quality_z_score` — Higher = more anomalous

```sql
SELECT * FROM spider_inflation_detection WHERE quality_z_score >= 2.0;
```

### 4. Temporal Anomalies

**Pattern**: Burst stamping — many stamps in a short period.

**Detection**:
- Track stamps per day per validator
- Identify days with unusually high activity
- Flag bursts > 2 stddevs above validator's average

**Signal**: `burst_z_score` — Higher = more unusual

```sql
SELECT * FROM spider_burst_stamping WHERE burst_z_score >= 3.0;
```

### 5. Proxy Self-Dealing

**Pattern**: Validators who concentrate stamps on a small set of subjects.

**Detection**:
- Track unique subjects per validator
- Calculate concentration ratio
- Flag validators where >50% of stamps go to one subject

**Signal**: `concentration_score` — Higher = more concentrated

```sql
SELECT * FROM spider_suspicious_concentration WHERE top_subject_pct >= 0.6;
```

## Combined Fraud Score

The protocol combines all signals into a weighted fraud score:

| Signal | Weight |
|--------|--------|
| Collusion | 30% |
| Rubber-stamping | 25% |
| Inflation | 25% |
| Concentration | 20% |

```sql
SELECT * FROM spider_fraud_scores WHERE combined_fraud_score >= 0.6;
```

## Usage

### Initial Setup

```bash
cd ~/.wasteland/commons
dolt sql < /path/to/spider-protocol-queries.sql
```

### Regular Monitoring

```bash
# Check for active alerts
dolt sql -q "SELECT * FROM spider_alerts ORDER BY score DESC"

# Get fraud scores for all flagged rigs
dolt sql -q "SELECT * FROM spider_fraud_scores WHERE combined_fraud_score > 0.5"
```

### Investigating a Specific Rig

```bash
# Stamps they gave
dolt sql -q "SELECT * FROM spider_rig_stamps_given WHERE validator = 'suspect_handle'"

# Stamps they received
dolt sql -q "SELECT * FROM spider_rig_stamps_received WHERE rig = 'suspect_handle'"

# Their completions and associated stamps
dolt sql -q "SELECT * FROM spider_completion_stamps WHERE completed_by = 'suspect_handle'"
```

## Recommended Thresholds

| Metric | Suspicious | High Risk |
|--------|------------|-----------|
| Collusion score | >= 0.5 | >= 0.8 |
| Rubber stamp score | >= 0.5 | >= 0.8 |
| Inflation z-score | >= 1.5 | >= 2.5 |
| Concentration | >= 40% | >= 60% |
| Combined score | >= 0.4 | >= 0.6 |

Adjust thresholds based on network size — small networks may need looser thresholds.

## Limitations

1. **Cold start**: Needs sufficient stamp history to detect patterns
2. **False positives**: Legitimate mentorship relationships may trigger concentration alerts
3. **Sybil attacks**: Cannot detect when multiple rigs are controlled by same entity
4. **Timing**: Only detects patterns, not intent

## Integration Points

### Automated Monitoring

Run Spider Protocol queries on a schedule (daily recommended):

```bash
#!/bin/bash
cd ~/.wasteland/commons
dolt pull upstream main
dolt sql -q "SELECT * FROM spider_alerts" > /tmp/spider-alerts.txt
# Send alerts to Discord/Slack if non-empty
```

### Trust Tier Impact

Spider Protocol scores can inform trust tier decisions:

- **Score >= 0.6**: Block trust tier promotion
- **Score >= 0.8**: Consider trust tier demotion
- **Multiple alerts**: Manual review required

### DAO Integration

For decentralized governance:

1. Spider alerts trigger proposal creation
2. DAO votes on enforcement action
3. Smart contract executes trust level changes

## Future Enhancements

1. **Graph analysis**: PageRank-style influence metrics
2. **Behavioral clustering**: ML-based anomaly detection
3. **Cross-wasteland analysis**: Federated fraud detection
4. **Real-time alerts**: WebSocket notifications for new patterns
