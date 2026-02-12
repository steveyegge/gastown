#!/usr/bin/env bash
# test-coop-broker-health.sh — Verify coop broker + mux health.
#
# Tests:
#   1. Coop broker pod is 2/2 ready (coop + mux sidecar)
#   2. Health endpoint returns 200
#   3. Mux HTML page is served at /mux
#   4. Broker pods API returns registered pods
#   5. WebSocket connects
#   6. Credential file exists on PVC
#
# Usage:
#   ./scripts/test-coop-broker-health.sh [NAMESPACE]

MODULE_NAME="coop-broker-health"
source "$(dirname "$0")/lib.sh"

log "Testing coop broker health in namespace: $E2E_NAMESPACE"

# ── Discover coop broker ─────────────────────────────────────────────
BROKER_POD=$(kube get pods --no-headers 2>/dev/null | grep "coop-broker" | head -1 | awk '{print $1}')
BROKER_SVC=$(kube get svc --no-headers 2>/dev/null | grep "coop-broker" | head -1 | awk '{print $1}')

# Get broker token from configmap or secret
BROKER_TOKEN=$(kube get configmap --no-headers 2>/dev/null | grep "coop-broker" | head -1 | awk '{print $1}')
if [[ -n "$BROKER_TOKEN" ]]; then
  BROKER_TOKEN=$(kube get configmap "$BROKER_TOKEN" -o jsonpath='{.data.BROKER_TOKEN}' 2>/dev/null || echo "")
fi
BROKER_TOKEN="${BROKER_TOKEN:-${BROKER_TOKEN_ENV:-V6T4jmuDY1GDgYDmSRaFa1wwd4RTkFKv}}"

log "Broker pod: ${BROKER_POD:-none}"
log "Broker svc: ${BROKER_SVC:-none}"

# ── Test 1: Pod is 2/2 ready ────────────────────────────────────────
test_pod_ready() {
  [[ -n "$BROKER_POD" ]] || return 1
  local ready_count
  ready_count=$(kube get pod "$BROKER_POD" -o jsonpath='{.status.containerStatuses[?(@.ready==true)].name}' 2>/dev/null | wc -w | tr -d ' ')
  assert_ge "$ready_count" 2
}
run_test "Coop broker pod containers ready (2/2)" test_pod_ready

# ── Test 2: Containers are coop + mux ───────────────────────────────
test_container_names() {
  [[ -n "$BROKER_POD" ]] || return 1
  local containers
  containers=$(kube get pod "$BROKER_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
  # Container is named "coop-broker" (mux built-in) + "credential-seeder"
  assert_contains "$containers" "coop-broker"
}
run_test "Broker has coop-broker container" test_container_names

# ── Port-forward to broker service ───────────────────────────────────
BROKER_PORT=""

setup_port_forward() {
  if [[ -n "$BROKER_SVC" ]]; then
    BROKER_PORT=$(start_port_forward "svc/$BROKER_SVC" 8080) || return 1
  else
    BROKER_PORT=$(start_port_forward "pod/$BROKER_POD" 8080) || return 1
  fi
}

# ── Test 3: Health endpoint returns 200 ──────────────────────────────
test_health_endpoint() {
  setup_port_forward || return 1
  local resp
  resp=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://127.0.0.1:${BROKER_PORT}/api/v1/health" 2>/dev/null)
  assert_eq "$resp" "200"
}
run_test "Health endpoint (/api/v1/health) returns 200" test_health_endpoint

# ── Test 4: Mux HTML page served ────────────────────────────────────
test_mux_page() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local body
  body=$(curl -s --connect-timeout 5 "http://127.0.0.1:${BROKER_PORT}/mux" 2>/dev/null)
  assert_contains "$body" "Coop Multiplexer"
}
run_test "Mux page (/mux) serves HTML" test_mux_page

# ── Test 5: Broker pods API returns registered pods ──────────────────
test_broker_pods_api() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  # Should have a "pods" array
  assert_contains "$resp" "pods"
}
run_test "Broker pods API returns pod list" test_broker_pods_api

# ── Test 6: Broker pods API requires auth ────────────────────────────
test_broker_pods_auth() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  assert_eq "$status" "401"
}
run_test "Broker pods API requires auth (401 without token)" test_broker_pods_auth

# ── Test 7: At least one agent pod registered ────────────────────────
# On a fresh namespace there may be no agent pods yet — skip rather than fail.
_BROKER_POD_COUNT="0"
if [[ -n "$BROKER_PORT" ]]; then
  _broker_resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  _BROKER_POD_COUNT=$(echo "$_broker_resp" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('pods',[])))" 2>/dev/null || echo "0")
fi

if [[ "${_BROKER_POD_COUNT:-0}" -eq 0 ]]; then
  skip_test "At least 1 agent pod registered with broker" "No agent pods registered (fresh namespace)"
else
  test_pods_registered() {
    assert_ge "$_BROKER_POD_COUNT" 1
  }
  run_test "At least 1 agent pod registered with broker" test_pods_registered
fi

# ── Test 8: Credential PVC mounted ──────────────────────────────────
test_credential_pvc() {
  [[ -n "$BROKER_POD" ]] || return 1
  # Check if the PVC volume mount exists
  local volumes
  volumes=$(kube get pod "$BROKER_POD" -o jsonpath='{.spec.volumes[*].name}' 2>/dev/null)
  assert_contains "$volumes" "credentials" || assert_contains "$volumes" "cred"
}
run_test "Credential PVC volume mounted" test_credential_pvc

# ── Summary ──────────────────────────────────────────────────────────
print_summary
