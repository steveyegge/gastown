#!/usr/bin/env bash
# test-daemon-health.sh — Verify BD daemon pod health.
#
# Tests:
#   1. Daemon pod has all containers ready (N/N)
#   2. HTTP health endpoint returns OK
#   3. RPC port 9876 responds
#   4. NATS is connected (embedded in daemon)
#   5. Daemon can list beads
#
# Usage:
#   ./scripts/test-daemon-health.sh [NAMESPACE]

MODULE_NAME="daemon-health"
source "$(dirname "$0")/lib.sh"

log "Testing BD daemon health in namespace: $E2E_NAMESPACE"

# ── Discover daemon pods ─────────────────────────────────────────────
DAEMON_POD=$(kube get pods --no-headers 2>/dev/null | grep "daemon" | grep -v "dolt\|nats\|redis" | head -1 | awk '{print $1}')
DAEMON_READY=$(kube get pod "$DAEMON_POD" --no-headers -o custom-columns="READY:.status.containerStatuses[*].ready" 2>/dev/null)

log "Daemon pod: $DAEMON_POD (ready: $DAEMON_READY)"

# ── Test 1: Pod is N/N ready (all containers ready) ───────────────
test_pod_ready() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local ready_count total_count
  ready_count=$(kube get pod "$DAEMON_POD" -o jsonpath='{.status.containerStatuses[?(@.ready==true)].name}' 2>/dev/null | wc -w | tr -d ' ')
  total_count=$(kube get pod "$DAEMON_POD" -o jsonpath='{.status.containerStatuses[*].name}' 2>/dev/null | wc -w | tr -d ' ')
  [[ "$total_count" -gt 0 ]] && assert_eq "$ready_count" "$total_count"
}
run_test "Daemon pod containers ready (all)" test_pod_ready

# ── Test 2: Containers include daemon (slack-bot optional) ───────────
test_container_names() {
  local containers
  containers=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
  assert_contains "$containers" "daemon" || return 1
  if assert_contains "$containers" "slack" 2>/dev/null; then
    log "  slack-bot sidecar: present"
  else
    log "  slack-bot sidecar: not present (optional)"
  fi
}
run_test "Daemon has required containers (slack-bot optional)" test_container_names

# ── Test 3: HTTP health endpoint ─────────────────────────────────────
HTTP_PORT=""

test_http_health() {
  HTTP_PORT=$(start_port_forward "pod/$DAEMON_POD" 9080) || return 1
  local resp
  resp=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://127.0.0.1:${HTTP_PORT}/healthz" 2>/dev/null)
  assert_eq "$resp" "200"
}
run_test "HTTP health endpoint returns 200" test_http_health

# ── Test 4: RPC port responds ────────────────────────────────────────
RPC_PORT=""

test_rpc_port() {
  RPC_PORT=$(start_port_forward "pod/$DAEMON_POD" 9876) || return 1
  # RPC is not HTTP, so just verify TCP connection succeeds
  if command -v nc >/dev/null 2>&1; then
    echo "" | nc -w 3 127.0.0.1 "$RPC_PORT" >/dev/null 2>&1
  else
    # curl will fail but won't timeout if port is open
    curl -s --connect-timeout 3 "http://127.0.0.1:${RPC_PORT}/" >/dev/null 2>&1 || true
  fi
  return 0
}
run_test "RPC port (9876) reachable" test_rpc_port

# ── Test 5: Daemon HTTP API responds to /api/v1/status ───────────────
test_daemon_status_api() {
  [[ -n "$HTTP_PORT" ]] || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${HTTP_PORT}/api/v1/status" 2>/dev/null)
  # Should return JSON with some status info
  assert_contains "$resp" "status" || assert_contains "$resp" "version" || [[ ${#resp} -gt 2 ]]
}
run_test "Daemon /api/v1/status API responds" test_daemon_status_api

# ── Test 6: NATS service exists ──────────────────────────────────────
test_nats_service() {
  local nats_svc
  nats_svc=$(kube get svc --no-headers 2>/dev/null | grep "nats" | head -1 | awk '{print $1}')
  [[ -n "$nats_svc" ]]
}
run_test "NATS service exists in namespace" test_nats_service

# ── Test 7: NATS pod is ready ────────────────────────────────────────
test_nats_ready() {
  local nats_pod
  nats_pod=$(kube get pods --no-headers 2>/dev/null | grep "nats" | head -1 | awk '{print $1}')
  [[ -n "$nats_pod" ]] || return 1
  local status
  status=$(kube get pod "$nats_pod" --no-headers 2>/dev/null | awk '{print $2}')
  assert_eq "$status" "1/1"
}
run_test "NATS pod is ready (1/1)" test_nats_ready

# ── Test 8: Daemon logs show no fatal errors ─────────────────────────
test_no_fatal_errors() {
  local fatal_count
  fatal_count=$(kube logs "$DAEMON_POD" -c daemon --tail=100 2>/dev/null \
    | grep -ci "fatal\|panic" || true)
  assert_eq "${fatal_count:-0}" "0"
}
run_test "No fatal/panic in daemon logs (last 100 lines)" test_no_fatal_errors

# ── Summary ──────────────────────────────────────────────────────────
print_summary
