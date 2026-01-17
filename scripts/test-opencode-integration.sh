#!/bin/bash
set -e

echo "OpenCode Integration Test Suite"
echo "================================"
echo ""

# Ensure OpenCode is installed
if ! command -v opencode &> /dev/null; then
    echo "❌ OpenCode not installed. Running setup..."
    bash scripts/setup-opencode.sh
fi

echo "✓ OpenCode version: $(opencode --version)"
echo ""

# Test 1: Basic session creation and resume
echo "Test 1: Basic Session Creation and Resume"
echo "----------------------------------------"
cd /tmp && mkdir -p opencode-integration && cd opencode-integration

# Create a simple session
SESSION_OUTPUT=$(timeout 20 opencode run --model opencode/gpt-5-nano "Create a file called test.txt with the text 'Hello from OpenCode'" 2>&1)
echo "$SESSION_OUTPUT"

# Check if file was created
if [ -f test.txt ]; then
    echo "✓ Test 1 PASSED: File created successfully"
    echo "  Content: $(cat test.txt)"
else
    echo "❌ Test 1 FAILED: File not created"
fi
echo ""

# Test 2: Session listing
echo "Test 2: Session Listing"
echo "----------------------"
SESSION_COUNT=$(opencode session list 2>&1 | tail -n +3 | wc -l)
echo "✓ Found $SESSION_COUNT sessions"
echo ""

# Test 3: Session export
echo "Test 3: Session Export"
echo "---------------------"
LATEST_SESSION=$(opencode session list 2>&1 | tail -n +3 | head -1 | awk '{print $1}')
if [ -n "$LATEST_SESSION" ]; then
    opencode export "$LATEST_SESSION" > /tmp/exported-session.json 2>&1
    if [ -f /tmp/exported-session.json ]; then
        SIZE=$(wc -c < /tmp/exported-session.json)
        echo "✓ Test 3 PASSED: Session exported ($SIZE bytes)"
    else
        echo "❌ Test 3 FAILED: Export failed"
    fi
else
    echo "⚠ Test 3 SKIPPED: No sessions found"
fi
echo ""

# Test 4: Plugin verification
echo "Test 4: Plugin Verification"
echo "--------------------------"
if [ -f ~/.config/opencode/opencode.jsonc ]; then
    echo "✓ OpenCode config exists"
    cat ~/.config/opencode/opencode.jsonc
else
    echo "❌ OpenCode config missing"
fi
echo ""

# Test 5: Model availability
echo "Test 5: Model Availability"
echo "-------------------------"
MODEL_COUNT=$(opencode models 2>&1 | wc -l)
echo "✓ Found $MODEL_COUNT models available"
echo ""

echo "Integration Test Summary"
echo "======================="
echo "All basic tests completed"
