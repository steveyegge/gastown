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

run_guard_checks() {
  check_no_replace_directives
  check_no_issues_jsonl
}

build_gt() {
  echo "[verify] build"
  make build
}

run_unit_tests() {
  echo "[verify] unit tests"
  local args=("-race" "-short" "-timeout=10m")
  if [[ -n "${GASTOWN_VERIFY_COVERPROFILE:-}" ]]; then
    args+=("-coverprofile=${GASTOWN_VERIFY_COVERPROFILE}")
  fi
  if command -v gotestsum >/dev/null 2>&1 && [[ -n "${GASTOWN_VERIFY_JUNIT_FILE:-}" ]]; then
    gotestsum --format testname --junitfile "${GASTOWN_VERIFY_JUNIT_FILE}" -- "${args[@]}" ./...
    return
  fi
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
  local args=("-tags=integration" "-timeout=20m" "-v" "./...")
  if command -v gotestsum >/dev/null 2>&1 && [[ -n "${GASTOWN_VERIFY_JUNIT_FILE:-}" ]]; then
    gotestsum --format testname --junitfile "${GASTOWN_VERIFY_JUNIT_FILE}" -- "${args[@]}"
    return
  fi
  go test -tags=integration -timeout=20m -v ./...
}

run_failure_tests() {
  echo "[verify] failure-mode tests"
  local args=("-tags=failure" "-timeout=10m" "-v" "./...")
  if command -v gotestsum >/dev/null 2>&1 && [[ -n "${GASTOWN_VERIFY_JUNIT_FILE:-}" ]]; then
    gotestsum --format testname --junitfile "${GASTOWN_VERIFY_JUNIT_FILE}" -- "${args[@]}"
    return
  fi
  go test -tags=failure -timeout=10m -v ./...
}

run_fuzz_tests() {
  echo "[verify] fuzz tests (30s)"
  # Run all fuzz targets for a short duration to catch obvious panics.
  # Corpus is not persisted here; use 'go test -fuzz=FuzzXxx -fuzztime=Xs' locally for deep runs.
  go test -fuzz=. -fuzztime=30s ./... 2>&1 || true
}

run_coverage_check() {
  echo "[verify] coverage check"
  go test -coverprofile=coverage.out ./...
  local pct
  pct=$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,""); print $3}')
  local floor="${GASTOWN_COVERAGE_FLOOR:-60}"
  if awk "BEGIN{exit ($pct+0 >= $floor+0)}"; then
    echo "[verify] coverage ${pct}% is below ${floor}% floor" >&2
    exit 1
  fi
  echo "[verify] coverage ${pct}% (floor: ${floor}%)"
}

case "$MODE" in
  guard)
    run_guard_checks
    ;;
  guard-replace)
    check_no_replace_directives
    ;;
  guard-issues-jsonl)
    check_no_issues_jsonl
    ;;
  build)
    build_gt
    ;;
  unit)
    run_unit_tests
    ;;
  lint)
    run_lint
    ;;
  pre-merge|full)
    run_guard_checks
    build_gt
    run_unit_tests
    run_lint
    ;;
  smoke)
    run_guard_checks
    build_gt
    ;;
  integration)
    run_integration_tests
    ;;
  failure)
    run_failure_tests
    ;;
  fuzz)
    run_fuzz_tests
    ;;
  coverage)
    run_coverage_check
    ;;
  *)
    echo "usage: $0 [guard|guard-replace|guard-issues-jsonl|build|unit|lint|pre-merge|smoke|integration|failure|fuzz|coverage|full]" >&2
    exit 2
    ;;
esac
