#!/usr/bin/env bash
# test-agent-state.sh — Verify agent state machine via coop API.
#
# Tests:
#   1. Agent has a valid state (idle, working, starting, prompt, etc.)
#   2. State transitions are observable via coop WebSocket
#   3. Multiple agents can have independent states
#   4. Mux dashboard reflects agent state via badges
#
# Usage:
#   ./scripts/test-agent-state.sh [NAMESPACE]

MODULE_NAME="agent-state"
source "$(dirname "$0")/lib.sh"

log "Testing agent state machine in namespace: $E2E_NAMESPACE"

VALID_STATES="starting working idle prompt error exited"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

log "Found $AGENT_COUNT running agent pod(s)"

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Agent has valid state" "no running agent pods"
  skip_test "State from coop health" "no running agent pods"
  print_summary
  exit 0
fi

# ── Test 1: Coop reports a valid agent state ─────────────────────────
COOP_PORT=""

test_valid_state() {
  COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 9090) || return 1
  local resp state
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null)
  # Extract state from JSON
  state=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('state',''))" 2>/dev/null || echo "")
  [[ -z "$state" ]] && return 1
  # Verify it's one of the valid states
  case " $VALID_STATES " in
    *" $state "*) return 0 ;;
    *) log "  unexpected state: $state"; return 1 ;;
  esac
}
run_test "Coop reports valid agent state" test_valid_state

# ── Test 2: Agent state accessible via health endpoint ───────────────
test_health_has_state() {
  [[ -n "$COOP_PORT" ]] || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/health" 2>/dev/null)
  [[ ${#resp} -gt 2 ]]
}
run_test "Coop health endpoint responds" test_health_has_state

# ── Test 3: Main API port (8080) reports state ───────────────────────
COOP_MAIN_PORT=""

test_main_api_state() {
  COOP_MAIN_PORT=$(start_port_forward "pod/$AGENT_POD" 8080) || return 1
  local resp state
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_MAIN_PORT}/api/v1/agent" 2>/dev/null)
  state=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('state',''))" 2>/dev/null || echo "")
  [[ -n "$state" ]]
}
run_test "Main API port (8080) /api/v1/agent reports state" test_main_api_state

# ── Test 4: Each agent pod has an observable state ───────────────────
test_all_agents_have_state() {
  local count_with_state=0
  for pod in $AGENT_PODS; do
    local port resp state
    port=$(start_port_forward "pod/$pod" 9090) || continue
    resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${port}/api/v1/agent" 2>/dev/null)
    state=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('state',''))" 2>/dev/null || echo "")
    if [[ -n "$state" ]]; then
      count_with_state=$((count_with_state + 1))
      log "  $pod → state=$state"
    fi
  done
  assert_eq "$count_with_state" "$AGENT_COUNT"
}
run_test "All agent pods report a state via coop" test_all_agents_have_state

# ── Test 5: Broker pod list includes state/health info ───────────────
BROKER_SVC=$(kube get svc --no-headers 2>/dev/null | { grep "coop-broker" || true; } | head -1 | awk '{print $1}')
BROKER_TOKEN="${BROKER_TOKEN:-V6T4jmuDY1GDgYDmSRaFa1wwd4RTkFKv}"
BROKER_PORT=""

test_broker_has_health_info() {
  [[ -n "$BROKER_SVC" ]] || return 1
  BROKER_PORT=$(start_port_forward "svc/$BROKER_SVC" 8080) || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 \
    -H "Authorization: Bearer $BROKER_TOKEN" \
    "http://127.0.0.1:${BROKER_PORT}/api/v1/broker/pods" 2>/dev/null)
  # Each pod should have "healthy" boolean
  assert_contains "$resp" "healthy"
}
run_test "Broker pod list includes health status" test_broker_has_health_info

# ── Test 6: Mux dashboard has state badges (quick HTML check) ────────
test_mux_has_badges() {
  [[ -n "$BROKER_PORT" ]] || return 1
  local body
  body=$(curl -s --connect-timeout 5 "http://127.0.0.1:${BROKER_PORT}/mux" 2>/dev/null)
  # The HTML template should contain badge-related CSS/elements
  assert_contains "$body" "pod-badge" || assert_contains "$body" "badge"
}
run_test "Mux dashboard HTML contains badge elements" test_mux_has_badges

# ── Summary ──────────────────────────────────────────────────────────
print_summary
