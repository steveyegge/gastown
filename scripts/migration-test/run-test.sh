#!/bin/bash
# run-test.sh - Full migration test orchestration
#
# Usage: ./scripts/migration-test/run-test.sh <town_root>
#
# Runs the complete migration test cycle:
#   1. Seed test data (v0.5.0 SQLite state)
#   2. Create pristine backup
#   3. Run migration (via mol-migration formula or manual steps)
#   4. Validate results
#   5. Reset to v0.5.0 state (optional, for re-runs)
#
# This script is designed for the GCE migration test lab VM (gt-mtte).
# It can also be run locally for development.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[X]${NC} $1"; exit 1; }
section() { echo -e "\n${BLUE}=== $1 ===${NC}\n"; }

TOWN_ROOT="${1:?Usage: run-test.sh <town_root>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKIP_MIGRATION="${SKIP_MIGRATION:-false}"
SKIP_RESET="${SKIP_RESET:-false}"

if [[ ! -d "$TOWN_ROOT" ]]; then
    fail "Town root does not exist: $TOWN_ROOT"
fi

echo "================================================"
echo "  Migration Test Runner"
echo "  Town: $TOWN_ROOT"
echo "  $(date)"
echo "================================================"

# ============================================
# PHASE 1: SEED DATA
# ============================================
section "Phase 1: Seed Data"

# Check if already seeded
if [[ -f "$TOWN_ROOT/.migration-test-counts.json" ]]; then
    warn "Seed data counts already exist. Re-seeding..."
fi

"$SCRIPT_DIR/seed-data.sh" "$TOWN_ROOT"
log "Phase 1 complete"

# ============================================
# PHASE 2: CREATE BACKUP
# ============================================
section "Phase 2: Create Backup"

BACKUP_DIR="$TOWN_ROOT/.migration-test-backup"
if [[ -d "$BACKUP_DIR" ]]; then
    warn "Backup already exists, removing for fresh backup..."
    rm -rf "$BACKUP_DIR"
fi

"$SCRIPT_DIR/setup-backup.sh" "$TOWN_ROOT"
log "Phase 2 complete"

# ============================================
# PHASE 3: VERIFY PRE-MIGRATION STATE
# ============================================
section "Phase 3: Pre-migration Verification"

# Verify all backends are SQLite
all_sqlite=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    metadata="$rig_dir/.beads/metadata.json"
    [[ -f "$metadata" ]] || continue

    backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
    if [[ "$backend" != "sqlite" && "$backend" != "" ]]; then
        warn "$rig_name already on $backend"
        all_sqlite=false
    else
        echo "  $rig_name: SQLite (ready for migration)"
    fi
done

if [[ "$all_sqlite" == "false" ]]; then
    warn "Some rigs already migrated. Test may not exercise full path."
fi

log "Phase 3 complete"

# ============================================
# PHASE 4: MIGRATION
# ============================================
section "Phase 4: Migration"

if [[ "$SKIP_MIGRATION" == "true" ]]; then
    warn "SKIP_MIGRATION=true, skipping migration phase"
    warn "Run migration manually, then re-run with SKIP_MIGRATION=false"
else
    log "Running migration for each rig..."

    for rig_dir in "$TOWN_ROOT"/*/; do
        rig_name=$(basename "$rig_dir")
        metadata="$rig_dir/.beads/metadata.json"
        [[ -f "$metadata" ]] || continue

        backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
        if [[ "$backend" == "dolt" ]]; then
            echo "  $rig_name: already Dolt, skipping"
            continue
        fi

        log "Migrating $rig_name..."
        cd "$rig_dir"
        if bd migrate dolt 2>&1; then
            log "$rig_name: migration complete"
        else
            warn "$rig_name: migration returned non-zero (check output)"
        fi
    done

    # Town-level migration
    if [[ -d "$TOWN_ROOT/.beads" ]]; then
        cd "$TOWN_ROOT"
        backend=$(python3 -c "import json; print(json.load(open('$TOWN_ROOT/.beads/metadata.json')).get('backend', 'sqlite'))" 2>/dev/null || echo "sqlite")
        if [[ "$backend" != "dolt" ]]; then
            log "Migrating town-level beads..."
            bd migrate dolt 2>&1 || warn "Town migration returned non-zero"
        fi
    fi

    # Consolidate Dolt databases
    log "Consolidating Dolt databases..."
    cd "$TOWN_ROOT"
    gt dolt migrate 2>&1 || warn "gt dolt migrate returned non-zero"

    # Start Dolt server
    log "Starting Dolt server..."
    gt dolt start 2>&1 || warn "gt dolt start returned non-zero"

    log "Phase 4 complete"
fi

# ============================================
# PHASE 5: VALIDATION
# ============================================
section "Phase 5: Validation"

"$SCRIPT_DIR/validate-migration.sh" "$TOWN_ROOT"
RESULT=$?

# ============================================
# PHASE 6: RESET (optional)
# ============================================
if [[ "$SKIP_RESET" != "true" && "$RESULT" -eq 0 ]]; then
    section "Phase 6: Reset"
    echo "Migration validated successfully."
    echo "To reset for another run:"
    echo "  $SCRIPT_DIR/reset-vm.sh $TOWN_ROOT"
    echo
    echo "Or set SKIP_RESET=false to auto-reset."
else
    if [[ "$RESULT" -ne 0 ]]; then
        warn "Validation had $RESULT failure(s). Not resetting."
        warn "Investigate, then run: $SCRIPT_DIR/reset-vm.sh $TOWN_ROOT"
    fi
fi

# ============================================
# SUMMARY
# ============================================
echo
echo "================================================"
if [[ "$RESULT" -eq 0 ]]; then
    echo -e "  ${GREEN}MIGRATION TEST PASSED${NC}"
else
    echo -e "  ${RED}MIGRATION TEST FAILED ($RESULT check failures)${NC}"
fi
echo "================================================"

exit $RESULT
