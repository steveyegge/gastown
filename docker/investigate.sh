#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_step() { echo -e "\n${BLUE}=== $1 ===${NC}\n"; }
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_debug() { echo -e "${CYAN}[DEBUG]${NC} $1"; }
log_test() { echo -e "${YELLOW}[TEST]${NC} $1"; }

TEST_REPO_URL="https://github.com/octocat/Hello-World.git"
TEST_BRANCH="master"

NESTED_BASE="$HOME/abcd/ghij"
TOWN_ROOT="$NESTED_BASE/gastown_ui"
RIG_NAME="testrig"

PASS_COUNT=0
FAIL_COUNT=0

assert_role() {
    local dir="$1"
    local expected="$2"
    local test_id="$3"
    local description="$4"
    
    echo ""
    log_test "$test_id: $description"
    log_debug "Directory: $dir"
    log_debug "Expected: $expected"
    
    if [ ! -d "$dir" ]; then
        if [ "$expected" = "ERROR" ]; then
            log_info "PASS - Directory doesn't exist (expected)"
            ((PASS_COUNT++))
            return 0
        fi
        log_error "FAIL - Directory doesn't exist: $dir"
        ((FAIL_COUNT++))
        return 1
    fi
    
    cd "$dir"
    output=$(gt prime 2>&1) || true
    
    if [ "$expected" = "ERROR" ]; then
        if echo "$output" | grep -qE "(not in a Gas Town|not found|error)"; then
            log_info "PASS - Got expected error"
            ((PASS_COUNT++))
            return 0
        else
            log_error "FAIL - Expected error but got output"
            ((FAIL_COUNT++))
            return 1
        fi
    fi
    
    detected=$(echo "$output" | grep -oE "(Mayor|Witness|Refinery|Polecat|Crew|Deacon|unknown)" | head -1 || echo "none")
    
    echo "$output" | grep -E "(Context|You are|Rig:)" | head -3
    
    if [ "$detected" = "$expected" ]; then
        log_info "PASS - Detected '$detected'"
        ((PASS_COUNT++))
    else
        log_error "FAIL - Detected '$detected' but expected '$expected'"
        ((FAIL_COUNT++))
    fi
}

assert_workspace_find() {
    local dir="$1"
    local should_find="$2"
    local test_id="$3"
    
    echo ""
    log_test "$test_id: workspace.Find() from $dir"
    
    if [ ! -d "$dir" ]; then
        mkdir -p "$dir"
    fi
    
    cd "$dir"
    result=$(gt prime 2>&1) || true
    
    if [ "$should_find" = "yes" ]; then
        if echo "$result" | grep -qE "(Mayor|Witness|Refinery|Polecat|Context)"; then
            log_info "PASS - Workspace found"
            ((PASS_COUNT++))
        else
            log_error "FAIL - Workspace NOT found"
            echo "$result" | head -5
            ((FAIL_COUNT++))
        fi
    else
        if echo "$result" | grep -qE "(not in a Gas Town|error)"; then
            log_info "PASS - Correctly detected NOT in workspace"
            ((PASS_COUNT++))
        else
            log_error "FAIL - Should not find workspace but did"
            ((FAIL_COUNT++))
        fi
    fi
}

log_step "NESTED PATH INVESTIGATION"
echo "Testing Gas Town workspace at deeply nested path"
echo "Path: $TOWN_ROOT"
echo ""

log_step "Step 1: Setup - Create nested directory structure"
rm -rf "$HOME/abcd" 2>/dev/null || true
mkdir -p "$NESTED_BASE"
log_info "Created: $NESTED_BASE"

log_step "Step 2: Create Gas Town workspace at nested path"
gt install "$TOWN_ROOT" --name gastown_ui --git 2>&1 | grep -E "(✓|Created|HQ)"
log_info "Workspace created at: $TOWN_ROOT"

log_step "Step 3: Verify workspace structure"
ls -la "$TOWN_ROOT"
echo ""
log_debug "mayor/town.json exists: $(test -f "$TOWN_ROOT/mayor/town.json" && echo 'YES' || echo 'NO')"

log_step "Step 4: Add test rig"
cd "$TOWN_ROOT"
gt rig add "$RIG_NAME" "$TEST_REPO_URL" --branch "$TEST_BRANCH" 2>&1 | grep -E "(✓|Created|Structure)" | head -10
log_info "Rig added: $RIG_NAME"

log_step "Step 5: Display full directory tree"
echo "Full path structure:"
find "$NESTED_BASE" -type d | head -30 | while read -r dir; do
    depth=$(($(echo "$dir" | tr -cd '/' | wc -c) - $(echo "$NESTED_BASE" | tr -cd '/' | wc -c)))
    indent=$(printf '%*s' $((depth * 2)) '')
    echo "${indent}$(basename "$dir")/"
done

log_step "Step 6: Test Cases - Role Detection"

log_debug "T1-T2: Outside workspace (should fail)"
assert_workspace_find "$HOME/abcd" "no" "T1"
assert_workspace_find "$NESTED_BASE" "no" "T2"

