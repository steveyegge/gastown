#!/usr/bin/env bash
# code-quality-scanner/run.sh — Find refactoring opportunities via lint + churn.
#
# Combines golangci-lint (complexity, duplication, long functions) with git churn
# analysis. The intersection = highest-value refactoring targets.

set -euo pipefail

TOWN_ROOT="${GT_TOWN_ROOT:-$(gt town root 2>/dev/null)}"

log() { echo "[code-quality-scanner] $*"; }

# --- Discover rig repos -------------------------------------------------------

RIG_JSON=$(gt rig list --json 2>/dev/null) || { log "SKIP: could not get rig list"; exit 0; }

REPOS=$(echo "$RIG_JSON" | python3 -c "
import json, sys
rigs = json.load(sys.stdin)
for r in rigs:
    p = r.get('repo_path') or ''
    if p: print(f'{r.get(\"name\",\"\")}\t{p}')
" 2>/dev/null)

if [ -z "$REPOS" ]; then
  log "SKIP: no rig repos found"
  exit 0
fi

# --- Scan each repo -----------------------------------------------------------

TOTAL_HOTSPOTS=0
TOTAL_COMPLEX=0
TOTAL_LONG=0
TOTAL_DUPL=0
CREATED=0

# Get existing refactor beads for deduplication
EXISTING=$(bd list --label refactor-opportunity --status open --json 2>/dev/null || echo "[]")

while IFS=$'\t' read -r RIG_NAME REPO_PATH; do
  [ -z "$REPO_PATH" ] && continue
  [ -d "$REPO_PATH" ] || continue

  # Check if it's a Go project
  if [ ! -f "$REPO_PATH/go.mod" ]; then
    continue
  fi

  log ""
  log "=== Scanning: $RIG_NAME ($REPO_PATH) ==="

  # --- Step 1: Git churn analysis (last 90 days) ---
  log "  Analyzing git churn (90 days)..."
  CHURN=$(cd "$REPO_PATH" && git log --since="90 days ago" --format= --name-only -- '*.go' 2>/dev/null \
    | sort | uniq -c | sort -rn | head -30)

  # Build churn map: file -> change count
  declare -A CHURN_MAP
  while read -r count file; do
    [ -z "$file" ] && continue
    CHURN_MAP["$file"]=$count
  done <<< "$CHURN"

  # --- Step 2: Complexity analysis via golangci-lint ---
  log "  Running complexity analysis..."

  # Run golangci-lint with complexity-focused linters
  # Use --out-format json for parseable output
  LINT_OUTPUT=$(cd "$REPO_PATH" && golangci-lint run \
    --enable gocognit,cyclop,funlen \
    --disable-all \
    --enable gocognit --enable cyclop --enable funlen \
    --out-format json \
    --timeout 5m \
    --issues-exit-code 0 \
    2>/dev/null || echo '{"Issues":[]}')

  # --- Step 3: Parse and cross-reference with churn ---
  RESULTS=$(echo "$LINT_OUTPUT" | python3 -c "
import json, sys, os

churn_raw = '''$CHURN'''
churn_map = {}
for line in churn_raw.strip().split('\n'):
    line = line.strip()
    if not line: continue
    parts = line.split(None, 1)
    if len(parts) == 2:
        churn_map[parts[1]] = int(parts[0])

try:
    data = json.load(sys.stdin)
except:
    data = {'Issues': []}

issues = data.get('Issues') or []
hotspots = []

for issue in issues:
    file = issue.get('Pos', {}).get('Filename', '')
    line = issue.get('Pos', {}).get('Line', 0)
    linter = issue.get('FromLinter', '')
    text = issue.get('Text', '')

    # Get churn count for this file
    churn = churn_map.get(file, 0)

    # Classify
    category = 'smell'
    if 'cognitive complexity' in text.lower() or 'cyclomatic complexity' in text.lower():
        category = 'complexity'
    elif 'lines' in text.lower() and 'func' in text.lower():
        category = 'long-function'

    # Priority: high churn + high complexity = hotspot
    is_hotspot = churn >= 5 and category == 'complexity'

    hotspots.append({
        'file': file,
        'line': line,
        'linter': linter,
        'text': text,
        'category': category,
        'churn': churn,
        'hotspot': is_hotspot
    })

# Sort: hotspots first, then by churn
hotspots.sort(key=lambda h: (-h['hotspot'], -h['churn']))

# Summarize
complex_count = sum(1 for h in hotspots if h['category'] == 'complexity')
long_count = sum(1 for h in hotspots if h['category'] == 'long-function')
hotspot_count = sum(1 for h in hotspots if h['hotspot'])

print(json.dumps({
    'issues': hotspots[:50],  # Top 50 issues
    'complex': complex_count,
    'long_functions': long_count,
    'hotspots': hotspot_count,
    'total': len(hotspots)
}))
" 2>/dev/null || echo '{"issues":[],"complex":0,"long_functions":0,"hotspots":0,"total":0}')

  HOTSPOTS=$(echo "$RESULTS" | python3 -c "import json,sys; print(json.load(sys.stdin)['hotspots'])" 2>/dev/null || echo "0")
  COMPLEX=$(echo "$RESULTS" | python3 -c "import json,sys; print(json.load(sys.stdin)['complex'])" 2>/dev/null || echo "0")
  LONG=$(echo "$RESULTS" | python3 -c "import json,sys; print(json.load(sys.stdin)['long_functions'])" 2>/dev/null || echo "0")

  TOTAL_HOTSPOTS=$((TOTAL_HOTSPOTS + HOTSPOTS))
  TOTAL_COMPLEX=$((TOTAL_COMPLEX + COMPLEX))
  TOTAL_LONG=$((TOTAL_LONG + LONG))

  log "  Found: $HOTSPOTS hotspot(s), $COMPLEX complex, $LONG long function(s)"

  # --- Step 4: Create beads for hotspots (high churn + complex) ---
  if [ "$HOTSPOTS" -gt 0 ]; then
    echo "$RESULTS" | python3 -c "
import json, sys, subprocess

data = json.load(sys.stdin)
existing = json.loads('''$(echo "$EXISTING" | sed "s/'/\"/g")''') if '''$(echo "$EXISTING")''' != '[]' else []
existing_titles = {e.get('title','') for e in existing}
created = 0

for issue in data['issues']:
    if not issue['hotspot']:
        continue

    title = f\"refactor: {issue['file']}:{issue['line']} — {issue['category']} (churn={issue['churn']})\"
    if title in existing_titles:
        continue

    desc = f\"**{issue['text']}**\n\nFile: {issue['file']}:{issue['line']}\nChurn: {issue['churn']} changes in 90 days\nLinter: {issue['linter']}\nRig: $RIG_NAME\n\nThis file is both complex AND frequently changed — high-value refactoring target.\"

    try:
        subprocess.run(['bd', 'create', title, '-t', 'task', '-p', '3',
                       '-d', desc, '-l', 'refactor-opportunity', '--silent'],
                      capture_output=True, timeout=10)
        created += 1
    except: pass

print(created)
" 2>/dev/null | read -r NEW_CREATED || NEW_CREATED=0
    CREATED=$((CREATED + ${NEW_CREATED:-0}))
  fi

  # Print top hotspots
  echo "$RESULTS" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for h in data['issues'][:10]:
    icon = '🔥' if h['hotspot'] else '⚠️'
    print(f\"  {icon} {h['file']}:{h['line']} [{h['category']}] churn={h['churn']} — {h['text'][:80]}\")
" 2>/dev/null

  unset CHURN_MAP

done <<< "$REPOS"

# --- Report -------------------------------------------------------------------

SUMMARY="$TOTAL_HOTSPOTS hotspot(s), $TOTAL_COMPLEX complex, $TOTAL_LONG long func(s), $CREATED bead(s) created"
log ""
log "=== Code Quality Summary: $SUMMARY ==="

bd create "code-quality-scanner: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:code-quality-scanner,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
