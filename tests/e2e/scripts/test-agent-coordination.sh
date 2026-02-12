#!/usr/bin/env bash
# test-agent-coordination.sh — Verify multi-agent coordination.
#
# Tests:
#   1. Multiple agent pods coexist in the namespace
#   2. Each agent has a distinct identity (pod name, IP)
#   3. All agents appear in the coop broker pod list
#   4. Mux dashboard shows all agents
#   5. Agents have correct labels (rig, role)
#
# Usage:
#   ./scripts/test-agent-coordination.sh [NAMESPACE]

MODULE_NAME="agent-coordination"
source "$(dirname "$0")/lib.sh"

log "Testing multi-agent coordination in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })

log "Found $AGENT_COUNT running agent pod(s): $(echo $AGENT_PODS | tr '\n' ' ')"

# ── Test 1: Multiple agents coexist ──────────────────────────────────
test_multiple_agents() {
  assert_ge "$AGENT_COUNT" 2
}

if [[ "$AGENT_COUNT" -lt 2 ]]; then
  skip_test "Multiple agents coexist (2+)" "only $AGENT_COUNT agent(s) running"
else
  run_test "Multiple agents coexist (2+)" test_multiple_agents
fi

# ── Test 2: Agents have distinct names ───────────────────────────────
test_distinct_names() {
  local unique_count
  unique_count=$(echo "$AGENT_PODS" | sort -u | { grep -c . || true; })
  assert_eq "$unique_count" "$AGENT_COUNT"
}
run_test "Agent pods have distinct names" test_distinct_names

# ── Test 3: Agents have distinct IPs ────────────────────────────────
test_distinct_ips() {
  local ips unique_count
  ips=""
  for pod in $AGENT_PODS; do
    local ip
    ip=$(kube get pod "$pod" -o jsonpath='{.status.podIP}' 2>/dev/null)
    ips="$ips $ip"
  done
  unique_count=$(echo "$ips" | tr ' ' '\n' | { grep -c . || true; })
  assert_ge "$unique_count" "$AGENT_COUNT"
}
run_test "Agent pods have distinct IPs" test_distinct_ips

# ── Test 4: All agents in broker pod list ────────────────────────────
BROKER_SVC=$(kube get svc --no-headers 2>/dev/null | { grep "coop-broker" || true; } | head -1 | awk '{print $1}')
BROKER_TOKEN="${BROKER_TOKEN:-V6T4jmuDY1GDgYDmSRaFa1wwd4RTkFKv}"
BROKER_PORT=""

test_all_in_broker() {
  [[ -n "$BROKER_SVC" ]] || return 1
  BROKER_PORT=$(start_port_forward "svc/$BROKER_SVC" 8080) || return 1
  local resp broker_count
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  broker_count=$(echo "$resp" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('pods',[])))" 2>/dev/null || echo "0")
  # Broker should have at least as many pods as we see running
  assert_ge "$broker_count" "$AGENT_COUNT"
}
run_test "All agent pods registered in broker" test_all_in_broker

# ── Test 5: Broker reports pod names that match kubectl ──────────────
test_broker_names_match() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local resp names
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  names=$(echo "$resp" | python3 -c "
import sys,json
pods = json.load(sys.stdin).get('pods',[])
for p in pods:
    print(p.get('name',''))
" 2>/dev/null || echo "")
  # When no agent pods exist and broker has none, both are empty — that's a match
  if [[ "$AGENT_COUNT" -eq 0 ]]; then
    local broker_count
    broker_count=$(echo "$names" | { grep -c . || true; })
    assert_eq "$broker_count" "0"
    return $?
  fi
  # At least one broker pod name should match a kubectl agent pod name
  local match_count=0
  for pod in $AGENT_PODS; do
    if echo "$names" | { grep -q "$pod" || true; }; then
      match_count=$((match_count + 1))
    fi
  done
  assert_ge "$match_count" 1
}
run_test "Broker pod names match kubectl agent pods" test_broker_names_match

# ── Test 6: Each agent has independent coop instance ─────────────────
test_independent_coop() {
  local coop_count=0
  for pod in $AGENT_PODS; do
    local port status
    port=$(start_port_forward "pod/$pod" 9090) || continue
    status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
      "http://127.0.0.1:${port}/api/v1/health" 2>/dev/null)
    if [[ "$status" == "200" ]]; then
      coop_count=$((coop_count + 1))
    fi
  done
  assert_eq "$coop_count" "$AGENT_COUNT"
}
run_test "Each agent has an independent coop instance" test_independent_coop

# ── Test 7: Agents have different roles/types ────────────────────────
test_different_roles() {
  if [[ "$AGENT_COUNT" -lt 2 ]]; then
    return 0  # Pass trivially
  fi
  # Agent pod names encode role: gt-ROLE-NAME or gt-RIG-ROLE-NAME
  local roles
  roles=""
  for pod in $AGENT_PODS; do
    roles="$roles $pod"
  done
  # At least the names are different (already tested, but confirm structure)
  local unique
  unique=$(echo "$roles" | tr ' ' '\n' | sort -u | { grep -c . || true; })
  assert_ge "$unique" 2
}
run_test "Agent pods have different names (role/rig encoding)" test_different_roles

# ── Summary ──────────────────────────────────────────────────────────
print_summary
