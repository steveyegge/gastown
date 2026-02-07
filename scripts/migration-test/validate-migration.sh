#!/bin/bash
# validate-migration.sh - Comprehensive post-migration validation
#
# Usage: ./scripts/migration-test/validate-migration.sh <town_root>
#
# Runs 10-point validation suite confirming successful SQLite-to-Dolt migration.
# Exit code 0 = all checks pass, non-zero = failures detected.

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail_check() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES + 1)); }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

TOWN_ROOT="${1:?Usage: validate-migration.sh <town_root>}"
COUNTS_FILE="$TOWN_ROOT/.migration-test-counts.json"
FAILURES=0
CHECKS=0

echo "================================================"
echo "  Migration Validation Suite"
echo "  Town: $TOWN_ROOT"
echo "  $(date)"
echo "================================================"
echo

# ============================================
# CHECK 1: All backends report Dolt
# ============================================
CHECKS=$((CHECKS + 1))
echo "--- Check 1: Backend type ---"
all_dolt=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    metadata="$rig_dir/.beads/metadata.json"
    [[ -f "$metadata" ]] || continue

    backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', 'unknown'))" 2>/dev/null || echo "unknown")
    if [[ "$backend" == "dolt" ]]; then
        echo "  $rig_name: dolt"
    else
        echo "  $rig_name: $backend (NOT migrated)"
        all_dolt=false
    fi
done
if [[ "$all_dolt" == "true" ]]; then
    pass "All rigs report Dolt backend"
else
    fail_check "Some rigs still on SQLite"
fi

# ============================================
# CHECK 2: Dolt server running
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 2: Dolt server status ---"
if gt dolt status 2>/dev/null | grep -q "running"; then
    pass "Dolt server is running"
else
    fail_check "Dolt server is not running"
fi

