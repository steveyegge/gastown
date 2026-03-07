#!/usr/bin/env bash
# compactor-dog/run.sh — Executable compaction script for agent dogs.
#
# Discovers production databases on the Dolt server, compacts (flattens)
# databases exceeding the commit threshold, verifies data integrity,
# runs dolt_gc, and reports results.
#
# This is the agent-executable counterpart to compactor_dog.go. The Go
# daemon uses database/sql connections; this script uses `dolt sql` CLI
# to achieve the same flatten algorithm.
#
# Modes:
#   --check-only   Monitor and report only (matches plugin.md philosophy)
#   (default)      Perform compaction on databases exceeding threshold
#
# Usage: ./run.sh [--threshold N] [--databases db1,db2,...] [--dry-run] [--check-only]

set -euo pipefail

# --- Configuration -----------------------------------------------------------

DOLT_HOST="${DOLT_HOST:-127.0.0.1}"
DOLT_PORT="${DOLT_PORT:-3307}"
DOLT_USER="${DOLT_USER:-root}"
COMMIT_THRESHOLD="${COMMIT_THRESHOLD:-500}"
# Default production databases (matches reaper.DefaultDatabases)
DEFAULT_DBS="hq,bd,gastown"
DRY_RUN=false
CHECK_ONLY=false
LOGFILE=""
LOCKFILE="/tmp/compactor-dog.lock"

# --- Argument parsing ---------------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    --threshold)   COMMIT_THRESHOLD="$2"; shift 2 ;;
    --databases)   DEFAULT_DBS="$2"; shift 2 ;;
    --dry-run)     DRY_RUN=true; shift ;;
    --check-only)  CHECK_ONLY=true; shift ;;
    --help|-h)
      echo "Usage: $0 [--threshold N] [--databases db1,db2,...] [--dry-run] [--check-only]"
      echo "  --threshold N        Commit count before compaction (default: 500)"
      echo "  --databases db1,...  Comma-separated database list (default: hq,bd,gastown)"
      echo "  --dry-run            Report only, don't compact"
      echo "  --check-only         Monitor and report only (no compaction)"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# --- Lock acquisition --------------------------------------------------------

# Prevent concurrent compaction runs (e.g., cron + manual).
if ! mkdir "$LOCKFILE" 2>/dev/null; then
  echo "[compactor-dog] ERROR: Another instance is running (lockfile: $LOCKFILE)"
  echo "[compactor-dog] If stale, remove with: rmdir $LOCKFILE"
  exit 1
fi
trap 'rmdir "$LOCKFILE" 2>/dev/null; rm -f "$LOGFILE"' EXIT

# --- Helpers ------------------------------------------------------------------

# Create a temp file for capturing stderr from dolt commands.
LOGFILE=$(mktemp /tmp/compactor-dog-stderr.XXXXXX)

