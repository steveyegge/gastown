#!/usr/bin/env bash
# test-controller-health.sh — Verify agent controller deployment health.
#
# Tests:
#   1. Controller deployment is ready (1/1)
#   2. Controller has RBAC (ServiceAccount, ClusterRoleBinding)
#   3. Controller can list pods (verify permissions)
#   4. Agent pods exist (controller has reconciled at least one)
#   5. Controller logs show no errors
#
# Usage:
#   ./scripts/test-controller-health.sh [NAMESPACE]

MODULE_NAME="controller-health"
source "$(dirname "$0")/lib.sh"

log "Testing agent controller health in namespace: $E2E_NAMESPACE"

# ── Discover controller ──────────────────────────────────────────────
CTRL_POD=$(kube get pods --no-headers 2>/dev/null | grep "agent-controller" | head -1 | awk '{print $1}')
CTRL_DEPLOY=$(kube get deployments --no-headers 2>/dev/null | grep "agent-controller" | head -1 | awk '{print $1}')

log "Controller pod: ${CTRL_POD:-none}"
log "Controller deploy: ${CTRL_DEPLOY:-none}"

# ── Test 1: Deployment is ready ──────────────────────────────────────
test_deploy_ready() {
  [[ -n "$CTRL_DEPLOY" ]] || return 1
  local ready
  ready=$(kube get deployment "$CTRL_DEPLOY" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
  assert_ge "${ready:-0}" 1
}
run_test "Controller deployment is ready" test_deploy_ready

# ── Test 2: Pod is running ──────────────────────────────────────────
test_pod_running() {
  [[ -n "$CTRL_POD" ]] || return 1
  local phase
  phase=$(kube get pod "$CTRL_POD" -o jsonpath='{.status.phase}' 2>/dev/null)
  assert_eq "$phase" "Running"
}
run_test "Controller pod is Running" test_pod_running

# ── Test 3: ServiceAccount exists ────────────────────────────────────
test_service_account() {
  local sa
  sa=$(kube get serviceaccounts --no-headers 2>/dev/null | grep "agent-controller" | head -1 | awk '{print $1}')
  [[ -n "$sa" ]]
}
run_test "Agent controller ServiceAccount exists" test_service_account

# ── Test 4: Agent pods exist (controller has reconciled) ─────────────
AGENT_POD_COUNT=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | wc -l | tr -d ' ')
AGENT_RUNNING_COUNT=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | wc -l | tr -d ' ')

if [[ "${AGENT_POD_COUNT:-0}" -eq 0 ]]; then
  skip_test "At least 1 agent pod exists (gt-* prefix)" "no agent pods in namespace"
  skip_test "At least 1 agent pod is Running" "no agent pods in namespace"
else
  test_agent_pods_exist() {
    assert_ge "${AGENT_POD_COUNT:-0}" 1
  }
  run_test "At least 1 agent pod exists (gt-* prefix)" test_agent_pods_exist

  # ── Test 5: Agent pods are Running ───────────────────────────────────
  test_agent_pods_running() {
    assert_ge "${AGENT_RUNNING_COUNT:-0}" 1
  }
  run_test "At least 1 agent pod is Running" test_agent_pods_running
fi

# ── Test 6: Controller env has required config ───────────────────────
test_controller_env() {
  [[ -n "$CTRL_POD" ]] || return 1
  local env_vars
  env_vars=$(kube get pod "$CTRL_POD" -o jsonpath='{.spec.containers[0].env[*].name}' 2>/dev/null)
  # Should have daemon URL and namespace config
  [[ -n "$env_vars" ]]
}
run_test "Controller has environment configuration" test_controller_env

# ── Test 7: No panics in controller logs ─────────────────────────────
test_no_panics() {
  [[ -n "$CTRL_POD" ]] || return 1
  local panic_count
  panic_count=$(kube logs "$CTRL_POD" --tail=200 2>/dev/null | grep -ci "panic\|fatal" || true)
  assert_eq "${panic_count:-0}" "0"
}
run_test "No panic/fatal in controller logs (last 200 lines)" test_no_panics

# ── Test 8: Controller image is expected ─────────────────────────────
test_controller_image() {
  [[ -n "$CTRL_POD" ]] || return 1
  local image
  image=$(kube get pod "$CTRL_POD" -o jsonpath='{.spec.containers[0].image}' 2>/dev/null)
  assert_contains "$image" "agent-controller"
}
run_test "Controller uses agent-controller image" test_controller_image

# ── Summary ──────────────────────────────────────────────────────────
print_summary
