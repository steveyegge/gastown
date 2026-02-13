#!/bin/bash
# test-edge-cases.sh - Edge case migration tests
#
# Usage: ./scripts/migration-test/test-edge-cases.sh <town_root>
#
# Tests missing from the happy-path suite:
#   1. Mid-migration crash recovery (kill after rig N, resume)
#   2. Concurrent bd access during migration (bd commands while migrating)
#   3. Metadata corruption repair (corrupt metadata.json, fix-metadata)
#   4. Idempotency (run migration twice, should succeed both times)
#
# Requires: setup-backup.sh to have run first (for reset capability).

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail_check() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES + 1)); }
pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
section() { echo -e "\n${BLUE}=== $1 ===${NC}\n"; }

TOWN_ROOT="${1:?Usage: test-edge-cases.sh <town_root>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="$TOWN_ROOT/.migration-test-backup"
FAILURES=0
CHECKS=0

if [[ ! -d "$TOWN_ROOT" ]]; then
    echo -e "${RED}[X]${NC} Town root does not exist: $TOWN_ROOT"
    exit 1
fi

echo "================================================"
echo "  Migration Edge Case Tests"
echo "  Town: $TOWN_ROOT"
echo "  $(date)"
echo "================================================"

# Helper: reset to pre-migration state
reset_state() {
    if [[ -d "$BACKUP_DIR" ]]; then
        "$SCRIPT_DIR/reset-vm.sh" "$TOWN_ROOT" 2>/dev/null || true
    else
        warn "No backup available for reset"
    fi
}

# Helper: count rigs that report dolt backend
count_dolt_rigs() {
    local count=0
    for rig_dir in "$TOWN_ROOT"/*/; do
        local rig_name=$(basename "$rig_dir")
        local metadata="$rig_dir/.beads/metadata.json"
        [[ -f "$metadata" ]] || continue

        local backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'unknown'))" 2>/dev/null || echo "unknown")
        if [[ "$backend" == "dolt" ]]; then
            count=$((count + 1))
        fi
    done
    echo "$count"
}

# Helper: count total rigs with beads
count_total_rigs() {
    local count=0
    for rig_dir in "$TOWN_ROOT"/*/; do
        local metadata="$rig_dir/.beads/metadata.json"
        [[ -f "$metadata" ]] || continue
        count=$((count + 1))
    done
    echo "$count"
}

# ============================================
# TEST 1: MID-MIGRATION CRASH RECOVERY
# ============================================
section "Test 1: Mid-Migration Crash Recovery"
CHECKS=$((CHECKS + 1))

log "Resetting to pre-migration state..."
reset_state

# Collect rig list
RIGS=()
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    metadata="$rig_dir/.beads/metadata.json"
    [[ -f "$metadata" ]] || continue
    RIGS+=("$rig_name")
