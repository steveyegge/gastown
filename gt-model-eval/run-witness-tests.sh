#!/bin/bash
# Script to run witness patrol tests for pi-kimi agent

set -e

echo "Running witness patrol tests for pi-kimi agent..."

# Run only the witness test files
npx promptfoo eval \
  --provider pi-kimi \
  --tests tests/witness-stuck.yaml \
  --tests tests/witness-cleanup.yaml \
  --tests tests/class-a-witness.yaml \
  --output local/pi-kimi-patrol-results/witness-results.json \
  --output local/pi-kimi-patrol-results/witness-results.html

echo "Witness tests completed!"
echo "Results saved to local/pi-kimi-patrol-results/"