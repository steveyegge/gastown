#!/bin/bash
set -e

# Configuration
TEST_CASE=${1:-"TestGastown_CreateFile"}
RUNTIME=${2:-"opencode"}
TIMEOUT=${3:-"120s"}

echo "=== Running E2E Test: $TEST_CASE ($RUNTIME) ==="
echo "Timeout: $TIMEOUT"

# Verify dependencies
if [ "$RUNTIME" == "opencode" ]; then
    opencode --version >/dev/null || { echo "Error: opencode not found"; exit 1; }
    # Verify model availability
    opencode run --model google/antigravity-gemini-3-flash "Reply: READY" >/dev/null || { echo "Error: opencode model verification failed"; exit 1; }
fi

# Clean test cache to ensure recompilation
go clean -testcache

# Run the test
# We use unbuffered output via 'go test -v'
cd "$(dirname "$0")/.."
export E2E_TIMEOUT=$TIMEOUT
export E2E_OPENCODE_MODEL="google/antigravity-gemini-3-flash"
export E2E_CLAUDE_MODEL="haiku"

# Run specific test case
go test -tags=e2e -v -run "${TEST_CASE}/${RUNTIME}" ./internal/e2e/... | tee /tmp/e2e_run.log
