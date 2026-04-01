#!/usr/bin/env bash
# doc-freshness/run.sh — Detect stale documentation.
#
# Tracks code-doc coupling: which .md files reference which code files/symbols.
# When code changes but the referencing docs don't, flags them as stale.

set -euo pipefail

TOWN_ROOT="${GT_TOWN_ROOT:-$(gt town root 2>/dev/null)}"

log() { echo "[doc-freshness] $*"; }

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

TOTAL_STALE=0
TOTAL_DEAD_REFS=0
TOTAL_CLI_DRIFT=0
CREATED=0

EXISTING=$(bd list --label doc-stale --status open --json 2>/dev/null || echo "[]")

while IFS=$'\t' read -r RIG_NAME REPO_PATH; do
  [ -z "$REPO_PATH" ] && continue
  [ -d "$REPO_PATH" ] || continue

  log ""
  log "=== Scanning: $RIG_NAME ($REPO_PATH) ==="

  cd "$REPO_PATH"

  # --- Step 1: Find all markdown docs ---
  MD_FILES=$(find . -name '*.md' -not -path './.git/*' -not -path './vendor/*' -not -path './node_modules/*' 2>/dev/null)
  MD_COUNT=$(echo "$MD_FILES" | grep -c . || echo "0")
  log "  Found $MD_COUNT markdown file(s)"

  # --- Step 2: Code-doc coupling — find stale docs ---
  log "  Analyzing code-doc coupling..."

  STALE_DOCS=$(python3 -c "
import os, re, subprocess, json
from datetime import datetime

repo = '$REPO_PATH'
stale = []

# Get all .md files
md_files = []
for root, dirs, files in os.walk(repo):
    dirs[:] = [d for d in dirs if d not in ('.git', 'vendor', 'node_modules', '.medici')]
    for f in files:
        if f.endswith('.md'):
            md_files.append(os.path.join(root, f))

for md_path in md_files:
    rel_md = os.path.relpath(md_path, repo)
    try:
        content = open(md_path).read()
    except:
        continue

    # Extract code references from the markdown:
    # 1. File paths: internal/foo/bar.go, cmd/foo.go
    # 2. Function names in backticks: \`FunctionName\`
    # 3. Command examples: gt <subcommand>, bd <subcommand>
    go_file_refs = set(re.findall(r'(?:internal|cmd|pkg)/[\w/]+\.go', content))
    func_refs = set(re.findall(r'\x60(\w{3,}(?:\.(?:go|toml|json|yaml))?)\x60', content))

    if not go_file_refs:
        continue

    # Get last modified time of the doc
    try:
        md_mtime = int(subprocess.check_output(
            ['git', '-C', repo, 'log', '-1', '--format=%ct', '--', rel_md],
            stderr=subprocess.DEVNULL, text=True).strip() or '0')
    except:
        continue

    if md_mtime == 0:
        continue

    # Check each referenced .go file
    stale_refs = []
    for go_ref in go_file_refs:
        go_path = os.path.join(repo, go_ref)
        if not os.path.exists(go_path):
            # Dead reference
            stale_refs.append(f'DEAD: {go_ref} (file no longer exists)')
            continue

        # Get last modified time of the code file
        try:
            code_mtime = int(subprocess.check_output(
                ['git', '-C', repo, 'log', '-1', '--format=%ct', '--', go_ref],
                stderr=subprocess.DEVNULL, text=True).strip() or '0')
        except:
            continue

        if code_mtime == 0:
            continue

        # Get lines changed since doc was last updated
        try:
            diff_stat = subprocess.check_output(
                ['git', '-C', repo, 'diff', '--stat',
                 f'--since-as-filter={datetime.fromtimestamp(md_mtime).isoformat()}',
                 '--', go_ref],
                stderr=subprocess.DEVNULL, text=True).strip()
        except:
            diff_stat = ''

        # Simpler: if code file was modified after doc, flag it
        if code_mtime > md_mtime:
            # Count how many commits changed the code since the doc was updated
            try:
                commit_count = int(subprocess.check_output(
                    ['git', '-C', repo, 'rev-list', '--count',
                     f'--since={md_mtime}', 'HEAD', '--', go_ref],
                    stderr=subprocess.DEVNULL, text=True).strip() or '0')
            except:
                commit_count = 0

            if commit_count >= 3:
                stale_refs.append(f'STALE: {go_ref} ({commit_count} commits since doc updated)')

    if stale_refs:
        stale.append({
            'doc': rel_md,
            'refs': stale_refs,
            'ref_count': len(stale_refs),
            'dead_count': sum(1 for r in stale_refs if r.startswith('DEAD')),
            'stale_count': sum(1 for r in stale_refs if r.startswith('STALE'))
        })

# Sort by most stale references first
stale.sort(key=lambda s: -s['ref_count'])

print(json.dumps(stale[:30]))
" 2>/dev/null || echo '[]')

  STALE_COUNT=$(echo "$STALE_DOCS" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  DEAD_COUNT=$(echo "$STALE_DOCS" | python3 -c "import json,sys; print(sum(d['dead_count'] for d in json.load(sys.stdin)))" 2>/dev/null || echo "0")

  log "  Found $STALE_COUNT stale doc(s), $DEAD_COUNT dead reference(s)"
  TOTAL_STALE=$((TOTAL_STALE + STALE_COUNT))
  TOTAL_DEAD_REFS=$((TOTAL_DEAD_REFS + DEAD_COUNT))

  # Print findings
  echo "$STALE_DOCS" | python3 -c "
import json, sys
docs = json.load(sys.stdin)
for d in docs[:15]:
    icon = '💀' if d['dead_count'] > 0 else '📝'
    print(f\"  {icon} {d['doc']} ({d['ref_count']} issue(s))\")
    for ref in d['refs'][:5]:
        print(f'      {ref}')
" 2>/dev/null

  # --- Step 3: CLI help drift ---
  log "  Checking CLI help drift..."

  if [ -f "$REPO_PATH/go.mod" ] && command -v gt >/dev/null 2>&1; then
    CLI_DRIFT=$(python3 -c "
import os, re, subprocess, json

repo = '$REPO_PATH'
drift = []

# Find docs that contain gt/bd command examples
for root, dirs, files in os.walk(repo):
    dirs[:] = [d for d in dirs if d not in ('.git', 'vendor', 'node_modules', '.medici')]
    for f in files:
        if not f.endswith('.md'): continue
        path = os.path.join(root, f)
        try:
            content = open(path).read()
        except:
            continue

        # Find command references like: gt estop, gt rig park, bd list
        commands = set(re.findall(r'(?:gt|bd)\s+(\w+(?:\s+\w+)?)', content))
        if not commands:
            continue

        # Check if those subcommands still exist
        missing = []
        for cmd in commands:
            parts = cmd.split()
            # Only check top-level subcommands (gt <cmd>)
            if len(parts) == 1:
                # Check if subcommand exists
                try:
                    result = subprocess.run(['gt', parts[0], '--help'],
                                          capture_output=True, timeout=5)
                    if result.returncode != 0 and b'unknown command' in result.stderr:
                        missing.append(f'gt {cmd}')
                except:
                    pass

        if missing:
            rel = os.path.relpath(path, repo)
            drift.append({'doc': rel, 'missing': missing})

print(json.dumps(drift[:10]))
" 2>/dev/null || echo '[]')

    CLI_COUNT=$(echo "$CLI_DRIFT" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    TOTAL_CLI_DRIFT=$((TOTAL_CLI_DRIFT + CLI_COUNT))

    if [ "$CLI_COUNT" -gt 0 ]; then
      log "  Found $CLI_COUNT doc(s) with CLI drift"
      echo "$CLI_DRIFT" | python3 -c "
import json, sys
for d in json.load(sys.stdin):
    print(f\"  ⚠️  {d['doc']}: references removed commands: {', '.join(d['missing'])}\")
" 2>/dev/null
    fi
  fi

  # --- Step 4: Create beads for stale docs ---
  if [ "$STALE_COUNT" -gt 0 ]; then
    echo "$STALE_DOCS" | python3 -c "
import json, sys, subprocess

docs = json.load(sys.stdin)
existing = json.loads('''$(echo "$EXISTING" | sed "s/'/\"/g")''') if '''$(echo "$EXISTING")''' != '[]' else []
existing_titles = {e.get('title','') for e in existing}
created = 0

for doc in docs:
    # Only create beads for docs with dead refs or significant staleness
    if doc['dead_count'] == 0 and doc['stale_count'] < 2:
        continue

    title = f\"doc-stale: {doc['doc']} ({doc['ref_count']} stale ref(s))\"
    if title in existing_titles:
        continue

    refs_text = '\n'.join(f'- {r}' for r in doc['refs'][:10])
    desc = f\"Documentation may be stale:\n\n{refs_text}\n\nRig: $RIG_NAME\"

    try:
        subprocess.run(['bd', 'create', title, '-t', 'task', '-p', '3',
                       '-d', desc, '-l', 'doc-stale', '--silent'],
                      capture_output=True, timeout=10)
        created += 1
    except: pass

print(created)
" 2>/dev/null | read -r NEW_CREATED || NEW_CREATED=0
    CREATED=$((CREATED + ${NEW_CREATED:-0}))
  fi

done <<< "$REPOS"

# --- Report -------------------------------------------------------------------

SUMMARY="$TOTAL_STALE stale doc(s), $TOTAL_DEAD_REFS dead ref(s), $TOTAL_CLI_DRIFT CLI drift, $CREATED bead(s) created"
log ""
log "=== Doc Freshness Summary: $SUMMARY ==="

bd create "doc-freshness: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:doc-freshness,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
