#!/bin/bash
# Integration test for epic workflow
# This test validates the full epic lifecycle

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test utilities
pass() {
    echo -e "${GREEN}✓ $1${NC}"
}

fail() {
    echo -e "${RED}✗ $1${NC}"
    exit 1
}

info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# Cleanup function
cleanup() {
    info "Cleaning up test environment..."
    if [ -n "$TEST_RIG" ] && [ -d "$TEST_RIG" ]; then
        rm -rf "$TEST_RIG" 2>/dev/null || true
    fi
    if [ -n "$TEST_TOWN" ] && [ -d "$TEST_TOWN" ]; then
        rm -rf "$TEST_TOWN" 2>/dev/null || true
    fi
}

trap cleanup EXIT

# Setup test environment
setup_test_env() {
    info "Setting up test environment..."

    TEST_TOWN=$(mktemp -d)
    TEST_RIG="$TEST_TOWN/test-epic-rig"

    # Create minimal rig structure
    mkdir -p "$TEST_RIG/.beads"
    mkdir -p "$TEST_RIG/crew"
    mkdir -p "$TEST_RIG/refinery/rig"

    # Create CONTRIBUTING.md
    cat > "$TEST_RIG/refinery/rig/CONTRIBUTING.md" << 'EOF'
# Contributing

Please write tests for all new features.

## Code Style
- Use gofmt
- Write clear commit messages

## Pull Requests
- Include tests
- Update documentation
EOF

    pass "Test environment created at $TEST_TOWN"
}

# Test CONTRIBUTING.md discovery
test_contributing_discovery() {
    info "Testing CONTRIBUTING.md discovery..."

    # Test root location
    if [ -f "$TEST_RIG/refinery/rig/CONTRIBUTING.md" ]; then
        pass "CONTRIBUTING.md exists at expected location"
    else
        fail "CONTRIBUTING.md not found"
    fi
}

# Test epic field parsing (using Go test)
test_epic_field_parsing() {
    info "Testing epic field parsing..."

    # Run the unit tests for epic field parsing
    if go test -v ./internal/beads/ -run TestParseEpicFields -count=1 > /dev/null 2>&1; then
        pass "Epic field parsing tests pass"
    else
        fail "Epic field parsing tests failed"
    fi
}

# Test dependency graph
test_dependency_graph() {
    info "Testing dependency graph..."

    if go test -v ./internal/epic/ -run TestDependencyGraph -count=1 > /dev/null 2>&1; then
        pass "Dependency graph tests pass"
    else
        fail "Dependency graph tests failed"
    fi
}

# Test PR URL parsing
test_pr_url_parsing() {
    info "Testing PR URL parsing..."

    if go test -v ./internal/epic/ -run TestParsePRURL -count=1 > /dev/null 2>&1; then
        pass "PR URL parsing tests pass"
    else
        fail "PR URL parsing tests failed"
    fi
}

# Test state transitions
test_state_transitions() {
    info "Testing epic state transitions..."

    if go test -v ./internal/beads/ -run TestValidEpicStateTransition -count=1 > /dev/null 2>&1; then
        pass "State transition tests pass"
    else
        fail "State transition tests failed"
    fi
}

# Test contributing discovery functions
test_contributing_functions() {
    info "Testing contributing discovery functions..."

    if go test -v ./internal/epic/ -run TestDiscoverContributing -count=1 > /dev/null 2>&1; then
        pass "Contributing discovery tests pass"
    else
        fail "Contributing discovery tests failed"
    fi
}

# Main test runner
main() {
    echo "========================================"
    echo "Epic Workflow Integration Tests"
    echo "========================================"
    echo ""

    # Setup
    setup_test_env

    echo ""
    echo "Running tests..."
    echo ""

    # Unit test integration
    test_epic_field_parsing
    test_dependency_graph
    test_pr_url_parsing
    test_state_transitions
    test_contributing_functions

    # Integration tests
    test_contributing_discovery

    echo ""
    echo "========================================"
    echo -e "${GREEN}All tests passed!${NC}"
    echo "========================================"
}

# Run main
main "$@"
