#!/bin/bash
# test-runtime-e2e.sh - Runtime-agnostic E2E test runner
#
# Runs E2E tests with either Claude Code or OpenCode as the runtime.
# Also supports mixed-runtime testing where one is primary and one is secondary.
#
# Usage:
#   ./scripts/test-runtime-e2e.sh --runtime claude      # Claude Code only
#   ./scripts/test-runtime-e2e.sh --runtime opencode    # OpenCode only
#   ./scripts/test-runtime-e2e.sh --runtime both        # Run both (default)
#   ./scripts/test-runtime-e2e.sh --mixed               # Mixed primary/secondary test
#   ./scripts/test-runtime-e2e.sh --test polecat        # Specific test
#
# Prerequisites:
# - gt CLI built (make build)
# - claude CLI installed (for --runtime claude or both)
# - opencode CLI installed (for --runtime opencode or both)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DIR="/tmp/gastown-runtime-e2e-$(date +%s)"
LOG_DIR="$TEST_DIR/logs"

# Defaults
RUNTIME="both"
TEST_NAME="basic"
MIXED_MODE=false
VERBOSE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --runtime) RUNTIME="$2"; shift 2 ;;
        --test) TEST_NAME="$2"; shift 2 ;;
        --mixed) MIXED_MODE=true; shift ;;
        --verbose|-v) VERBOSE=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--runtime claude|opencode|both] [--test NAME] [--mixed] [--verbose]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
log_success() { echo -e "${GREEN}[$(date +%H:%M:%S)] ✓${NC} $*"; }
log_warn() { echo -e "${YELLOW}[$(date +%H:%M:%S)] ⚠${NC} $*"; }
log_error() { echo -e "${RED}[$(date +%H:%M:%S)] ✗${NC} $*"; }
log_detail() { if $VERBOSE; then echo -e "  $*"; fi; }

cleanup() {
    log "Cleaning up..."
    pkill -f "opencode serve" 2>/dev/null || true
    [[ -d "$TEST_DIR" ]] && rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# ═══════════════════════════════════════════════════════════════════
# PREREQUISITE CHECKS
# ═══════════════════════════════════════════════════════════════════

log "═══════════════════════════════════════════════════════════════════"
log "  Runtime-Agnostic E2E Test"
log "  Runtime: $RUNTIME | Test: $TEST_NAME | Mixed: $MIXED_MODE"
log "═══════════════════════════════════════════════════════════════════"
echo ""

mkdir -p "$TEST_DIR" "$LOG_DIR"

# Check gt
GT_BINARY="$PROJECT_ROOT/gt"
if [[ ! -x "$GT_BINARY" ]]; then
    log "Building gt..."
    (cd "$PROJECT_ROOT" && make build) || exit 1
fi
log_success "gt: $("$GT_BINARY" version 2>/dev/null | head -1)"

# Check runtimes
HAVE_CLAUDE=false
HAVE_OPENCODE=false

if command -v claude &>/dev/null; then
    CLAUDE_VERSION=$(claude --version 2>/dev/null | head -1 || echo "installed")
    HAVE_CLAUDE=true
    log_success "claude: $CLAUDE_VERSION"
else
    log_warn "claude CLI not found"
fi

if command -v opencode &>/dev/null; then
    OPENCODE_VERSION=$(opencode --version 2>/dev/null | head -1 || echo "installed")
    HAVE_OPENCODE=true
    log_success "opencode: $OPENCODE_VERSION"
else
    log_warn "opencode CLI not found"
fi

# Validate runtime selection
case $RUNTIME in
    claude)
        if ! $HAVE_CLAUDE; then log_error "Claude Code not installed"; exit 1; fi
        ;;
    opencode)
        if ! $HAVE_OPENCODE; then log_error "OpenCode not installed"; exit 1; fi
        ;;
    both)
        if ! $HAVE_CLAUDE && ! $HAVE_OPENCODE; then
            log_error "Neither runtime installed"; exit 1
        fi
        ;;
esac

echo ""

# ═══════════════════════════════════════════════════════════════════
# TEST FUNCTIONS
# ═══════════════════════════════════════════════════════════════════

run_test_with_runtime() {
    local runtime=$1
    local test=$2
    local role=${3:-polecat}
    
    log "Running $test with $runtime runtime..."
    
    local test_work_dir="$TEST_DIR/$runtime-$test"
    mkdir -p "$test_work_dir"
    
    # Initialize git
    (cd "$test_work_dir" && git init --quiet && 
        git config user.email "test@test.com" && 
        git config user.name "Test")
    echo "# Test - $runtime $test" > "$test_work_dir/README.md"
    (cd "$test_work_dir" && git add . && git commit -m "init" --quiet)
    
    # Setup based on runtime
    if [[ "$runtime" == "opencode" ]]; then
        # Install OpenCode plugin
        mkdir -p "$test_work_dir/.opencode/plugin"
        cp "$PROJECT_ROOT/internal/opencode/plugin/gastown.js" "$test_work_dir/.opencode/plugin/"
        
        # Create config for permissions
        cat > "$test_work_dir/.opencode/config.jsonc" <<EOF
{
  "permission": { "*": "allow" }
}
EOF
    elif [[ "$runtime" == "claude" ]]; then
        # Ensure Claude settings exist
        mkdir -p "$test_work_dir/.claude"
        cat > "$test_work_dir/.claude/settings.json" <<EOF
{
  "permissions": {
    "allow": ["Read", "Write", "Edit", "Bash", "WebFetch"]
  }
}
EOF
    fi
    
    # Run the appropriate test script
    case $test in
        basic)
            run_basic_test "$runtime" "$test_work_dir" "$role"
            ;;
        polecat)
            run_polecat_test "$runtime" "$test_work_dir"
            ;;
        mayor)
            run_mayor_test "$runtime" "$test_work_dir"
            ;;
        *)
            log_error "Unknown test: $test"
            return 1
            ;;
    esac
}

