+++
name = "gitignore-reconcile"
description = "Auto-untrack files that are tracked but match an active .gitignore rule"
version = 1

[gate]
type = "cooldown"
duration = "6h"

[tracking]
labels = ["plugin:gitignore-reconcile", "category:git-hygiene"]
digest = true

[execution]
timeout = "10m"
notify_on_failure = true
severity = "low"
+++

# Gitignore Reconcile

Scans all rig repos for files that are tracked in git but now match an active
`.gitignore` rule. On clean `main` branches, runs `git rm --cached` to untrack
them and commits. On dirty branches or active polecat worktrees, creates a
chore bead instead to avoid interference.

Root cause: `.gitignore` rules only block NEW files. Files committed before the
rule was added continue to be tracked until manually untracked.

## Step 1: Enumerate rig repos

```bash
RIG_JSON=$(gt rig list --json 2>/dev/null)
if [ $? -ne 0 ] || [ -z "$RIG_JSON" ]; then
  echo "SKIP: could not get rig list"
  exit 0
fi

RIG_PATHS=$(echo "$RIG_JSON" | jq -r '.[] | select(.repo_path != null and .repo_path != "") | .repo_path // empty' 2>/dev/null)
if [ -z "$RIG_PATHS" ]; then
  echo "SKIP: no rigs with repo paths"
  exit 0
fi

RIG_COUNT=$(echo "$RIG_PATHS" | wc -l | tr -d ' ')
echo "Checking $RIG_COUNT rig repo(s) for tracked+ignored files"
```

## Step 2: For each rig repo, find and untrack gitignored files

```bash
TOTAL_UNTRACKED=0
TOTAL_BEADS=0
ERRORS=""

while IFS= read -r REPO_PATH; do
  [ -z "$REPO_PATH" ] && continue

  if ! git -C "$REPO_PATH" rev-parse --git-dir >/dev/null 2>&1; then
    continue
  fi

  echo ""
  echo "=== $REPO_PATH ==="

  # Find tracked files that match gitignore rules
  IGNORED_TRACKED=$(git -C "$REPO_PATH" ls-files --ignored --exclude-standard --cached 2>/dev/null)
  if [ -z "$IGNORED_TRACKED" ]; then
    echo "  Clean — no tracked+ignored files"
    continue
  fi

  FILE_COUNT=$(echo "$IGNORED_TRACKED" | wc -l | tr -d ' ')
  echo "  Found $FILE_COUNT tracked+ignored file(s)"

  # Check branch state
  CURRENT_BRANCH=$(git -C "$REPO_PATH" branch --show-current 2>/dev/null)
  IS_DIRTY=$(git -C "$REPO_PATH" status --porcelain 2>/dev/null | grep -v "^??" | head -1)
  HAS_POLECATS=$(git -C "$REPO_PATH" branch 2>/dev/null | grep -E "^\+?\s+polecat/" | head -1)

  if [ -n "$IS_DIRTY" ] || [ -n "$HAS_POLECATS" ] || [ "$CURRENT_BRANCH" != "main" ]; then
    # Create a chore bead instead of interfering
    REASON=""
    [ -n "$IS_DIRTY" ] && REASON="dirty working tree"
    [ -n "$HAS_POLECATS" ] && REASON="${REASON:+$REASON, }active polecat worktrees"
    [ "$CURRENT_BRANCH" != "main" ] && REASON="${REASON:+$REASON, }not on main ($CURRENT_BRANCH)"
    echo "  SKIP: $REASON — creating chore bead"
    REPO_NAME=$(basename "$REPO_PATH")
    bd create "gitignore-reconcile: $REPO_NAME has $FILE_COUNT tracked+ignored file(s)" \
      -t chore \
      -l "plugin:gitignore-reconcile,category:git-hygiene" \
      -d "Repo: $REPO_PATH\nSkipped: $REASON\nFiles:\n$IGNORED_TRACKED" \
      --silent 2>/dev/null || true
    TOTAL_BEADS=$((TOTAL_BEADS + 1))
    continue
  fi

  # Safe to untrack: clean main branch, no active polecats
  echo "$IGNORED_TRACKED" | while IFS= read -r FILE; do
    [ -z "$FILE" ] && continue
    echo "  Untracking: $FILE"
    git -C "$REPO_PATH" rm --cached "$FILE" 2>/dev/null || true
  done

  # Commit if anything was staged
  STAGED=$(git -C "$REPO_PATH" diff --cached --name-only 2>/dev/null)
  if [ -n "$STAGED" ]; then
    COUNT=$(echo "$STAGED" | wc -l | tr -d ' ')
    git -C "$REPO_PATH" commit -m "chore: untrack $COUNT file(s) now matched by .gitignore

Auto-committed by gitignore-reconcile plugin.
Files untracked:
$(echo "$STAGED" | head -10)$([ $(echo "$STAGED" | wc -l) -gt 10 ] && echo "...and more")" \
      --author="Gas Town <gastown@local>" 2>/dev/null || true
    echo "  Committed untracking of $COUNT file(s)"
    TOTAL_UNTRACKED=$((TOTAL_UNTRACKED + COUNT))

    # Push (best effort)
    git -C "$REPO_PATH" push origin main 2>/dev/null || echo "  WARN: push failed (committed locally)"
  fi
done
```

## Record Result

```bash
SUMMARY="gitignore-reconcile: $TOTAL_UNTRACKED file(s) untracked, $TOTAL_BEADS chore bead(s) created"
echo ""
echo "=== Gitignore Reconcile Summary ==="
echo "$SUMMARY"

RESULT="success"
[ -n "$ERRORS" ] && RESULT="warning"

bd create "$SUMMARY" -t chore --ephemeral \
  -l "type:plugin-run,plugin:gitignore-reconcile,result:$RESULT" \
  -d "$SUMMARY" --silent 2>/dev/null || true
```
