#!/usr/bin/env bash
# submodule-gitignore/run.sh — Inject Gas Town gitignore entries into rig project repos.
#
# Scans all rigs, finds their project repo checkouts (mayor/rig/), and ensures
# Gas Town operational files are gitignored. Commits + pushes changes with || true.
# Idempotent via guard marker comment.

set -euo pipefail

GUARD_MARKER="# Gas Town operational files (managed by submodule-gitignore plugin)"

# Gas Town files that should be gitignored in project repos.
# These are created by GT at runtime and must not leak into project history.
# NOTE: .beads/ is NOT included — beads manages its own .beads/.gitignore
# (created by bd init) which selectively ignores runtime files. Adding .beads/
# here would override that and break bd sync. This has regressed before.
GT_PATTERNS=(
  ".runtime/"
  ".claude/"
  ".logs/"
  "state.json"
  "config.json"
  "__pycache__/"
  "crew/"
  "polecats/"
  "refinery/"
  "witness/"
  "mayor/"
  "archive/"
  ".beads/.locks/"
  ".beads/locks/"
  ".beads/audit.log"
  ".beads/metadata.json"
  ".beads/PRIME.md"
  ".beads/.gt-types-configured"
  ".beads/backup/"
  ".beads/dolt-server.port"
  ".repo.git/"
  ".land-worktree/"
)

log() { echo "[submodule-gitignore] $*"; }

# --- Discover town root -------------------------------------------------------

GT_ROOT="${GT_ROOT:-$HOME/gt}"
if [ ! -d "$GT_ROOT" ]; then
  log "ERROR: GT_ROOT=$GT_ROOT does not exist"
  exit 1
fi

# --- Enumerate rigs ------------------------------------------------------------

RIG_JSON=$(gt rig list --json 2>/dev/null) || {
  log "SKIP: could not get rig list"
  exit 0
}

RIG_NAMES=$(echo "$RIG_JSON" | python3 -c "
import json, sys
rigs = json.load(sys.stdin)
for r in rigs:
    print(r['name'])
" 2>/dev/null) || {
  log "SKIP: could not parse rig list"
  exit 0
}

if [ -z "$RIG_NAMES" ]; then
  log "SKIP: no rigs found"
  exit 0
fi

RIG_COUNT=$(echo "$RIG_NAMES" | wc -l | tr -d ' ')
log "Found $RIG_COUNT rig(s) to scan"

# --- Helper: check if pattern is covered by existing gitignore -----------------

