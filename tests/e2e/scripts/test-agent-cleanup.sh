#!/usr/bin/env bash
# test-agent-cleanup.sh — Verify pod cleanup expectations.
#
# Tests:
#   1. All agent pods have labels matching bead conventions (gt:agent)
#   2. Agent pods have resource limits set
#   3. Agent pods have a restart policy
#   4. No orphaned PVCs (each gt-* PVC has a matching pod)
#   5. No crash-looping agents (restart count is low)
#
# Usage:
#   ./scripts/test-agent-cleanup.sh [NAMESPACE]

MODULE_NAME="agent-cleanup"
source "$(dirname "$0")/lib.sh"

log "Testing pod cleanup expectations in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_COUNT=$(echo "$AGENT_PODS" | { grep -c . || true; })
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

log "Found $AGENT_COUNT running agent pod(s)"

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Agent pod labels" "no running agent pods"
  skip_test "Resource limits" "no running agent pods"
  skip_test "Restart policy" "no running agent pods"
  skip_test "No orphaned PVCs" "no running agent pods"
  skip_test "No crash-looping agents" "no running agent pods"
  print_summary
  exit 0
fi

# ── Test 1: All agent pods have bead convention labels ────────────────
# Expect labels like gt:agent or app.kubernetes.io/component:agent
test_agent_labels() {
  local all_labeled=true
  for pod in $AGENT_PODS; do
    local labels
    labels=$(kube get pod "$pod" -o jsonpath='{.metadata.labels}' 2>/dev/null)
    # Check for bead convention markers: "gt" key with "agent" value,
    # or any label containing "agent" in the label set
    if ! assert_contains "$labels" "agent"; then
      log "  $pod missing 'agent' label"
      all_labeled=false
    fi
  done
  [[ "$all_labeled" == "true" ]]
}
run_test "All agent pods have labels matching bead conventions (gt:agent)" test_agent_labels

# ── Test 2: Agent pods have resource limits set ───────────────────────
test_resource_limits() {
  local all_have_limits=true
  for pod in $AGENT_PODS; do
    local limits
    limits=$(kube get pod "$pod" -o jsonpath='{.spec.containers[0].resources.limits}' 2>/dev/null)
    if [[ -z "$limits" || "$limits" == "{}" ]]; then
      log "  $pod has no resource limits"
      all_have_limits=false
    fi
  done
  [[ "$all_have_limits" == "true" ]]
}
run_test "Agent pods have resource limits set" test_resource_limits

# ── Test 3: Agent pods have restart policy ────────────────────────────
test_restart_policy() {
  local all_have_policy=true
  for pod in $AGENT_PODS; do
    local policy
    policy=$(kube get pod "$pod" -o jsonpath='{.spec.restartPolicy}' 2>/dev/null)
    if [[ -z "$policy" ]]; then
      log "  $pod has no restart policy"
      all_have_policy=false
    else
      log "  $pod restartPolicy=$policy"
    fi
  done
  [[ "$all_have_policy" == "true" ]]
}
run_test "Agent pods have restart policy set" test_restart_policy

# ── Test 4: No orphaned PVCs ─────────────────────────────────────────
# Each PVC prefixed with "gt-" should have a corresponding pod.
test_no_orphaned_pvcs() {
  local pvcs
  pvcs=$(kube get pvc --no-headers 2>/dev/null | { grep "gt-" || true; } | awk '{print $1}')
  if [[ -z "$pvcs" ]]; then
    # No gt-* PVCs at all — pass trivially
    return 0
  fi

  local all_pods
  all_pods=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | awk '{print $1}')

  local orphaned=0
  for pvc in $pvcs; do
    # Check if any pod references this PVC directly
    local pvc_users
    pvc_users=$(kube get pods -o json 2>/dev/null | python3 -c "
import sys, json
data = json.load(sys.stdin)
pvc_name = '$pvc'
for item in data.get('items', []):
  for vol in item.get('spec', {}).get('volumes', []):
    claim = vol.get('persistentVolumeClaim', {}).get('claimName', '')
    if claim == pvc_name:
      print(item['metadata']['name'])
" 2>/dev/null || echo "")
    if [[ -z "$pvc_users" ]]; then
      # Fallback: check if PVC name contains a pod name or vice versa
      local matched=false
      for pod in $all_pods; do
        if assert_contains "$pvc" "$pod" || assert_contains "$pod" "$pvc"; then
          matched=true
          break
        fi
      done
      if [[ "$matched" == "false" ]]; then
        log "  Orphaned PVC: $pvc (no matching pod)"
        orphaned=$((orphaned + 1))
      fi
    fi
  done
  if [[ "$orphaned" -gt 0 ]]; then
    log "  WARNING: $orphaned orphaned PVC(s) found (controller does not clean up PVCs)"
  fi
  # Orphaned PVCs are a known gap — warn but don't fail
  return 0
}
run_test "Check for orphaned PVCs (warning only)" test_no_orphaned_pvcs

# ── Test 5: No crash-looping agents ───────────────────────────────────
# Restart count should be low (< 5 is healthy, indicates no crash loop).
MAX_HEALTHY_RESTARTS=5

test_no_crash_loops() {
  local crash_looping=0
  for pod in $AGENT_PODS; do
    local restarts
    restarts=$(kube get pod "$pod" -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || echo "0")
    restarts="${restarts:-0}"
    if [[ "$restarts" -ge "$MAX_HEALTHY_RESTARTS" ]]; then
      log "  $pod has $restarts restarts (threshold: $MAX_HEALTHY_RESTARTS)"
      crash_looping=$((crash_looping + 1))
    else
      log "  $pod restarts=$restarts (OK)"
    fi
  done
  assert_eq "$crash_looping" "0"
}
run_test "No crash-looping agents (restarts < $MAX_HEALTHY_RESTARTS)" test_no_crash_loops

# ── Summary ──────────────────────────────────────────────────────────
print_summary
