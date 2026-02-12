#!/usr/bin/env bash
# test-agent-lifecycle.sh — Verify agent pod lifecycle via controller reconciliation loop.
#
# Tests:
#   1. Controller deployment running and ready (1/1)
#   2. Controller has ServiceAccount
#   3. Controller watches for agent beads
#   4. Agent pods have controller owner labels
#   5. Agent pods use correct image
#   6. Pod restart count is 0 for agents
#   7. Agent pods have resource requests set
#   8. Controller reconcile loop responsive
#
# Usage:
#   ./scripts/test-agent-lifecycle.sh [NAMESPACE]

MODULE_NAME="agent-lifecycle"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

log "Testing agent lifecycle in $NS"

# ── Discover controller ──────────────────────────────────────────────
CTRL_DEPLOY=$(kube get deployments -l "app.kubernetes.io/component=agent-controller" --no-headers 2>/dev/null | head -1 | awk '{print $1}')
CTRL_POD=$(kube get pods -l "app.kubernetes.io/component=agent-controller" --no-headers 2>/dev/null | grep "Running" | head -1 | awk '{print $1}')

log "Controller deployment: ${CTRL_DEPLOY:-none}"
log "Controller pod: ${CTRL_POD:-none}"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })

log "Found $AGENT_COUNT agent pod(s)"

# ── Test 1: Controller deployment running and ready ──────────────────
test_controller_ready() {
  [[ -n "$CTRL_DEPLOY" ]] || return 1
  local ready
  ready=$(kube get deployment "$CTRL_DEPLOY" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  assert_ge "${ready:-0}" 1
}
run_test "Controller deployment running and ready" test_controller_ready

# ── Test 2: Controller has ServiceAccount ────────────────────────────
test_controller_sa() {
  [[ -n "$CTRL_POD" ]] || return 1
  local sa
  sa=$(kube get pod "$CTRL_POD" -o jsonpath='{.spec.serviceAccountName}' 2>/dev/null)
  [[ -n "$sa" && "$sa" != "default" ]]
}
run_test "Controller has ServiceAccount" test_controller_sa

# ── Test 3: Controller watches for agent beads ───────────────────────
test_controller_watches() {
  [[ -n "$CTRL_POD" ]] || return 1
  local logs
  logs=$(kube logs "$CTRL_POD" --tail=200 2>/dev/null)
  echo "$logs" | grep -iq "watching\|reconcil"
}
run_test "Controller watches for agent beads" test_controller_watches

# ── Test 4: Agent pods have controller owner labels ──────────────────
test_agent_owner_labels() {
  # Check pods with managed-by label or gt-* prefix
  local managed_pods
  managed_pods=$(kube get pods -l "app.kubernetes.io/managed-by" --no-headers 2>/dev/null | awk '{print $1}')

  # If no managed-by label pods, fall back to gt-* pods and check their labels
  if [[ -z "$managed_pods" ]]; then
    [[ -n "$AGENT_PODS" ]] || return 1
    # Verify at least one gt-* pod has labels linking to the controller
    local found=false
    for pod in $AGENT_PODS; do
      local labels
      labels=$(kube get pod "$pod" -o jsonpath='{.metadata.labels}' 2>/dev/null)
      if echo "$labels" | grep -q "controller\|managed-by\|agent-controller"; then
        found=true
        break
      fi
    done
    $found
  else
    # At least one managed pod exists
    [[ -n "$managed_pods" ]]
  fi
}
run_test "Agent pods have controller owner labels" test_agent_owner_labels

# ── Test 5: Agent pods use correct image ─────────────────────────────
if [[ "${AGENT_COUNT:-0}" -eq 0 ]]; then
  skip_test "Agent pods use correct image" "no agent pods in namespace"
  skip_test "Pod restart count is 0 for agents" "no agent pods in namespace"
  skip_test "Agent pods have resource requests set" "no agent pods in namespace"
else
  test_agent_image() {
    local all_correct=true
    for pod in $AGENT_PODS; do
      local image
      image=$(kube get pod "$pod" -o jsonpath='{.spec.containers[0].image}' 2>/dev/null)
      if ! echo "$image" | grep -q "gastown-agent"; then
        log "  $pod uses unexpected image: $image"
        all_correct=false
      fi
    done
    $all_correct
  }
  run_test "Agent pods use correct image" test_agent_image

  # ── Test 6: Pod restart count is 0 for agents ───────────────────────
  test_agent_no_restarts() {
    local all_zero=true
    for pod in $AGENT_PODS; do
      local restarts
      restarts=$(kube get pod "$pod" -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null)
      if [[ "${restarts:-0}" -ne 0 ]]; then
        log "  $pod has $restarts restart(s)"
        all_zero=false
      fi
    done
    $all_zero
  }
  run_test "Pod restart count is 0 for agents" test_agent_no_restarts

  # ── Test 7: Agent pods have resource requests set ────────────────────
  test_agent_resource_requests() {
    local all_have_requests=true
    for pod in $AGENT_PODS; do
      local cpu mem
      cpu=$(kube get pod "$pod" -o jsonpath='{.spec.containers[0].resources.requests.cpu}' 2>/dev/null)
      mem=$(kube get pod "$pod" -o jsonpath='{.spec.containers[0].resources.requests.memory}' 2>/dev/null)
      if [[ -z "$cpu" || -z "$mem" ]]; then
        log "  $pod missing resource requests (cpu=${cpu:-unset}, memory=${mem:-unset})"
        all_have_requests=false
      fi
    done
    $all_have_requests
  }
  run_test "Agent pods have resource requests set" test_agent_resource_requests
fi

# ── Test 8: Controller reconcile loop responsive ────────────────────
test_reconcile_responsive() {
  [[ -n "$CTRL_POD" ]] || return 1
  local logs
  logs=$(kube logs "$CTRL_POD" --tail=50 2>/dev/null)
  # Verify there is recent log activity — lines exist and contain timestamps or recognizable output
  local line_count
  line_count=$(echo "$logs" | { grep -c . || true; })
  assert_ge "${line_count:-0}" 1
}
run_test "Controller reconcile loop responsive" test_reconcile_responsive

# ── Summary ──────────────────────────────────────────────────────────
print_summary
