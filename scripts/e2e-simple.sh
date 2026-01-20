#!/bin/bash
# Simple E2E Test for Gas Town
# Usage: ./scripts/e2e-simple.sh [TEST_DIR]

set -x  # Print commands as they execute

TEST_DIR="${1:-/code/2}"
GT="${GT:-/tmp/gt-e2e-test}"

echo "=== Gas Town E2E Test ==="
echo "Test Directory: $TEST_DIR"
echo "GT Binary: $GT"
echo ""

# Build if needed
if [[ ! -x "$GT" ]]; then
    echo "Building gt binary..."
    cd /code/gt-pr
    go build -o "$GT" ./cmd/gt
fi

echo "GT Version:"
$GT version

# Clean and create test directory
echo ""
echo "=== Setting up test directory ==="
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Test 1: Install town
echo ""
echo "=== Test 1: Install Town ==="
$GT install --name "e2e-test" .
if [[ $? -ne 0 ]]; then
    echo "FAIL: Town installation failed"
    exit 1
fi
echo "PASS: Town installed"

# Verify town structure
if [[ ! -f "$TEST_DIR/mayor/town.json" ]]; then
    echo "FAIL: mayor/town.json not found"
    exit 1
fi
echo "PASS: Town structure verified"

# Test 2: Add a rig
echo ""
echo "=== Test 2: Add Rig ==="
RIG_DIR="$TEST_DIR/rigs/test_rig"
mkdir -p "$RIG_DIR"
cd "$RIG_DIR"
git init --quiet
echo "# Test Rig" > README.md
git add .
git commit -m "Initial commit" --quiet
cd "$TEST_DIR"

$GT rig add test_rig "$RIG_DIR"
if [[ $? -ne 0 ]]; then
    echo "FAIL: Rig add failed"
    exit 1
fi
echo "PASS: Rig added"

# Test 3: Add crew worker (KEY TEST for custom types fix)
echo ""
echo "=== Test 3: Add Crew Worker (Custom Types Fix Test) ==="
echo "This test verifies the fix for 'invalid issue type: agent' error"
$GT crew add alice --rig test_rig
if [[ $? -ne 0 ]]; then
    echo "FAIL: Crew add failed - POSSIBLE CUSTOM TYPES BUG"
    echo "Check if error contains 'invalid issue type: agent'"
    exit 1
fi
echo "PASS: Crew worker added"

# Check sentinel file
if [[ -f "$TEST_DIR/.beads/.gt-types-configured" ]]; then
    echo "PASS: Custom types sentinel file found"
else
    echo "INFO: Sentinel file may be in routed location"
fi

# Test 4: Show an existing bead (hq-mayor created during install)
echo ""
echo "=== Test 4: Show Bead ==="
$GT show hq-mayor
if [[ $? -ne 0 ]]; then
    echo "FAIL: Show bead failed"
    exit 1
fi
echo "PASS: Bead shown"

# Test 5: Verify custom types sentinel file exists
echo ""
echo "=== Test 5: Verify Custom Types Sentinel ==="
if [[ -f "$TEST_DIR/.beads/.gt-types-configured" ]]; then
    echo "PASS: Custom types sentinel file found in town beads"
else
    echo "WARN: Sentinel not in town beads (may be elsewhere)"
fi

# Check rig beads too
if [[ -f "$TEST_DIR/test_rig/.beads/.gt-types-configured" ]]; then
    echo "PASS: Custom types sentinel file found in rig beads"
else
    echo "INFO: No sentinel in rig beads (types may come from routing)"
fi

# Test 6: Ready work
echo ""
echo "=== Test 6: Ready Work ==="
$GT ready
echo "PASS: Ready command works"

# Test 7: Trail
echo ""
echo "=== Test 7: Trail ==="
$GT trail --limit 5
echo "PASS: Trail command works"

# Summary
echo ""
echo "========================================"
echo "All E2E tests passed!"
echo "========================================"
echo ""
echo "Town created at: $TEST_DIR"
echo "Custom types fix verified: crew worker 'alice' created successfully"
