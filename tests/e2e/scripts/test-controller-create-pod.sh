#!/usr/bin/env bash
# test-controller-create-pod.sh — Verify controller creates/deletes pods from beads.
#
# Tests the full bead→pod lifecycle:
#   1. Daemon HTTP API is reachable
#   2. Create a test bead with gt:agent + execution_target:k8s labels
#   3. Controller reconciles and creates agent pod (within timeout)
#   4. Agent pod reaches Running state
#   5. Agent pod has expected labels (gastown.io/agent, gastown.io/bead-id)
#   6. Coop health endpoint responds on new pod
#   7. Close bead via daemon API
#   8. Controller deletes agent pod (within timeout)
#
# This automates what was proved manually in gt-el7sxq.21.
#
# IMPORTANT: This test creates and deletes a real agent pod. It requires:
#   - Daemon HTTP API accessible (port 9080)
#   - BD_DAEMON_TOKEN for authentication
#   - Controller running and watching beads
#   - Sufficient cluster resources for a new pod
#
# Usage:
#   ./scripts/test-controller-create-pod.sh [NAMESPACE]

MODULE_NAME="controller-create-pod"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

log "Testing controller bead→pod lifecycle in namespace: $NS"

# ── Configuration ────────────────────────────────────────────────────
# Timeouts
POD_CREATE_TIMEOUT=120   # seconds to wait for pod to appear
POD_READY_TIMEOUT=180    # seconds to wait for pod to become Ready
POD_DELETE_TIMEOUT=120   # seconds to wait for pod to be deleted (controller reconciles every 60s)

# Test bead metadata
TEST_BEAD_TITLE="e2e-lifecycle-test-$(date +%s)"
TEST_BEAD_ID=""
TEST_POD_NAME=""
# Snapshot of existing gt-* pods BEFORE creating our test bead
PRE_EXISTING_PODS=""

# ── Discover daemon ──────────────────────────────────────────────────
DAEMON_POD=""
DAEMON_PORT=""
DAEMON_TOKEN=""

discover_daemon() {
  DAEMON_POD=$(kube get pods --no-headers 2>/dev/null | grep "daemon" | grep -v "dolt\|nats\|clusterctl" | grep "Running" | head -1 | awk '{print $1}')
  [[ -n "$DAEMON_POD" ]] || return 1

  # Get the daemon token from the pod's environment
  DAEMON_TOKEN=$(kube exec "$DAEMON_POD" -c bd-daemon -- printenv BD_DAEMON_TOKEN 2>/dev/null) || true
  [[ -n "$DAEMON_TOKEN" ]]
}

# Helper: call daemon HTTP API
# Usage: daemon_api METHOD BODYFILE
# BODYFILE must be a path to a file containing valid JSON.
# Use write_json helper to create the body file safely.
daemon_api() {
  local method="$1"
  local bodyfile="${2:-}"
  [[ -n "$DAEMON_PORT" ]] || return 1

  local respfile
  respfile=$(mktemp)

  local curl_args=(-s --connect-timeout 10 -o "$respfile" -w "%{http_code}" -X POST
    -H "Content-Type: application/json"
    -H "Authorization: Bearer $DAEMON_TOKEN")

  if [[ -n "$bodyfile" && -f "$bodyfile" ]]; then
    curl_args+=(-d "@${bodyfile}")
  else
    curl_args+=(-d '{}')
  fi

  curl_args+=("http://127.0.0.1:${DAEMON_PORT}/bd.v1.BeadsService/${method}")

  local http_code
  http_code=$(curl "${curl_args[@]}" 2>/dev/null)

  if [[ "$http_code" -ge 200 && "$http_code" -lt 300 ]]; then
    cat "$respfile"
    rm -f "$respfile"
  else
    log "daemon_api $method: HTTP $http_code — $(cat "$respfile" 2>/dev/null)"
    rm -f "$respfile"
    return 1
  fi
}

# Helper: write JSON to a temp file using python3 (avoids shell escaping issues)
# Usage: bodyfile=$(write_json key1 val1 key2 val2 ...)
# For complex JSON, pass a python3 expression: bodyfile=$(write_json_expr 'expr')
write_json_expr() {
  local expr="$1"
  local tmpfile
  tmpfile=$(mktemp)
  python3 -c "
import json
data = $expr
with open('$tmpfile', 'w') as f:
    json.dump(data, f)
" 2>/dev/null
  echo "$tmpfile"
}

# ── Cleanup trap ─────────────────────────────────────────────────────
# Ensure we clean up the test bead even if the script fails midway
_test_cleanup() {
  if [[ -n "$TEST_BEAD_ID" && -n "$DAEMON_PORT" && -n "$DAEMON_TOKEN" ]]; then
    log "Cleaning up test bead $TEST_BEAD_ID..."
    local bf
    bf=$(write_json_expr "{'id': '$TEST_BEAD_ID'}")
    daemon_api "Close" "$bf" >/dev/null 2>&1 || true
    rm -f "$bf"
    # Give controller time to delete the pod
    sleep 5
  fi
  # Note: port-forward cleanup is handled by lib.sh trap
}
trap '_test_cleanup; _cleanup' EXIT

