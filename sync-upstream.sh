#!/bin/bash
set -e

# Sync gastown fork with upstream, preserving local-only patches.
# Run from ~/Documents/gt
#
# Usage: ./sync-upstream.sh [--dry-run] [--dir=PATH]

WORK_DIR="$HOME/Documents/gt/gastown/crew/beercan"
GT_BIN="$HOME/.local/bin/gt"
BD_BIN="$HOME/.local/bin/bd"

# Local-only commits to cherry-pick (order matters)
BUGFIXES=(
  # d5fdc4b8 — merged upstream (see 1d36391b / d918e676)
  # d43497f3 — merged upstream (routing fixes in 30265a9c et al)
  # 9527aff1 — merged upstream (hooks_sync.go already uses preset fields directly)
  # 5c518c48 — faultline ingest handler deleted upstream; fix no longer applicable
)

FAULTLINE=(
  a77d8779  # feat: propagate FAULTLINE_DSN to all Gas Town agents
  0659d0db  # feat: add faultline health checks to gt doctor
  cdd9f481  # feat: integrate gtfaultline SDK for error reporting
)

ESTOP=(
  # dcc3bd30 — merged upstream (c98c4ddf)
  # 5bd6898b — merged upstream (estop feature landed)
  # 38b1df23 — merged upstream (estop feature landed)
)

SLING_FIX=(
  # bb0ea1c2 — merged upstream (103b6aaa extended cross-rig routing to convoy/dep)
)

# Quota probe — from beercan worktree feat/quota-probe-clean branch
QUOTA_PROBE=(
  fe60b077  # feat: add --probe flag to gt quota status
)

DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --dir=*) WORK_DIR="${arg#--dir=}" ;;
  esac
done

cd "$WORK_DIR" || { echo "ERROR: $WORK_DIR not found"; exit 1; }

echo "=== Syncing gastown fork with upstream ==="
echo "Working in: $(pwd)"
echo ""

# Fetch upstream
echo "Fetching upstream..."
git fetch upstream

# Show what we're about to do
BEHIND=$(git log --oneline HEAD..upstream/main | wc -l | tr -d ' ')
LOCAL=$(git log --oneline upstream/main..HEAD | grep -cv "bd: backup\|WIP: checkpoint\|Merge " || true)
echo "  Upstream is $BEHIND commits ahead"
echo "  We have ~$LOCAL meaningful local commits"
echo "  Cherry-picking $(( ${#BUGFIXES[@]} + ${#FAULTLINE[@]} + ${#ESTOP[@]} + ${#SLING_FIX[@]} + ${#QUOTA_PROBE[@]} )) commits after reset"
echo ""

if $DRY_RUN; then
  echo "[DRY RUN] Would reset to upstream/main and cherry-pick:"
  for h in "${BUGFIXES[@]}" "${FAULTLINE[@]}" "${ESTOP[@]}" "${SLING_FIX[@]}" "${QUOTA_PROBE[@]}"; do
    git log --oneline -1 "$h" 2>/dev/null || echo "  $h (not found — may need fetch)"
  done
  exit 0
fi

# Safety: save current HEAD in case we need to recover
SAVED_HEAD=$(git rev-parse HEAD)
echo "Saved current HEAD: $SAVED_HEAD"
echo "  (recover with: git reset --hard $SAVED_HEAD)"
echo ""

read -p "Reset to upstream/main and cherry-pick local patches? [y/N] " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "Aborted."
  exit 0
fi

echo ""
echo "Resetting to upstream/main..."
git reset --hard upstream/main

echo ""
echo "Cherry-picking bugfixes..."
for h in "${BUGFIXES[@]}"; do
  echo "  $(git log --oneline -1 "$h")"
  git cherry-pick "$h" || {
    echo "  CONFLICT on $h — resolve manually, then run:"
    echo "    git cherry-pick --continue"
    echo "  Or skip with: git cherry-pick --skip"
    exit 1
  }
done

echo ""
echo "Cherry-picking faultline integration..."
for h in "${FAULTLINE[@]}"; do
  echo "  $(git log --oneline -1 "$h")"
  git cherry-pick "$h" || {
    echo "  CONFLICT on $h — resolve manually"
    exit 1
  }
done

echo ""
echo "Cherry-picking estop feature..."
for h in "${ESTOP[@]}"; do
  echo "  $(git log --oneline -1 "$h")"
  git cherry-pick "$h" || {
    echo "  CONFLICT on $h — resolve manually"
    exit 1
  }
done

echo ""
echo "Cherry-picking sling fix (may need conflict resolution)..."
for h in "${SLING_FIX[@]}"; do
  echo "  $(git log --oneline -1 "$h")"
  git cherry-pick "$h" || {
    echo ""
    echo "  CONFLICT on sling_convoy.go — expected."
    echo "  Resolve, then: git add -A && git cherry-pick --continue"
    exit 1
  }
done

echo ""
echo "Cherry-picking quota probe..."
for h in "${QUOTA_PROBE[@]}"; do
  echo "  $(git log --oneline -1 "$h")"
  git cherry-pick "$h" || {
    echo "  CONFLICT on $h — resolve manually"
    exit 1
  }
done

echo ""
echo "Building gt..."
go build -o "$GT_BIN" ./cmd/gt/

if [[ -d "./cmd/bd" ]]; then
  echo "Building bd..."
  go build -o "$BD_BIN" ./cmd/bd/
fi

echo ""
echo "Verifying..."
"$GT_BIN" --version
[[ -d "./cmd/bd" ]] && "$BD_BIN" --version || echo "(bd not in this repo — skipped)"

echo ""
echo "=== Sync complete ==="
echo ""
echo "Local patches applied on top of upstream/main."
echo "To push:  git push origin main --force-with-lease"
echo "To undo:  git reset --hard $SAVED_HEAD"
