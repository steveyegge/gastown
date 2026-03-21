#!/usr/bin/env bash
# dolt-archive/run.sh — Deterministic JSONL backup + git push + dolt push.
#
# Exports production databases to JSONL, commits to git backup repo,
# and pushes Dolt remotes. JSONL is the last-resort recovery layer.
#
# Usage: ./run.sh [--databases db1,db2,...] [--skip-git] [--skip-dolt-push]

set -euo pipefail

# --- Configuration -----------------------------------------------------------

DOLT_HOST="${DOLT_HOST:-127.0.0.1}"
DOLT_PORT="${DOLT_PORT:-3307}"
DOLT_USER="${DOLT_USER:-root}"
TOWN_ROOT="${GT_TOWN_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
DOLT_DATA_DIR="${DOLT_DATA_DIR:-$TOWN_ROOT/.dolt-data}"
JSONL_EXPORT_DIR="${TOWN_ROOT}/.dolt-archive/jsonl"
BACKUP_REPO="${TOWN_ROOT}/.dolt-archive/git"
DEFAULT_DBS="auto"
SKIP_GIT=false
SKIP_DOLT_PUSH=false

# --- Argument parsing --------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --databases)    DEFAULT_DBS="$2"; shift 2 ;;
    --skip-git)     SKIP_GIT=true; shift ;;
    --skip-dolt-push) SKIP_DOLT_PUSH=true; shift ;;
    --help|-h)
      echo "Usage: $0 [--databases db1,db2,...] [--skip-git] [--skip-dolt-push]"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# --- Helpers -----------------------------------------------------------------

log() {
  echo "[dolt-archive] $*"
}

LOGFILE=$(mktemp /tmp/dolt-archive-stderr.XXXXXX)
trap 'rm -f "$LOGFILE"' EXIT

dolt_query() {
  local db="$1"
  local query="$2"
  local args=(dolt --host "$DOLT_HOST" --port "$DOLT_PORT" --no-tls -u "$DOLT_USER" -p "")
  if [[ -n "$db" ]]; then
    args+=(--use-db "$db")
  fi
  args+=(sql -q "$query" --result-format csv)
  "${args[@]}" 2>>"$LOGFILE" | tail -n +2 | tr -d '\r'
}

dolt_query_json() {
  local db="$1"
  local query="$2"
  dolt --host "$DOLT_HOST" --port "$DOLT_PORT" --no-tls -u "$DOLT_USER" -p "" \
    --use-db "$db" sql -q "$query" --result-format json 2>>"$LOGFILE"
}

# --- Step 1: Discover databases ----------------------------------------------

if [[ "$DEFAULT_DBS" == "auto" ]]; then
  log "Auto-discovering databases from Dolt server..."
  DISCOVERED=$(dolt_query "" "SHOW DATABASES" | grep -vE '^(information_schema|mysql|dolt)$')
  if [[ -z "$DISCOVERED" ]]; then
    log "ERROR: No user databases found on Dolt server at $DOLT_HOST:$DOLT_PORT"
    exit 1
  fi
  IFS=$'\n' read -ra PROD_DBS <<< "$DISCOVERED"
  log "Discovered ${#PROD_DBS[@]} databases: ${PROD_DBS[*]}"
else
  IFS=',' read -ra PROD_DBS <<< "$DEFAULT_DBS"
fi

# --- Step 2: JSONL export ----------------------------------------------------

log "Starting archive cycle (databases: ${PROD_DBS[*]})"
mkdir -p "$JSONL_EXPORT_DIR"

EXPORTED=0
EXPORT_FAILED=0
EXPORT_ERRORS=""

for DB in "${PROD_DBS[@]}"; do
  EXPORT_FILE="$JSONL_EXPORT_DIR/${DB}-$(date +%Y%m%d-%H%M).jsonl"
  LATEST_LINK="$JSONL_EXPORT_DIR/${DB}-latest.jsonl"

  log "Exporting $DB..."

  # Try bd export first (native beads export)
  if bd export --db "$DB" --format jsonl > "$EXPORT_FILE" 2>/dev/null; then
    LINE_COUNT=$(wc -l < "$EXPORT_FILE" | tr -d ' ')
    FILE_SIZE=$(du -h "$EXPORT_FILE" | cut -f1)
    log "  $DB: $LINE_COUNT issues exported ($FILE_SIZE) [bd export]"
    ln -sf "$(basename "$EXPORT_FILE")" "$LATEST_LINK"
    EXPORTED=$((EXPORTED + 1))
  else
    # Fallback: query Dolt directly for issues table
    if dolt_query_json "$DB" "SELECT * FROM issues ORDER BY id" > "$EXPORT_FILE" 2>/dev/null && [[ -s "$EXPORT_FILE" ]]; then
      LINE_COUNT=$(wc -l < "$EXPORT_FILE" | tr -d ' ')
      log "  $DB: exported via SQL ($LINE_COUNT lines)"
      ln -sf "$(basename "$EXPORT_FILE")" "$LATEST_LINK"
      EXPORTED=$((EXPORTED + 1))
    else
      log "  WARN: $DB export failed"
      rm -f "$EXPORT_FILE"
      EXPORT_FAILED=$((EXPORT_FAILED + 1))
      EXPORT_ERRORS="${EXPORT_ERRORS}${DB} "
    fi
  fi