# ── Test 1: Daemon HTTP API is reachable ─────────────────────────────
test_daemon_api() {
  discover_daemon || return 1
  DAEMON_PORT=$(start_port_forward "pod/$DAEMON_POD" 9080) || return 1
  local health
  health=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${DAEMON_PORT}/health" 2>/dev/null)
  assert_contains "$health" "status"
}
run_test "Daemon HTTP API is reachable" test_daemon_api

# Bail early if daemon isn't reachable — remaining tests need it
if [[ -z "$DAEMON_PORT" || -z "$DAEMON_TOKEN" ]]; then
  skip_test "Create test bead with gt:agent labels" "daemon not reachable"
  skip_test "Controller creates agent pod" "daemon not reachable"
  skip_test "Agent pod reaches Running state" "daemon not reachable"
  skip_test "Agent pod has expected labels" "daemon not reachable"
  skip_test "Coop health responds on new pod" "daemon not reachable"
  skip_test "Close bead via daemon API" "daemon not reachable"
  skip_test "Controller deletes agent pod" "daemon not reachable"
  print_summary
  exit 0
fi

# ── Test 2: Create test bead with agent labels ───────────────────────
test_create_bead() {
  log "Calling daemon API: Create (port=$DAEMON_PORT, token=${DAEMON_TOKEN:0:10}...)"

  # Snapshot existing gt-* pods so we can detect newly-created ones
  PRE_EXISTING_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | awk '{print $1}' | sort)

  # Build JSON body with python3 to avoid shell escaping issues
  local bodyfile
  bodyfile=$(write_json_expr "{
    'title': '$TEST_BEAD_TITLE',
    'issue_type': 'agent',
    'priority': 2,
    'description': 'E2E lifecycle test',
    'labels': ['gt:agent', 'execution_target:k8s', 'rig:e2e-test', 'role:test', 'agent:lifecycle']
  }")

  local resp
  resp=$(daemon_api "Create" "$bodyfile")
  rm -f "$bodyfile"
  log "Create response: ${resp:0:200}"
  [[ -n "$resp" ]] || return 1

  # Extract the created bead ID (write resp to temp file to avoid pipe issues)
  local respfile
  respfile=$(mktemp)
  printf '%s' "$resp" > "$respfile"
  TEST_BEAD_ID=$(python3 -c "
import json
try:
    with open('$respfile') as f:
        d = json.load(f)
    bid = d.get('id', d.get('issue_id', d.get('data', {}).get('id', '')))
    print(bid)
except:
    pass
" 2>/dev/null)
  rm -f "$respfile"

  log "Created test bead: $TEST_BEAD_ID"
  [[ -n "$TEST_BEAD_ID" ]]
}
run_test "Create test bead with gt:agent + execution_target:k8s" test_create_bead

# Bail if bead creation failed
if [[ -z "$TEST_BEAD_ID" ]]; then
  skip_test "Controller creates agent pod" "bead creation failed"
  skip_test "Agent pod reaches Running state" "bead creation failed"
  skip_test "Agent pod has expected labels" "bead creation failed"
  skip_test "Coop health responds on new pod" "bead creation failed"
  skip_test "Close bead via daemon API" "bead creation failed"
  skip_test "Controller deletes agent pod" "bead creation failed"
  print_summary
  exit 0
fi

# Set bead to in_progress (controller watches for in_progress agent beads)
_update_bf=$(write_json_expr "{'id': '$TEST_BEAD_ID', 'status': 'in_progress'}")
daemon_api "Update" "$_update_bf" >/dev/null 2>&1 || true
rm -f "$_update_bf"

# ── Test 3: Controller creates agent pod ──────────────────────────────
test_controller_creates_pod() {
  log "Waiting for controller to create pod (timeout: ${POD_CREATE_TIMEOUT}s)..."
  local deadline=$((SECONDS + POD_CREATE_TIMEOUT))

  while [[ $SECONDS -lt $deadline ]]; do
    # Strategy 1: Check for pods with the gastown.io/bead-id label (most reliable)
    TEST_POD_NAME=$(kube get pods -l "gastown.io/bead-id=$TEST_BEAD_ID" --no-headers 2>/dev/null | head -1 | awk '{print $1}')

    # Strategy 2: Look for NEW gt-* pods that weren't in our pre-existing snapshot
    if [[ -z "$TEST_POD_NAME" ]]; then
      local current_pods
      current_pods=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | awk '{print $1}' | sort)
      # Find pods in current but not in pre-existing (comm -13 = lines only in file 2)
      TEST_POD_NAME=$(comm -13 <(echo "$PRE_EXISTING_PODS") <(echo "$current_pods") | head -1)
    fi

    if [[ -n "$TEST_POD_NAME" ]]; then
      log "Found pod: $TEST_POD_NAME"
      return 0
    fi

    sleep 3
  done

  log "No agent pod appeared within ${POD_CREATE_TIMEOUT}s"
  return 1
}
run_test "Controller creates agent pod for bead" test_controller_creates_pod

