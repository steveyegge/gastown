#!/usr/bin/env bash
# outbound-pr-tracker/run.sh — Track our PRs on upstream repos.
#
# Monitors CI status, maintainer feedback, and merge/close events for PRs
# we've submitted to upstream repos. Creates beads for actionable items.

set -euo pipefail

log() { echo "[outbound-pr-tracker] $*"; }

# --- Preflight ---------------------------------------------------------------

gh auth status 2>/dev/null || { log "SKIP: gh CLI not authenticated"; exit 0; }

# Discover rigs and their upstream remotes
RIG_JSON=$(gt rig list --json 2>/dev/null) || { log "SKIP: could not get rig list"; exit 0; }

# Build list of upstream repos with their rig info
REPOS=$(echo "$RIG_JSON" | python3 -c "
import json, sys, subprocess, re
rigs = json.load(sys.stdin)
seen = set()
for r in rigs:
    p = r.get('repo_path') or ''
    if not p: continue
    try:
        upstream = subprocess.check_output(['git', '-C', p, 'remote', 'get-url', 'upstream'],
                                           stderr=subprocess.DEVNULL, text=True).strip()
        origin = subprocess.check_output(['git', '-C', p, 'remote', 'get-url', 'origin'],
                                         stderr=subprocess.DEVNULL, text=True).strip()
        m_up = re.search(r'github\.com[:/](.+?)(?:\.git)?$', upstream)
        m_or = re.search(r'github\.com[:/](.+?)(?:\.git)?$', origin)
        if m_up and m_or:
            repo = m_up.group(1)
            org = m_or.group(1).split('/')[0]
            if repo not in seen:
                seen.add(repo)
                print(f'{repo}\t{org}\t{r.get(\"name\",\"\")}')
    except: pass
" 2>/dev/null)

if [ -z "$REPOS" ]; then
  log "SKIP: no upstream repos found"
  exit 0
fi

REPO_COUNT=$(echo "$REPOS" | wc -l | tr -d ' ')
log "Tracking outbound PRs for $REPO_COUNT upstream repo(s)..."

SINCE=$(date -v-7d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "2020-01-01T00:00:00Z")

TOTAL_OPEN=0
TOTAL_MERGED=0
TOTAL_CREATED=0

# --- Process each upstream repo -----------------------------------------------

while IFS=$'\t' read -r UPSTREAM OUR_ORG RIG_NAME; do
  [ -z "$UPSTREAM" ] && continue
  log ""
  log "=== $UPSTREAM (author: $OUR_ORG) ==="

  # Fetch open PRs by our org
  OPEN_PRS=$(gh pr list --repo "$UPSTREAM" --author "$OUR_ORG" --state open \
    --json number,title,url,statusCheckRollup,reviews,comments,updatedAt \
    --limit 50 2>/dev/null || echo "[]")

  OPEN_COUNT=$(echo "$OPEN_PRS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  TOTAL_OPEN=$((TOTAL_OPEN + OPEN_COUNT))

  # Fetch recently closed PRs
  CLOSED_PRS=$(gh pr list --repo "$UPSTREAM" --author "$OUR_ORG" --state closed \
    --json number,title,url,state,mergedAt,closedAt,updatedAt \
    --limit 50 2>/dev/null || echo "[]")

  # Categorize with python
  RESULT=$(python3 -c "
import json, sys

open_prs = json.loads('''$(echo "$OPEN_PRS")''')
closed_prs = json.loads('''$(echo "$CLOSED_PRS")''')
since = '$SINCE'

ci_failing = []
changes_requested = []
comments = []
approved = []
waiting = []

for pr in open_prs:
    n, t, u = pr['number'], pr['title'], pr['url']
    checks = pr.get('statusCheckRollup') or []
    total = len(checks)
    passing = sum(1 for c in checks if c.get('conclusion') in ('SUCCESS','NEUTRAL','SKIPPED') or c.get('state') == 'SUCCESS')
    ci_pass = total > 0 and total == passing

    failed = [c.get('name','?') for c in checks if c.get('conclusion') in ('FAILURE','CANCELLED','TIMED_OUT') or c.get('state') in ('FAILURE','ERROR')]
    if failed:
        ci_failing.append({'num': n, 'title': t, 'url': u, 'checks': ', '.join(failed)})

    reviews = pr.get('reviews') or []
    has_cr = any(r.get('state') == 'CHANGES_REQUESTED' for r in reviews)
    has_ap = any(r.get('state') == 'APPROVED' for r in reviews)
    cmts = pr.get('comments') or []

    if has_cr:
        reviewer = next((r.get('author',{}).get('login','?') for r in reversed(reviews) if r.get('state') == 'CHANGES_REQUESTED'), '?')
        changes_requested.append({'num': n, 'title': t, 'url': u, 'reviewer': reviewer})
    elif has_ap:
        approved.append({'num': n, 'title': t, 'url': u})
    elif cmts:
        last = cmts[-1].get('author',{}).get('login','')
        if last != '$OUR_ORG':
            comments.append({'num': n, 'title': t, 'url': u, 'commenter': last})
        else:
            waiting.append({'num': n, 'title': t, 'url': u, 'ci': 'pass' if ci_pass else 'fail'})
    else:
        waiting.append({'num': n, 'title': t, 'url': u, 'ci': 'pass' if ci_pass else 'fail'})

merged = [{'num': p['number'], 'title': p['title'], 'url': p['url']}
          for p in closed_prs if p.get('mergedAt') and (p.get('closedAt','') >= since or p.get('updatedAt','') >= since)]
rejected = [{'num': p['number'], 'title': p['title'], 'url': p['url']}
            for p in closed_prs if not p.get('mergedAt') and (p.get('closedAt','') >= since or p.get('updatedAt','') >= since)]

print(json.dumps({'ci_failing': ci_failing, 'changes_requested': changes_requested,
                   'comments': comments, 'approved': approved, 'waiting': waiting,
                   'merged': merged, 'rejected': rejected}))
" 2>/dev/null || echo '{}')

  # Print summary
  for cat in ci_failing changes_requested comments approved merged rejected waiting; do
    COUNT=$(echo "$RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('$cat',[])))" 2>/dev/null || echo "0")
    [ "$COUNT" -gt 0 ] && log "  $cat: $COUNT"
  done

  MERGED_COUNT=$(echo "$RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('merged',[])))" 2>/dev/null || echo "0")
  TOTAL_MERGED=$((TOTAL_MERGED + MERGED_COUNT))

  # Create beads for actionable items
  EXISTING=$(bd list --label outbound-pr --status open --json 2>/dev/null || echo "[]")

  CREATED=$(echo "$RESULT" | python3 -c "
import json, sys, subprocess

data = json.load(sys.stdin)
existing = json.loads('''$(echo "$EXISTING" | sed "s/'/\"/g")''') if '''$(echo "$EXISTING")''' != '[]' else []
existing_titles = {e.get('title','') for e in existing}
created = 0

for item in data.get('ci_failing', []):
    title = f\"Outbound PR #{item['num']}: CI failing ({item['checks']})\"
    if title in existing_titles: continue
    try:
        subprocess.run(['bd', 'create', title, '-t', 'task', '-p', '2',
                       '-d', f\"CI failing on {item['url']}\nChecks: {item['checks']}\",
                       '-l', 'outbound-pr,ci-failure', '--silent'],
                      capture_output=True, timeout=10)
        created += 1
    except: pass

for item in data.get('changes_requested', []):
    title = f\"Outbound PR #{item['num']}: changes requested by {item['reviewer']}\"
    if title in existing_titles: continue
    try:
        subprocess.run(['bd', 'create', title, '-t', 'task', '-p', '2',
                       '-d', f\"Changes requested on {item['url']}\nReviewer: {item['reviewer']}\",
                       '-l', 'outbound-pr,changes-requested', '--silent'],
                      capture_output=True, timeout=10)
        created += 1
    except: pass

for item in data.get('merged', []):
    for e in existing:
        if f\"#{item['num']}:\" in e.get('title', ''):
            try:
                subprocess.run(['bd', 'close', e['id'], '--reason', f\"merged: PR #{item['num']}\"],
                              capture_output=True, timeout=10)
            except: pass

print(created)
" 2>/dev/null || echo "0")
  TOTAL_CREATED=$((TOTAL_CREATED + CREATED))

done <<< "$REPOS"

# --- Report -------------------------------------------------------------------

SUMMARY="$REPO_COUNT repo(s): $TOTAL_OPEN open, $TOTAL_MERGED merged, $TOTAL_CREATED bead(s) created"
log ""
log "=== Outbound PR Tracker Summary ==="
log "$SUMMARY"

bd create "outbound-pr-tracker: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:outbound-pr-tracker,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