done
TOTAL_RIGS=${#RIGS[@]}

if [[ $TOTAL_RIGS -lt 2 ]]; then
    warn "Need at least 2 rigs for crash recovery test, found $TOTAL_RIGS"
    fail_check "Insufficient rigs for crash recovery test"
else
    # Migrate only the first rig
    FIRST_RIG="${RIGS[0]}"
    log "Migrating only first rig ($FIRST_RIG), simulating crash..."
    cd "$TOWN_ROOT/$FIRST_RIG"
    bd migrate dolt 2>&1 || true

    # Verify partial state: one migrated, rest still sqlite
    dolt_count=$(count_dolt_rigs)
    log "After partial migration: $dolt_count/$TOTAL_RIGS rigs on Dolt"

    if [[ "$dolt_count" -lt 1 ]]; then
        fail_check "Crash recovery: first rig not migrated"
    else
        # Resume migration for remaining rigs
        log "Resuming migration for remaining rigs..."
        for rig_name in "${RIGS[@]}"; do
            metadata="$TOWN_ROOT/$rig_name/.beads/metadata.json"
            backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
            if [[ "$backend" == "dolt" ]]; then
                echo "  $rig_name: already Dolt, skipping"
                continue
            fi
            log "Migrating $rig_name..."
            cd "$TOWN_ROOT/$rig_name"
            bd migrate dolt 2>&1 || warn "$rig_name: migration returned non-zero"
        done

        # Consolidate and start
        cd "$TOWN_ROOT"
        gt dolt migrate 2>&1 || true
        gt dolt start 2>&1 || true

        # Validate all migrated
        final_count=$(count_dolt_rigs)
        if [[ "$final_count" -eq "$TOTAL_RIGS" ]]; then
            pass "Crash recovery: all $TOTAL_RIGS rigs migrated after resume"
        else
            fail_check "Crash recovery: only $final_count/$TOTAL_RIGS rigs migrated after resume"
        fi

        # Validate data integrity via bd commands
        bd_works=true
        for rig_name in "${RIGS[@]}"; do
            cd "$TOWN_ROOT/$rig_name"
            if ! bd stats 2>/dev/null | grep -q "total\|count\|issues"; then
                echo "  $rig_name: bd stats failed after crash recovery"
                bd_works=false
            fi
        done
        if [[ "$bd_works" == "true" ]]; then
            log "All rigs operational after crash recovery"
        else
            warn "Some rigs have bd issues after crash recovery"
        fi
    fi
fi

# ============================================
# TEST 2: CONCURRENT BD ACCESS DURING MIGRATION
# ============================================
section "Test 2: Concurrent BD Access During Migration"
CHECKS=$((CHECKS + 1))

log "Resetting to pre-migration state..."
reset_state

# Pick a rig to migrate while running concurrent bd commands
TARGET_RIG="${RIGS[0]}"
log "Will migrate $TARGET_RIG while running concurrent bd operations"

# Start background bd operations against the rig
CONCURRENT_LOG="$TOWN_ROOT/.concurrent-test.log"
> "$CONCURRENT_LOG"

# Background: repeatedly run bd status/list while migration is in progress
(
    for i in $(seq 1 20); do
        cd "$TOWN_ROOT/$TARGET_RIG"
        bd stats 2>>"$CONCURRENT_LOG" || echo "CONCURRENT_ERR: bd stats attempt $i" >> "$CONCURRENT_LOG"
        bd list --limit 1 2>>"$CONCURRENT_LOG" || echo "CONCURRENT_ERR: bd list attempt $i" >> "$CONCURRENT_LOG"
        sleep 0.1
    done
) &
CONCURRENT_PID=$!

# Run migration while background bd commands execute
cd "$TOWN_ROOT/$TARGET_RIG"
bd migrate dolt 2>&1 || true

# Wait for concurrent operations to finish
wait $CONCURRENT_PID 2>/dev/null || true

# Check for crashes (segfaults, panics)
CRASH_COUNT=0
if [[ -f "$CONCURRENT_LOG" ]]; then
    CRASH_COUNT=$(grep -c "panic\|SIGSEGV\|fatal\|signal" "$CONCURRENT_LOG" 2>/dev/null || echo "0")
fi

if [[ "$CRASH_COUNT" -eq 0 ]]; then
    pass "Concurrent access: no crashes during migration"
else
    fail_check "Concurrent access: $CRASH_COUNT crash(es) detected during migration"
fi

# Verify the rig is usable after migration
cd "$TOWN_ROOT/$TARGET_RIG"
if bd stats 2>/dev/null | grep -q "total\|count\|issues"; then
    log "Rig $TARGET_RIG operational after concurrent test"
else
    warn "Rig $TARGET_RIG may need attention after concurrent test"
fi

rm -f "$CONCURRENT_LOG"

# ============================================
# TEST 3: METADATA CORRUPTION SCENARIO
# ============================================
section "Test 3: Metadata Corruption and Repair"
CHECKS=$((CHECKS + 1))

# First ensure we have a fully migrated state to corrupt
log "Setting up migrated state for corruption test..."
reset_state
for rig_name in "${RIGS[@]}"; do
    metadata="$TOWN_ROOT/$rig_name/.beads/metadata.json"
    backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
    if [[ "$backend" != "dolt" ]]; then
        cd "$TOWN_ROOT/$rig_name"
        bd migrate dolt 2>&1 || true
    fi
done
cd "$TOWN_ROOT"
gt dolt migrate 2>&1 || true
gt dolt start 2>&1 || true

# Now corrupt metadata.json files
CORRUPT_RIG="${RIGS[0]}"
log "Corrupting metadata.json for $CORRUPT_RIG..."
CORRUPT_META="$TOWN_ROOT/$CORRUPT_RIG/.beads/metadata.json"

if [[ -f "$CORRUPT_META" ]]; then
    # Save original for comparison
    cp "$CORRUPT_META" "$CORRUPT_META.bak"

    # Corruption scenario 1: Write garbage JSON
    echo '{corrupt!!!not-json' > "$CORRUPT_META"
    log "Wrote garbage JSON to metadata.json"

    # Run fix-metadata
    cd "$TOWN_ROOT"
    if gt dolt fix-metadata 2>&1; then
        log "fix-metadata completed"
    else
        warn "fix-metadata returned non-zero"
    fi

    # Verify repair
    if [[ -f "$CORRUPT_META" ]]; then
        backend=$(python3 -c "import json; print(json.load(open('$CORRUPT_META')).get('backend', 'unknown'))" 2>/dev/null || echo "parse-error")
        if [[ "$backend" == "dolt" ]]; then
            pass "Metadata corruption: fix-metadata repaired corrupt JSON"
        else
            fail_check "Metadata corruption: repair failed (backend=$backend)"
        fi
    else
        fail_check "Metadata corruption: metadata.json missing after repair"
    fi

    # Corruption scenario 2: Wrong backend value
    log "Testing wrong backend value..."
    echo '{"backend": "sqlite", "database": "beads.db"}' > "$CORRUPT_META"
    cd "$TOWN_ROOT"
    gt dolt fix-metadata 2>&1 || true

    backend=$(python3 -c "import json; print(json.load(open('$CORRUPT_META')).get('backend', 'unknown'))" 2>/dev/null || echo "parse-error")
    if [[ "$backend" == "dolt" ]]; then
        log "Wrong backend value corrected"
    else
        warn "Backend still incorrect after fix: $backend"
    fi

    # Corruption scenario 3: Empty file
    log "Testing empty metadata.json..."
    > "$CORRUPT_META"
    cd "$TOWN_ROOT"
    gt dolt fix-metadata 2>&1 || true

    backend=$(python3 -c "import json; print(json.load(open('$CORRUPT_META')).get('backend', 'unknown'))" 2>/dev/null || echo "parse-error")
    if [[ "$backend" == "dolt" ]]; then
        log "Empty file repaired"
    else
        warn "Empty file repair failed: $backend"
    fi

    rm -f "$CORRUPT_META.bak"
else
    warn "No metadata.json found for $CORRUPT_RIG, skipping corruption test"
fi

# ============================================
# TEST 4: IDEMPOTENCY
# ============================================
section "Test 4: Idempotency (Migrate Twice)"
CHECKS=$((CHECKS + 1))

log "Resetting to pre-migration state..."
reset_state

# First migration pass
log "Pass 1: Running full migration..."
for rig_name in "${RIGS[@]}"; do
    metadata="$TOWN_ROOT/$rig_name/.beads/metadata.json"
    backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
    if [[ "$backend" != "dolt" ]]; then
        cd "$TOWN_ROOT/$rig_name"
        bd migrate dolt 2>&1 || true
    fi
done
cd "$TOWN_ROOT"
gt dolt stop 2>/dev/null || true
gt dolt migrate 2>&1 || true
gt dolt start 2>&1 || true

# Record counts after first pass
declare -A PASS1_COUNTS
for rig_name in "${RIGS[@]}"; do
    cd "$TOWN_ROOT/$rig_name"
    count=$(bd list --json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    PASS1_COUNTS[$rig_name]=$count
done

log "Pass 1 counts: ${PASS1_COUNTS[*]}"

# Second migration pass (should be no-op or succeed harmlessly)
log "Pass 2: Running migration again (should be idempotent)..."
PASS2_ERRORS=0

for rig_name in "${RIGS[@]}"; do
    cd "$TOWN_ROOT/$rig_name"
    # bd migrate dolt should either succeed or say "already migrated"
    output=$(bd migrate dolt 2>&1) || true
    # Check for unexpected errors (not "already migrated" type messages)
    if echo "$output" | grep -qi "fatal\|panic\|corrupt"; then
        echo "  $rig_name: unexpected error in pass 2: $output"
        PASS2_ERRORS=$((PASS2_ERRORS + 1))
    fi
done

cd "$TOWN_ROOT"
gt dolt stop 2>/dev/null || true
output=$(gt dolt migrate 2>&1) || true
if echo "$output" | grep -qi "fatal\|panic\|corrupt"; then
    warn "gt dolt migrate had unexpected error in pass 2"
    PASS2_ERRORS=$((PASS2_ERRORS + 1))
fi
gt dolt start 2>&1 || true

# Run fix-metadata twice (must be harmless)
gt dolt fix-metadata 2>&1 || true
gt dolt fix-metadata 2>&1 || true

# Verify counts match
count_mismatch=false
for rig_name in "${RIGS[@]}"; do
    cd "$TOWN_ROOT/$rig_name"
    pass2_count=$(bd list --json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    pass1_count="${PASS1_COUNTS[$rig_name]}"

    if [[ "$pass1_count" != "$pass2_count" ]]; then
        echo "  $rig_name: count mismatch: pass1=$pass1_count pass2=$pass2_count"
        count_mismatch=true
    fi
done

if [[ "$count_mismatch" == "false" && "$PASS2_ERRORS" -eq 0 ]]; then
    pass "Idempotency: second migration pass succeeded with matching counts"
else
    fail_check "Idempotency: mismatches or errors in second pass"
fi

# ============================================
# SUMMARY
# ============================================
echo
echo "================================================"
echo "  Edge Case Test Results: $((CHECKS - FAILURES))/$CHECKS passed"
if [[ $FAILURES -gt 0 ]]; then
    echo -e "  ${RED}$FAILURES check(s) failed${NC}"
else
    echo -e "  ${GREEN}All checks passed${NC}"
fi
echo "================================================"

exit $FAILURES
