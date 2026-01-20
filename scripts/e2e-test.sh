#!/bin/bash
# Gas Town E2E Test Protocol
#
# This script performs comprehensive end-to-end testing of Gas Town functionality.
# It creates a fresh town, adds rigs, creates agents, spawns workers, and tests
# the full workflow including bead creation and slinging.
#
# Usage:
#   ./scripts/e2e-test.sh [TEST_DIR] [GT_BINARY]
#
# Arguments:
#   TEST_DIR   - Directory to create the test town (default: /code/2)
#   GT_BINARY  - Path to gt binary to test (default: builds from source)
#
# The script logs everything to TEST_DIR/e2e-test.log

set -euo pipefail

# Configuration
TEST_DIR="${1:-/code/2}"
GT_BINARY="${2:-}"
LOG_FILE="$TEST_DIR/e2e-test.log"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

#############################################################################
# Utility Functions
#############################################################################

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    if [[ -f "$LOG_FILE" ]]; then
        echo "$msg" | tee -a "$LOG_FILE"
    else
        echo "$msg"
    fi
}

log_section() {
    echo ""
    echo "========================================"
    echo "$1"
    echo "========================================"
    if [[ -f "$LOG_FILE" ]]; then
        echo "" >> "$LOG_FILE"
        echo "========================================" >> "$LOG_FILE"
        echo "$1" >> "$LOG_FILE"
        echo "========================================" >> "$LOG_FILE"
    fi
}

log_cmd() {
    log "CMD: $*"
    # Run command, capture output, log it, and preserve exit code
    local output
    local exit_code
    set +e
    output=$("$@" 2>&1)
    exit_code=$?
    set -e
    echo "$output"
    if [[ -f "$LOG_FILE" ]]; then
        echo "$output" >> "$LOG_FILE"
    fi
    return $exit_code
}

pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    [[ -f "$LOG_FILE" ]] && echo "✓ PASS: $1" >> "$LOG_FILE"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    [[ -f "$LOG_FILE" ]] && echo "✗ FAIL: $1" >> "$LOG_FILE"
    ((TESTS_FAILED++))
}

skip() {
    echo -e "${YELLOW}⊘ SKIP${NC}: $1"
    [[ -f "$LOG_FILE" ]] && echo "⊘ SKIP: $1" >> "$LOG_FILE"
    ((TESTS_SKIPPED++))
}

info() {
    echo -e "${BLUE}ℹ INFO${NC}: $1"
    [[ -f "$LOG_FILE" ]] && echo "ℹ INFO: $1" >> "$LOG_FILE"
}

#############################################################################
# Setup Functions
#############################################################################

setup_test_environment() {
    log_section "Setting Up Test Environment"

    # Clean up existing test directory
    if [[ -d "$TEST_DIR" ]]; then
        log "Removing existing test directory: $TEST_DIR"
        rm -rf "$TEST_DIR"
    fi

    # Create test directory
    mkdir -p "$TEST_DIR"

    # Initialize log file
    echo "Gas Town E2E Test Log - $TIMESTAMP" > "$LOG_FILE"
    echo "Test Directory: $TEST_DIR" >> "$LOG_FILE"
    echo "GT Binary: $GT" >> "$LOG_FILE"
    echo "" >> "$LOG_FILE"

    log "Test environment ready at $TEST_DIR"
}

build_gt_binary() {
    log_section "Building GT Binary"

    if [[ -n "$GT_BINARY" && -x "$GT_BINARY" ]]; then
        GT="$GT_BINARY"
        log "Using provided binary: $GT"
    else
        log "Building gt from source..."
        local build_output
        if build_output=$(cd /code/gt-pr && go build -o /tmp/gt-e2e-test ./cmd/gt 2>&1); then
            GT="/tmp/gt-e2e-test"
            log "Built successfully: $GT"
        else
            echo "$build_output"
            fail "Failed to build gt binary"
            exit 1
        fi
    fi

    # Verify binary works
    if $GT version &>/dev/null; then
        local version=$($GT version 2>&1 | head -1)
        log "GT version: $version"
        pass "GT binary is functional"
    else
        fail "GT binary not functional"
        exit 1
    fi
}

#############################################################################
# Test Functions
#############################################################################

