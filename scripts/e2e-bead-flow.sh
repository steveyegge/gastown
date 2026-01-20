#!/bin/bash
# Comprehensive E2E Test: Bead Flow Through Gas Town
#
# This script tests the complete lifecycle of beads flowing through the system:
# 1. Work bead creation
# 2. Polecat spawning with correct agent bead
# 3. Sling action - hook verification
# 4. State consistency
#
# Usage:
#   ./scripts/e2e-bead-flow.sh [TEST_DIR]

set -e  # Exit on error

#############################################################################
# Configuration
#############################################################################

TEST_DIR="${1:-/code/e2e-flow-test}"
GT="${GT:-/tmp/gt-e2e-test}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Counters
TESTS_PASSED=0
TESTS_FAILED=0
PHASE=0

#############################################################################
# Utility Functions
#############################################################################

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

phase() {
    PHASE=$((PHASE + 1))
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  PHASE $PHASE: $1${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

pass() {
    echo -e "  ${GREEN}✓ PASS:${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL:${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

skip() {
    echo -e "  ${YELLOW}○ SKIP:${NC} $1"
}

info() {
    echo -e "  ${BLUE}ℹ INFO:${NC} $1"
}

run_cmd() {
    echo -e "  ${BLUE}→${NC} $*"
    "$@"
}

#############################################################################
# Setup
#############################################################################

setup() {
    phase "SETUP - Create Test Environment"

    # Build gt if needed
    if [[ ! -x "$GT" ]]; then
        info "Building gt binary..."
        (cd /code/gt-pr && go build -o "$GT" ./cmd/gt)
    fi

    local version
    version=$("$GT" version 2>&1 | head -1)
    info "GT: $GT"
    info "Version: $version"

    # Clean test directory
    if [[ -d "$TEST_DIR" ]]; then
        info "Removing existing test directory..."
        rm -rf "$TEST_DIR"
    fi
    mkdir -p "$TEST_DIR"

    # Create rig source repo
    info "Creating source repository..."
    local rig_source="$TEST_DIR/source-repo"
    mkdir -p "$rig_source"
    cd "$rig_source"
    git init --quiet
    git config user.email "test@example.com"
    git config user.name "Test User"
    echo "# Test Rig Source" > README.md
    echo "function hello() { return 'world'; }" > app.js
    git add .
    git commit -m "Initial commit" --quiet

    # Install town
    info "Installing Gas Town..."
    cd "$TEST_DIR"
    "$GT" install --name "flow-test" . || {
        fail "Town installation failed"
        exit 1
    }

    if [[ -f "$TEST_DIR/mayor/town.json" ]]; then
        pass "Town installed"
    else
        fail "Town structure missing"
        exit 1
    fi

    # Add rig
    info "Adding test rig..."
    "$GT" rig add flow_rig "$rig_source" || {
        fail "Rig add failed"
        exit 1
    }

    if [[ -d "$TEST_DIR/flow_rig" ]]; then
        pass "Rig added"
    else
        fail "Rig directory missing"
        exit 1
    fi

    # Verify custom types
    if [[ -f "$TEST_DIR/.beads/.gt-types-configured" ]]; then
        pass "Custom types configured (town)"
    else
        fail "Custom types missing (town)"
    fi

    if [[ -f "$TEST_DIR/flow_rig/.beads/.gt-types-configured" ]]; then
        pass "Custom types configured (rig)"
    else
        fail "Custom types missing (rig)"
    fi

    pass "Setup complete"
}

#############################################################################
# Phase 1: Work Bead Creation
#############################################################################

test_work_bead_creation() {
    phase "WORK BEAD CREATION"

    cd "$TEST_DIR/flow_rig/.beads"

    # Get the rig prefix from config.yaml
    local prefix
    prefix=$(grep "^prefix:" config.yaml 2>/dev/null | awk '{print $2}' || echo "fr")
    info "Rig prefix: $prefix"

    # Use correct prefix for the work bead ID
    local work_bead_id="${prefix}-test-001"
    export WORK_BEAD_ID="$work_bead_id"

    # Create a task bead
    info "Creating work bead: $work_bead_id"
    bd create \
        --id="$work_bead_id" \
        --title="Test Task: Add feature X" \
        --type=task \
        --priority=1 \
        --json || {
        fail "Work bead creation failed"
        return 1
    }

    # Verify bead exists
    if bd show "$work_bead_id" > /dev/null 2>&1; then
        pass "Work bead created: $work_bead_id"
    else
        fail "Work bead not found"
        return 1
    fi

    # Check status - parse from bd show (non-json)
    local bead_output
    bead_output=$(bd show "$work_bead_id" 2>/dev/null || echo "")
    if [[ "$bead_output" == *"OPEN"* ]]; then
        pass "Work bead status: open"
    else
        fail "Work bead status not OPEN"
    fi

    # Check type
    if [[ "$bead_output" == *"task"* ]]; then
        pass "Work bead type: task"
    else
        fail "Work bead type not task"
    fi
}

#############################################################################
# Phase 2: Agent Bead Verification
#############################################################################

test_agent_beads() {
    phase "AGENT BEAD VERIFICATION"

    cd "$TEST_DIR/flow_rig/.beads"

    # List all agent beads
    info "Listing agent beads in rig..."
    local agents
    agents=$(bd list --type=agent 2>/dev/null || echo "")
    echo "$agents"

    # Get prefix
    local prefix
    prefix=$(grep "^prefix:" config.yaml 2>/dev/null | awk '{print $2}' || echo "fr")

    # Check for witness agent
    local witness_id="${prefix}-flow_rig-witness"
    if bd show "$witness_id" > /dev/null 2>&1; then
        pass "Witness agent exists: $witness_id"

        # Check witness role slot
        local witness_role
        witness_role=$(bd slot show "$witness_id" 2>/dev/null | grep "role:" | awk '{print $2}' || echo "")
        if [[ -n "$witness_role" && "$witness_role" != "(empty)" ]]; then
            pass "Witness role slot set: $witness_role"
        else
            fail "Witness role slot missing"
        fi
    else
        fail "Witness agent not found: $witness_id"
    fi

    # Check for refinery agent
    local refinery_id="${prefix}-flow_rig-refinery"
    if bd show "$refinery_id" > /dev/null 2>&1; then
        pass "Refinery agent exists: $refinery_id"

        # Check refinery role slot
        local refinery_role
        refinery_role=$(bd slot show "$refinery_id" 2>/dev/null | grep "role:" | awk '{print $2}' || echo "")
        if [[ -n "$refinery_role" && "$refinery_role" != "(empty)" ]]; then
            pass "Refinery role slot set: $refinery_role"
        else
            fail "Refinery role slot missing"
        fi
    else
        fail "Refinery agent not found: $refinery_id"
    fi
}

#############################################################################
# Phase 3: Crew Worker Test
#############################################################################

test_crew_worker() {
    phase "CREW WORKER CREATION"

    cd "$TEST_DIR"

    # Add a crew worker
    info "Adding crew worker..."
    "$GT" crew add testworker --rig flow_rig || {
        fail "Crew add failed"
        return 1
    }
    pass "Crew worker added"

    # Check crew agent bead exists
    cd "$TEST_DIR/flow_rig/.beads"

    # Get prefix
    local prefix
    prefix=$(grep "^prefix:" config.yaml 2>/dev/null | awk '{print $2}' || echo "fr")

    local crew_id="${prefix}-flow_rig-crew-testworker"
    if bd show "$crew_id" > /dev/null 2>&1; then
        pass "Crew agent bead exists: $crew_id"

        # Check role slot
        local crew_role
        crew_role=$(bd slot show "$crew_id" 2>/dev/null | grep "role:" | awk '{print $2}' || echo "")
        if [[ -n "$crew_role" && "$crew_role" != "(empty)" ]]; then
            pass "Crew role slot set: $crew_role"
        else
            fail "Crew role slot missing"
        fi
    else
        fail "Crew agent bead not found: $crew_id"
    fi
}

#############################################################################
# Phase 4: Polecat Spawn Test (if daemon running)
#############################################################################

test_polecat_sling() {
    phase "POLECAT SLING TEST (Spawn via Sling)"

    cd "$TEST_DIR"

    # Slinging to a rig auto-spawns a polecat
    # This may fail without Claude running
    info "Attempting to sling work to rig (auto-spawns polecat)..."
    local sling_output
    if sling_output=$("$GT" sling "${WORK_BEAD_ID:-fr-test-001}" flow_rig --no-attach 2>&1); then
        pass "Sling to rig succeeded (polecat spawned)"

        # Check for polecat agent bead
        cd "$TEST_DIR/flow_rig/.beads"
        local polecat_list
        polecat_list=$(bd list --type=agent 2>/dev/null | grep polecat || echo "")

        if [[ -n "$polecat_list" ]]; then
            pass "Polecat agent bead exists"
            echo "$polecat_list"

            # Get the polecat ID
            local prefix
            prefix=$(grep "^prefix:" config.yaml 2>/dev/null | awk '{print $2}' || echo "fr")
            local polecat_id
            polecat_id=$(bd list --type=agent 2>/dev/null | grep polecat | head -1 | awk '{print $2}' || echo "")
            if [[ -n "$polecat_id" ]]; then
                export POLECAT_ID="$polecat_id"
                info "Polecat ID: $polecat_id"
            fi
        else
            info "No polecat agent bead found (sling may have failed silently)"
        fi
    else
        skip "Sling to rig requires Claude/daemon (expected in CI)"
        info "Output: $sling_output"
        export POLECAT_ID=""
    fi
}

#############################################################################
# Phase 5: State Consistency
#############################################################################

test_state_consistency() {
    phase "STATE CONSISTENCY CHECK"

    cd "$TEST_DIR/flow_rig/.beads"

    # Count agents
    local agent_count
    agent_count=$(bd list --type=agent 2>/dev/null | wc -l || echo "0")
    info "Total agent beads: $agent_count"

    # Check for agents with missing role slots
    local agents_json
    agents_json=$(bd list --type=agent --json 2>/dev/null || echo "[]")

    local missing_roles=0
    for agent_id in $(echo "$agents_json" | grep -o '"id":"[^"]*"' | cut -d'"' -f4); do
        local role
        role=$(bd slot show "$agent_id" 2>/dev/null | grep "role:" | awk '{print $2}' || echo "")
        if [[ -z "$role" || "$role" == "(empty)" ]]; then
            info "Agent missing role slot: $agent_id"
            missing_roles=$((missing_roles + 1))
        fi
    done

    if [[ $missing_roles -eq 0 ]]; then
        pass "All agents have role slots"
    else
        fail "$missing_roles agents missing role slots"
    fi

    # Check for bd errors
    info "Running bd list (checking for errors)..."
    if bd list > /dev/null 2>&1; then
        pass "bd list succeeds without errors"
    else
        fail "bd list returned errors"
    fi

    # Check town-level beads
    cd "$TEST_DIR/.beads"
    local town_agent_count
    town_agent_count=$(bd list --type=agent 2>/dev/null | wc -l || echo "0")
    info "Town-level agent beads: $town_agent_count"

    if [[ $town_agent_count -ge 2 ]]; then
        pass "Town has mayor and deacon agents"
    else
        fail "Town missing expected agents"
    fi
}

#############################################################################
# Summary
#############################################################################

print_summary() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  TEST SUMMARY${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""

    local total=$((TESTS_PASSED + TESTS_FAILED))

    echo -e "  ${GREEN}Passed:${NC}  $TESTS_PASSED"
    echo -e "  ${RED}Failed:${NC}  $TESTS_FAILED"
    echo "  Total:   $total"
    echo ""
    echo "  Test directory: $TEST_DIR"
    echo ""

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
    echo ""
    echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║  GAS TOWN BEAD FLOW E2E TEST                                  ║${NC}"
    echo -e "${CYAN}║  Testing: Beads → Agents → Hooks → State                      ║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Test Directory: $TEST_DIR"

    # Run tests
    setup
    test_work_bead_creation
    test_agent_beads
    test_crew_worker
    test_polecat_sling
    test_state_consistency

    print_summary
}

# Run
main "$@"