pattern_covered() {
  local content="$1"
  local pattern="$2"

  # Strip leading slash for comparison
  local norm_pattern="${pattern#/}"

  while IFS= read -r line; do
    line="${line#"${line%%[![:space:]]*}"}"  # trim leading whitespace
    [ -z "$line" ] && continue
    [[ "$line" == \#* ]] && continue  # skip comments

    local norm_line="${line#/}"

    # Exact match
    [ "$norm_line" = "$norm_pattern" ] && return 0

    # Trailing slash variants
    [ "$norm_line" = "${norm_pattern%/}" ] && return 0
    [ "${norm_line}/" = "$norm_pattern" ] && return 0

    # Broader directory covers specific subpath (e.g. .beads/ covers .beads/locks/)
    if [[ "$norm_line" == */ ]] && [[ "$norm_pattern" == "$norm_line"* ]]; then
      return 0
    fi
  done <<< "$content"

  return 1
}

# --- Process each rig ----------------------------------------------------------

UPDATED=0
SKIPPED=0
ERRORS=()

while IFS= read -r RIG_NAME; do
  [ -z "$RIG_NAME" ] && continue

  # Find the rig's project repo checkout
  REPO_PATH="$GT_ROOT/$RIG_NAME/mayor/rig"

  if [ ! -d "$REPO_PATH" ]; then
    log "  $RIG_NAME: no mayor/rig checkout, skipping"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  # Verify it's a git repo
  if ! git -C "$REPO_PATH" rev-parse --git-dir >/dev/null 2>&1; then
    log "  $RIG_NAME: mayor/rig is not a git repo, skipping"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  GITIGNORE="$REPO_PATH/.gitignore"

  # Read existing content
  EXISTING=""
  if [ -f "$GITIGNORE" ]; then
    EXISTING=$(cat "$GITIGNORE")
  fi

  # Check for guard marker — if present, already managed
  if echo "$EXISTING" | grep -qF "$GUARD_MARKER"; then
    log "  $RIG_NAME: guard block present, skipping"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  # Find missing patterns
  MISSING=()
  for pattern in "${GT_PATTERNS[@]}"; do
    if ! pattern_covered "$EXISTING" "$pattern"; then
      MISSING+=("$pattern")
    fi
  done

  if [ ${#MISSING[@]} -eq 0 ]; then
    log "  $RIG_NAME: all patterns present (no guard marker, but covered)"
    SKIPPED=$((SKIPPED + 1))
    continue
  fi

  log "  $RIG_NAME: injecting ${#MISSING[@]} pattern(s)"

  # Append guard block with missing patterns
  {
    # Ensure we start on a new line
    if [ -n "$EXISTING" ] && [ "$(tail -c1 "$GITIGNORE" 2>/dev/null | wc -l)" -eq 0 ]; then
      echo ""
    fi
    echo ""
    echo "$GUARD_MARKER"
    for pattern in "${MISSING[@]}"; do
      echo "$pattern"
    done
  } >> "$GITIGNORE"

  # Commit + push (|| true — best effort, don't fail the plugin)
  if git -C "$REPO_PATH" add .gitignore 2>/dev/null; then
    if git -C "$REPO_PATH" diff --cached --quiet 2>/dev/null; then
      log "  $RIG_NAME: no effective changes after staging"
    else
      COMMIT_MSG="chore: add Gas Town gitignore entries

Injected by submodule-gitignore plugin. Prevents Gas Town operational
files from appearing as untracked in the project repository."

      if git -C "$REPO_PATH" commit -m "$COMMIT_MSG" 2>/dev/null; then
        log "  $RIG_NAME: committed gitignore update"
        if git -C "$REPO_PATH" push 2>/dev/null; then
          log "  $RIG_NAME: pushed to remote"
        else
          log "  $RIG_NAME: WARN: push failed (will retry next run)"
        fi

        # Update parent submodule pointer if applicable
        PARENT_DIR="$GT_ROOT/$RIG_NAME"
        if [ -f "$PARENT_DIR/.gitmodules" ] 2>/dev/null; then
          git -C "$PARENT_DIR" add mayor/rig 2>/dev/null || true
          git -C "$PARENT_DIR" commit -m "chore: update mayor/rig submodule pointer" 2>/dev/null || true
          git -C "$PARENT_DIR" push 2>/dev/null || true
          log "  $RIG_NAME: updated parent submodule pointer"
        fi

        UPDATED=$((UPDATED + 1))
      else
        log "  $RIG_NAME: WARN: commit failed"
        ERRORS+=("$RIG_NAME:commit-failed")
      fi
    fi
  else
    log "  $RIG_NAME: WARN: git add failed"
    ERRORS+=("$RIG_NAME:add-failed")
  fi
done <<< "$RIG_NAMES"

# --- Report --------------------------------------------------------------------

SUMMARY="$RIG_COUNT rig(s) scanned: $UPDATED updated, $SKIPPED skipped, ${#ERRORS[@]} error(s)"
log ""
log "=== Submodule Gitignore Summary ==="
log "$SUMMARY"

RESULT="success"
[[ ${#ERRORS[@]} -gt 0 ]] && RESULT="warning"

bd create "submodule-gitignore: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:submodule-gitignore,result:$RESULT \
  --silent 2>/dev/null || true

if [[ ${#ERRORS[@]} -gt 0 ]]; then
  gt escalate "submodule-gitignore: ${#ERRORS[@]} error(s): ${ERRORS[*]}" \
    -s low \
    --reason "Gitignore injection failed for: ${ERRORS[*]}" 2>/dev/null || true
fi