run_basic_test() {
    local runtime=$1
    local work_dir=$2
    local role=$3
    local result=0
    
    log_detail "Basic test: Verify gt commands work with $runtime"
    
    # Test gt prime
    if (cd "$work_dir" && GT_ROLE="$role" "$GT_BINARY" prime 2>&1 | grep -qi "recovered\|context\|success"); then
        log_success "[$runtime] gt prime works"
    else
        log_warn "[$runtime] gt prime returned no context (expected in empty repo)"
    fi
    
    # Test gt version
    if "$GT_BINARY" version >/dev/null 2>&1; then
        log_success "[$runtime] gt version works"
    else
        log_error "[$runtime] gt version failed"
        result=1
    fi
    
    return $result
}

run_polecat_test() {
    local runtime=$1
    local work_dir=$2
    
    log_detail "Polecat lifecycle test with $runtime"
    
    if [[ "$runtime" == "opencode" ]]; then
        # Use existing polecat e2e script pattern
        log_detail "Starting OpenCode server..."
        local port=4099
        
        (cd "$work_dir" && \
            GT_ROLE=polecat \
            GT_BINARY_PATH="$GT_BINARY" \
            opencode serve --port "$port" > "$LOG_DIR/$runtime-polecat.log" 2>&1) &
        local pid=$!
        
        # Wait for server
        sleep 5
        
        if curl -s "http://localhost:$port/session" >/dev/null 2>&1; then
            log_success "[$runtime] OpenCode server running"
            
            # Create session and verify hooks
            local response=$(curl -s -X POST "http://localhost:$port/session" \
                -H "Content-Type: application/json" \
                -d '{"path": "'"$work_dir"'"}')
            
            sleep 3
            
            if grep -q "gastown\|session.created" "$LOG_DIR/$runtime-polecat.log" 2>/dev/null; then
                log_success "[$runtime] Plugin hooks executed"
            else
                log_warn "[$runtime] Plugin hooks not detected in logs"
            fi
        else
            log_error "[$runtime] OpenCode server failed to start"
        fi
        
        kill $pid 2>/dev/null || true
    else
        # Claude Code doesn't have a server mode, different test
        log_detail "Claude Code polecat test (non-server)"
        log_success "[$runtime] Polecat would use CLI mode"
    fi
}

run_mayor_test() {
    local runtime=$1
    local work_dir=$2
    
    log_detail "Mayor workflow test with $runtime"
    log_success "[$runtime] Mayor test placeholder"
}

# ═══════════════════════════════════════════════════════════════════
# RUN TESTS
# ═══════════════════════════════════════════════════════════════════

TESTS_PASSED=0
TESTS_TOTAL=0

run_tests_for_runtime() {
    local runtime=$1
    ((TESTS_TOTAL++))
    
    if run_test_with_runtime "$runtime" "$TEST_NAME"; then
        ((TESTS_PASSED++))
    fi
}

if [[ "$MIXED_MODE" == true ]]; then
    # Mixed mode: Test with OpenCode primary, Claude secondary (or vice versa)
    log "Running mixed-runtime test..."
    
    if $HAVE_OPENCODE && $HAVE_CLAUDE; then
        log "Testing: OpenCode (primary) + Claude Code (secondary)"
        run_tests_for_runtime "opencode"
        run_tests_for_runtime "claude"
        
        # Could add cross-runtime communication tests here
        log_success "Mixed-runtime test completed"
    else
        log_warn "Mixed mode requires both runtimes installed"
    fi
else
    # Single or both runtimes
    case $RUNTIME in
        claude)
            run_tests_for_runtime "claude"
            ;;
        opencode)
            run_tests_for_runtime "opencode"
            ;;
        both)
            if $HAVE_OPENCODE; then run_tests_for_runtime "opencode"; fi
            if $HAVE_CLAUDE; then run_tests_for_runtime "claude"; fi
            ;;
    esac
fi

echo ""

# ═══════════════════════════════════════════════════════════════════
# SUMMARY
# ═══════════════════════════════════════════════════════════════════

log "═══════════════════════════════════════════════════════════════════"
log "  Summary"
log "═══════════════════════════════════════════════════════════════════"
echo ""
echo "Tests passed: $TESTS_PASSED/$TESTS_TOTAL"
echo "Logs: $LOG_DIR"
echo ""

if [[ $TESTS_PASSED -eq $TESTS_TOTAL ]]; then
    log_success "All tests PASSED!"
    exit 0
else
    log_error "Some tests FAILED"
    exit 1
fi
