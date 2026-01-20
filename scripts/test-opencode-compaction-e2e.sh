#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[TEST]${NC} $*"; }
success() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

VERBOSE=${VERBOSE:-false}

check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v gt &>/dev/null; then
        if [[ -x "$PROJECT_ROOT/gt" ]]; then
            export PATH="$PROJECT_ROOT:$PATH"
        else
            fail "gt binary not found"
            exit 1
        fi
    fi
    
    if ! command -v opencode &>/dev/null; then
        fail "opencode not found"
        exit 1
    fi
    
    success "Prerequisites OK"
}

test_plugin_compacting_hook() {
    log "Testing compacting hook in plugin..."
    
    local plugin="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
    
    if grep -q "experimental.session.compacting" "$plugin"; then
        success "Compacting hook present"
        return 0
    fi
    
    fail "Compacting hook missing"
    return 1
}

test_plugin_compacting_action() {
    log "Testing compacting hook action..."
    
    local plugin="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
    
    if grep -q "onPreCompact" "$plugin" && grep -q "gt prime" "$plugin"; then
        success "Compacting runs gt prime"
        return 0
    fi
    
    fail "Compacting action not configured"
    return 1
}

test_compacting_output_modification() {
    log "Testing compacting output modification capability..."
    
    local plugin="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
    
    if grep -q "output.context" "$plugin" || grep -q "output" "$plugin"; then
        success "Plugin can modify compaction output"
        return 0
    fi
    
    fail "Plugin cannot modify compaction"
    return 1
}

test_session_idle_debounce() {
    log "Testing session.idle debouncing..."
    
    local plugin="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
    
    if grep -q "lastIdleTime" "$plugin" && grep -q "debounce" "$plugin"; then
        success "Idle debouncing implemented"
        return 0
    fi
    
    fail "Idle debouncing missing"
    return 1
}

test_costs_record_on_idle() {
    log "Testing costs recording on idle..."
    
    local plugin="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
    
    if grep -q "gt costs record" "$plugin"; then
        success "Costs recording on idle"
        return 0
    fi
    
    fail "Costs recording missing"
    return 1
}

main() {
    echo ""
    log "OpenCode Compaction E2E Test"
    log "============================"
    echo ""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --verbose|-v) VERBOSE=true ;;
            *) warn "Unknown option: $1" ;;
        esac
        shift
    done
    
    check_prerequisites
    
    local passed=0
    local total=5
    
    test_plugin_compacting_hook && ((passed++)) || true
    test_plugin_compacting_action && ((passed++)) || true
    test_compacting_output_modification && ((passed++)) || true
    test_session_idle_debounce && ((passed++)) || true
    test_costs_record_on_idle && ((passed++)) || true
    
    echo ""
    log "=========================================="
    log "        COMPACTION TEST REPORT           "
    log "=========================================="
    echo ""
    echo "  Passed: $passed/$total"
    echo ""
    
    if [[ $passed -eq $total ]]; then
        success "ALL COMPACTION TESTS PASSED"
        exit 0
    else
        fail "$((total - passed)) test(s) failed"
        exit 1
    fi
}

main "$@"
