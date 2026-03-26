#!/usr/bin/env bash
# Pre-release safety check. Called by release.yml before cutting a release tag.
# Runs: smoke, vuln scan, version sanity.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

echo "[release-check] smoke"
./scripts/ci/verify.sh smoke

echo "[release-check] vulnerability scan"
if ! command -v govulncheck >/dev/null 2>&1; then
  echo "[release-check] govulncheck not found — install with: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2
  exit 1
fi
govulncheck ./...

echo "[release-check] version sanity"
./gt version

echo "[release-check] passed"
