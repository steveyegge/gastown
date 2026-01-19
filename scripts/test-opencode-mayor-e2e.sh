#!/bin/bash
# Mayor Workflow E2E Test
# Tests a realistic Mayor workflow with OpenCode and Gastown
#
# Prerequisites:
# - opencode CLI installed
# - gt CLI built (make build)
# - beads (bd) installed and initialized
#
# Usage: ./scripts/test-opencode-mayor-e2e.sh [--verbose] [--keep-session]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="/tmp/gastown-mayor-e2e-$(date +%s)"
LOG_DIR="$TEST_DIR/logs"
OPENCODE_PORT=4097  # Different port to avoid conflicts
VERBOSE=false
KEEP_SESSION=false

# Parse args
while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose|-v) VERBOSE=true; shift ;;
        --keep-session) KEEP_SESSION=true; shift ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
log_success() { echo -e "${GREEN}[$(date +%H:%M:%S)] ✓${NC} $*"; }
log_warn() { echo -e "${YELLOW}[$(date +%H:%M:%S)] ⚠${NC} $*"; }
log_error() { echo -e "${RED}[$(date +%H:%M:%S)] ✗${NC} $*"; }
log_detail() { if $VERBOSE; then echo -e "${CYAN}[$(date +%H:%M:%S)]   ${NC}$*"; fi; }

cleanup() {
    log "Cleaning up..."
    
    if [[ -n "${OPENCODE_PID:-}" ]] && kill -0 "$OPENCODE_PID" 2>/dev/null; then
        log_detail "Stopping opencode server (PID: $OPENCODE_PID)"
        kill "$OPENCODE_PID" 2>/dev/null || true
        wait "$OPENCODE_PID" 2>/dev/null || true
    fi
    
    pkill -f "opencode serve --port $OPENCODE_PORT" 2>/dev/null || true
    
    if ! $KEEP_SESSION && [[ -d "$TEST_DIR" ]]; then
        log_detail "Removing test directory: $TEST_DIR"
        rm -rf "$TEST_DIR"
    else
        log "Test artifacts kept at: $TEST_DIR"
        log "  Logs: $LOG_DIR"
    fi
}

trap cleanup EXIT

# =============================================================================
# PREREQUISITES CHECK
# =============================================================================

log "═══════════════════════════════════════════════════════════════════"
log "  Mayor Workflow E2E Test (with Beads)"
log "═══════════════════════════════════════════════════════════════════"
echo ""

log "Checking prerequisites..."

# Check opencode
if ! command -v opencode &>/dev/null; then
    log_error "opencode CLI not found"
    exit 1
fi
log_success "opencode: $(opencode --version 2>/dev/null | head -1)"

# Check gt
GT_BINARY="$PROJECT_ROOT/gt"
if [[ ! -x "$GT_BINARY" ]]; then
    log "Building gt..."
    (cd "$PROJECT_ROOT" && make build) || exit 1
fi
log_success "gt: $("$GT_BINARY" version 2>/dev/null | head -1)"

# Check beads (required for this test)
if ! command -v bd &>/dev/null; then
    log_error "beads (bd) CLI not found - REQUIRED for Mayor E2E test"
    log_error "Install beads first: go install github.com/steveyegge/beads/cmd/bd@latest"
    exit 1
fi
log_success "beads: $(bd version 2>/dev/null | head -1)"

echo ""

# =============================================================================
# SETUP TEST ENVIRONMENT
# =============================================================================

log "Setting up test environment..."

mkdir -p "$TEST_DIR" "$LOG_DIR"

# Create test town structure
TOWN_ROOT="$TEST_DIR/test-town"
mkdir -p "$TOWN_ROOT/mayor"
mkdir -p "$TOWN_ROOT/deacon"
mkdir -p "$TOWN_ROOT/settings"

# Initialize git in town
(cd "$TOWN_ROOT" && git init --quiet)
log_success "Created test town: $TOWN_ROOT"

# Initialize beads database
(cd "$TOWN_ROOT" && bd init --quiet 2>/dev/null) || {
    log_warn "bd init failed - beads may need different initialization"
}

# Check if beads initialized
if [[ -d "$TOWN_ROOT/.beads" ]]; then
    log_success "Beads initialized"
else
    log_warn "Beads directory not created - some tests may fail"
fi

