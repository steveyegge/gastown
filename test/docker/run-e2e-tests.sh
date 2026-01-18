#!/bin/bash
# Build and run the Gastown E2E test container
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Build the Docker image
echo "Building gastown-e2e-test image..."
docker build \
    --build-arg GASTOWN_REPO=https://github.com/mzkoch/gastown.git \
    --build-arg GASTOWN_BRANCH=feature/copilot-native-support \
    -t gastown-e2e-test \
    .

# Run the tests
echo ""
echo "Running E2E tests..."
docker run --rm \
    -e GITHUB_TOKEN="${GITHUB_TOKEN:-}" \
    gastown-e2e-test

echo ""
echo "E2E tests completed!"
