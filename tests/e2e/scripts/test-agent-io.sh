#!/usr/bin/env bash
# test-agent-io.sh — Verify agent input/output via coop API.
#
# Tests:
#   1. GET /api/v1/screen returns content (JSON)
#   2. GET /api/v1/screen/text returns non-empty text
#   3. POST /api/v1/input endpoint accepts text (200/204, not 404)
#   4. POST /api/v1/input/keys endpoint accepts keystrokes (200/204, not 404)
#   5. POST /api/v1/agent/nudge endpoint works
#   6. Agent state available on main API port (8080)
#   7. Health port (9090) does NOT serve input API
#
# Note: Uses port 8080 (full coop API) for input tests.
#       Port 9090 is health-only — input/nudge/respond do NOT work there.
#       The agent may have an expired OAuth token; we only verify coop API
#       responsiveness, not actual agent behavior.
#
# Usage:
#   ./scripts/test-agent-io.sh [NAMESPACE]

MODULE_NAME="agent-io"
source "$(dirname "$0")/lib.sh"

log "Testing agent input/output via coop API in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Screen endpoint returns content" "no running agent pods"
  skip_test "Input endpoint accepts text" "no running agent pods"
  skip_test "Keys endpoint accepts keystrokes" "no running agent pods"
  print_summary
  exit 0
fi

log "Using agent pod: $AGENT_POD"

# ── Port-forward to agent's main API port (8080) ─────────────────────
COOP_PORT=""

setup_coop_port() {
  if [[ -z "$COOP_PORT" ]]; then
    COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 8080) || return 1
  fi
  return 0
}

# ── Test 1: GET /api/v1/screen returns 200 ────────────────────────────
test_screen_json() {
  setup_coop_port || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    "http://127.0.0.1:${COOP_PORT}/api/v1/screen" 2>/dev/null)
  assert_eq "$status" "200"
}
run_test "GET /api/v1/screen returns 200 (JSON)" test_screen_json

# ── Test 2: Screen text endpoint has content ──────────────────────────
test_screen_has_content() {
  setup_coop_port || return 1
  local content
  content=$(curl -s --connect-timeout 5 \
    "http://127.0.0.1:${COOP_PORT}/api/v1/screen/text" 2>/dev/null)
  [[ ${#content} -gt 0 ]]
}
run_test "GET /api/v1/screen/text returns non-empty content" test_screen_has_content

# ── Test 3: POST /api/v1/input accepts text ───────────────────────────
# We send empty text to avoid disrupting a running agent.
# We only verify the endpoint responds (200/204), not 404 or 405.
test_input_endpoint() {
  setup_coop_port || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    -X POST -H "Content-Type: application/json" \
    -d '{"text":""}' \
    "http://127.0.0.1:${COOP_PORT}/api/v1/input" 2>/dev/null)
  # Accept 200 or 204 (input accepted) or 400 (bad request but route exists)
  [[ "$status" == "200" || "$status" == "204" || "$status" == "400" ]]
}
run_test "POST /api/v1/input endpoint accepts requests" test_input_endpoint

# ── Test 4: POST /api/v1/input/keys accepts keystrokes ───────────────
# We send an empty keys array to avoid disrupting a running agent.
test_keys_endpoint() {
  setup_coop_port || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    -X POST -H "Content-Type: application/json" \
    -d '{"keys":[]}' \
    "http://127.0.0.1:${COOP_PORT}/api/v1/input/keys" 2>/dev/null)
  # Accept 200 or 204 (keys accepted) or 400 (bad request but route exists)
  [[ "$status" == "200" || "$status" == "204" || "$status" == "400" ]]
}
run_test "POST /api/v1/input/keys endpoint accepts requests" test_keys_endpoint

# ── Test 5: POST /api/v1/agent/nudge works ────────────────────────────
test_nudge_endpoint() {
  setup_coop_port || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    -X POST -H "Content-Type: application/json" \
    -d '{"message":"e2e-test-nudge"}' \
    "http://127.0.0.1:${COOP_PORT}/api/v1/agent/nudge" 2>/dev/null)
  [[ "$status" == "200" || "$status" == "204" ]]
}
run_test "POST /api/v1/agent/nudge endpoint works" test_nudge_endpoint

# ── Test 6: Agent state available on main port ────────────────────────
test_agent_state_main() {
  setup_coop_port || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null)
  assert_contains "$resp" "state"
}
run_test "Agent state available on main API port (8080)" test_agent_state_main

# ── Test 7: Health port (9090) does NOT serve input API ───────────────
HEALTH_PORT=""

test_health_port_no_input() {
  HEALTH_PORT=$(start_port_forward "pod/$AGENT_POD" 9090) || return 1
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 \
    -X POST -H "Content-Type: application/json" \
    -d '{"text":""}' \
    "http://127.0.0.1:${HEALTH_PORT}/api/v1/input" 2>/dev/null)
  # Should be 404 or some non-2xx — input only works on port 8080
  [[ "$status" != "200" && "$status" != "204" ]]
}
run_test "Health port (9090) does NOT serve input API" test_health_port_no_input

# ── Summary ──────────────────────────────────────────────────────────
print_summary
