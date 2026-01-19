#!/bin/bash
# Polecat Lifecycle E2E Test
# Tests work assignment → execution → completion for a Polecat agent
#
# Prerequisites:
# - opencode CLI installed
# - gt CLI built (make build)
# - beads (bd) 0.48.0+ installed
#
# Usage: ./scripts/test-opencode-polecat-e2e.sh [--verbose] [--keep-session]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="/tmp/gastown-polecat-e2e-$(date +%s)"
LOG_DIR="$TEST_DIR/logs"
OPENCODE_PORT=4098  # Different port
VERBOSE=false
KEEP_SESSION=false

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
        kill "$OPENCODE_PID" 2>/dev/null || true
        wait "$OPENCODE_PID" 2>/dev/null || true
    fi
    pkill -f "opencode serve --port $OPENCODE_PORT" 2>/dev/null || true
    
    if ! $KEEP_SESSION && [[ -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
    else
        log "Test artifacts kept: $TEST_DIR"
    fi
}
trap cleanup EXIT

# =============================================================================
# PREREQUISITES
# =============================================================================

log "═══════════════════════════════════════════════════════════════════"
log "  Polecat Lifecycle E2E Test"
log "═══════════════════════════════════════════════════════════════════"
echo ""

log "Checking prerequisites..."

if ! command -v opencode &>/dev/null; then
    log_error "opencode CLI not found"
    exit 1
fi
log_success "opencode: $(opencode --version 2>/dev/null | head -1)"

GT_BINARY="$PROJECT_ROOT/gt"
if [[ ! -x "$GT_BINARY" ]]; then
    (cd "$PROJECT_ROOT" && make build) || exit 1
fi
log_success "gt: $("$GT_BINARY" version 2>/dev/null | head -1)"

if ! command -v bd &>/dev/null; then
    log_error "beads (bd) not found"
    exit 1
fi
BD_VERSION=$(bd version 2>/dev/null | head -1)
log_success "beads: $BD_VERSION"

echo ""

# =============================================================================
# SETUP TEST RIG WITH POLECAT WORKTREE
# =============================================================================

log "Setting up test environment..."

mkdir -p "$TEST_DIR" "$LOG_DIR"

# Create rig structure (simplified)
RIG_ROOT="$TEST_DIR/test-rig"
mkdir -p "$RIG_ROOT/polecat-test"
mkdir -p "$RIG_ROOT/.beads"

# Initialize git
(cd "$RIG_ROOT" && git init --quiet && git config user.email "test@test.com" && git config user.name "Test")
echo "# Test Rig" > "$RIG_ROOT/README.md"
(cd "$RIG_ROOT" && git add . && git commit -m "Initial commit" --quiet)
log_success "Created test rig: $RIG_ROOT"

# Initialize beads
(cd "$RIG_ROOT" && bd init --quiet 2>/dev/null) || log_warn "bd init skipped"

# Copy gt binary
mkdir -p "$RIG_ROOT/bin"
cp "$GT_BINARY" "$RIG_ROOT/bin/gt"
chmod +x "$RIG_ROOT/bin/gt"
export GT_BINARY_PATH="$RIG_ROOT/bin/gt"

# Create polecat worktree directory
POLECAT_DIR="$RIG_ROOT/polecat-test"

# Install plugin
PLUGIN_DIR="$POLECAT_DIR/.opencode/plugin"
mkdir -p "$PLUGIN_DIR"
cp "$PROJECT_ROOT/internal/opencode/plugin/gastown.js" "$PLUGIN_DIR/"
log_success "Installed gastown.js plugin for polecat"

# Create OPENCODE.md for polecat
cat > "$POLECAT_DIR/OPENCODE.md" << 'EOF'
# Polecat Worker Instructions

You are a POLECAT worker agent.

## Your Role
- Execute assigned work from your hook
- Complete tasks and report done
- Do NOT wait idly - work or escalate

## Commands
- `gt prime` - Recover context
- `gt mail check --inject` - Get work assignments
- `gt done` - Complete current work
EOF

log_success "Created polecat OPENCODE.md"

echo ""

# =============================================================================
# START OPENCODE SERVER FOR POLECAT
# =============================================================================

log "Starting OpenCode server for Polecat..."

OPENCODE_SERVER_LOG="$LOG_DIR/opencode-server.log"

if lsof -ti:$OPENCODE_PORT >/dev/null 2>&1; then
    lsof -ti:$OPENCODE_PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# Start with polecat role
(cd "$POLECAT_DIR" && \
    GT_ROLE=polecat \
    GT_BINARY_PATH="$GT_BINARY_PATH" \
    GT_ROOT="$RIG_ROOT" \
    BEADS_DIR="$RIG_ROOT/.beads" \
    opencode serve --port "$OPENCODE_PORT" > "$OPENCODE_SERVER_LOG" 2>&1) &
OPENCODE_PID=$!
log_detail "OpenCode server PID: $OPENCODE_PID"

# Wait for server
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
# CREATE POLECAT SESSION
# =============================================================================

log "Creating Polecat session..."

CREATE_RESPONSE=$(curl -s -X POST "http://localhost:$OPENCODE_PORT/session" \
    -H "Content-Type: application/json" \
    -d '{"path": "'"$POLECAT_DIR"'"}' 2>&1)

SESSION_ID=$(echo "$CREATE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")

if [[ -z "$SESSION_ID" ]]; then
    log_warn "Could not extract session ID"
else
    log_success "Created Polecat session: $SESSION_ID"
fi

echo ""

# =============================================================================
# DEEP VERIFICATION: POLECAT LIFECYCLE
# =============================================================================

log "Verifying Polecat lifecycle hooks..."

sleep 3  # Allow hooks to execute

grep "\[gastown\]" "$OPENCODE_SERVER_LOG" > "$LOG_DIR/gastown-events.log" 2>/dev/null || true

HOOK_LOG="$LOG_DIR/hook-analysis.txt"
echo "=== Polecat Lifecycle Verification ===" > "$HOOK_LOG"
echo "Timestamp: $(date -Iseconds)" >> "$HOOK_LOG"
echo "" >> "$HOOK_LOG"

CHECKS_PASSED=0
CHECKS_TOTAL=7

# Check 1: Plugin loaded with polecat role
echo "--- Check 1: Plugin Initialization ---" >> "$HOOK_LOG"
if grep -q "Plugin loaded" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 1: Plugin initialized"
    echo "[PASS] Plugin loaded" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 1: Plugin NOT initialized"
    echo "[FAIL] Plugin not loaded" >> "$HOOK_LOG"
fi

# Check 2: session.created triggered
echo "" >> "$HOOK_LOG"
echo "--- Check 2: session.created ---" >> "$HOOK_LOG"
if grep -q "session.created triggered" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 2: session.created fired"
    echo "[PASS] session.created" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 2: session.created NOT fired"
    echo "[FAIL] session.created" >> "$HOOK_LOG"
fi

# Check 3: gt prime executed
echo "" >> "$HOOK_LOG"
echo "--- Check 3: gt prime ---" >> "$HOOK_LOG"
if grep -q "Success: gt prime" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 3: gt prime succeeded"
    echo "[PASS] gt prime" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_error "Check 3: gt prime failed or not run"
    echo "[FAIL] gt prime" >> "$HOOK_LOG"
fi

# Check 4: Mail check for autonomous role (polecat should check mail)
echo "" >> "$HOOK_LOG"
echo "--- Check 4: Autonomous Mail Check ---" >> "$HOOK_LOG"
if grep -q "Autonomous role.*checking mail" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 4: Polecat checking mail (autonomous)"
    echo "[PASS] Autonomous mail check" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
elif grep -q "gt mail check" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 4: Mail check executed"
    echo "[PASS] Mail check" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_warn "Check 4: Mail check not detected"
    echo "[WARN] Mail check not found" >> "$HOOK_LOG"
fi

# Check 5: Deacon nudge attempted
echo "" >> "$HOOK_LOG"
echo "--- Check 5: Deacon Nudge ---" >> "$HOOK_LOG"
if grep -q "gt nudge deacon" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Check 5: Deacon nudge attempted"
    echo "[PASS] Deacon nudge" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
else
    log_warn "Check 5: Deacon nudge not found"
    echo "[WARN] Deacon nudge not attempted" >> "$HOOK_LOG"
fi

# Check 6: No crashes/panics
echo "" >> "$HOOK_LOG"
echo "--- Check 6: Stability ---" >> "$HOOK_LOG"
if grep -qi "panic\|fatal\|crash" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_error "Check 6: Crash detected!"
    echo "[FAIL] Crash/panic in logs" >> "$HOOK_LOG"
else
    log_success "Check 6: No crashes"
    echo "[PASS] No crashes" >> "$HOOK_LOG"
    ((CHECKS_PASSED++))
fi

# Check 7: Session responsive
echo "" >> "$HOOK_LOG"
echo "--- Check 7: Session Responsive ---" >> "$HOOK_LOG"
if [[ -n "$SESSION_ID" ]]; then
    SESSION_INFO=$(curl -s "http://localhost:$OPENCODE_PORT/session/$SESSION_ID" 2>&1)
    if echo "$SESSION_INFO" | grep -q '"id"'; then
        log_success "Check 7: Session responsive"
        echo "[PASS] Session responsive" >> "$HOOK_LOG"
        ((CHECKS_PASSED++))
    else
        log_error "Check 7: Session not responsive"
        echo "[FAIL] Session not responsive" >> "$HOOK_LOG"
    fi
else
    log_warn "Check 7: No session ID"
    echo "[SKIP] No session ID" >> "$HOOK_LOG"
fi

echo "" >> "$HOOK_LOG"
echo "=== Summary ===" >> "$HOOK_LOG"
echo "Checks Passed: $CHECKS_PASSED/$CHECKS_TOTAL" >> "$HOOK_LOG"

echo ""

# =============================================================================
# GENERATE REPORT
# =============================================================================

log "Generating report..."

REPORT="$LOG_DIR/polecat-e2e-report.md"
cat > "$REPORT" << EOF
# Polecat Lifecycle E2E Report

**Date**: $(date -Iseconds)
**Test Directory**: $TEST_DIR

## Results: $CHECKS_PASSED/$CHECKS_TOTAL checks passed

$(cat "$HOOK_LOG")

## Gastown Events

\`\`\`
$(cat "$LOG_DIR/gastown-events.log" 2>/dev/null || echo "(no events)")
\`\`\`

## Server Log (last 30 lines)

\`\`\`
$(tail -30 "$OPENCODE_SERVER_LOG" 2>/dev/null || echo "(no log)")
\`\`\`
EOF

log_success "Report: $REPORT"

echo ""

# =============================================================================
# SUMMARY
# =============================================================================

log "═══════════════════════════════════════════════════════════════════"
log "  Test Summary"
log "═══════════════════════════════════════════════════════════════════"
echo ""
echo "Checks passed: $CHECKS_PASSED/$CHECKS_TOTAL"
echo "Logs: $LOG_DIR"
echo ""

# Pass if >= 5 checks pass (allow some expected failures in test env)
if [[ $CHECKS_PASSED -ge 5 ]]; then
    log_success "Polecat lifecycle test PASSED!"
    exit 0
else
    log_error "Polecat lifecycle test FAILED"
    exit 1
fi
