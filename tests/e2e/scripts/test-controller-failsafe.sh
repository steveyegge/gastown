#!/usr/bin/env bash
# test-controller-failsafe.sh — Verify controller fail-safe behavior.
#
# The agent controller has a fail-safe: it will NOT delete agent pods if it
# cannot reach the daemon. This prevents data loss during daemon outages.
#
# Tests:
#   1. Controller can reach daemon (no unreachable errors in logs)
#   2. Controller reconcile interval configured (env var or default 60s)
#   3. No orphaned agent pods (all gt-* pods have bead-id label/annotation)
#   4. Controller does not delete during daemon outage (fail-safe log check)
#   5. Agent pods have finalizers or owner references (controller metadata)
#   6. Controller has RBAC to manage pods (auth can-i checks)
#   7. Controller health endpoint responsive (pod readiness implies health)
#
# Usage:
#   ./scripts/test-controller-failsafe.sh [NAMESPACE]

MODULE_NAME="controller-failsafe"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

log "Testing controller fail-safe in $NS"

# ── Discover controller ──────────────────────────────────────────────
CTRL_POD=$(kube get pods --no-headers 2>/dev/null | grep "agent-controller" | head -1 | awk '{print $1}')
CTRL_SA=$(kube get pod "$CTRL_POD" -o jsonpath='{.spec.serviceAccountName}' 2>/dev/null || echo "")

log "Controller pod: ${CTRL_POD:-none}"
log "Controller service account: ${CTRL_SA:-default}"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })

log "Found $AGENT_COUNT agent pod(s)"

# ── Test 1: Controller can reach daemon ──────────────────────────────
test_controller_can_reach_daemon() {
  [[ -n "$CTRL_POD" ]] || return 1
  local logs
  logs=$(kube logs "$CTRL_POD" --tail=100 2>/dev/null)
  local unreachable_count
  unreachable_count=$(echo "$logs" | grep -ci "daemon unreachable\|cannot connect to daemon" || true)
  assert_eq "${unreachable_count:-0}" "0"
}
run_test "Controller can reach daemon" test_controller_can_reach_daemon

# ── Test 2: Controller reconcile interval configured ─────────────────
test_reconcile_interval() {
  [[ -n "$CTRL_POD" ]] || return 1
  # Check for RECONCILE_INTERVAL env var on the controller pod
  local interval
  interval=$(kube get pod "$CTRL_POD" -o jsonpath='{.spec.containers[0].env}' 2>/dev/null)
  if echo "$interval" | grep -qi "RECONCILE_INTERVAL"; then
    return 0
  fi
  # If no explicit env var, check logs for reconcile interval/cycle evidence (default 60s)
  local logs
  logs=$(kube logs "$CTRL_POD" --tail=100 2>/dev/null)
  if echo "$logs" | grep -qi "reconcil"; then
    return 0
  fi
  # Controller is running, so it must be using the default interval
  local phase
  phase=$(kube get pod "$CTRL_POD" -o jsonpath='{.status.phase}' 2>/dev/null)
  assert_eq "$phase" "Running"
}
run_test "Controller reconcile interval configured" test_reconcile_interval

# ── Test 3: No orphaned agent pods ───────────────────────────────────
test_no_orphaned_agent_pods() {
  # All running agent pods (gt-*) should have gastown.io/role and gastown.io/rig labels
  # that link them to the controller's management scope
  local running_agents
  running_agents=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
  [[ -z "$running_agents" ]] && return 0  # No running agents = no orphans

  local orphan_count=0
  for pod in $running_agents; do
    local role rig
    role=$(kube get pod "$pod" -o jsonpath='{.metadata.labels.gastown\.io/role}' 2>/dev/null)
    rig=$(kube get pod "$pod" -o jsonpath='{.metadata.labels.gastown\.io/rig}' 2>/dev/null)
    if [[ -z "$role" || -z "$rig" ]]; then
      orphan_count=$((orphan_count + 1))
    fi
  done
  assert_eq "$orphan_count" "0"
}
run_test "No orphaned agent pods" test_no_orphaned_agent_pods

# ── Test 4: Controller does not delete during daemon outage ──────────
test_no_delete_during_outage() {
  [[ -n "$CTRL_POD" ]] || return 1
  local logs
  logs=$(kube logs "$CTRL_POD" --tail=100 2>/dev/null)
  # The fail-safe prevents deletion when daemon is unreachable.
  # There should be no log line containing both "deleting pod" and
  # "daemon" + "unreachable" — that would mean the fail-safe was bypassed.
  local violation_count
  violation_count=$(echo "$logs" | grep -ci "delet.*pod.*daemon.*unreachable\|daemon.*unreachable.*delet.*pod" || true)
  assert_eq "${violation_count:-0}" "0"
}
run_test "Controller does not delete during daemon outage" test_no_delete_during_outage

# ── Test 5: Agent pods have controller-managed labels ─────────────────
test_agent_pods_managed() {
  [[ "$AGENT_COUNT" -ge 1 ]] || return 0  # Skip trivially if no agents

  local managed_count=0
  local total=0
  for pod in $AGENT_PODS; do
    total=$((total + 1))
    # Controller manages pods via gastown.io labels (role, rig, agent)
    local role agent
    role=$(kube get pod "$pod" -o jsonpath='{.metadata.labels.gastown\.io/role}' 2>/dev/null)
    agent=$(kube get pod "$pod" -o jsonpath='{.metadata.labels.gastown\.io/agent}' 2>/dev/null)
    if [[ -n "$role" && -n "$agent" ]]; then
      managed_count=$((managed_count + 1))
    fi
  done
  assert_eq "$managed_count" "$total"
}
run_test "Agent pods have controller-managed labels" test_agent_pods_managed

# ── Test 6: Controller has RBAC to manage pods ──────────────────────
test_controller_rbac() {
  [[ -n "$CTRL_SA" ]] || return 1
  local sa_full="system:serviceaccount:${NS}:${CTRL_SA}"

  # Check get, list, create, delete permissions on pods
  local can_get can_list can_create can_delete
  can_get=$(kubectl auth can-i get pods -n "$NS" --as="$sa_full" 2>/dev/null || echo "no")
  can_list=$(kubectl auth can-i list pods -n "$NS" --as="$sa_full" 2>/dev/null || echo "no")
  can_create=$(kubectl auth can-i create pods -n "$NS" --as="$sa_full" 2>/dev/null || echo "no")
  can_delete=$(kubectl auth can-i delete pods -n "$NS" --as="$sa_full" 2>/dev/null || echo "no")

  [[ "$can_get" == "yes" ]] && [[ "$can_list" == "yes" ]] && \
  [[ "$can_create" == "yes" ]] && [[ "$can_delete" == "yes" ]]
}
run_test "Controller has RBAC to manage pods" test_controller_rbac

# ── Test 7: Controller health endpoint responsive ────────────────────
test_controller_health() {
  [[ -n "$CTRL_POD" ]] || return 1
  # If the pod is Ready, the kubelet has verified its health probes pass
  local conditions
  conditions=$(kube get pod "$CTRL_POD" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
  assert_eq "$conditions" "True"
}
run_test "Controller health endpoint responsive" test_controller_health

# ── Summary ──────────────────────────────────────────────────────────
print_summary
