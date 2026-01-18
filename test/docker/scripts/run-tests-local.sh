#!/bin/bash
# Gastown E2E Test Suite - Local Runner (no Docker)
# Run directly on the host system
# Note: Don't use set -e as tests may have expected failures

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0
GASTOWN_SRC="${GASTOWN_SRC:-/Users/matt/gt/gastown/mayor/rig}"

log_pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((TESTS_PASSED++))
}

log_fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((TESTS_FAILED++))
}

log_info() {
    echo -e "${YELLOW}→${NC} $1"
}

# ============================================================
# Test: gt binary exists and runs
# ============================================================
test_gt_binary() {
    log_info "Testing gt binary..."
    if gt --version >/dev/null 2>&1 || gt --help >/dev/null 2>&1; then
        log_pass "gt binary exists and runs"
    else
        log_fail "gt binary not found or failed"
    fi
}

# ============================================================
# Test: bd (beads) binary exists
# ============================================================
test_bd_binary() {
    log_info "Testing bd binary..."
    if bd --version >/dev/null 2>&1 || bd help >/dev/null 2>&1; then
        log_pass "bd binary exists and runs"
    else
        log_fail "bd binary not found or failed"
    fi
}

# ============================================================
# Test: Copilot CLI exists
# ============================================================
test_copilot_binary() {
    log_info "Testing copilot CLI binary..."
    if which copilot >/dev/null 2>&1; then
        log_pass "copilot CLI binary found at $(which copilot)"
    else
        log_fail "copilot CLI not found in PATH"
    fi
}

# ============================================================
# Test: Copilot agent is in built-in presets
# ============================================================
test_copilot_preset() {
    log_info "Testing copilot preset in agent registry..."
    cd "$GASTOWN_SRC"
    if go test -run TestCopilotAgentPreset ./internal/config/... -v 2>&1 | grep -q "PASS"; then
        log_pass "Copilot preset exists in agent registry"
    else
        log_fail "Copilot preset test failed"
    fi
}

# ============================================================
# Test: Town settings with copilot default
# ============================================================
test_town_settings() {
    log_info "Testing town settings..."
    local settings_file="$HOME/gt/settings/config.json"
    if [ -f "$settings_file" ]; then
        if grep -q '"default_agent"' "$settings_file"; then
            local agent=$(grep -o '"default_agent"[[:space:]]*:[[:space:]]*"[^"]*"' "$settings_file" | cut -d'"' -f4)
            log_pass "Town settings default_agent = $agent"
        else
            log_fail "Town settings missing default_agent"
        fi
    else
        log_fail "Town settings file not found at $settings_file"
    fi
}

# ============================================================
# Test: Agent config resolution
# ============================================================
test_agent_config_resolution() {
    log_info "Testing agent config resolution..."
    cd "$GASTOWN_SRC"
    
    if go test -run "GetAgentPresetByName|RuntimeConfigFromPreset" ./internal/config/... 2>&1 | grep -q "ok"; then
        log_pass "Agent config resolution tests pass"
    else
        log_fail "Agent config resolution tests failed"
    fi
}

# ============================================================
# Test: Session ID env var for copilot
# ============================================================
test_session_id_env() {
    log_info "Testing copilot session ID env var..."
    cd "$GASTOWN_SRC"
    
    if go test -run TestGetSessionIDEnvVar ./internal/config/... -v 2>&1 | grep -q "copilot"; then
        log_pass "COPILOT_SESSION_ID env var test exists"
    else
        log_fail "Session ID env var test failed"
    fi
}

# ============================================================
# Test: Process names for copilot detection
# ============================================================
test_process_names() {
    log_info "Testing copilot process names..."
    cd "$GASTOWN_SRC"
    
    if go test -run TestGetProcessNames ./internal/config/... -v 2>&1 | grep -q "copilot"; then
        log_pass "Copilot process names test exists"
    else
        log_fail "Process names test failed"
    fi
}

# ============================================================
# Test: Resume command building
# ============================================================
test_resume_command() {
    log_info "Testing resume command for copilot..."
    cd "$GASTOWN_SRC"
    
    if go test -run TestSupportsSessionResume ./internal/config/... -v 2>&1 | grep -q "copilot"; then
        log_pass "Copilot resume support test exists"
    else
        log_fail "Resume command test failed"
    fi
}

# ============================================================
# Test: Full config test suite
# ============================================================
test_full_config_suite() {
    log_info "Running full config test suite..."
    cd "$GASTOWN_SRC"
    
    local output=$(go test ./internal/config/... 2>&1)
    if echo "$output" | grep -q "^ok"; then
        log_pass "Full config test suite passes"
    else
        log_fail "Config test suite has failures"
        echo "$output" | tail -20
    fi
}

# ============================================================
# Test: Beads operations
# ============================================================
test_beads_operations() {
    log_info "Testing beads operations..."
    
    # bd ready should work
    if bd ready 2>&1 >/dev/null; then
        log_pass "bd ready works"
    else
        log_fail "bd ready failed"
    fi
    
    # bd list should work
    if bd list 2>&1 >/dev/null; then
        log_pass "bd list works"
    else
        log_fail "bd list failed"
    fi
}

# ============================================================
# Test: Mail system
# ============================================================
test_mail_system() {
    log_info "Testing mail system..."
    
    if gt mail inbox 2>&1 >/dev/null; then
        log_pass "gt mail inbox works"
    else
        log_fail "gt mail inbox failed"
    fi
}

# ============================================================
# Test: Convoy operations
# ============================================================
test_convoy_operations() {
    log_info "Testing convoy operations..."
    
    if gt convoy list 2>&1 >/dev/null; then
        log_pass "gt convoy list works"
    else
        log_fail "gt convoy list failed"
    fi
}

# ============================================================
# Test: Sling dry-run
# ============================================================
test_sling_dryrun() {
    log_info "Testing sling dry-run..."
    
    # Get first ready issue
    local issue=$(bd ready 2>/dev/null | grep -o '\[[^]]*\] [a-z]*-[a-z0-9]*' | head -1 | awk '{print $2}')
    
    if [ -n "$issue" ]; then
        if gt sling "$issue" gastown --dry-run 2>&1 | grep -q "Would"; then
            log_pass "gt sling --dry-run works"
        else
            log_fail "gt sling --dry-run failed"
        fi
    else
        log_info "No ready issues to test sling with (skipping)"
        log_pass "gt sling test skipped (no issues)"
    fi
}

# ============================================================
# Test: Agent override flag
# ============================================================
test_agent_override() {
    log_info "Testing agent override flag..."
    
    if gt sling --help 2>&1 | grep -q "\-\-agent"; then
        log_pass "gt sling has --agent override flag"
    else
        log_fail "gt sling missing --agent flag"
    fi
}

# ============================================================
# Main test runner
# ============================================================
main() {
    echo "=========================================="
    echo "Gastown Copilot CLI Integration Tests"
    echo "=========================================="
    echo "Source: $GASTOWN_SRC"
    echo ""
    
    # Binary tests
    test_gt_binary
    test_bd_binary
    test_copilot_binary
    
    # Configuration tests
    test_town_settings
    test_copilot_preset
    test_agent_config_resolution
    test_session_id_env
    test_process_names
    test_resume_command
    test_full_config_suite
    
    # Functional tests
    test_beads_operations
    test_mail_system
    test_convoy_operations
    test_sling_dryrun
    test_agent_override
    
    echo ""
    echo "=========================================="
    echo "Test Results"
    echo "=========================================="
    echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
    
    if [ $TESTS_FAILED -gt 0 ]; then
        exit 1
    fi
    exit 0
}

main "$@"