done

# Prune old exports (keep last 24 snapshots per DB)
for DB in "${PROD_DBS[@]}"; do
  ALL_SNAPS=(); while IFS= read -r _s; do ALL_SNAPS+=("$_s"); done < <(ls -t "$JSONL_EXPORT_DIR/${DB}-2"*.jsonl 2>/dev/null || true)
  if (( ${#ALL_SNAPS[@]} > 24 )); then
    printf '%s\n' "${ALL_SNAPS[@]:24}" | xargs rm -f
    log "Pruned old $DB snapshots"
  fi
done

log "JSONL export: $EXPORTED succeeded, $EXPORT_FAILED failed"

# --- Step 3: Git commit and push ---------------------------------------------

GIT_PUSHED=false

if ! $SKIP_GIT && [[ -d "$BACKUP_REPO/.git" ]]; then
  log ""
  log "=== Git Push ==="

  # Copy latest JSONL files to git repo
  for DB in "${PROD_DBS[@]}"; do
    LATEST="$JSONL_EXPORT_DIR/${DB}-latest.jsonl"
    if [[ -L "$LATEST" ]]; then
      REAL_FILE="$JSONL_EXPORT_DIR/$(readlink "$LATEST")"
      if [[ -f "$REAL_FILE" ]]; then
        cp "$REAL_FILE" "$BACKUP_REPO/${DB}.jsonl"
      fi
    elif [[ -f "$LATEST" ]]; then
      cp "$LATEST" "$BACKUP_REPO/${DB}.jsonl"
    fi
  done

  cd "$BACKUP_REPO"

  if git diff --quiet && git diff --staged --quiet; then
    log "No changes to commit"
  else
    git add *.jsonl 2>/dev/null || true
    git commit -m "Archive snapshot $(date +%Y-%m-%d-%H%M)" \
      --author="Gas Town Archive <archive@gastown.local>" 2>/dev/null || true

    if git remote get-url origin > /dev/null 2>&1; then
      if git push origin main 2>/dev/null; then
        GIT_PUSHED=true
        log "Pushed to GitHub"
      else
        log "WARN: Git push to remote failed"
      fi
    else
      log "WARN: No git remote configured for backup repo"
    fi
  fi
elif ! $SKIP_GIT; then
  log "No git backup repo at $BACKUP_REPO — skipping git push"
fi

# --- Step 4: Dolt native push ------------------------------------------------

DOLT_PUSHED=0
DOLT_PUSH_FAILED=0

if ! $SKIP_DOLT_PUSH; then
  log ""
  log "=== Dolt Push ==="

  for DB in "${PROD_DBS[@]}"; do
    DB_DIR="$DOLT_DATA_DIR/$DB"

    if [[ ! -d "$DB_DIR/.dolt" ]]; then
      log "  $DB: no .dolt directory, skipping"
      continue
    fi

    REMOTES=$(cd "$DB_DIR" && { dolt remote -v 2>/dev/null | grep -v "^$" | head -5 || true; })
    if [[ -z "$REMOTES" ]]; then
      log "  $DB: no remotes configured, skipping"
      continue
    fi

    log "  $DB: pushing to remotes..."
    cd "$DB_DIR"

    for REMOTE_NAME in $(dolt remote -v 2>/dev/null | awk '{print $1}' | sort -u || true); do
      if timeout 120 dolt push "$REMOTE_NAME" main 2>/dev/null; then
        log "    $REMOTE_NAME: pushed"
        DOLT_PUSHED=$((DOLT_PUSHED + 1))
      else
        log "    $REMOTE_NAME: FAILED"
        DOLT_PUSH_FAILED=$((DOLT_PUSH_FAILED + 1))
      fi
    done
  done

  log "Dolt push: $DOLT_PUSHED succeeded, $DOLT_PUSH_FAILED failed"
fi

# --- Step 5: Report results --------------------------------------------------

log ""
log "=== Archive Cycle Complete ==="

SUMMARY="Archive: jsonl=$EXPORTED/$((EXPORTED + EXPORT_FAILED)), git=${GIT_PUSHED}, dolt_push=$DOLT_PUSHED/$((DOLT_PUSHED + DOLT_PUSH_FAILED))"
log "$SUMMARY"

RESULT="success"
if [[ "$EXPORT_FAILED" -gt 0 ]] || [[ "$DOLT_PUSH_FAILED" -gt 0 ]]; then
  RESULT="warning"
fi

bd create "$SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:dolt-archive,result:$RESULT \
  -d "$SUMMARY" --silent 2>/dev/null || true

if [[ "$EXPORT_FAILED" -gt 0 ]]; then
  gt escalate "dolt-archive: JSONL export failed for $EXPORT_FAILED databases ($EXPORT_ERRORS)" \
    -s critical \
    --reason "JSONL is our last-resort recovery layer. Failed databases: $EXPORT_ERRORS" 2>/dev/null || true
fi

log "Done."
