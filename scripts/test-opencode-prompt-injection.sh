#!/bin/bash
# Test script to isolate opencode prompt injection methods
# Goal: Find a working way to inject a prompt into opencode programmatically

set -e
cd "$(dirname "$0")/.."
PROJECT_ROOT=$(pwd)

echo "=== OpenCode Prompt Injection Test ==="
echo "Project Root: $PROJECT_ROOT"
echo ""

# Setup test environment
TEST_DIR=$(mktemp -d)
echo "Test Dir: $TEST_DIR"
cd "$TEST_DIR"

# Initialize a minimal git repo
git init --initial-branch=main
git config user.email "test@test.com"
git config user.name "Test"
echo "module testproject" > go.mod
git add go.mod
git commit -m "init"

echo ""
echo "=== Method 1: opencode run (non-interactive) ==="
echo "This uses opencode's built-in non-interactive mode"

# opencode run should work for simple tasks
timeout 60 opencode run --model google/antigravity-gemini-3-flash \
  "Create a file called hello.txt with the text 'Hello World'" 2>&1 | head -50 || true

if [ -f hello.txt ]; then
    echo "✓ Method 1 PASSED: File created"
    cat hello.txt
else
    echo "✗ Method 1 FAILED: File not created"
fi

echo ""
echo "=== Method 2: opencode run with JSON output ==="
rm -f hello.txt

timeout 60 opencode run --model google/antigravity-gemini-3-flash \
  --json \
  "Create a file called hello2.txt with the text 'Hello JSON'" 2>&1 | head -100 || true

if [ -f hello2.txt ]; then
    echo "✓ Method 2 PASSED: File created"
    cat hello2.txt
else
    echo "✗ Method 2 FAILED: File not created"
fi

echo ""
echo "=== Method 3: opencode with stdin prompt ==="
rm -f hello.txt hello2.txt

echo "Create a file called hello3.txt with the text 'Hello Stdin'" | \
timeout 60 opencode run --model google/antigravity-gemini-3-flash 2>&1 | head -50 || true

if [ -f hello3.txt ]; then
    echo "✓ Method 3 PASSED: File created"
    cat hello3.txt
else
    echo "✗ Method 3 FAILED: File not created"
fi

echo ""
echo "=== Cleanup ==="
cd /
rm -rf "$TEST_DIR"
echo "Done"