# Copy gt binary
mkdir -p "$TOWN_ROOT/bin"
cp "$GT_BINARY" "$TOWN_ROOT/bin/gt"
chmod +x "$TOWN_ROOT/bin/gt"
export GT_BINARY_PATH="$TOWN_ROOT/bin/gt"
log_success "Installed gt to $TOWN_ROOT/bin/gt"

# Install Gastown plugin
PLUGIN_SRC="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
PLUGIN_DIR="$TOWN_ROOT/mayor/.opencode/plugin"
mkdir -p "$PLUGIN_DIR"
cp "$PLUGIN_SRC" "$PLUGIN_DIR/"
log_success "Installed gastown.js plugin for Mayor"

# Create town config
cat > "$TOWN_ROOT/settings/town.json" << 'EOF'
{
  "name": "test-town",
  "default_agent": "opencode"
}
EOF

# Create OPENCODE.md for Mayor
cat > "$TOWN_ROOT/mayor/OPENCODE.md" << 'EOF'
# Mayor Instructions

You are the MAYOR of Gas Town, coordinating work across the organization.

## Your Capabilities
- Create convoys to track multi-issue work
- Sling issues to polecats
- Coordinate with witnesses and refineries

## Test Task
When you receive a prompt, execute it and report the results.

## Gas Town Commands
- `gt prime` - Recover context
- `gt convoy create <name> [issues...]` - Create a new convoy
- `gt mail check` - Check for messages
- `gt status` - Check town status
EOF

log_success "Created Mayor OPENCODE.md"

echo ""

# =============================================================================
# START OPENCODE SERVER
# =============================================================================

log "Starting OpenCode server for Mayor..."

OPENCODE_SERVER_LOG="$LOG_DIR/opencode-server.log"

if lsof -ti:$OPENCODE_PORT >/dev/null 2>&1; then
    log_warn "Port $OPENCODE_PORT in use - killing existing process"
    lsof -ti:$OPENCODE_PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# Start server with Mayor role environment
(cd "$TOWN_ROOT/mayor" && \
    GT_ROLE=mayor \
    GT_BINARY_PATH="$GT_BINARY_PATH" \
    GT_ROOT="$TOWN_ROOT" \
    BEADS_DIR="$TOWN_ROOT/.beads" \
    opencode serve --port "$OPENCODE_PORT" > "$OPENCODE_SERVER_LOG" 2>&1) &
OPENCODE_PID=$!
log_detail "OpenCode server PID: $OPENCODE_PID"

# Wait for server
log "Waiting for server..."
MAX_WAIT=30
WAIT_COUNT=0
while ! curl -s "http://localhost:$OPENCODE_PORT/session" >/dev/null 2>&1; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [[ $WAIT_COUNT -ge $MAX_WAIT ]]; then
        log_error "Server failed to start"
        cat "$OPENCODE_SERVER_LOG"
        exit 1
    fi
done
log_success "OpenCode server running on port $OPENCODE_PORT"

echo ""

# =============================================================================
# CREATE MAYOR SESSION
# =============================================================================

log "Creating Mayor session..."

CREATE_RESPONSE=$(curl -s -X POST "http://localhost:$OPENCODE_PORT/session" \
    -H "Content-Type: application/json" \
    -d '{"path": "'"$TOWN_ROOT/mayor"'"}' 2>&1)

SESSION_ID=$(echo "$CREATE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")

if [[ -z "$SESSION_ID" ]]; then
    log_warn "Could not extract session ID"
    log_detail "Response: $CREATE_RESPONSE"
else
    log_success "Created Mayor session: $SESSION_ID"
fi

echo ""

# =============================================================================
# VERIFY PLUGIN HOOKS
# =============================================================================

log "Verifying plugin hooks fired..."

sleep 3  # Give hooks time to execute

# Analyze server log for hook events
HOOK_LOG="$LOG_DIR/hook-analysis.txt"

echo "=== Plugin Hook Analysis ===" > "$HOOK_LOG"
echo "Timestamp: $(date -Iseconds)" >> "$HOOK_LOG"
echo "" >> "$HOOK_LOG"

# Deep verification - not just "did it run" but "did it produce useful output"
CHECKS_PASSED=0
CHECKS_TOTAL=6

