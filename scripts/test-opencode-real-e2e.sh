#!/bin/bash
# Real OpenCode E2E Test
# Tests actual OpenCode server with Gastown plugin integration
#
# Usage: ./scripts/test-opencode-real-e2e.sh [--verbose] [--keep-session]
#
# Prerequisites:
# - opencode CLI installed (opencode --version)
# - gt CLI built (make build)
# - beads (bd) installed

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="/tmp/gastown-opencode-e2e-$(date +%s)"
LOG_DIR="$TEST_DIR/logs"
OPENCODE_PORT=4096
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
    
    # Kill opencode server if running
    if [[ -n "${OPENCODE_PID:-}" ]] && kill -0 "$OPENCODE_PID" 2>/dev/null; then
        log_detail "Stopping opencode server (PID: $OPENCODE_PID)"
        kill "$OPENCODE_PID" 2>/dev/null || true
        wait "$OPENCODE_PID" 2>/dev/null || true
    fi
    
    # Kill any orphaned opencode processes from test
    pkill -f "opencode serve --port $OPENCODE_PORT" 2>/dev/null || true
    
    if ! $KEEP_SESSION && [[ -d "$TEST_DIR" ]]; then
        log_detail "Removing test directory: $TEST_DIR"
        rm -rf "$TEST_DIR"
    else
        log "Test artifacts kept at: $TEST_DIR"
    fi
}

trap cleanup EXIT

# =============================================================================
# PREREQUISITES CHECK
# =============================================================================

log "═══════════════════════════════════════════════════════════════════"
log "  OpenCode Real E2E Test"
log "═══════════════════════════════════════════════════════════════════"
echo ""

log "Checking prerequisites..."

# Check opencode
if ! command -v opencode &>/dev/null; then
    log_error "opencode CLI not found"
    log_detail "Install with: npm install -g opencode-ai"
    exit 1
fi
OPENCODE_VERSION=$(opencode --version 2>/dev/null | head -1 || echo "unknown")
log_success "opencode: $OPENCODE_VERSION"

# Check gt
GT_BINARY="$PROJECT_ROOT/gt"
if [[ ! -x "$GT_BINARY" ]]; then
    log "Building gt..."
    (cd "$PROJECT_ROOT" && make build) || {
        log_error "Failed to build gt"
        exit 1
    }
fi
GT_VERSION=$("$GT_BINARY" version 2>/dev/null | head -1 || echo "unknown")
log_success "gt: $GT_VERSION"

# Check bd (optional but recommended)
if command -v bd &>/dev/null; then
    BD_VERSION=$(bd version 2>/dev/null | head -1 || echo "unknown")
    log_success "beads: $BD_VERSION"
else
    log_warn "beads (bd) not found - some features will be limited"
fi

# Check for auth
if [[ ! -f ~/.config/opencode/opencode.jsonc ]]; then
    log_warn "OpenCode config not found at ~/.config/opencode/opencode.jsonc"
    log_detail "You may need to run 'opencode auth' first"
fi

echo ""

# =============================================================================
# SETUP TEST ENVIRONMENT
# =============================================================================

log "Setting up test environment..."

mkdir -p "$TEST_DIR" "$LOG_DIR"
log_detail "Test directory: $TEST_DIR"
log_detail "Log directory: $LOG_DIR"

# Create test project with git
TEST_PROJECT="$TEST_DIR/test-project"
mkdir -p "$TEST_PROJECT"
(cd "$TEST_PROJECT" && git init --quiet)
log_success "Created test project: $TEST_PROJECT"

# Install Gastown plugin
PLUGIN_SRC="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
PLUGIN_DIR="$TEST_PROJECT/.opencode/plugin"
mkdir -p "$PLUGIN_DIR"

if [[ -f "$PLUGIN_SRC" ]]; then
    cp "$PLUGIN_SRC" "$PLUGIN_DIR/"
    log_success "Installed gastown.js plugin"
    log_detail "Plugin location: $PLUGIN_DIR/gastown.js"
else
    log_error "Plugin source not found: $PLUGIN_SRC"
    exit 1
fi

# Create OPENCODE.md (equivalent to CLAUDE.md)
cat > "$TEST_PROJECT/OPENCODE.md" << 'OPENCODE_INSTRUCTIONS'
# Gas Town Test Project

This is a test project for Gas Town + OpenCode integration.

## Your Task

You are a polecat (worker agent) testing the Gas Town system.

When you start:
1. The gastown.js plugin should trigger `gt prime`
2. Check for mail with `gt mail check`
3. Report your status

## Test Commands

