#!/usr/bin/env bash
# lib.sh — shared test helpers for E2E health test modules.
#
# Source this from each test module:
#   source "$(dirname "$0")/lib.sh"

set -euo pipefail

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
DIM='\033[2m'
NC='\033[0m'

# ── Counters ─────────────────────────────────────────────────────────
_PASSED=0
_FAILED=0
_SKIPPED=0
_TOTAL=0
_MODULE_NAME="${MODULE_NAME:-unknown}"

# ── Namespace ────────────────────────────────────────────────────────
# Namespace can come from:
#   1. E2E_NAMESPACE env var
#   2. First positional argument
#   3. Default: gastown-next
E2E_NAMESPACE="${E2E_NAMESPACE:-${1:-gastown-next}}"
export E2E_NAMESPACE

# ── Port forwarding state ────────────────────────────────────────────
_PF_PIDS=()

# ── Logging ──────────────────────────────────────────────────────────
log()  { echo -e "${BLUE}[$_MODULE_NAME]${NC} $*"; }
ok()   { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
skip() { echo -e "${YELLOW}[SKIP]${NC} $*"; }
dim()  { echo -e "${DIM}$*${NC}"; }

# ── Test runner ──────────────────────────────────────────────────────
# Usage: run_test "test name" command arg1 arg2 ...
run_test() {
  local name="$1"
  shift
  _TOTAL=$((_TOTAL + 1))

  if "$@" 2>/dev/null; then
    ok "$name"
    _PASSED=$((_PASSED + 1))
  else
    fail "$name"
    _FAILED=$((_FAILED + 1))
  fi
  # Always return 0 so set -e doesn't abort on first failure
  return 0
}

# Usage: skip_test "test name" "reason"
skip_test() {
  local name="$1"
  local reason="${2:-}"
  _TOTAL=$((_TOTAL + 1))
  _SKIPPED=$((_SKIPPED + 1))
  skip "$name${reason:+ ($reason)}"
}

# Usage: skip_all "reason" — mark entire module as skipped and print summary
skip_all() {
  local reason="${1:-}"
  skip "All tests in $_MODULE_NAME${reason:+: $reason}"
  print_summary
}

# ── Assertions ───────────────────────────────────────────────────────
assert_eq() {
  local actual="$1" expected="$2"
  [[ "$actual" == "$expected" ]]
}

assert_contains() {
  local haystack="$1" needle="$2"
  [[ "$haystack" == *"$needle"* ]]
}

assert_match() {
  local value="$1" pattern="$2"
  [[ "$value" =~ $pattern ]]
}

assert_gt() {
  local actual="$1" threshold="$2"
  [[ "$actual" -gt "$threshold" ]]
}

assert_ge() {
  local actual="$1" threshold="$2"
  [[ "$actual" -ge "$threshold" ]]
}

# ── kubectl helpers ──────────────────────────────────────────────────
kube() {
  kubectl -n "$E2E_NAMESPACE" "$@"
}

# Wait for a pod label selector to be ready. Returns 0 if ready, 1 if timeout.
wait_pod_ready() {
  local selector="$1"
  local timeout="${2:-60}"
  kubectl wait --for=condition=ready pod -l "$selector" \
    -n "$E2E_NAMESPACE" --timeout="${timeout}s" >/dev/null 2>&1
}

# Get pod name by label selector (first match)
get_pod() {
  local selector="$1"
  kube get pods -l "$selector" --no-headers -o custom-columns=":metadata.name" 2>/dev/null | head -1
}

# Get pod readiness as "ready/total" (e.g. "2/2")
get_pod_ready_status() {
  local pod="$1"
  kube get pod "$pod" --no-headers -o custom-columns=":status.containerStatuses[*].ready" 2>/dev/null
}

# ── Port-forward helpers ─────────────────────────────────────────────
# Start port-forward in background. Returns local port.
# Usage: local_port=$(start_port_forward svc/my-svc 8080)
start_port_forward() {
  local target="$1"
  local remote_port="$2"
  local local_port="${3:-0}"  # 0 = auto-assign

  if [[ "$local_port" == "0" ]]; then
    # Find a free port
    local_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()')
  fi

  kubectl port-forward -n "$E2E_NAMESPACE" "$target" "${local_port}:${remote_port}" >/dev/null 2>&1 &
  local pf_pid=$!
  _PF_PIDS+=("$pf_pid")

  # Wait for port-forward to be ready (TCP connection test — works for any protocol)
  local deadline=$((SECONDS + 15))
  while [[ $SECONDS -lt $deadline ]]; do
    if (echo "" | nc -w 1 127.0.0.1 "$local_port" >/dev/null 2>&1) || \
       (python3 -c "import socket; s=socket.socket(); s.settimeout(1); s.connect(('127.0.0.1',$local_port)); s.close()" 2>/dev/null); then
      break
    fi
    # Also check that the process is still alive
    if ! kill -0 "$pf_pid" 2>/dev/null; then
      return 1
    fi
    sleep 0.5
  done

  echo "$local_port"
}

# Stop all port-forwards started by this script.
stop_port_forwards() {
  for pid in "${_PF_PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  _PF_PIDS=()
}

# ── Summary ──────────────────────────────────────────────────────────
print_summary() {
  echo ""
  echo -e "${BLUE}━━━ $_MODULE_NAME summary ━━━${NC}"
  echo -e "  Total:   $_TOTAL"
  echo -e "  ${GREEN}Passed:  $_PASSED${NC}"
  if [[ $_FAILED -gt 0 ]]; then
    echo -e "  ${RED}Failed:  $_FAILED${NC}"
  fi
  if [[ $_SKIPPED -gt 0 ]]; then
    echo -e "  ${YELLOW}Skipped: $_SKIPPED${NC}"
  fi
  echo ""

  if [[ $_FAILED -gt 0 ]]; then
    return 1
  fi
  return 0
}

# ── Cleanup trap ─────────────────────────────────────────────────────
_cleanup() {
  stop_port_forwards
}
trap _cleanup EXIT