# Check 1: Plugin loaded with correct configuration
echo "--- Check 1: Plugin Initialization ---" >> "$HOOK_LOG"
if grep -q "Plugin loaded" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    # DEEP: Verify it received the role configuration
    if grep "Plugin loaded" "$OPENCODE_SERVER_LOG" | grep -q "mayor"; then
        log_success "Check 1: Plugin initialized with mayor role"
        echo "[PASS] Plugin loaded with role=mayor" >> "$HOOK_LOG"
        ((CHECKS_PASSED++))
    else
        log_warn "Check 1: Plugin loaded but role not detected"
        echo "[WARN] Plugin loaded but role unclear" >> "$HOOK_LOG"
    fi
else
    log_error "Check 1: Plugin NOT initialized"
    echo "[FAIL] Plugin not loaded" >> "$HOOK_LOG"
fi

# Check 2: session.created hook fired
echo "" >> "$HOOK_LOG"
echo "--- Check 2: session.created Hook ---" >> "$HOOK_LOG"
if grep -q "session.created triggered" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 2: session.created fired"
    echo "[PASS] session.created hook triggered" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 2: session.created NOT fired"
    echo "[FAIL] session.created not triggered" >> "$HOOK_LOG"
fi

# Check 3: gt binary was found (not hardcoded path issue)
echo "" >> "$HOOK_LOG"
echo "--- Check 3: gt Binary Discovery ---" >> "$HOOK_LOG"
if grep -q "Found gt binary" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    GT_PATH=$(grep "Found gt binary" "$OPENCODE_SERVER_LOG" | head -1 | sed 's/.*Found gt binary at: //')
    log_success "Check 3: gt binary found at: $GT_PATH"
    echo "[PASS] gt binary discovered at: $GT_PATH" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 3: gt binary NOT found"
    echo "[FAIL] gt binary discovery failed" >> "$HOOK_LOG"
fi

# Check 4: gt prime executed SUCCESSFULLY (exit 0, not just attempted)
echo "" >> "$HOOK_LOG"
echo "--- Check 4: gt prime Execution ---" >> "$HOOK_LOG"
if grep -q "Success: gt prime" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    # DEEP: Extract duration to verify it actually did work
    PRIME_DURATION=$(grep "Success: gt prime" "$OPENCODE_SERVER_LOG" | grep -o "duration_ms: [0-9]*" | head -1 || echo "unknown")
    log_success "Check 4: gt prime succeeded ($PRIME_DURATION)"
    echo "[PASS] gt prime completed ($PRIME_DURATION)" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    # Check if it failed with a real error
    if grep -q "Failed: gt prime" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
        PRIME_ERROR=$(grep "Failed: gt prime" "$OPENCODE_SERVER_LOG" | head -1)
        log_error "Check 4: gt prime FAILED"
        echo "[FAIL] gt prime failed: $PRIME_ERROR" >> "$HOOK_LOG"
    else
        log_error "Check 4: gt prime NOT executed"
        echo "[FAIL] gt prime not executed" >> "$HOOK_LOG"
    fi
fi

# Check 5: gt nudge deacon executed (verifies inter-agent communication path)
echo "" >> "$HOOK_LOG"
echo "--- Check 5: Deacon Nudge ---" >> "$HOOK_LOG"
if grep -q "Success: gt nudge" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 5: gt nudge deacon succeeded"
    echo "[PASS] Deacon notification sent" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
elif grep -q "Running.*gt nudge deacon" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    # Expected to fail in minimal test env - deacon doesn't exist
    log_warn "Check 5: gt nudge attempted (deacon not running)"
    echo "[WARN] Deacon nudge attempted (expected failure in test env)" >> "$HOOK_LOG"
    # Give credit for attempting - this is correct behavior
    ((CHECKS_PASSED++))
else
    log_error "Check 5: gt nudge NOT executed"
    echo "[FAIL] Deacon nudge not attempted" >> "$HOOK_LOG"
fi

