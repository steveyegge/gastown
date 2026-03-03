#!/usr/bin/env bash
#
# Tests for context-budget-guard.sh
#
# Run: bash scripts/guards/context-budget-guard_test.sh
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GUARD="$SCRIPT_DIR/context-budget-guard.sh"
PASS=0
FAIL=0

# ── Helpers ──────────────────────────────────────────────────────────────────

run_guard() {
    # Run the guard with the given env vars in a clean GT_* environment.
    # Unset all GT_* vars to prevent test pollution from the current shell.
    local exit_code=0
    STDERR=$(env -u GT_ROLE -u GT_POLECAT -u GT_CREW -u GT_MAYOR -u GT_DEACON \
        -u GT_WITNESS -u GT_REFINERY -u GT_CONTEXT_BUDGET_DISABLE \
        -u GT_CONTEXT_BUDGET_WARN -u GT_CONTEXT_BUDGET_SOFT_GATE \
        -u GT_CONTEXT_BUDGET_HARD_GATE -u GT_CONTEXT_BUDGET_MAX_TOKENS \
        -u GT_CONTEXT_BUDGET_TOKENS -u GT_CONTEXT_BUDGET_HARD_GATE_ROLES \
        "$@" bash "$GUARD" 2>&1) || exit_code=$?
    echo "$exit_code"
}

assert_exit() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    if [[ "$actual" == "$expected" ]]; then
        echo "  PASS: $test_name (exit $actual)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $test_name (expected exit $expected, got $actual)"
        FAIL=$((FAIL + 1))
    fi
}

# Create a temporary transcript directory mimicking Claude Code's layout.
# Returns the tmpdir root; the working directory is $tmpdir/workdir.
# The project dir is dynamically computed to match what the guard will derive from pwd.
setup_transcript() {
    local tokens="$1"
    local cache_create="${2:-0}"
    local cache_read="${3:-0}"
    local tmpdir
    tmpdir=$(mktemp -d)

    # Create a working directory the guard will cd into
    local workdir="$tmpdir/workdir"
    mkdir -p "$workdir"

    # Claude Code project dir: $HOME/.claude/projects/<cwd-with-slashes-replaced-by-dashes>
    # The guard computes: $HOME/.claude/projects/$(pwd | tr '/' '-')
    local project_name
    project_name=$(echo "$workdir" | tr '/' '-')
    local project_dir="$tmpdir/.claude/projects/$project_name"
    mkdir -p "$project_dir"

    # Write a transcript with the specified token counts
    cat > "$project_dir/session.jsonl" <<JSONL
{"type":"human"}
{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","usage":{"input_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":500}}}
{"type":"human"}
{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","usage":{"input_tokens":$tokens,"cache_creation_input_tokens":$cache_create,"cache_read_input_tokens":$cache_read,"output_tokens":1000}}}
JSONL

    echo "$tmpdir"
}

cleanup() {
    rm -rf "$1"
}

# ── Tests ────────────────────────────────────────────────────────────────────

echo "=== context-budget-guard tests ==="
echo ""

# Test 1: Disabled guard exits 0
echo "Test: disabled guard"
code=$(run_guard GT_CONTEXT_BUDGET_DISABLE=1)
assert_exit "disabled guard exits 0" "0" "$code"

# Test 2: Pre-computed tokens under threshold exits 0
echo "Test: under threshold (pre-computed tokens)"
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=50000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=mayor)
assert_exit "50k/200k tokens exits 0" "0" "$code"

# Test 3: Pre-computed tokens over hard gate, hard-gated role → exit 2
echo "Test: over hard gate, hard-gated role (pre-computed)"
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=190000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=mayor)
assert_exit "190k/200k mayor exits 2" "2" "$code"

# Test 4: Pre-computed tokens over hard gate, warn-only role → exit 0
echo "Test: over hard gate, warn-only role (pre-computed)"
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=190000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=crew)
assert_exit "190k/200k crew exits 0" "0" "$code"

# Test 5: Pre-computed tokens over hard gate, polecat → exit 0
echo "Test: over hard gate, polecat role (pre-computed)"
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=190000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_POLECAT=alpha)
assert_exit "190k/200k polecat exits 0" "0" "$code"

# Test 6: Custom hard-gate roles
echo "Test: custom hard-gate roles"
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=190000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=crew GT_CONTEXT_BUDGET_HARD_GATE_ROLES=crew,polecat)
assert_exit "crew in custom hard-gate roles exits 2" "2" "$code"

# Test 7: Threshold ordering validation (inverted thresholds reset to defaults)
echo "Test: inverted thresholds reset to defaults"
# With warn=0.95, soft=0.90, hard=0.70 — should reset to 0.75/0.85/0.92
# At 160k/200k = 0.80, default warn (0.75) should fire but not hard gate
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=160000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=mayor GT_CONTEXT_BUDGET_WARN=0.95 GT_CONTEXT_BUDGET_SOFT_GATE=0.90 GT_CONTEXT_BUDGET_HARD_GATE=0.70)
assert_exit "inverted thresholds reset, 80% exits 0" "0" "$code"

# Test 8: Unknown role gets hard-gated
echo "Test: unknown role hard-gated"
# run_guard already clears all GT_* role vars, so no role will be detected
code=$(run_guard GT_CONTEXT_BUDGET_TOKENS=190000 GT_CONTEXT_BUDGET_MAX_TOKENS=200000)
assert_exit "unknown role over hard gate exits 2" "2" "$code"

# Test 9: Transcript with cache tokens (integration test)
if command -v jq &>/dev/null; then
    echo "Test: transcript with cache tokens"
    TEST_TMPDIR=$(setup_transcript 8000 50000 120000)
    # Total: 8000 + 50000 + 120000 = 178000, which is 89% of 200k → soft gate (warn-only for crew)
    code=0
    env -u GT_ROLE -u GT_POLECAT -u GT_CREW -u GT_MAYOR -u GT_DEACON -u GT_WITNESS -u GT_REFINERY \
        -u GT_CONTEXT_BUDGET_DISABLE -u GT_CONTEXT_BUDGET_TOKENS \
        HOME="$TEST_TMPDIR" GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=crew \
        bash -c "cd '$TEST_TMPDIR/workdir' && bash '$GUARD'" >/dev/null 2>&1 || code=$?
    assert_exit "cache tokens summed correctly (178k/200k crew) exits 0" "0" "$code"
    cleanup "$TEST_TMPDIR"

    # Test 10: Transcript tokens over hard gate for hard-gated role
    echo "Test: transcript tokens hard gate"
    TEST_TMPDIR=$(setup_transcript 150000 20000 20000)
    # Total: 150000 + 20000 + 20000 = 190000, which is 95% → hard gate
    code=0
    env -u GT_ROLE -u GT_POLECAT -u GT_CREW -u GT_MAYOR -u GT_DEACON -u GT_WITNESS -u GT_REFINERY \
        -u GT_CONTEXT_BUDGET_DISABLE -u GT_CONTEXT_BUDGET_TOKENS \
        HOME="$TEST_TMPDIR" GT_CONTEXT_BUDGET_MAX_TOKENS=200000 GT_ROLE=mayor \
        bash -c "cd '$TEST_TMPDIR/workdir' && bash '$GUARD'" >/dev/null 2>&1 || code=$?
    assert_exit "transcript 190k/200k mayor exits 2" "2" "$code"
    cleanup "$TEST_TMPDIR"
else
    echo "  SKIP: transcript tests (jq not available)"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]] && exit 0 || exit 1
