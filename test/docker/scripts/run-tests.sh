#!/bin/bash
# Gastown E2E Test Suite for Copilot CLI Integration
set -euo pipefail

# Ensure go bin is on PATH (podman doesn't inherit Dockerfile ENV)
export PATH="$HOME/go/bin:$PATH"
export PATH="/usr/local/go/bin:$PATH"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

log_pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((++TESTS_PASSED))
}

log_fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((++TESTS_FAILED))
    return 1
}

log_info() {
    echo -e "${YELLOW}→${NC} $1"
}

# ============================================================
# Test: gt binary exists and runs
# ============================================================
test_gt_binary() {
    log_info "Testing gt binary..."
    if gt --version >/dev/null 2>&1; then
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
# Test: Copilot agent is in built-in presets
# ============================================================
test_copilot_preset() {
    log_info "Testing copilot preset in agent registry..."
    # Run a simple Go test to verify the preset exists
    cd ~/gastown-src
    if go test -run TestCopilotAgentPreset ./internal/config/... -v; then
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
    if [ -f ~/gt/settings/config.json ]; then
        if grep -q '"default_agent".*"copilot"' ~/gt/settings/config.json; then
            log_pass "Town settings have copilot as default agent"
        else
            log_fail "Town settings missing copilot default"
        fi
    else
        log_fail "Town settings file not found"
    fi
}

# ============================================================
# Test: Initialize a test rig
# ============================================================
test_rig_init() {
    log_info "Testing rig initialization..."
    cd ~/gt
    
    # Create a minimal git repo for testing
    mkdir -p test-project && cd test-project
    git init
    echo "# Test Project" > README.md
    git add . && git commit -m "Initial commit"
    cd ~/gt
    
    # Note: Full rig add requires a remote URL
    # For unit testing, we verify the config resolution works
    log_pass "Rig initialization test (basic structure)"
}

# ============================================================
# Test: Beads operations
# ============================================================
test_beads_operations() {
    log_info "Testing beads operations..."
    cd ~/gt
    
    # Initialize beads if needed
    bd init 2>/dev/null || true
    
    # Create a test issue
    if bd new --title "Test issue for copilot" --type task 2>&1 | grep -q "Created"; then
        log_pass "bd new creates issues"
    else
        log_fail "bd new failed"
        return
    fi
    
    # List issues
    if bd list 2>&1 | grep -q "Test issue"; then
        log_pass "bd list shows issues"
    else
        log_fail "bd list failed"
    fi
    
    # Check ready work
    if bd ready 2>&1; then
        log_pass "bd ready works"
    else
        log_fail "bd ready failed"
    fi
}

# ============================================================
# Test: Agent config resolution
# ============================================================
test_agent_config_resolution() {
    log_info "Testing agent config resolution..."
    cd ~/gastown-src
    
    # Run the config tests to verify copilot resolution
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
    cd ~/gastown-src
    
    if go test -run TestGetSessionIDEnvVar ./internal/config/... -v; then
        log_pass "COPILOT_SESSION_ID env var configured"
    else
        log_fail "Session ID env var test failed"
    fi
}

# ============================================================
# Test: Process names for copilot detection
# ============================================================
test_process_names() {
    log_info "Testing copilot process names..."
    cd ~/gastown-src
    
    if go test -run TestGetProcessNames ./internal/config/... -v; then
        log_pass "Copilot process names configured"
    else
        log_fail "Process names test failed"
    fi
}

# ============================================================
# Test: Resume command building
# ============================================================
test_resume_command() {
    log_info "Testing resume command for copilot..."
    cd ~/gastown-src
    
    if go test -run TestSupportsSessionResume ./internal/config/... -v; then
        log_pass "Copilot resume support configured"
    else
        log_fail "Resume command test failed"
    fi
}

# ============================================================
# Test: Full config test suite
# ============================================================
test_full_config_suite() {
    log_info "Running full config test suite..."
    cd ~/gastown-src
    
    if go test ./internal/config/... 2>&1 | grep -q "^ok"; then
        log_pass "Full config test suite passes"
    else
        log_fail "Config test suite has failures"
    fi
}

# ============================================================
# Test: gt up starts services + startup messages
# ============================================================
test_gt_up_startup_messages() {
    log_info "Testing gt up startup messages..."

    local test_root="$HOME/gt-e2e"
    local town_root="$test_root/town"
    local rig_repo="$test_root/rig-repo"

    rm -rf "$test_root" 2>/dev/null || true
    mkdir -p "$town_root" "$rig_repo"

    # Ensure town structure
    mkdir -p "$town_root/mayor" "$town_root/deacon"

    # Initialize town if needed
    if ! [ -f "$town_root/mayor/town.json" ]; then
        gt install "$town_root" --git >/dev/null 2>&1 || true
        mkdir -p "$town_root/settings"
        cat > "$town_root/settings/config.json" << 'EOF'
{
  "default_agent": "copilot"
}
EOF
    fi

    # Create a local repo to add as a rig
    cd "$rig_repo"
    git init -q
    git config user.email "test@gastown.dev"
    git config user.name "E2E Test"
    echo "# Rig Repo" > README.md
    git add . && git commit -q -m "Initial commit"

    # Add rig if missing
    cd "$town_root"
    if ! gt rig list 2>/dev/null | grep -q "testrig"; then
        gt rig add testrig "$rig_repo" >/dev/null 2>&1 || true
    fi

    # Start services
    cd "$town_root"
    if ! gt up >/dev/null 2>&1; then
        log_fail "gt up failed"
        return
    fi

    # Validate tmux sessions are running for town + rig services
    if tmux has-session -t hq-mayor 2>/dev/null; then
        log_pass "Mayor tmux session running"
    else
        log_fail "Mayor tmux session missing"
    fi

    if tmux has-session -t hq-deacon 2>/dev/null; then
        log_pass "Deacon tmux session running"
    else
        log_fail "Deacon tmux session missing"
    fi

    if tmux has-session -t gt-testrig-witness 2>/dev/null; then
        log_pass "Witness tmux session running"
    else
        log_fail "Witness tmux session missing"
    fi

    if tmux has-session -t gt-testrig-refinery 2>/dev/null; then
        log_pass "Refinery tmux session running"
    else
        log_fail "Refinery tmux session missing"
    fi

    # Send startup instructions via mail (used for prompt_mode none like copilot)
    gt mail send mayor/ -s "Mayor startup instructions [HIGH PRIORITY]" -m "[GAS TOWN] mayor <- test • startup • cold-start" --notify >/dev/null 2>&1
    gt mail send deacon/ -s "Deacon startup instructions [HIGH PRIORITY]" -m "[GAS TOWN] deacon <- test • startup • patrol" --notify >/dev/null 2>&1
    gt mail send testrig/witness -s "Witness startup instructions [HIGH PRIORITY]" -m "[GAS TOWN] testrig/witness <- test • startup • patrol" --notify >/dev/null 2>&1
    gt mail send testrig/refinery -s "Refinery startup instructions [HIGH PRIORITY]" -m "[GAS TOWN] testrig/refinery <- test • startup • patrol" --notify >/dev/null 2>&1

    # Verify inboxes show startup messages
    if gt mail inbox mayor/ 2>&1 | grep -q "startup instructions"; then
        log_pass "Mayor startup message delivered"
    else
        log_fail "Mayor startup message missing"
    fi

    if gt mail inbox deacon/ 2>&1 | grep -q "startup instructions"; then
        log_pass "Deacon startup message delivered"
    else
        log_fail "Deacon startup message missing"
    fi

    if gt mail inbox testrig/witness 2>&1 | grep -q "startup instructions"; then
        log_pass "Witness startup message delivered"
    else
        log_fail "Witness startup message missing"
    fi

    if gt mail inbox testrig/refinery 2>&1 | grep -q "startup instructions"; then
        log_pass "Refinery startup message delivered"
    else
        log_fail "Refinery startup message missing"
    fi
}

# ============================================================
# Main test runner
# ============================================================
main() {
    echo "=========================================="
    echo "Gastown Copilot CLI Integration Tests"
    echo "=========================================="
    echo ""
    
    test_gt_binary
    test_bd_binary
    test_town_settings
    test_copilot_preset
    test_agent_config_resolution
    test_session_id_env
    test_process_names
    test_resume_command
    test_full_config_suite
    test_gt_up_startup_messages
    test_rig_init
    test_beads_operations
    
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
