#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

MODE="${1:-pre-merge}"

check_no_replace_directives() {
  if grep -qE '^replace\s' go.mod; then
    echo "[verify] go.mod must not contain replace directives" >&2
    grep -nE '^replace\s' go.mod >&2 || true
    exit 1
  fi
}

check_no_issues_jsonl() {
  if [[ -f .beads/issues.jsonl ]]; then
    echo "[verify] .beads/issues.jsonl must not exist in the repository" >&2
    exit 1
  fi
}

build_gt() {
  echo "[verify] build"
  make build
}

run_unit_tests() {
  echo "[verify] unit tests"
  go test -race -short -timeout=10m ./...
}

run_lint() {
  echo "[verify] lint"
  if ! command -v golangci-lint >/dev/null 2>&1; then
    echo "[verify] golangci-lint is required for lint mode" >&2
    exit 1
  fi
  golangci-lint run --timeout=5m
}

run_integration_tests() {
  echo "[verify] integration tests"
  if command -v gotestsum >/dev/null 2>&1; then
    gotestsum --format testname -- -tags=integration -timeout=15m -v ./internal/cmd/...
    return
  fi
  go test -tags=integration -timeout=15m -v ./internal/cmd/...
}

case "$MODE" in
  pre-merge|full)
    check_no_replace_directives
    check_no_issues_jsonl
    build_gt
    run_unit_tests
    run_lint
    ;;
  smoke)
    check_no_replace_directives
    check_no_issues_jsonl
    build_gt
    ;;
  integration)
    run_integration_tests
    ;;
  *)
    echo "usage: $0 [pre-merge|smoke|integration|full]" >&2
    exit 2
    ;;
esac
