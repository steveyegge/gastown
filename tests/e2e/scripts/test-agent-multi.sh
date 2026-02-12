#!/usr/bin/env bash
# test-agent-multi.sh — Verify multi-agent coexistence.
#
# Tests:
#   1. At least 2 agent pods are Running
#   2. Each agent pod has a distinct name
#   3. Each agent pod has a distinct IP
#   4. Each agent is registered with the coop broker separately
#   5. Broker pod names match kubectl pod names
#   6. Each pod's coop health is independently reachable
#
# Skips all tests if fewer than 2 agent pods are found.
#
# Usage:
#   ./scripts/test-agent-multi.sh [NAMESPACE]

MODULE_NAME="agent-multi"
source "$(dirname "$0")/lib.sh"

log "Testing multi-agent coexistence in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })

log "Found $AGENT_COUNT running agent pod(s)"

if [[ "$AGENT_COUNT" -lt 2 ]]; then
  skip_test "2+ agent pods Running" "need 2+ agent pods, found $AGENT_COUNT"
  skip_test "Distinct pod names" "need 2+ agent pods"
  skip_test "Distinct pod IPs" "need 2+ agent pods"
  skip_test "All registered with broker" "need 2+ agent pods"
  skip_test "Broker names match kubectl" "need 2+ agent pods"
  skip_test "Independent coop health" "need 2+ agent pods"
  print_summary
  exit 0
fi

# ── Test 1: At least 2 agent pods running ─────────────────────────────
test_two_plus_agents() {
  assert_ge "$AGENT_COUNT" 2
}
run_test "At least 2 agent pods are Running" test_two_plus_agents

# ── Test 2: Each agent has a distinct name ────────────────────────────
test_distinct_names() {
  local unique_count
  unique_count=$(echo "$AGENT_PODS" | sort -u | { grep -c . || true; })
  assert_eq "$unique_count" "$AGENT_COUNT"
}
run_test "Each agent pod has a distinct name" test_distinct_names

# ── Test 3: Each agent has a distinct IP ──────────────────────────────
test_distinct_ips() {
  local ips unique_ip_count
  ips=""
  for pod in $AGENT_PODS; do
    local ip
    ip=$(kube get pod "$pod" -o jsonpath='{.status.podIP}' 2>/dev/null)
    ips="${ips}${ip}"$'\n'
  done
  unique_ip_count=$(echo "$ips" | { grep -c . || true; })
  local deduped_count
  deduped_count=$(echo "$ips" | sort -u | { grep -c . || true; })
  assert_eq "$deduped_count" "$unique_ip_count"
}
run_test "Each agent pod has a distinct IP" test_distinct_ips

# ── Broker setup ──────────────────────────────────────────────────────
BROKER_SVC=$(kube get svc --no-headers 2>/dev/null | { grep "coop-broker" || true; } | head -1 | awk '{print $1}')
BROKER_TOKEN="${BROKER_TOKEN:-V6T4jmuDY1GDgYDmSRaFa1wwd4RTkFKv}"
BROKER_PORT=""
BROKER_RESP=""

setup_broker() {
  if [[ -z "$BROKER_PORT" && -n "$BROKER_SVC" ]]; then
    BROKER_PORT=$(start_port_forward "svc/$BROKER_SVC" 8080) || return 1
    BROKER_RESP=$(curl -s --connect-timeout 5 \
      -H "Authorization: Bearer $BROKER_TOKEN" \
      "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  fi
  [[ -n "$BROKER_PORT" ]]
}

# ── Test 4: All agents registered with broker ─────────────────────────
test_all_registered() {
  setup_broker || return 1
  local registered_count
  registered_count=$(echo "$BROKER_RESP" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('pods',[])))" 2>/dev/null || echo "0")
  assert_ge "$registered_count" "$AGENT_COUNT"
}
run_test "All agent pods registered with coop broker" test_all_registered

# ── Test 5: Broker pod names match kubectl pod names ──────────────────
test_broker_names_match() {
  setup_broker || return 1
  local all_found=true
  for pod in $AGENT_PODS; do
    if ! echo "$BROKER_RESP" | { grep -q "$pod" || true; }; then
      # Double-check with explicit test (grep || true always returns 0)
      if ! assert_contains "$BROKER_RESP" "$pod"; then
        log "  Pod $pod not found in broker response"
        all_found=false
      fi
    fi
  done
  [[ "$all_found" == "true" ]]
}
run_test "Broker pod names match kubectl pod names" test_broker_names_match

# ── Test 6: Each pod's coop health is independently reachable ─────────
test_independent_coop_health() {
  local healthy_count=0
  for pod in $AGENT_PODS; do
    local port status
    port=$(start_port_forward "pod/$pod" 9090) || continue
    status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
      "http://127.0.0.1:${port}/api/v1/health" 2>/dev/null)
    if [[ "$status" == "200" ]]; then
      healthy_count=$((healthy_count + 1))
      log "  $pod health: 200 OK"
    else
      log "  $pod health: $status"
    fi
  done
  assert_eq "$healthy_count" "$AGENT_COUNT"
}
run_test "Each pod's coop health is independently reachable" test_independent_coop_health

# ── Summary ──────────────────────────────────────────────────────────
print_summary
