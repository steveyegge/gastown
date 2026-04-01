#!/usr/bin/env bash
# quality-review/run.sh — Analyze merge quality trends from refinery data.
#
# Queries quality-review result wisps from last 24h, computes per-worker
# trends, and alerts on quality breaches.

set -euo pipefail

log() { echo "[quality-review] $*"; }

# --- Query results ------------------------------------------------------------

log "Fetching quality-review results from last 24h..."

RESULTS=$(bd list --json --all -l type:plugin-run,plugin:quality-review-result --limit 0 2>/dev/null || echo "[]")

# Filter to last 24h and compute trends with python
ANALYSIS=$(python3 -c "
import json, sys
from datetime import datetime, timedelta, timezone

results = json.loads('''$(echo "$RESULTS" | sed "s/'/\"/g")''')
if not results:
    print(json.dumps({'workers': [], 'total': 0, 'breaches': 0, 'warnings': 0}))
    sys.exit(0)

cutoff = datetime.now(timezone.utc) - timedelta(hours=24)

# Parse worker data from labels
workers = {}
for r in results:
    created = r.get('created_at', '')
    try:
        dt = datetime.fromisoformat(created.replace('Z', '+00:00'))
        if dt < cutoff:
            continue
    except:
        continue

    labels = {l.get('name',''): l.get('name','') for l in r.get('labels', [])}
    # Extract from label format: worker:name, score:0.85, etc.
    worker = None
    score = None
    rig = None
    rec = None
    for l in r.get('labels', []):
        name = l.get('name', '')
        if name.startswith('worker:'):
            worker = name.split(':', 1)[1]
        elif name.startswith('score:'):
            try: score = float(name.split(':', 1)[1])
            except: pass
        elif name.startswith('rig:'):
            rig = name.split(':', 1)[1]
        elif name.startswith('recommendation:'):
            rec = name.split(':', 1)[1]

    if worker and score is not None:
        if worker not in workers:
            workers[worker] = {'scores': [], 'rejections': 0, 'total': 0, 'rig': rig or '?'}
        workers[worker]['scores'].append(score)
        workers[worker]['total'] += 1
        if rec == 'request_changes':
            workers[worker]['rejections'] += 1

# Compute trends
output = []
breaches = 0
warnings = 0
for name, data in workers.items():
    scores = data['scores']
    avg = sum(scores) / len(scores)
    rejection_rate = data['rejections'] / data['total'] if data['total'] > 0 else 0

    # Trend: compare first half vs second half
    mid = len(scores) // 2
    if mid > 0:
        first_half = sum(scores[:mid]) / mid
        second_half = sum(scores[mid:]) / len(scores[mid:])
        diff = second_half - first_half
        if diff > 0.05: trend = 'improving'
        elif diff < -0.05: trend = 'declining'
        else: trend = 'stable'
    else:
        trend = 'stable'

    if avg < 0.45:
        status = 'BREACH'
        breaches += 1
    elif avg < 0.60:
        status = 'WARN'
        warnings += 1
    else:
        status = 'OK'

    output.append({
        'name': name, 'rig': data['rig'], 'avg': round(avg, 2),
        'count': data['total'], 'rejections': data['rejections'],
        'rejection_rate': round(rejection_rate * 100, 1),
        'trend': trend, 'status': status
    })

print(json.dumps({
    'workers': sorted(output, key=lambda w: w['avg']),
    'total': sum(d['total'] for d in workers.values()),
    'breaches': breaches,
    'warnings': warnings
}))
" 2>/dev/null || echo '{"workers":[],"total":0,"breaches":0,"warnings":0}')

TOTAL=$(echo "$ANALYSIS" | python3 -c "import json,sys; print(json.load(sys.stdin)['total'])" 2>/dev/null || echo "0")
WORKER_COUNT=$(echo "$ANALYSIS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['workers']))" 2>/dev/null || echo "0")
BREACHES=$(echo "$ANALYSIS" | python3 -c "import json,sys; print(json.load(sys.stdin)['breaches'])" 2>/dev/null || echo "0")
WARNINGS=$(echo "$ANALYSIS" | python3 -c "import json,sys; print(json.load(sys.stdin)['warnings'])" 2>/dev/null || echo "0")

if [ "$TOTAL" -eq 0 ]; then
  log "No quality-review results in last 24h. Nothing to analyze."
  bd create "quality-review: No results in last 24h" -t chore --ephemeral \
    -l type:plugin-run,plugin:quality-review,result:success \
    -d "No quality-review results in last 24h." --silent 2>/dev/null || true
  exit 0
fi

# --- Print trends -------------------------------------------------------------

log "Analyzed $WORKER_COUNT workers over $TOTAL reviews:"
echo "$ANALYSIS" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for w in data['workers']:
    icon = '🔴' if w['status'] == 'BREACH' else ('🟡' if w['status'] == 'WARN' else '🟢')
    print(f\"  {icon} {w['name']} ({w['rig']}): avg={w['avg']} reviews={w['count']} rejections={w['rejection_rate']}% trend={w['trend']}\")
" 2>/dev/null

# --- Alert on breaches --------------------------------------------------------

if [ "$BREACHES" -gt 0 ]; then
  log "Sending breach alerts..."
  echo "$ANALYSIS" | python3 -c "
import json, sys, subprocess
data = json.load(sys.stdin)
for w in data['workers']:
    if w['status'] != 'BREACH': continue
    msg = f\"Worker: {w['name']}\nRig: {w['rig']}\nAvg Score: {w['avg']}\nReviews: {w['count']}\nRejection Rate: {w['rejection_rate']}%\nTrend: {w['trend']}\n\nAction: Review recent merges from this worker for quality issues.\"
    subprocess.run(['gt', 'mail', 'send', 'mayor/', '-s', f\"Quality BREACH: {w['name']}\", '--stdin'],
                  input=msg, text=True, capture_output=True, timeout=10)
    subprocess.run(['gt', 'escalate', f\"Quality BREACH: {w['name']} (avg: {w['avg']})\",
                   '-s', 'medium'], capture_output=True, timeout=10)
" 2>/dev/null
fi

# --- Report -------------------------------------------------------------------

SUMMARY="$WORKER_COUNT worker(s), $TOTAL review(s): $BREACHES breach(es), $WARNINGS warning(s)"
log "=== Quality Review Summary: $SUMMARY ==="

bd create "quality-review: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:quality-review,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
