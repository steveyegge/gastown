+++
name = "guardian-trend-monitor"
description = "Monitor Guardian quality scores and escalate when workers are in BREACH state"
version = 1

[gate]
type = "cooldown"
duration = "30m"

[tracking]
labels = ["plugin:guardian-trend-monitor", "category:quality-monitoring"]
digest = true

[execution]
timeout = "1m"
notify_on_failure = true
severity = "medium"
+++

# Guardian Trend Monitor

Periodically checks Guardian quality judgment scores and escalates when any
worker's average score falls into BREACH territory (< 0.45). Reads data from
`gt judgment status --json` (populated by the Refinery's `quality-review`
formula step via `gt judgment record`).

Phase 1 (measurement-only): scores are recorded but do not gate merges. This
plugin provides async visibility into quality trends so operators can intervene
before problems accumulate.

Requires: `gt judgment` CLI (available when Guardian persistence is enabled).

## Detection

Check if judgment data exists:

```bash
STATUS=$(gt judgment status --json 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$STATUS" ]; then
  echo "SKIP: gt judgment status not available or no data"
  exit 0
fi

WORKER_COUNT=$(echo "$STATUS" | jq 'length')
if [ "$WORKER_COUNT" -eq 0 ]; then
  echo "SKIP: no judgment data recorded yet"
  exit 0
fi
```

## Action

### Step 1: Identify workers in BREACH or WARN state

```bash
BREACH_WORKERS=$(echo "$STATUS" | jq -c '[.[] | select(.status == "BREACH")]')
WARN_WORKERS=$(echo "$STATUS" | jq -c '[.[] | select(.status == "WARN")]')

BREACH_COUNT=$(echo "$BREACH_WORKERS" | jq 'length')
WARN_COUNT=$(echo "$WARN_WORKERS" | jq 'length')
OK_COUNT=$(echo "$STATUS" | jq '[.[] | select(.status == "OK")] | length')
```

### Step 2: Escalate BREACH workers via mail

```bash
if [ "$BREACH_COUNT" -gt 0 ]; then
  BREACH_DETAILS=$(echo "$BREACH_WORKERS" | jq -r '.[] | "\(.worker) (rig: \(.rig), avg: \(.avg_score | tostring), reviews: \(.total_reviews))"')

  MAIL_BODY="Guardian quality BREACH detected for $BREACH_COUNT worker(s):

$BREACH_DETAILS

Average score below 0.45 threshold. Review recent merges for quality issues.

Run 'gt judgment history --polecat <worker>' for details."

  gt mail send --subject "Guardian BREACH: $BREACH_COUNT worker(s) below quality threshold" \
    --body "$MAIL_BODY" \
    2>/dev/null || true

  gt activity emit guardian_breach \
    --message "Guardian BREACH: $BREACH_COUNT worker(s) below quality threshold" \
    2>/dev/null || true
fi
```

### Step 3: Log warnings (no escalation)

```bash
if [ "$WARN_COUNT" -gt 0 ]; then
  WARN_DETAILS=$(echo "$WARN_WORKERS" | jq -r '.[] | "\(.worker) (avg: \(.avg_score | tostring))"')

  gt activity emit guardian_warn \
    --message "Guardian WARN: $WARN_COUNT worker(s) approaching quality threshold: $WARN_DETAILS" \
    2>/dev/null || true
fi
```

## Record Result

```bash
SUMMARY="checked $WORKER_COUNT workers: $OK_COUNT ok, $WARN_COUNT warn, $BREACH_COUNT breach"
echo "$SUMMARY"
```

On success:
```bash
bd create "guardian-trend-monitor: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:guardian-trend-monitor,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "guardian-trend-monitor: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:guardian-trend-monitor,result:failure \
  -d "Guardian trend monitor failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: guardian-trend-monitor" \
  --severity low \
  --reason "$ERROR"
```
