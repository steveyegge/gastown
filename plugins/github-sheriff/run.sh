#!/usr/bin/env bash
# github-sheriff/run.sh — Monitor GitHub CI on open PRs, create beads for failures.
#
# Categorizes PRs as easy-wins or needs-review. Creates ci-failure beads
# for new CI failures (deduplicates against existing beads).

set -euo pipefail

TOWN_ROOT="${GT_TOWN_ROOT:-$(gt town root 2>/dev/null)}"

log() { echo "[github-sheriff] $*"; }

# --- Preflight ---------------------------------------------------------------

gh auth status 2>/dev/null || {
  log "SKIP: gh CLI not authenticated"
  exit 0
}

# Discover repos from all rigs
RIG_JSON=$(gt rig list --json 2>/dev/null) || {
  log "SKIP: could not get rig list"
  exit 0
}

REPOS=$(echo "$RIG_JSON" | python3 -c "
import json, sys, subprocess
rigs = json.load(sys.stdin)
seen = set()
for r in rigs:
    p = r.get('repo_path') or ''
    if not p: continue
    try:
        url = subprocess.check_output(['git', '-C', p, 'remote', 'get-url', 'origin'],
                                        stderr=subprocess.DEVNULL, text=True).strip()
        # Extract owner/repo from git URL
        import re
        m = re.search(r'github\.com[:/](.+?)(?:\.git)?$', url)
        if m:
            repo = m.group(1)
            if repo not in seen:
                seen.add(repo)
                print(f'{repo}\t{r.get(\"name\",\"\")}\t{p}')
    except: pass
" 2>/dev/null)

if [ -z "$REPOS" ]; then
  log "SKIP: no GitHub repos found"
  exit 0
fi

REPO_COUNT=$(echo "$REPOS" | wc -l | tr -d ' ')
log "Checking $REPO_COUNT repo(s)..."

# --- Process each repo --------------------------------------------------------

TOTAL_PRS=0
TOTAL_EASY=0
TOTAL_REVIEW=0
TOTAL_CREATED=0

# Date 7 days ago (macOS compatible)
SINCE=$(date -v-7d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -d '7 days ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "2020-01-01T00:00:00Z")

while IFS=$'\t' read -r REPO RIG_NAME RIG_PATH; do
  [ -z "$REPO" ] && continue
  log ""
  log "=== $REPO ==="

  PRS=$(gh pr list --repo "$REPO" --state open \
    --json number,title,author,additions,deletions,mergeable,statusCheckRollup,url,updatedAt \
    --limit 100 2>/dev/null) || {
    log "  Failed to fetch PRs for $REPO"
    continue
  }

  # Filter to recent PRs
  PRS=$(echo "$PRS" | python3 -c "
import json, sys
prs = json.load(sys.stdin)
since = '$SINCE'
print(json.dumps([p for p in prs if p.get('updatedAt','') >= since]))
" 2>/dev/null)

  PR_COUNT=$(echo "$PRS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

  if [ "$PR_COUNT" -eq 0 ]; then
    log "  No recent open PRs"
    continue
  fi

  TOTAL_PRS=$((TOTAL_PRS + PR_COUNT))

  # Categorize PRs
  RESULT=$(echo "$PRS" | python3 -c "
import json, sys

prs = json.load(sys.stdin)
easy_wins = []
needs_review = []
failures = []

for pr in prs:
    num = pr['number']
    title = pr['title']
    author = pr.get('author', {}).get('login', 'unknown')
    adds = pr.get('additions', 0)
    dels = pr.get('deletions', 0)
    total = adds + dels
    mergeable = pr.get('mergeable', '')
    checks = pr.get('statusCheckRollup', []) or []

    total_checks = len(checks)
    passing = sum(1 for c in checks if c.get('conclusion') in ('SUCCESS','NEUTRAL','SKIPPED') or c.get('state') == 'SUCCESS')
    ci_pass = total_checks > 0 and total_checks == passing

    # Collect failures
    for c in checks:
        conc = c.get('conclusion', '')
        state = c.get('state', '')
        if conc in ('FAILURE','CANCELLED','TIMED_OUT') or state in ('FAILURE','ERROR'):
            failures.append(f\"{num}|{title}|{c.get('name','')}|{c.get('detailsUrl','')}\")

    if mergeable == 'MERGEABLE' and ci_pass and total < 200:
        easy_wins.append(f'PR #{num}: {title} (by {author}, +{adds}/-{dels})')
    else:
        reasons = []
        if mergeable != 'MERGEABLE': reasons.append('conflicts')
        if not ci_pass: reasons.append('ci-failing')
        if total >= 200: reasons.append(f'large({total}loc)')
        needs_review.append(f'PR #{num}: {title} (by {author}, {\" \".join(reasons)})')

print(json.dumps({'easy_wins': easy_wins, 'needs_review': needs_review, 'failures': failures}))
" 2>/dev/null)

  EASY=$(echo "$RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['easy_wins']))" 2>/dev/null || echo "0")
  REVIEW=$(echo "$RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['needs_review']))" 2>/dev/null || echo "0")
  FAIL_COUNT=$(echo "$RESULT" | python3 -c "import json,sys; print(len(json.load(sys.stdin)['failures']))" 2>/dev/null || echo "0")

  TOTAL_EASY=$((TOTAL_EASY + EASY))
  TOTAL_REVIEW=$((TOTAL_REVIEW + REVIEW))

  if [ "$EASY" -gt 0 ]; then
    log "  Easy wins ($EASY):"
    echo "$RESULT" | python3 -c "import json,sys; [print(f'    {w}') for w in json.load(sys.stdin)['easy_wins']]" 2>/dev/null
  fi
  if [ "$REVIEW" -gt 0 ]; then
    log "  Needs review ($REVIEW):"
    echo "$RESULT" | python3 -c "import json,sys; [print(f'    {w}') for w in json.load(sys.stdin)['needs_review']]" 2>/dev/null
  fi

  # Create CI failure beads (only for repos we own)
  REPO_OWNER=$(echo "$REPO" | cut -d'/' -f1)
  if [ "$REPO_OWNER" = "outdoorsea" ] && [ "$FAIL_COUNT" -gt 0 ]; then
    # Get existing ci-failure beads for deduplication
    EXISTING=$(bd list --label ci-failure --status open --json 2>/dev/null || echo "[]")

    echo "$RESULT" | python3 -c "
import json, sys, subprocess

data = json.load(sys.stdin)
existing = json.loads('''$(echo "$EXISTING" | tr "'" '"')''') if '''$(echo "$EXISTING")''' != '[]' else []
existing_titles = {e.get('title','') for e in existing}
created = 0

for f in data['failures']:
    parts = f.split('|', 3)
    if len(parts) < 3: continue
    pr_num, pr_title, check_name = parts[0], parts[1], parts[2]
    check_url = parts[3] if len(parts) > 3 else ''

    bead_title = f'CI failure: {check_name} on PR #{pr_num}'
    if bead_title in existing_titles:
        continue

    desc = f'CI check \`{check_name}\` failed on PR #{pr_num} ({pr_title})\nPR: https://github.com/$REPO/pull/{pr_num}'
    if check_url:
        desc += f'\nCheck: {check_url}'

    try:
        subprocess.run(['bd', 'create', bead_title, '-t', 'task', '-p', '2',
                       '-d', desc, '-l', 'ci-failure', '--silent'],
                      capture_output=True, timeout=10)
        created += 1
    except: pass

print(created)
" 2>/dev/null | read -r NEW_CREATED || NEW_CREATED=0
    TOTAL_CREATED=$((TOTAL_CREATED + ${NEW_CREATED:-0}))
  fi

done <<< "$REPOS"

# --- Report -------------------------------------------------------------------

SUMMARY="$REPO_COUNT repo(s), $TOTAL_PRS PR(s): $TOTAL_EASY easy win(s), $TOTAL_REVIEW need review, $TOTAL_CREATED bead(s) created"
log ""
log "=== GitHub Sheriff Summary ==="
log "$SUMMARY"

bd create "github-sheriff: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:github-sheriff,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