log_debug "T3-T4: Town root and mayor directory"
assert_role "$TOWN_ROOT" "Mayor" "T3" "Town root should be Mayor"
assert_role "$TOWN_ROOT/mayor" "Mayor" "T4" "mayor/ directory should be Mayor"

log_debug "T5-T7: Rig-level roles"
assert_role "$TOWN_ROOT/$RIG_NAME/refinery/rig" "Refinery" "T5" "refinery/rig should be Refinery"
assert_role "$TOWN_ROOT/$RIG_NAME/witness" "Witness" "T6" "witness/ should be Witness"
assert_role "$TOWN_ROOT/$RIG_NAME/mayor/rig" "Mayor" "T7" "rig's mayor/rig should be Mayor"

log_debug "T8: Rig root (edge case)"
assert_role "$TOWN_ROOT/$RIG_NAME" "unknown" "T8" "bare rig root should be unknown"

log_debug "T9: Subdirectory inheritance"
SUBDIR="$TOWN_ROOT/$RIG_NAME/refinery/rig/deep/nested/path"
mkdir -p "$SUBDIR"
assert_role "$SUBDIR" "Refinery" "T9" "nested subdir should inherit Refinery"

log_step "Step 7: Test GT_ROLE environment variable mismatch"
echo ""
log_test "T10: GT_ROLE mismatch detection"
cd "$TOWN_ROOT"
output=$(GT_ROLE="testrig/refinery" gt prime 2>&1)
if echo "$output" | grep -q "MISMATCH"; then
    log_info "PASS - Mismatch correctly detected"
    echo "$output" | grep -E "(MISMATCH|suggests)" | head -3
    ((PASS_COUNT++))
else
    log_error "FAIL - Mismatch not detected"
    ((FAIL_COUNT++))
fi

log_step "Step 8: Start refinery and verify session"
echo ""
log_test "T11: Refinery session setup"
cd "$TOWN_ROOT"
gt refinery start "$RIG_NAME" 2>&1 | head -5

sleep 2

SESSION_NAME="gt-$RIG_NAME-refinery"
if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
    log_info "Session exists: $SESSION_NAME"
    
    echo ""
    log_debug "Session environment:"
    tmux show-environment -t "$SESSION_NAME" 2>&1 | grep -E "^GT_" || echo "  (none)"
    
    echo ""
    log_debug "Session working directory:"
    tmux send-keys -t "$SESSION_NAME" "pwd" Enter
    sleep 1
    session_pwd=$(tmux capture-pane -t "$SESSION_NAME" -p | grep -E "^/" | tail -1)
    echo "  $session_pwd"
    
    expected_pwd="$TOWN_ROOT/$RIG_NAME/refinery/rig"
    if [ "$session_pwd" = "$expected_pwd" ]; then
        log_info "PASS - Session working directory correct"
        ((PASS_COUNT++))
    else
        log_error "FAIL - Expected: $expected_pwd"
        log_error "       Got: $session_pwd"
        ((FAIL_COUNT++))
    fi
    
    echo ""
    log_debug "gt prime output in session:"
    tmux send-keys -t "$SESSION_NAME" "gt prime 2>&1 | grep -E '(Context|Role|Refinery)' | head -5" Enter
    sleep 2
    tmux capture-pane -t "$SESSION_NAME" -p | tail -10
else
    log_error "Session $SESSION_NAME does not exist"
    ((FAIL_COUNT++))
fi

log_step "Step 9: Test gt refinery queue from different locations"
echo ""
log_test "T12: gt refinery queue from town root"
cd "$TOWN_ROOT"
log_debug "Running: gt refinery queue $RIG_NAME"
queue_output=$(gt refinery queue "$RIG_NAME" 2>&1) || true
echo "$queue_output" | head -5
if echo "$queue_output" | grep -qE "(queue|empty|pending|\[)"; then
    log_info "PASS - Queue command executed"
    ((PASS_COUNT++))
else
    log_warn "Queue command had issues (may be beads config)"
fi

echo ""
log_test "T13: gt refinery queue from refinery/rig (infer rig)"
cd "$TOWN_ROOT/$RIG_NAME/refinery/rig"
log_debug "Running: gt refinery queue (no rig arg, should infer)"
queue_output=$(gt refinery queue 2>&1) || true
echo "$queue_output" | head -5

log_step "RESULTS SUMMARY"
echo ""
echo "================================"
echo "  PASSED: $PASS_COUNT"
echo "  FAILED: $FAIL_COUNT"
echo "================================"
echo ""

if [ $FAIL_COUNT -eq 0 ]; then
    log_info "All tests passed!"
else
    log_error "$FAIL_COUNT test(s) failed"
fi

log_step "Key Paths Reference"
echo "Nested base:     $NESTED_BASE"
echo "Town root:       $TOWN_ROOT"
echo "Rig root:        $TOWN_ROOT/$RIG_NAME"
echo "Refinery rig:    $TOWN_ROOT/$RIG_NAME/refinery/rig"
echo "Witness:         $TOWN_ROOT/$RIG_NAME/witness"
echo ""
echo "To continue investigating:"
echo "  docker run --rm -it docker-gastown-debug bash"
echo "  cd $TOWN_ROOT/$RIG_NAME/refinery/rig && gt prime"
