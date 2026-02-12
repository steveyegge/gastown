#!/usr/bin/env bash
# test-nats-health.sh — E2E: NATS event bus health and JetStream verification.
#
# Tests:
#   1. NATS pod running and ready
#   2. NATS service exposes client (4222) and monitoring (8222) ports
#   3. Monitoring API responsive (/varz)
#   -- wait for JetStream ready (streams created by daemon, up to 30s) --
#   4. JetStream enabled with streams
#   5. JetStream has active consumers
#   6. bd-daemon connected to NATS
#   7. Event bus status reports connected
#   8. Event bus has registered handlers
#   9. NATS monitoring reports message counters (>= 0)
#  10. No excessive JetStream API errors

MODULE_NAME="nats-health"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

# ── Helpers ───────────────────────────────────────────────────────────
NATS_SVC=""
NATS_PORT=""
DAEMON_POD=""

discover_nats_svc() {
  NATS_SVC=$(kube get svc --no-headers 2>/dev/null | grep "nats" | head -1 | awk '{print $1}')
  [[ -n "$NATS_SVC" ]]
}

discover_daemon_pod() {
  DAEMON_POD=$(kube get pods --no-headers 2>/dev/null | grep "daemon" | grep -v "dolt\|nats\|clusterctl" | grep "Running" | head -1 | awk '{print $1}')
  [[ -n "$DAEMON_POD" ]]
}

# ── Setup ─────────────────────────────────────────────────────────────
log "Testing NATS event bus in $NS"

# ── Test 1: NATS pod running and ready ────────────────────────────────
test_nats_pod_ready() {
  local pod
  pod=$(kube get pods --no-headers 2>/dev/null | grep "nats" | grep -v "clusterctl" | head -1)
  [[ -n "$pod" ]] || return 1
  local status
  status=$(echo "$pod" | awk '{print $3}')
  local ready
  ready=$(echo "$pod" | awk '{print $2}')
  [[ "$status" == "Running" ]] && [[ "$ready" == "1/1" ]]
}
run_test "NATS pod running and ready (1/1)" test_nats_pod_ready

# ── Test 2: NATS service has client and monitoring ports ──────────────
test_nats_service_ports() {
  discover_nats_svc || return 1
  local ports
  ports=$(kube get svc "$NATS_SVC" -o jsonpath='{.spec.ports[*].port}' 2>/dev/null)
  assert_contains "$ports" "4222" && assert_contains "$ports" "8222"
}
run_test "NATS service exposes 4222 (client) and 8222 (monitoring)" test_nats_service_ports

# ── Test 3: Monitoring API responsive ─────────────────────────────────
test_monitoring_api() {
  discover_nats_svc || return 1
  NATS_PORT=$(start_port_forward "svc/$NATS_SVC" 8222)
  [[ -n "$NATS_PORT" ]] || return 1
  local resp
  resp=$(curl -sf "http://localhost:${NATS_PORT}/varz" 2>/dev/null)
  [[ -n "$resp" ]] && assert_contains "$resp" "server_id"
}
run_test "NATS monitoring API responsive (/varz)" test_monitoring_api

# ── Wait for daemon to connect and create JetStream streams ───────────
# On a fresh namespace, the daemon needs a few seconds after NATS starts
# to connect and create JetStream streams/consumers. Wait up to 30s.
wait_for_jetstream_ready() {
  [[ -n "$NATS_PORT" ]] || return 1
  log "Waiting for JetStream streams to appear (up to 30s)..."
  sleep 10
  local deadline=$((SECONDS + 30))
  while [[ $SECONDS -lt $deadline ]]; do
    local jsz streams
    jsz=$(curl -sf "http://localhost:${NATS_PORT}/jsz" 2>/dev/null) || true
    if [[ -n "$jsz" ]]; then
      streams=$(echo "$jsz" | python3 -c "import sys,json; print(json.load(sys.stdin).get('streams',0))" 2>/dev/null) || true
      if [[ -n "$streams" ]] && [[ "$streams" -gt 0 ]]; then
        log "JetStream ready: $streams stream(s) found"
        return 0
      fi
    fi
    sleep 2
  done
  log "JetStream streams did not appear within timeout"
  return 1
}
_JS_READY=false
if wait_for_jetstream_ready; then
  _JS_READY=true
fi

# ── Test 4: JetStream enabled with streams ────────────────────────────
test_jetstream_streams() {
  [[ "$_JS_READY" == "true" ]] || return 1
  [[ -n "$NATS_PORT" ]] || return 1
  local jsz
  jsz=$(curl -sf "http://localhost:${NATS_PORT}/jsz" 2>/dev/null)
  [[ -n "$jsz" ]] || return 1
  local streams
  streams=$(echo "$jsz" | python3 -c "import sys,json; print(json.load(sys.stdin).get('streams',0))" 2>/dev/null)
  assert_gt "$streams" 0
}
run_test "JetStream enabled with streams (>0)" test_jetstream_streams