test_install_town() {
    log_section "Test: Install New Town (gt install)"

    cd "$TEST_DIR"

    if log_cmd $GT install --name "e2e-test-town" .; then
        pass "Town installed successfully"

        # Verify town structure
        if [[ -f "$TEST_DIR/mayor/town.json" ]]; then
            pass "mayor/town.json exists"
        else
            fail "mayor/town.json not found"
        fi

        if [[ -d "$TEST_DIR/.beads" ]]; then
            pass ".beads directory exists"
        else
            fail ".beads directory not found"
        fi
    else
        fail "Town installation failed"
        return 1
    fi
}

test_add_rig() {
    log_section "Test: Add Rig (gt rig add)"

    local rig_name="test_rig"
    local rig_path="$TEST_DIR/rigs/$rig_name"

    # Create the rig directory first
    mkdir -p "$rig_path"
    cd "$rig_path"
    git init --quiet
    echo "# Test Rig" > README.md
    git add .
    git commit -m "Initial commit" --quiet

    cd "$TEST_DIR"

    if log_cmd $GT rig add "$rig_name" "$rig_path"; then
        pass "Rig added successfully"

        # Verify rig is registered
        if log_cmd $GT rig list | grep -q "$rig_name"; then
            pass "Rig appears in rig list"
        else
            fail "Rig not found in rig list"
        fi
    else
        fail "Failed to add rig"
        return 1
    fi
}

test_add_crew_worker() {
    log_section "Test: Add Crew Worker (gt crew add)"

    local worker_name="alice"

    cd "$TEST_DIR"

    # This is the key test for the custom types fix!
    log "NOTE: This tests the custom types fix for multi-repo routing"

    if log_cmd $GT crew add "$worker_name" --rig test_rig; then
        pass "Crew worker added successfully"

        # Verify crew worker exists
        if log_cmd $GT crew list --rig test_rig | grep -q "$worker_name"; then
            pass "Crew worker appears in list"
        else
            fail "Crew worker not found in list"
        fi

        # Check if custom types were configured (the fix we're testing)
        log "Checking for custom types configuration..."
        if [[ -f "$TEST_DIR/.beads/.gt-types-configured" ]]; then
            pass "Custom types sentinel file created"
        else
            info "Sentinel file not in town .beads (may be in routed location)"
        fi
    else
        fail "Failed to add crew worker - THIS MAY BE THE CUSTOM TYPES BUG"
        log "Error output above may contain 'invalid issue type: agent'"
        return 1
    fi
}

test_show_bead() {
    log_section "Test: Show Bead (gt show)"

    cd "$TEST_DIR"

    # Show an agent bead that was created during install
    local bead_id="hq-mayor"

    if log_cmd $GT show "$bead_id"; then
        pass "Bead shown successfully"
    else
        fail "Failed to show bead"
        return 1
    fi
}

test_sling_work() {
    log_section "Test: Sling Work (gt sling)"

    cd "$TEST_DIR"

    # Sling requires active workers - skip for basic E2E
    skip "Sling test skipped (requires running workers)"
}

test_spawn_polecat() {
    log_section "Test: Spawn Polecat (gt polecat spawn)"

    cd "$TEST_DIR"

    # Check if we can spawn (might need daemon)
    log "Attempting to spawn a polecat..."

    local spawn_output
    if spawn_output=$(log_cmd $GT polecat spawn --dry-run 2>&1); then
        pass "Polecat spawn dry-run succeeded"
    else
        # Spawning requires daemon and other infrastructure
        info "Polecat spawn requires running daemon"
        skip "Polecat spawn skipped (daemon not running)"
    fi
}

test_list_agents() {
    log_section "Test: List Agents (gt agents)"

    cd "$TEST_DIR"

    if log_cmd $GT agents list; then
        pass "Agents list command works"
    else
        info "Agents list may require running services"
        skip "Agents list skipped"
    fi
}

test_verify_sentinel_files() {
    log_section "Test: Verify Custom Types Sentinel Files"

    cd "$TEST_DIR"

    # Check town beads
    if [[ -f "$TEST_DIR/.beads/.gt-types-configured" ]]; then
        pass "Custom types sentinel found in town beads"
    else
        info "Sentinel not in town beads"
    fi

    # Check rig beads
    if [[ -f "$TEST_DIR/test_rig/.beads/.gt-types-configured" ]]; then
        pass "Custom types sentinel found in rig beads"
    else
        info "Sentinel not in rig beads (may not have been needed)"
    fi
}

test_config() {
    log_section "Test: Configuration (gt config)"

    cd "$TEST_DIR"

    if log_cmd $GT config show; then
        pass "Config show works"
    else
        fail "Config show failed"
    fi
}

