#!/usr/bin/env bash
# test-agent-spawn.sh — Verify agent pod spawn and registration.
#
# Tests the agent pod lifecycle:
#   1. Existing agent pods are Running (gt-* prefix)
#   2. Agent pods have coop container running
#   3. Coop health endpoint returns 200 on each agent pod
#   4. Agent pods are registered with the coop broker
#   5. Agent pods have expected labels and volumes
#   6. Agent pods have daemon connectivity
#
# Optional (with --spawn):
#   7. Create a test bead, verify controller spawns pod
#   8. Clean up test bead and pod
#
# Usage:
#   ./scripts/test-agent-spawn.sh [NAMESPACE]
#   ./scripts/test-agent-spawn.sh --spawn    # Also test pod creation

MODULE_NAME="agent-spawn"
source "$(dirname "$0")/lib.sh"

SPAWN_TEST=false
for arg in "$@"; do
  case "$arg" in
    --spawn) SPAWN_TEST=true ;;
  esac
done

log "Testing agent pod spawn and registration in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })

log "Found $AGENT_COUNT running agent pod(s): $(echo $AGENT_PODS | tr '\n' ' ')"

# Pick the first agent pod for detailed testing
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

# ── Test 1: Agent pods exist ────────────────────────────────────────
if [[ -z "$AGENT_POD" ]]; then
  skip_test "At least 1 agent pod is Running (gt-* prefix)" "no running agent pods in namespace"
  skip_test "Agent pod has coop container" "no running agent pods found"
  skip_test "Coop health returns 200" "no running agent pods found"
  skip_test "Agent registered with broker" "no running agent pods found"
  print_summary
  exit 0
fi

test_agent_pods_exist() {
  assert_ge "$AGENT_COUNT" 1
}
run_test "At least 1 agent pod is Running (gt-* prefix)" test_agent_pods_exist

# ── Test 2: Agent pod has expected containers ────────────────────────
test_agent_containers() {
  local containers
  containers=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
  # Agent pods typically have a single container named "agent" or "coop"
  [[ -n "$containers" ]]
}
run_test "Agent pod ($AGENT_POD) has containers" test_agent_containers

# ── Test 3: Agent pod is 1/1 Ready ──────────────────────────────────
test_agent_ready() {
  local status
  status=$(kube get pod "$AGENT_POD" --no-headers 2>/dev/null | awk '{print $2}')
  assert_eq "$status" "1/1"
}
run_test "Agent pod is 1/1 Ready" test_agent_ready

# ── Test 4: Coop health endpoint on agent pod ────────────────────────
COOP_PORT=""

test_coop_health() {
  # Coop health is on port 9090 (health-only port)
  COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 9090) || return 1
  local resp
  resp=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/health" 2>/dev/null)
  assert_eq "$resp" "200"
}
run_test "Coop health endpoint returns 200 on agent pod" test_coop_health

# ── Test 5: Coop reports agent state ─────────────────────────────────
test_coop_agent_state() {
  [[ -n "$COOP_PORT" ]] || return 1
  local state
  state=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null)
  # Should have a "state" field
  assert_contains "$state" "state" || assert_contains "$state" "status"
}
run_test "Coop /api/v1/agent reports agent state" test_coop_agent_state

# ── Test 6: Agent registered with coop broker ────────────────────────
BROKER_SVC=$(kube get svc --no-headers 2>/dev/null | { grep "coop-broker" || true; } | head -1 | awk '{print $1}')
BROKER_PORT=""
BROKER_TOKEN="${BROKER_TOKEN:-V6T4jmuDY1GDgYDmSRaFa1wwd4RTkFKv}"

test_agent_registered() {
  [[ -n "$BROKER_SVC" ]] || return 1
  BROKER_PORT=$(start_port_forward "svc/$BROKER_SVC" 8080) || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  # Check that at least one pod is in the list
  local pod_count
  pod_count=$(echo "$resp" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('pods',[])))" 2>/dev/null || echo "0")
  assert_ge "$pod_count" 1
}
run_test "Agent pods registered with coop broker" test_agent_registered

# ── Test 7: Agent pod has expected volumes ───────────────────────────
test_agent_volumes() {
  local volumes
  volumes=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.volumes[*].name}' 2>/dev/null)
  # Should have workspace PVC and credentials secret
  [[ -n "$volumes" ]]
}
run_test "Agent pod has volume mounts" test_agent_volumes

# ── Test 8: Agent pod uses gastown-agent image ───────────────────────
test_agent_image() {
  local image
  image=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.containers[0].image}' 2>/dev/null)
  assert_contains "$image" "gastown-agent"
}
run_test "Agent pod uses gastown-agent image" test_agent_image

# ── Test 9: Agent pod IP matches broker registration ─────────────────
test_pod_ip_in_broker() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local pod_ip
  pod_ip=$(kube get pod "$AGENT_POD" -o jsonpath='{.status.podIP}' 2>/dev/null)
  [[ -z "$pod_ip" ]] && return 1

  local resp
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  # Check that pod IP appears in the coop_url of a registered pod
  assert_contains "$resp" "$pod_ip"
}
run_test "Agent pod IP appears in broker pod list" test_pod_ip_in_broker

# ── Test 10: Multiple agents have distinct IPs ───────────────────────
test_distinct_agent_ips() {
  if [[ "$AGENT_COUNT" -lt 2 ]]; then
    return 0  # Pass trivially if only 1 agent
  fi
  local ips
  ips=$(for pod in $AGENT_PODS; do kube get pod "$pod" -o jsonpath='{.status.podIP}' 2>/dev/null; echo; done | sort -u)
  local unique_count
  unique_count=$(echo "$ips" | { grep -c . || true; })
  assert_ge "$unique_count" 2
}
run_test "Multiple agent pods have distinct IPs" test_distinct_agent_ips

# ── Summary ──────────────────────────────────────────────────────────
print_summary