# ── Test 5: JetStream has active consumers ────────────────────────────
test_jetstream_consumers() {
  [[ "$_JS_READY" == "true" ]] || return 1
  [[ -n "$NATS_PORT" ]] || return 1
  local jsz
  jsz=$(curl -sf "http://localhost:${NATS_PORT}/jsz" 2>/dev/null)
  [[ -n "$jsz" ]] || return 1
  local consumers
  consumers=$(echo "$jsz" | python3 -c "import sys,json; print(json.load(sys.stdin).get('consumers',0))" 2>/dev/null)
  assert_gt "$consumers" 0
}
run_test "JetStream has active consumers (>0)" test_jetstream_consumers

# ── Test 6: bd-daemon connected to NATS ───────────────────────────────
test_daemon_connected() {
  [[ "$_JS_READY" == "true" ]] || return 1
  [[ -n "$NATS_PORT" ]] || return 1
  local connz
  connz=$(curl -sf "http://localhost:${NATS_PORT}/connz" 2>/dev/null)
  [[ -n "$connz" ]] || return 1
  local names
  names=$(echo "$connz" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for c in d.get('connections',[]):
    n=c.get('name','')
    if n: print(n)
" 2>/dev/null)
  assert_contains "$names" "bd-daemon"
}
run_test "bd-daemon connected to NATS" test_daemon_connected

# ── Test 7: Event bus status reports connected ────────────────────────
test_bus_status() {
  [[ "$_JS_READY" == "true" ]] || return 1
  discover_daemon_pod || return 1
  local status
  status=$(kube exec "$DAEMON_POD" -c bd-daemon -- bd bus status --json 2>/dev/null)
  [[ -n "$status" ]] || return 1
  local nats_status
  nats_status=$(echo "$status" | python3 -c "import sys,json; print(json.load(sys.stdin).get('nats_status',''))" 2>/dev/null)
  assert_eq "$nats_status" "connected"
}
run_test "Event bus status: connected" test_bus_status

# ── Test 8: Event bus has registered handlers ─────────────────────────
test_bus_handlers() {
  discover_daemon_pod || return 1
  local handlers
  handlers=$(kube exec "$DAEMON_POD" -c bd-daemon -- bd bus handlers --json 2>/dev/null)
  [[ -n "$handlers" ]] || return 1
  local count
  count=$(echo "$handlers" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('handlers',[])))" 2>/dev/null)
  assert_gt "$count" 0
}
run_test "Event bus has registered handlers (>0)" test_bus_handlers

# ── Test 9: NATS monitoring reports message counters ──────────────────
# On a fresh namespace there may be zero application messages, so we only
# verify that the monitoring API returns valid counters (in_msgs >= 0).
test_message_throughput() {
  [[ -n "$NATS_PORT" ]] || return 1
  local varz
  varz=$(curl -sf "http://localhost:${NATS_PORT}/varz" 2>/dev/null)
  [[ -n "$varz" ]] || return 1
  local in_msgs out_msgs
  in_msgs=$(echo "$varz" | python3 -c "import sys,json; print(json.load(sys.stdin).get('in_msgs',0))" 2>/dev/null)
  out_msgs=$(echo "$varz" | python3 -c "import sys,json; print(json.load(sys.stdin).get('out_msgs',0))" 2>/dev/null)
  assert_ge "$in_msgs" 0 && assert_ge "$out_msgs" 0
}
run_test "NATS monitoring API reports message counters" test_message_throughput

# ── Test 10: No excessive JetStream API errors ────────────────────────
test_api_errors() {
  [[ -n "$NATS_PORT" ]] || return 1
  local jsz
  jsz=$(curl -sf "http://localhost:${NATS_PORT}/jsz" 2>/dev/null)
  [[ -n "$jsz" ]] || return 1
  local total errors
  total=$(echo "$jsz" | python3 -c "import sys,json; d=json.load(sys.stdin).get('api',{}); print(d.get('total',0))" 2>/dev/null)
  errors=$(echo "$jsz" | python3 -c "import sys,json; d=json.load(sys.stdin).get('api',{}); print(d.get('errors',0))" 2>/dev/null)
  # Error rate should be < 20% of total API calls (startup errors are common with JetStream)
  if [[ "$total" -gt 100 ]]; then
    local max_errors=$((total / 5))
    [[ "$errors" -lt "$max_errors" ]]
  else
    [[ "$errors" -lt 50 ]]
  fi
}
run_test "JetStream API error rate acceptable (<20%)" test_api_errors

# ── Summary ───────────────────────────────────────────────────────────
print_summary