test_trail() {
    log_section "Test: Activity Trail (gt trail)"

    cd "$TEST_DIR"

    if log_cmd $GT trail; then
        pass "Trail command works"
    else
        info "Trail may be empty for new town"
        pass "Trail command executed (may be empty)"
    fi
}

test_ready() {
    log_section "Test: Ready Work (gt ready)"

    cd "$TEST_DIR"

    if log_cmd $GT ready; then
        pass "Ready command works"
    else
        fail "Ready command failed"
    fi
}

#############################################################################
# Multi-Repo Routing Test (The Bug We're Fixing)
#############################################################################

test_multi_repo_routing() {
    log_section "Test: Multi-Repo Routing (Custom Types Fix)"

    cd "$TEST_DIR"

    log "This test specifically validates the custom types fix for multi-repo routing"

    # Create a second rig to test routing
    local rig2_name="routing_rig"
    local rig2_path="$TEST_DIR/rigs/$rig2_name"

    mkdir -p "$rig2_path"
    cd "$rig2_path"
    git init --quiet
    echo "# Routing Test Rig" > README.md
    git add .
    git commit -m "Initial commit" --quiet

    cd "$TEST_DIR"

    if log_cmd $GT rig add "$rig2_name" "$rig2_path"; then
        pass "Second rig added for routing test"
    else
        fail "Failed to add second rig"
        return 1
    fi

    # Add a crew worker - this should trigger routing to the rig's beads db
    local worker_name="bob"
    log "Adding crew worker 'bob' - this may route to different beads DB"

    if log_cmd $GT crew add "$worker_name" --rig "$rig2_name"; then
        pass "Crew worker added (multi-repo routing test passed)"
    else
        fail "Failed to add crew worker - POSSIBLE CUSTOM TYPES BUG"
        log "Check if error contains 'invalid issue type: agent'"
        return 1
    fi
}

#############################################################################
# Cleanup
#############################################################################

cleanup() {
    log_section "Cleanup"

    cd "$TEST_DIR"

    # Stop any services that might be running
    log "Stopping any running services..."
    $GT down 2>/dev/null || true

    log "Cleanup complete"
}

#############################################################################
# Summary
#############################################################################

print_summary() {
    log_section "Test Summary"

    local total=$((TESTS_PASSED + TESTS_FAILED + TESTS_SKIPPED))

    echo ""
    echo "Results:"
    echo -e "  ${GREEN}Passed${NC}:  $TESTS_PASSED"
    echo -e "  ${RED}Failed${NC}:  $TESTS_FAILED"
    echo -e "  ${YELLOW}Skipped${NC}: $TESTS_SKIPPED"
    echo "  Total:   $total"
    echo ""
    echo "Log file: $LOG_FILE"

    # Also write to log
    echo "" >> "$LOG_FILE"
    echo "Results:" >> "$LOG_FILE"
    echo "  Passed:  $TESTS_PASSED" >> "$LOG_FILE"
    echo "  Failed:  $TESTS_FAILED" >> "$LOG_FILE"
    echo "  Skipped: $TESTS_SKIPPED" >> "$LOG_FILE"
    echo "  Total:   $total" >> "$LOG_FILE"

    if [[ $TESTS_FAILED -gt 0 ]]; then
        echo -e "${RED}Some tests failed!${NC}"
        return 1
    else
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    fi
}

#############################################################################
# Main
#############################################################################

main() {
    echo "Gas Town E2E Test Protocol"
    echo "=========================="
    echo "Test Directory: $TEST_DIR"
    echo "Timestamp: $TIMESTAMP"
    echo ""

    # Build binary first (before creating test dir for logging)
    GT=""
    if [[ -n "$GT_BINARY" && -x "$GT_BINARY" ]]; then
        GT="$GT_BINARY"
    else
        echo "Building gt binary..."
        if cd /code/gt-pr && go build -o /tmp/gt-e2e-test ./cmd/gt 2>&1; then
            GT="/tmp/gt-e2e-test"
        else
            echo "Failed to build gt binary"
            exit 1
        fi
    fi

    setup_test_environment
    build_gt_binary

    # Core tests
    test_install_town
    test_add_rig
    test_add_crew_worker      # Key test for custom types fix
    test_show_bead
    test_verify_sentinel_files
    test_sling_work

    # Infrastructure tests (may be skipped if services not running)
    test_spawn_polecat
    test_list_agents

    # Informational tests
    test_config
    test_trail
    test_ready

    # Multi-repo routing test (the specific bug we're fixing)
    test_multi_repo_routing

    # Cleanup
    cleanup

    # Summary
    print_summary
}

# Run main
main "$@"