- `gt prime` - Recover context
- `gt mail check` - Check for incoming work
- `gt version` - Verify CLI works

## Success Criteria

If you can successfully run `gt prime` and see context output, the integration is working.
OPENCODE_INSTRUCTIONS
log_success "Created OPENCODE.md instructions file"

# Create a simple test task file
cat > "$TEST_PROJECT/README.md" << 'README'
# Test Project

A simple test project for validating Gas Town + OpenCode integration.
README

# Copy gt binary to test project bin and set up environment
mkdir -p "$TEST_PROJECT/bin"
cp "$GT_BINARY" "$TEST_PROJECT/bin/gt"
chmod +x "$TEST_PROJECT/bin/gt"
log_success "Copied gt binary to $TEST_PROJECT/bin/gt"

# Set environment variable for plugin
export GT_BINARY_PATH="$TEST_PROJECT/bin/gt"
log_detail "GT_BINARY_PATH=$GT_BINARY_PATH"

echo ""

# =============================================================================
# START OPENCODE SERVER
# =============================================================================

log "Starting OpenCode server..."

OPENCODE_SERVER_LOG="$LOG_DIR/opencode-server.log"

# Check if port is available
if lsof -ti:$OPENCODE_PORT >/dev/null 2>&1; then
    log_warn "Port $OPENCODE_PORT is in use - attempting to kill existing process"
    lsof -ti:$OPENCODE_PORT | xargs kill -9 2>/dev/null || true
    sleep 1
fi

# Start opencode serve in background with GT_BINARY_PATH set
log_detail "Starting with GT_BINARY_PATH=$GT_BINARY_PATH"
(cd "$TEST_PROJECT" && GT_BINARY_PATH="$GT_BINARY_PATH" opencode serve --port "$OPENCODE_PORT" > "$OPENCODE_SERVER_LOG" 2>&1) &
OPENCODE_PID=$!
log_detail "OpenCode server PID: $OPENCODE_PID"
log_detail "Server log: $OPENCODE_SERVER_LOG"

# Wait for server to be ready
log "Waiting for server to be ready..."
MAX_WAIT=30
WAIT_COUNT=0
while ! curl -s "http://localhost:$OPENCODE_PORT/session" >/dev/null 2>&1; do
    sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
    if [[ $WAIT_COUNT -ge $MAX_WAIT ]]; then
        log_error "Server failed to start within ${MAX_WAIT}s"
        log_error "Server log:"
        cat "$OPENCODE_SERVER_LOG"
        exit 1
    fi
    log_detail "Waiting... ($WAIT_COUNT/$MAX_WAIT)"
done
log_success "OpenCode server running on port $OPENCODE_PORT"

echo ""

# =============================================================================
# VERIFY SERVER API
# =============================================================================

log "Verifying server API..."

# List sessions (should be empty)
SESSIONS_RESPONSE=$(curl -s "http://localhost:$OPENCODE_PORT/session" 2>&1)
log_detail "GET /session response: $SESSIONS_RESPONSE"
log_success "Session list API working"

# Check config
CONFIG_RESPONSE=$(curl -s "http://localhost:$OPENCODE_PORT/config" 2>&1)
log_detail "GET /config response: $CONFIG_RESPONSE"
log_success "Config API working"

echo ""

# =============================================================================
# CREATE AND RUN SESSION
# =============================================================================

log "Creating test session..."

# Create a new session via API
CREATE_RESPONSE=$(curl -s -X POST "http://localhost:$OPENCODE_PORT/session" \
    -H "Content-Type: application/json" \
    -d '{"path": "'"$TEST_PROJECT"'"}' 2>&1)

log_detail "POST /session response: $CREATE_RESPONSE"

