#!/bin/bash
# seed-data.sh - Create test beads exercising migration edge cases
#
# Usage: ./scripts/migration-test/seed-data.sh <town_root>
#
# Creates 12+ beads across town and rig .beads/ directories to exercise
# the full SQLite-to-Dolt migration path including edge cases.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[X]${NC} $1"; exit 1; }

TOWN_ROOT="${1:?Usage: seed-data.sh <town_root>}"

if [[ ! -d "$TOWN_ROOT" ]]; then
    fail "Town root does not exist: $TOWN_ROOT"
fi

# Ensure bd is available
command -v bd &> /dev/null || fail "bd not found in PATH"

log "Seeding migration test data in $TOWN_ROOT"

# Helper: create a bead in a given .beads directory
create_bead() {
    local beads_dir="$1"
    local id="$2"
    local title="$3"
    local type="${4:-task}"
    local status="${5:-open}"
    local priority="${6:-2}"
    local extra_flags="${7:-}"

    cd "$beads_dir/.." # bd operates from parent of .beads
    bd create --id "$id" --title "$title" --type "$type" --priority "$priority" $extra_flags 2>/dev/null || true
    if [[ "$status" == "closed" ]]; then
        bd close "$id" --reason "test seed: closed" 2>/dev/null || true
    fi
}

# Helper: add label to a bead
add_label() {
    local beads_dir="$1"
    local id="$2"
    local label="$3"

    cd "$beads_dir/.."
    bd label "$id" "$label" 2>/dev/null || true
}

# Helper: add dependency
add_dep() {
    local beads_dir="$1"
    local from="$2"
    local to="$3"

    cd "$beads_dir/.."
    bd dep "$from" "$to" 2>/dev/null || true
}

# ============================================
# TOWN-LEVEL BEADS (.beads at town root)
# ============================================
TOWN_BEADS="$TOWN_ROOT/.beads"
if [[ -d "$TOWN_BEADS" ]]; then
    log "Seeding town-level beads..."

    # hq- prefix beads (mail, convoys, escalations)
    create_bead "$TOWN_BEADS" "hq-seed1" "Test mail message" "message" "open" "2"
    create_bead "$TOWN_BEADS" "hq-seed2" "Test convoy tracker" "convoy" "open" "2"
    create_bead "$TOWN_BEADS" "hq-seed3" "Closed escalation" "message" "closed" "1"
    create_bead "$TOWN_BEADS" "hq-seed4" "Open high-priority work" "task" "open" "1"

    # Add labels
    add_label "$TOWN_BEADS" "hq-seed1" "from:test-agent"
    add_label "$TOWN_BEADS" "hq-seed1" "read"
    add_label "$TOWN_BEADS" "hq-seed3" "escalation"

    log "Created 4 town-level beads"
else
    warn "No town-level .beads directory found, skipping"
fi

# ============================================
# RIG-LEVEL BEADS
# ============================================
RIGS_SEEDED=0
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    rig_beads="$rig_dir/.beads"

    # Skip non-rig directories
    [[ -d "$rig_beads" ]] || continue
    [[ -f "$rig_beads/metadata.json" ]] || continue

    log "Seeding beads in rig: $rig_name"

    # Standard beads with various types
    create_bead "$rig_beads" "${rig_name:0:2}-seed1" "Feature with dependencies" "feature" "open" "1"
    create_bead "$rig_beads" "${rig_name:0:2}-seed2" "Bug with unicode: \xC3\xA9\xC3\xA0\xC3\xBC \xE2\x9C\x93" "bug" "open" "0"
    create_bead "$rig_beads" "${rig_name:0:2}-seed3" "Closed task for count verification" "task" "closed" "2"
    create_bead "$rig_beads" "${rig_name:0:2}-seed4" "Epic with children" "epic" "open" "1"
    create_bead "$rig_beads" "${rig_name:0:2}-seed5" "Child of epic" "task" "open" "2"

    # Dependencies (parent-child, blocks)
    add_dep "$rig_beads" "${rig_name:0:2}-seed5" "${rig_name:0:2}-seed4"

    # Labels
    add_label "$rig_beads" "${rig_name:0:2}-seed1" "migration-test"
    add_label "$rig_beads" "${rig_name:0:2}-seed2" "unicode-edge-case"
    add_label "$rig_beads" "${rig_name:0:2}-seed4" "has-children"

    # Ephemeral wisp-like bead
    create_bead "$rig_beads" "${rig_name:0:2}-wisp-test1" "Ephemeral patrol wisp" "task" "closed" "3"

    RIGS_SEEDED=$((RIGS_SEEDED + 1))
    log "Created 6 beads in $rig_name"
done

if [[ $RIGS_SEEDED -eq 0 ]]; then
    warn "No rigs found with .beads directories"
fi

# ============================================
# RECORD COUNTS FOR VALIDATION
# ============================================
COUNTS_FILE="$TOWN_ROOT/.migration-test-counts.json"
log "Recording pre-migration counts to $COUNTS_FILE"

echo "{" > "$COUNTS_FILE"
echo '  "seeded_at": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'",' >> "$COUNTS_FILE"
echo '  "rigs": {' >> "$COUNTS_FILE"

first=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    rig_beads="$rig_dir/.beads"
    [[ -d "$rig_beads" && -f "$rig_beads/metadata.json" ]] || continue

    cd "$rig_dir"
    count=$(bd list --json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

    if [[ "$first" == "true" ]]; then
        first=false
    else
        echo "," >> "$COUNTS_FILE"
    fi
    printf '    "%s": %s' "$rig_name" "$count" >> "$COUNTS_FILE"
done

# Town-level count
if [[ -d "$TOWN_ROOT/.beads" ]]; then
    cd "$TOWN_ROOT"
    town_count=$(bd list --json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    if [[ "$first" == "true" ]]; then
        first=false
    else
        echo "," >> "$COUNTS_FILE"
    fi
    printf '    "town": %s' "$town_count" >> "$COUNTS_FILE"
fi

echo "" >> "$COUNTS_FILE"
echo "  }" >> "$COUNTS_FILE"
echo "}" >> "$COUNTS_FILE"

log "Seed data complete. Counts saved to $COUNTS_FILE"