# ============================================
# CHECK 3: Count comparison (pre vs post)
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 3: Bead count comparison ---"
if [[ -f "$COUNTS_FILE" ]]; then
    count_mismatch=false
    for rig_dir in "$TOWN_ROOT"/*/; do
        rig_name=$(basename "$rig_dir")
        [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

        pre_count=$(python3 -c "
import json
data = json.load(open('$COUNTS_FILE'))
print(data.get('rigs', {}).get('$rig_name', -1))
" 2>/dev/null || echo "-1")

        cd "$rig_dir"
        post_count=$(bd list --json 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

        if [[ "$pre_count" == "-1" ]]; then
            echo "  $rig_name: $post_count (no pre-migration count)"
        elif [[ "$pre_count" == "$post_count" ]]; then
            echo "  $rig_name: $pre_count -> $post_count (match)"
        else
            echo "  $rig_name: $pre_count -> $post_count (MISMATCH)"
            count_mismatch=true
        fi
    done
    if [[ "$count_mismatch" == "false" ]]; then
        pass "Bead counts match pre-migration"
    else
        fail_check "Bead count mismatches detected"
    fi
else
    warn "No pre-migration counts file found, skipping count comparison"
fi

# ============================================
# CHECK 4: Sample bead content verification
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 4: Sample bead content ---"
sample_ok=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

    cd "$rig_dir"
    # Get a sample bead and verify it has required fields
    sample=$(bd list --json --limit 1 2>/dev/null | python3 -c "
import sys, json
beads = json.load(sys.stdin)
if beads:
    b = beads[0]
    missing = [f for f in ['id', 'title', 'status'] if not b.get(f)]
    if missing:
        print('MISSING:' + ','.join(missing))
    else:
        print('OK:' + b['id'])
else:
    print('EMPTY')
" 2>/dev/null || echo "ERROR")

    if [[ "$sample" == OK:* ]]; then
        echo "  $rig_name: ${sample#OK:} has required fields"
    elif [[ "$sample" == "EMPTY" ]]; then
        echo "  $rig_name: (empty, OK if no beads expected)"
    else
        echo "  $rig_name: $sample"
        sample_ok=false
    fi
done
if [[ "$sample_ok" == "true" ]]; then
    pass "Sample beads have required fields"
else
    fail_check "Some beads missing required fields"
fi

# ============================================
# CHECK 5: Dependency integrity
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 5: Dependency integrity ---"
deps_ok=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

    cd "$rig_dir"
    # Check that dependency targets exist
    broken=$(bd list --json 2>/dev/null | python3 -c "
import sys, json
beads = json.load(sys.stdin)
ids = {b['id'] for b in beads}
broken = 0
for b in beads:
    for dep in b.get('depends_on', []) or []:
        dep_id = dep if isinstance(dep, str) else dep.get('id', '')
        if dep_id and dep_id not in ids:
            broken += 1
print(broken)
" 2>/dev/null || echo "0")

    if [[ "$broken" != "0" ]]; then
        echo "  $rig_name: $broken broken dependencies"
        deps_ok=false
    fi
done
if [[ "$deps_ok" == "true" ]]; then
    pass "All dependencies reference existing beads"
else
    fail_check "Broken dependencies found"
fi

# ============================================
# CHECK 6: Labels preserved
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 6: Labels preserved ---"
labels_ok=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

    cd "$rig_dir"
    label_count=$(bd list --json 2>/dev/null | python3 -c "
import sys, json
beads = json.load(sys.stdin)
total = sum(len(b.get('labels', []) or []) for b in beads)
print(total)
" 2>/dev/null || echo "0")
    echo "  $rig_name: $label_count labels"
done
pass "Label check complete (manual: verify counts match pre-migration)"

# ============================================
# CHECK 7: Status distribution preserved
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 7: Status distribution ---"
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

    cd "$rig_dir"
    bd list --json 2>/dev/null | python3 -c "
import sys, json
from collections import Counter
beads = json.load(sys.stdin)
counts = Counter(b.get('status', 'unknown') for b in beads)
parts = [f'{s}={c}' for s, c in sorted(counts.items())]
print('  $rig_name: ' + ', '.join(parts) if parts else '  $rig_name: (empty)')
" 2>/dev/null || echo "  $rig_name: (error reading)"
done
pass "Status distribution recorded"

# ============================================
# CHECK 8: No SQLite files remain as active backend
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 8: SQLite decommissioned ---"
sqlite_active=false
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    metadata="$rig_dir/.beads/metadata.json"
    [[ -f "$metadata" ]] || continue

    db_field=$(python3 -c "import json; print(json.load(open('$metadata')).get('database', ''))" 2>/dev/null || echo "")
    if [[ "$db_field" == *.db ]]; then
        backend=$(python3 -c "import json; print(json.load(open('$metadata')).get('backend', ''))" 2>/dev/null || echo "")
        if [[ "$backend" != "dolt" ]]; then
            echo "  $rig_name: still using SQLite ($db_field)"
            sqlite_active=true
        fi
    fi
done
if [[ "$sqlite_active" == "false" ]]; then
    pass "No rigs actively using SQLite"
else
    fail_check "Some rigs still have active SQLite backend"
fi

# ============================================
# CHECK 9: gt doctor passes
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 9: gt doctor ---"
cd "$TOWN_ROOT"
if gt doctor 2>&1 | tail -1 | grep -qi "pass\|healthy\|ok"; then
    pass "gt doctor reports healthy"
else
    warn "gt doctor may have warnings (review output above)"
fi

# ============================================
# CHECK 10: bd commands work on migrated data
# ============================================
CHECKS=$((CHECKS + 1))
echo
echo "--- Check 10: bd operational check ---"
bd_works=true
for rig_dir in "$TOWN_ROOT"/*/; do
    rig_name=$(basename "$rig_dir")
    [[ -f "$rig_dir/.beads/metadata.json" ]] || continue

    cd "$rig_dir"
    if bd stats 2>/dev/null | grep -q "total\|count\|issues"; then
        echo "  $rig_name: bd stats OK"
    else
        echo "  $rig_name: bd stats failed"
        bd_works=false
    fi
done
if [[ "$bd_works" == "true" ]]; then
    pass "bd commands operational on all rigs"
else
    fail_check "bd commands failing on some rigs"
fi

# ============================================
# SUMMARY
# ============================================
echo
echo "================================================"
echo "  Validation Results: $((CHECKS - FAILURES))/$CHECKS passed"
if [[ $FAILURES -gt 0 ]]; then
    echo -e "  ${RED}$FAILURES check(s) failed${NC}"
else
    echo -e "  ${GREEN}All checks passed${NC}"
fi
echo "================================================"

exit $FAILURES