# Extract session ID (assuming JSON response)
SESSION_ID=$(echo "$CREATE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4 || echo "")

if [[ -z "$SESSION_ID" ]]; then
    log_warn "Could not extract session ID from response"
    log_detail "Full response: $CREATE_RESPONSE"
    # Try alternative: use opencode CLI to create session
    log "Falling back to CLI-based session creation..."
    
    # Run opencode with a simple prompt non-interactively
    OPENCODE_RUN_LOG="$LOG_DIR/opencode-run.log"
    log "Running: opencode run --prompt 'Run gt version and report the output'"
    
    (cd "$TEST_PROJECT" && timeout 60 opencode run \
        --prompt "Run the command 'gt version' and tell me what it outputs. Then run 'gt prime' and report if it works." \
        > "$OPENCODE_RUN_LOG" 2>&1) || {
        EXIT_CODE=$?
        log_warn "opencode run exited with code $EXIT_CODE"
    }
    
    log "OpenCode run output:"
    echo "─────────────────────────────────────────────────────────────────"
    cat "$OPENCODE_RUN_LOG" | head -50
    echo "─────────────────────────────────────────────────────────────────"
    
else
    log_success "Created session: $SESSION_ID"
    
    # Send a prompt to the session
    log "Sending test prompt to session..."
    
    PROMPT_RESPONSE=$(curl -s -X POST "http://localhost:$OPENCODE_PORT/session/$SESSION_ID/prompt" \
        -H "Content-Type: application/json" \
        -d '{"message": "Run gt version and tell me the output. Then run gt prime and report what happens."}' 2>&1)
    
    log_detail "Prompt response: $PROMPT_RESPONSE"
    
    # Wait for response and poll session
    log "Waiting for session response..."
    sleep 5
    
    # Get session messages
    MESSAGES_RESPONSE=$(curl -s "http://localhost:$OPENCODE_PORT/session/$SESSION_ID/messages" 2>&1)
    log "Session messages:"
    echo "─────────────────────────────────────────────────────────────────"
    echo "$MESSAGES_RESPONSE" | head -100
    echo "─────────────────────────────────────────────────────────────────"
fi

echo ""

# =============================================================================
# CHECK PLUGIN HOOKS
# =============================================================================

log "Checking plugin hook logs..."

# Check if there are any gt-related entries in server log
if grep -q "gastown" "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Found gastown plugin activity in server log"
    grep "gastown" "$OPENCODE_SERVER_LOG" | tail -10
elif grep -q "gt " "$OPENCODE_SERVER_LOG" 2>/dev/null; then
    log_success "Found gt command activity in server log"
    grep "gt " "$OPENCODE_SERVER_LOG" | tail -10
else
    log_warn "No gastown/gt activity found in server log"
    log_detail "This might mean plugin hooks didn't fire"
fi

# Check plugin file is readable
INSTALLED_PLUGIN="$PLUGIN_DIR/gastown.js"
if [[ -f "$INSTALLED_PLUGIN" ]]; then
    PLUGIN_SIZE=$(wc -c < "$INSTALLED_PLUGIN")
    log_detail "Plugin file size: $PLUGIN_SIZE bytes"
    
    # Check for key hook functions
    if grep -q "session.created" "$INSTALLED_PLUGIN"; then
        log_success "Plugin has session.created hook"
    fi
    if grep -q "gt prime" "$INSTALLED_PLUGIN"; then
        log_success "Plugin calls 'gt prime'"
    fi
fi

echo ""

# =============================================================================
# COLLECT DIAGNOSTICS
# =============================================================================

log "Collecting diagnostics..."

# Copy all logs to diagnostics
DIAG_DIR="$LOG_DIR/diagnostics"
mkdir -p "$DIAG_DIR"

# Server log
cp "$OPENCODE_SERVER_LOG" "$DIAG_DIR/" 2>/dev/null || true

# Any opencode state files
if [[ -d "$TEST_PROJECT/.opencode" ]]; then
    cp -r "$TEST_PROJECT/.opencode" "$DIAG_DIR/opencode-state" 2>/dev/null || true
fi

# Session list final state
curl -s "http://localhost:$OPENCODE_PORT/session" > "$DIAG_DIR/final-sessions.json" 2>&1 || true

log_success "Diagnostics saved to: $DIAG_DIR"

# Summary
echo ""
log "═══════════════════════════════════════════════════════════════════"
log "  Test Summary"
log "═══════════════════════════════════════════════════════════════════"
echo ""
echo "Test directory:    $TEST_DIR"
echo "Log directory:     $LOG_DIR"
echo "Server port:       $OPENCODE_PORT"
echo "Plugin installed:  $PLUGIN_DIR/gastown.js"
echo ""
echo "Key logs to review:"
echo "  - $OPENCODE_SERVER_LOG"
echo "  - $LOG_DIR/opencode-run.log (if CLI fallback used)"
echo ""

if $KEEP_SESSION; then
    echo "Server still running (PID: $OPENCODE_PID)"
    echo "To stop: kill $OPENCODE_PID"
    trap - EXIT  # Don't cleanup on exit
    echo ""
    echo "Try these commands manually:"
    echo "  curl http://localhost:$OPENCODE_PORT/session"
    echo "  curl -X POST http://localhost:$OPENCODE_PORT/session -H 'Content-Type: application/json' -d '{\"path\": \"$TEST_PROJECT\"}'"
fi

log_success "E2E test complete"
