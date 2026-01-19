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

# Check for each expected hook
HOOKS_PASSED=0
HOOKS_TOTAL=3

# Hook 1: Plugin loaded
if grep -q "Plugin loaded" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Hook: Plugin initialized"
    echo "[PASS] Plugin loaded" >> "$HOOK_LOG"
    ((HOOKS_PASSED++))
else
    log_error "Hook: Plugin NOT initialized"
    echo "[FAIL] Plugin loaded" >> "$HOOK_LOG"
fi

# Hook 2: session.created
if grep -q "session.created triggered" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Hook: session.created fired"
    echo "[PASS] session.created" >> "$HOOK_LOG"
    ((HOOKS_PASSED++))
else
    log_error "Hook: session.created NOT fired"
    echo "[FAIL] session.created" >> "$HOOK_LOG"
fi

# Hook 3: gt prime executed
if grep -q "Success: gt prime" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Hook: gt prime succeeded"
    echo "[PASS] gt prime" >> "$HOOK_LOG"
    ((HOOKS_PASSED++))
elif grep -q "Running.*gt prime" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_warn "Hook: gt prime attempted but may have failed"
    echo "[WARN] gt prime - attempted" >> "$HOOK_LOG"
else
    log_error "Hook: gt prime NOT executed"
    echo "[FAIL] gt prime" >> "$HOOK_LOG"
fi

echo "" >> "$HOOK_LOG"
echo "Hooks Passed: $HOOKS_PASSED/$HOOKS_TOTAL" >> "$HOOK_LOG"

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