# Validate that a name is safe for use in SQL (alphanumeric, underscore, hyphen).
validate_name() {
  local name="$1"
  local context="$2"
  if [[ ! "$name" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    log "ERROR: Unsafe $context name rejected: '$name'"
    return 1
  fi
  return 0
}

# Run a SQL query against the Dolt server, returning CSV without header.
# Global flags (--host, --port, --no-tls, -u, -p) must go BEFORE the sql subcommand.
# Use --use-db for database selection.
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

# Run a SQL statement (no result expected) against a specific database.
dolt_exec() {
  local db="$1"
  local query="$2"
  dolt --host "$DOLT_HOST" --port "$DOLT_PORT" --no-tls -u "$DOLT_USER" -p "" --use-db "$db" \
    sql -q "$query" --result-format csv >/dev/null 2>>"$LOGFILE"
}

log() {
  echo "[compactor-dog] $*"
}

# --- Step 1: Discover production databases ------------------------------------

log "Starting compaction cycle (threshold=$COMMIT_THRESHOLD, dry_run=$DRY_RUN, check_only=$CHECK_ONLY)"

# If databases were explicitly provided, use those. Otherwise, auto-discover
# from the server and filter out system/test databases.
if [[ "$DEFAULT_DBS" == "auto" ]]; then
  ALL_DBS=$(dolt_query "" "SHOW DATABASES" | grep -v -E '^(information_schema|mysql|dolt_cluster|testdb_|beads_t|beads_pt|doctest_)$')
  if [[ -z "$ALL_DBS" ]]; then
    log "ERROR: No production databases found (is Dolt running on $DOLT_HOST:$DOLT_PORT?)"
    gt escalate "compactor-dog: no databases found" -s MEDIUM \
      --reason "Dolt server at $DOLT_HOST:$DOLT_PORT returned no production databases" 2>/dev/null || true
    exit 1
  fi
else
  # Convert comma-separated list to newline-separated
  ALL_DBS=$(echo "$DEFAULT_DBS" | tr ',' '\n')
fi

# Validate all database names before proceeding.
VALIDATED_DBS=""
while IFS= read -r DB; do
  [[ -z "$DB" ]] && continue
  if validate_name "$DB" "database"; then
    VALIDATED_DBS="${VALIDATED_DBS}${DB}"$'\n'
  fi
done <<< "$ALL_DBS"
ALL_DBS="$VALIDATED_DBS"

DB_COUNT=$(printf '%s' "$ALL_DBS" | grep -c . || true)
log "Production databases ($DB_COUNT): $(printf '%s' "$ALL_DBS" | tr '\n' ' ')"

if [[ "$DB_COUNT" -eq 0 ]]; then
  log "ERROR: No valid databases to process"
  exit 1
fi

# --- Step 2: Count commits per database and identify candidates ---------------

log ""
log "=== Commit Counts ==="

declare -a CANDIDATES=()
declare -a SKIPPED=()
REPORT=""

while IFS= read -r DB; do
  [[ -z "$DB" ]] && continue

  COUNT=$(dolt_query "$DB" "SELECT COUNT(*) AS cnt FROM dolt_log" 2>/dev/null | head -1)
  if [[ -z "$COUNT" || "$COUNT" == "null" ]]; then
    log "  $DB: ERROR querying commit count (skipping)"
    REPORT="${REPORT}${DB}: error\n"
    continue
  fi

  log "  $DB: $COUNT commits"
  REPORT="${REPORT}${DB}: ${COUNT} commits\n"

  if [[ "$COUNT" -ge "$COMMIT_THRESHOLD" ]]; then
    CANDIDATES+=("$DB:$COUNT")
  else
    SKIPPED+=("$DB")
  fi
done <<< "$ALL_DBS"

log ""
log "Candidates for compaction: ${#CANDIDATES[@]}"
log "Skipped (below threshold): ${#SKIPPED[@]}"

if [[ ${#CANDIDATES[@]} -eq 0 ]]; then
  log "All databases within threshold ($COMMIT_THRESHOLD). No compaction needed."
  SUMMARY="compactor-dog: all ${DB_COUNT} DBs below threshold ($COMMIT_THRESHOLD commits)"
  bd create "$SUMMARY" -t chore --ephemeral \
    -l type:plugin-run,plugin:compactor-dog,result:success \
    -d "$SUMMARY" --silent 2>/dev/null || true
  exit 0
fi

# --- Check-only mode: report candidates and exit -----------------------------

if $CHECK_ONLY; then
  log ""
  log "=== CHECK-ONLY — databases exceeding threshold: ==="
  for entry in "${CANDIDATES[@]}"; do
    log "  ${entry%%:*} (${entry##*:} commits) — recommends compaction"
  done
  SUMMARY="compactor-dog: ${#CANDIDATES[@]} DBs exceed threshold ($COMMIT_THRESHOLD commits)"
  bd create "$SUMMARY" -t chore --ephemeral \
    -l type:plugin-run,plugin:compactor-dog,result:check-only \
    -d "$SUMMARY" --silent 2>/dev/null || true
  exit 0
fi

if $DRY_RUN; then
  log ""
  log "=== DRY RUN — would compact: ==="
  for entry in "${CANDIDATES[@]}"; do
    log "  ${entry%%:*} (${entry##*:} commits)"
  done
  exit 0
fi

# --- Step 3: Compact (flatten) each candidate database ------------------------

COMPACTED=0
ERRORS=0
ERROR_DETAILS=""

# Temp file for pre-flight row counts (bash 3.2 compatible — no associative arrays).
PRE_COUNTS_FILE=$(mktemp /tmp/compactor-dog-precounts.XXXXXX)
trap 'rmdir "$LOCKFILE" 2>/dev/null; rm -f "$LOGFILE" "$PRE_COUNTS_FILE"' EXIT

for entry in "${CANDIDATES[@]}"; do
  DB="${entry%%:*}"
  COMMIT_COUNT="${entry##*:}"

  log ""
  log "=== Compacting $DB ($COMMIT_COUNT commits) ==="

  # Step 3a: Record pre-flight row counts for integrity verification.
  log "  Recording pre-flight row counts..."
  PRE_TABLES=$(dolt_query "$DB" \
    "SELECT table_name FROM information_schema.tables WHERE table_schema = '$DB' AND table_name NOT LIKE 'dolt_%'")

  # Clear pre-counts file for this database.
  : > "$PRE_COUNTS_FILE"
  TABLE_COUNT=0

  while IFS= read -r TABLE; do
    [[ -z "$TABLE" ]] && continue
    if ! validate_name "$TABLE" "table"; then
      continue
    fi
    ROW_COUNT=$(dolt_query "$DB" "SELECT COUNT(*) FROM \`$TABLE\`" 2>/dev/null | head -1)
    printf '%s\t%s\n' "$TABLE" "${ROW_COUNT:-0}" >> "$PRE_COUNTS_FILE"
    TABLE_COUNT=$((TABLE_COUNT + 1))
  done <<< "$PRE_TABLES"

  log "  Pre-flight: $TABLE_COUNT tables recorded"

  # Step 3b: Find root (earliest) commit hash.
  ROOT_HASH=$(dolt_query "$DB" "SELECT commit_hash FROM dolt_log ORDER BY date ASC LIMIT 1" 2>/dev/null | head -1)
  if [[ -z "$ROOT_HASH" ]]; then
    log "  ERROR: Could not find root commit for $DB"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}${DB}: no root commit\n"
    continue
  fi
  log "  Root commit: ${ROOT_HASH:0:8}"

  # Step 3c: Soft-reset to root commit (moves parent pointer, keeps data staged).
  log "  Soft-resetting to root..."
  if ! dolt_exec "$DB" "CALL DOLT_RESET('--soft', '$ROOT_HASH')"; then
    log "  ERROR: Soft reset failed for $DB"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}${DB}: soft reset failed\n"
    continue
  fi

  # Step 3d: Commit all data as a single commit.
  COMMIT_MSG="compaction: flatten history to single commit"
  log "  Committing flattened data..."
  if ! dolt_exec "$DB" "CALL DOLT_COMMIT('-Am', '$COMMIT_MSG')"; then
    log "  ERROR: Flatten commit failed for $DB"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}${DB}: commit failed\n"
    continue
  fi

  # --- Step 4: Verify data integrity (row counts before/after) ----------------

  log "  Verifying integrity..."
  INTEGRITY_OK=true

  while IFS= read -r TABLE; do
    [[ -z "$TABLE" ]] && continue
    if ! validate_name "$TABLE" "table"; then
      continue
    fi
    POST_COUNT=$(dolt_query "$DB" "SELECT COUNT(*) FROM \`$TABLE\`" 2>/dev/null | head -1)
    PRE=$(grep -P "^${TABLE}\t" "$PRE_COUNTS_FILE" 2>/dev/null | cut -f2)
    if [[ -z "$PRE" ]]; then
      log "  WARNING: Table $TABLE appeared after compaction (new table?)"
      continue
    fi
    if [[ "$POST_COUNT" != "$PRE" ]]; then
      log "  INTEGRITY FAILURE: $DB.$TABLE — pre=$PRE post=$POST_COUNT"
      INTEGRITY_OK=false
    fi
  done <<< "$PRE_TABLES"

  # Check for missing tables (tables present before but gone after).
  POST_TABLES=$(dolt_query "$DB" \
    "SELECT table_name FROM information_schema.tables WHERE table_schema = '$DB' AND table_name NOT LIKE 'dolt_%'")
  while IFS=$'\t' read -r TABLE _; do
    [[ -z "$TABLE" ]] && continue
    if ! printf '%s' "$POST_TABLES" | grep -qx "$TABLE"; then
      log "  INTEGRITY FAILURE: Table $TABLE missing after compaction"
      INTEGRITY_OK=false
    fi
  done < "$PRE_COUNTS_FILE"

  if ! $INTEGRITY_OK; then
    log "  ERROR: Integrity check FAILED for $DB"
    log "  WARNING: DATABASE LEFT IN COMPACTED STATE — MANUAL INSPECTION REQUIRED"
    ERRORS=$((ERRORS + 1))
    ERROR_DETAILS="${ERROR_DETAILS}${DB}: integrity check failed (DB left in compacted state)\n"
    gt escalate "compactor-dog: integrity failure in $DB" -s HIGH \
      --reason "Row count mismatch after flatten compaction on $DB. DATABASE LEFT IN COMPACTED STATE — MANUAL INSPECTION REQUIRED." 2>/dev/null || true
    continue
  fi

  # Verify final commit count
  FINAL_COUNT=$(dolt_query "$DB" "SELECT COUNT(*) AS cnt FROM dolt_log" 2>/dev/null | head -1)
  log "  Integrity verified ($TABLE_COUNT tables). $FINAL_COUNT commits remain."

  # --- Step 5: Run dolt_gc after compaction -----------------------------------

  log "  Running dolt_gc..."
  if dolt_exec "$DB" "CALL dolt_gc()"; then
    log "  GC complete."
  else
    log "  WARNING: dolt_gc failed for $DB (non-fatal)"
  fi

  COMPACTED=$((COMPACTED + 1))
done

# --- Step 6: Report results ---------------------------------------------------

log ""
log "=== Compaction Cycle Complete ==="
log "  Compacted: $COMPACTED"
log "  Skipped:   ${#SKIPPED[@]}"
log "  Errors:    $ERRORS"

SUMMARY="compactor-dog: compacted=$COMPACTED skipped=${#SKIPPED[@]} errors=$ERRORS (threshold=$COMMIT_THRESHOLD)"

if [[ $ERRORS -gt 0 ]]; then
  log ""
  log "Error details:"
  printf '%b\n' "$ERROR_DETAILS" | while read -r line; do
    [[ -n "$line" ]] && log "  $line"
  done

  gt escalate "compactor-dog: $ERRORS databases had compaction errors" -s MEDIUM \
    --reason "Compaction cycle completed with errors. $SUMMARY" 2>/dev/null || true

  bd create "compactor-dog: ERRORS — $SUMMARY" -t chore --ephemeral \
    -l type:plugin-run,plugin:compactor-dog,result:warning \
    -d "Compaction completed with $ERRORS errors. $SUMMARY" --silent 2>/dev/null || true
else
  bd create "$SUMMARY" -t chore --ephemeral \
    -l type:plugin-run,plugin:compactor-dog,result:success \
    -d "$SUMMARY" --silent 2>/dev/null || true
fi

log "Done."
