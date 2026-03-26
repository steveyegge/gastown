#!/usr/bin/env bash
# Mutation testing on changed packages. Verifies that tests catch logic errors,
# not just that tests pass. Run as a Refinery post-squash gate for high-priority MRs.
#
# Requires: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
# Exit 0 = mutation score acceptable; exit 1 = score too low or tool missing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

if ! command -v gremlins >/dev/null 2>&1; then
  echo "[mutate] gremlins not found — install with: go install github.com/go-gremlins/gremlins/cmd/gremlins@latest" >&2
  exit 1
fi

# Find Go packages changed relative to main (non-test files only).
CHANGED=$(git diff --name-only origin/main...HEAD 2>/dev/null \
  | grep '\.go$' \
  | grep -v '_test\.go' \
  | xargs -I{} dirname {} 2>/dev/null \
  | sort -u \
  | grep -v '^\.git' || true)

if [[ -z "$CHANGED" ]]; then
  echo "[mutate] no non-test Go files changed — skipping"
  exit 0
fi

echo "[mutate] running mutation tests on changed packages:"
echo "$CHANGED" | sed 's/^/  /'

FAILED=0
while IFS= read -r pkg; do
  echo "[mutate] package: ./$pkg"
  if ! gremlins unleash "./$pkg"; then
    echo "[mutate] FAILED: mutation score too low in ./$pkg" >&2
    FAILED=1
  fi
done <<< "$CHANGED"

if [[ $FAILED -ne 0 ]]; then
  echo "[mutate] one or more packages have insufficient mutation coverage" >&2
  exit 1
fi

echo "[mutate] passed"