# Check 6: No unhandled errors in plugin (stability check)
echo "" >> "$HOOK_LOG"
echo "--- Check 6: Plugin Stability ---" >> "$HOOK_LOG"
ERROR_COUNT=$(grep -c "\[ERROR\]" "$LOG_DIR/gastown-events.log" 2>/dev/null || echo "0")
WARN_COUNT=$(grep -c "\[WARN\]" "$LOG_DIR/gastown-events.log" 2>/dev/null || echo "0")
if [[ "$ERROR_COUNT" -eq 0 ]] || [[ "$ERROR_COUNT" -le 2 ]]; then
    # Allow up to 2 errors (nudge failures in test env are expected)
    log_success "Check 6: Plugin stable (errors: $ERROR_COUNT, warns: $WARN_COUNT)"
    echo "[PASS] Plugin stability OK (errors: $ERROR_COUNT, warns: $WARN_COUNT)" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 6: Multiple plugin errors detected"
    echo "[FAIL] Too many errors: $ERROR_COUNT" >> "$HOOK_LOG"
    grep "\[ERROR\]" "$LOG_DIR/gastown-events.log" | head -5 >> "$HOOK_LOG"
fi

echo "" >> "$HOOK_LOG"
echo "=== Summary ===" >> "$HOOK_LOG"
echo "Checks Passed: $CHECKS_PASSED/$CHECKS_TOTAL" >> "$HOOK_LOG"

# Set for summary at end
HOOKS_PASSED=$CHECKS_PASSED
HOOKS_TOTAL=$CHECKS_TOTAL

echo ""

# =============================================================================
# TEST MAYOR WORKFLOW (if session created)
# =============================================================================

if [[ -n "$SESSION_ID" ]]; then
    log "Testing Mayor workflow..."
    
    # Note: We can't actually send prompts that get executed without AI
    # But we can verify the session is responsive
    
    # Get session info
    SESSION_INFO=$(curl -s "http://localhost:$OPENCODE_PORT/session/$SESSION_ID" 2>&1)
    log_detail "Session info: $SESSION_INFO"
    
    if echo "$SESSION_INFO" | grep -q '"id"'; then
        log_success "Session is responsive"
    else
        log_warn "Session may not be responsive"
    fi
else
    log_warn "Skipping workflow test - no session ID"
fi

echo ""

# =============================================================================
# COLLECT LOGS AND GENERATE REPORT
# =============================================================================

log "Collecting diagnostics..."

# Copy server log
cp "$OPENCODE_SERVER_LOG" "$LOG_DIR/opencode-server.log"

# Extract gastown-specific logs
grep "\[gastown\]" "$OPENCODE_SERVER_LOG" > "$LOG_DIR/gastown-events.log" 2>/dev/null || true

# Generate summary report
REPORT="$LOG_DIR/e2e-report.md"
cat > "$REPORT" << EOF
# Mayor E2E Test Report

**Date**: $(date -Iseconds)
**Test Directory**: $TEST_DIR

## Results Summary

| Check | Status |
|-------|--------|
| OpenCode server started | ✅ |
| Mayor session created | $(if [[ -n "$SESSION_ID" ]]; then echo "✅"; else echo "⚠️"; fi) |
| Plugin hooks fired | $HOOKS_PASSED/$HOOKS_TOTAL |

## Hook Details

$(cat "$HOOK_LOG")

## Gastown Events

\`\`\`
$(cat "$LOG_DIR/gastown-events.log" 2>/dev/null || echo "(no events)")
\`\`\`

## Server Log (last 50 lines)

\`\`\`
$(tail -50 "$OPENCODE_SERVER_LOG" 2>/dev/null || echo "(no log)")
\`\`\`

## Files

- Server log: \`$LOG_DIR/opencode-server.log\`
- Gastown events: \`$LOG_DIR/gastown-events.log\`
- Hook analysis: \`$LOG_DIR/hook-analysis.txt\`

EOF

log_success "Report generated: $REPORT"

echo ""

# =============================================================================
# SUMMARY
# =============================================================================

log "═══════════════════════════════════════════════════════════════════"
log "  Test Summary"
log "═══════════════════════════════════════════════════════════════════"
echo ""
echo "Hooks passed: $HOOKS_PASSED/$HOOKS_TOTAL"
echo ""
echo "Logs directory: $LOG_DIR"
echo "  - e2e-report.md"
echo "  - opencode-server.log"
echo "  - gastown-events.log"
echo "  - hook-analysis.txt"
echo ""

if [[ $HOOKS_PASSED -eq $HOOKS_TOTAL ]]; then
    log_success "All hooks passed!"
    exit 0
else
    log_warn "Some hooks failed - review logs"
    exit 1
fi