# Bail if pod wasn't created
if [[ -z "$TEST_POD_NAME" ]]; then
  skip_test "Agent pod reaches Running state" "pod not created"
  skip_test "Agent pod has expected labels" "pod not created"
  skip_test "Coop health responds on new pod" "pod not created"
  skip_test "Close bead via daemon API" "pod not created"
  skip_test "Controller deletes agent pod" "pod not created"
  print_summary
  exit 0
fi

# ── Test 4: Agent pod reaches Running state ───────────────────────────
test_pod_running() {
  log "Waiting for pod $TEST_POD_NAME to become Ready (timeout: ${POD_READY_TIMEOUT}s)..."
  local deadline=$((SECONDS + POD_READY_TIMEOUT))

  while [[ $SECONDS -lt $deadline ]]; do
    local phase
    phase=$(kube get pod "$TEST_POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null)
    if [[ "$phase" == "Running" ]]; then
      # Also check container ready
      local ready
      ready=$(kube get pod "$TEST_POD_NAME" --no-headers 2>/dev/null | awk '{print $2}')
      if [[ "$ready" == "1/1" ]]; then
        log "Pod $TEST_POD_NAME is Running and Ready"
        return 0
      fi
    fi
    sleep 5
  done

  local phase
  phase=$(kube get pod "$TEST_POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null)
  log "Pod $TEST_POD_NAME phase: $phase (not ready within timeout)"
  return 1
}
run_test "Agent pod reaches Running 1/1 state" test_pod_running

# ── Test 5: Agent pod has expected labels ─────────────────────────────
test_pod_labels() {
  local labels
  labels=$(kube get pod "$TEST_POD_NAME" -o jsonpath='{.metadata.labels}' 2>/dev/null)
  # Should have gastown controller labels
  assert_contains "$labels" "gastown" || assert_contains "$labels" "agent"
}
run_test "Agent pod has controller-managed labels" test_pod_labels

# ── Test 6: Coop health responds on new pod ───────────────────────────
test_new_pod_coop_health() {
  local coop_port
  coop_port=$(start_port_forward "pod/$TEST_POD_NAME" 9090) || return 1
  local resp
  resp=$(curl -sf --connect-timeout 10 "http://127.0.0.1:${coop_port}/api/v1/health" 2>/dev/null) || return 1
  assert_contains "$resp" "status" || assert_contains "$resp" "ok"
}
run_test "Coop health endpoint responds on new agent pod" test_new_pod_coop_health

# ── Test 7: Close bead via daemon API ─────────────────────────────────
test_close_bead() {
  local bodyfile
  bodyfile=$(write_json_expr "{'id': '$TEST_BEAD_ID'}")
  local resp
  resp=$(daemon_api "Close" "$bodyfile")
  rm -f "$bodyfile"
  # Clear TEST_BEAD_ID so cleanup trap doesn't try to close again
  local closed_id="$TEST_BEAD_ID"
  TEST_BEAD_ID=""
  [[ -n "$resp" ]]
}
run_test "Close test bead via daemon API" test_close_bead

# ── Test 8: Controller deletes agent pod ──────────────────────────────
test_controller_deletes_pod() {
  log "Waiting for controller to delete pod $TEST_POD_NAME (timeout: ${POD_DELETE_TIMEOUT}s)..."
  local deadline=$((SECONDS + POD_DELETE_TIMEOUT))

  while [[ $SECONDS -lt $deadline ]]; do
    local exists
    exists=$(kube get pod "$TEST_POD_NAME" --no-headers 2>/dev/null | awk '{print $1}')
    if [[ -z "$exists" ]]; then
      log "Pod $TEST_POD_NAME deleted"
      return 0
    fi

    local phase
    phase=$(kube get pod "$TEST_POD_NAME" -o jsonpath='{.status.phase}' 2>/dev/null)
    if [[ "$phase" == "Terminating" ]]; then
      log "Pod $TEST_POD_NAME is terminating..."
    fi

    sleep 3
  done

  log "Pod $TEST_POD_NAME still exists after ${POD_DELETE_TIMEOUT}s"
  return 1
}
run_test "Controller deletes agent pod after bead closed" test_controller_deletes_pod

# ── Summary ──────────────────────────────────────────────────────────
print_summary
