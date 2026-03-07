#!/usr/bin/env bash
# session-hygiene/run.sh — Deterministic session cleanup script.
#
# Identifies and kills zombie tmux sessions (prefix not in rigs.json)
# and orphaned dog sessions (tmux session exists but dog not in kennel).
#
# CRITICAL: This script reads rigs.json for valid prefixes. It does NOT
# use `gt rig list` which returns rig names (not beads prefixes) and
# caused two incidents where all crew sessions were killed.
#
# Usage: ./run.sh [--dry-run]

set -euo pipefail

# --- Configuration -----------------------------------------------------------

GT_TOWN_ROOT="${GT_TOWN_ROOT:-$HOME/gt}"
RIGS_JSON_PATH="${GT_TOWN_ROOT}/mayor/rigs.json"
DRY_RUN=false

# --- Argument parsing ---------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)  DRY_RUN=true; shift ;;
    --help|-h)
      echo "Usage: $0 [--dry-run]"
      echo "  --dry-run  Report zombies/orphans without killing them"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# --- Helpers ------------------------------------------------------------------

log() {
  echo "[session-hygiene] $*"
}

# --- Step 1: Get valid rig prefixes from rigs.json ----------------------------

if [ ! -f "$RIGS_JSON_PATH" ]; then
  log "SKIP: rigs.json not found at $RIGS_JSON_PATH"
  exit 0
fi

RIGS_FILE=$(cat "$RIGS_JSON_PATH" 2>/dev/null)
if [ -z "$RIGS_FILE" ]; then
  log "SKIP: could not read rigs.json"
  exit 0
fi

# Extract both rig names and beads prefixes as valid session prefixes
RIG_NAMES=$(echo "$RIGS_FILE" | jq -r '.rigs | keys[]' 2>/dev/null)
BEADS_PREFIXES=$(echo "$RIGS_FILE" | jq -r '.rigs[].beads.prefix // empty' 2>/dev/null)
VALID_PREFIXES=$(printf '%s\n%s' "$RIG_NAMES" "$BEADS_PREFIXES" | sort -u)
if [ -z "$VALID_PREFIXES" ]; then
  log "SKIP: no rigs found in rigs.json"
  exit 0
fi

log "Valid prefixes: $(echo "$VALID_PREFIXES" | tr '\n' ' ')"

# --- Step 2: List tmux sessions ----------------------------------------------

SESSIONS=$(tmux list-sessions -F '#{session_name}' 2>/dev/null) || true
if [ -z "$SESSIONS" ]; then
  log "No tmux sessions running"
  exit 0
fi

SESSION_COUNT=$(echo "$SESSIONS" | wc -l | tr -d ' ')
log "Found $SESSION_COUNT tmux sessions"

# --- Step 3: Identify zombie sessions ----------------------------------------

# A session is legitimate if its prefix (before first dash) matches a known
# rig name, beads prefix, or the "hq" namespace.
# Pattern: <prefix>-<role>-<name> (e.g., gt-crew-bear, hq-dog-alpha)

ZOMBIES=()

while IFS= read -r SESSION; do
  [ -z "$SESSION" ] && continue

  # Extract prefix (everything before the first dash)
  PREFIX=$(echo "$SESSION" | cut -d'-' -f1)

  # Allow hq prefix (town-level agents: deacon, dogs, mayor)
  if [ "$PREFIX" = "hq" ]; then
    continue
  fi

  # Check against valid rig prefixes
  VALID=false
  while IFS= read -r RIG; do
    if [ "$PREFIX" = "$RIG" ]; then
      VALID=true
      break
    fi
  done <<< "$VALID_PREFIXES"

  if [ "$VALID" = "false" ]; then
    ZOMBIES+=("$SESSION")
  fi
done <<< "$SESSIONS"

# --- Step 4: Kill zombie sessions ---------------------------------------------

KILLED=0
ZOMBIE_COUNT=${#ZOMBIES[@]}
for ZOMBIE in "${ZOMBIES[@]+"${ZOMBIES[@]}"}"; do
  [ -z "$ZOMBIE" ] && continue
  if $DRY_RUN; then
    log "DRY RUN: would kill zombie session: $ZOMBIE"
  else
    log "Killing zombie session: $ZOMBIE"
    tmux kill-session -t "$ZOMBIE" 2>/dev/null && KILLED=$((KILLED + 1))
  fi
done

# --- Step 5: Check for orphaned dog sessions ----------------------------------

# Dog sessions follow the pattern hq-dog-<name>. Cross-reference against
# the kennel to find sessions for dogs that no longer exist.

DOG_JSON=$(gt dog list --json 2>/dev/null || echo "[]")
KNOWN_DOGS=$(echo "$DOG_JSON" | jq -r '.[].name // empty' 2>/dev/null)

ORPHANED=0
while IFS= read -r SESSION; do
  [ -z "$SESSION" ] && continue

  # Match hq-dog-* pattern
  case "$SESSION" in
    hq-dog-*)
      DOG_NAME="${SESSION#hq-dog-}"

      # Check if this dog exists in the kennel
      FOUND=false
      while IFS= read -r DOG; do
        if [ "$DOG_NAME" = "$DOG" ]; then
          FOUND=true
          break
        fi
      done <<< "$KNOWN_DOGS"

      if [ "$FOUND" = "false" ]; then
        if $DRY_RUN; then
          log "DRY RUN: would kill orphaned dog session: $SESSION (dog '$DOG_NAME' not in kennel)"
        else
          log "Killing orphaned dog session: $SESSION (dog '$DOG_NAME' not in kennel)"
          tmux kill-session -t "$SESSION" 2>/dev/null && ORPHANED=$((ORPHANED + 1))
        fi
      fi
      ;;
  esac
done <<< "$SESSIONS"

# --- Step 6: Report results ---------------------------------------------------

if $DRY_RUN; then
  SUMMARY="DRY RUN: checked $SESSION_COUNT sessions, $ZOMBIE_COUNT zombie(s) found, would check orphaned dogs"
else
  SUMMARY="Checked $SESSION_COUNT sessions: $KILLED zombie(s) killed, $ORPHANED orphaned dog session(s) killed, $ZOMBIE_COUNT zombie(s) found"
fi
log "$SUMMARY"

if ! $DRY_RUN; then
  bd create "session-hygiene: $SUMMARY" -t chore --ephemeral \
    -l type:plugin-run,plugin:session-hygiene,result:success \
    -d "$SUMMARY" --silent 2>/dev/null || true
fi

log "Done."
