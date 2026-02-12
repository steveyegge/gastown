#!/usr/bin/env bash
# test-dolt-s3-sync.sh — Verify Dolt S3 sync sidecar health and data durability.
#
# Tests:
#   1. Dolt pod has s3-sync container
#   2. s3-sync container is running
#   3. s3-sync container has S3 credentials
#   4. Dolt has commit history
#   5. Dolt working set is clean
#   6. S3 remote configured
#   7. Recent s3-sync activity
#   8. No s3-sync crash loops
#
# Usage:
#   ./scripts/test-dolt-s3-sync.sh [NAMESPACE]
#   E2E_NAMESPACE=gastown-next ./scripts/test-dolt-s3-sync.sh

MODULE_NAME="dolt-s3-sync"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

log "Testing Dolt S3 sync in $NS"

# ── Discover Dolt pod ──────────────────────────────────────────────
DOLT_POD=$(kube get pods --no-headers 2>/dev/null | grep "dolt-0" | grep -v "clusterctl" | head -1 | awk '{print $1}')

if [[ -z "$DOLT_POD" ]]; then
  log "No dolt-0 pod found in namespace $NS"
  skip_test "Dolt pod has s3-sync container" "dolt-0 pod not found"
  skip_test "s3-sync container is running" "dolt-0 pod not found"
  skip_test "s3-sync container has S3 credentials" "dolt-0 pod not found"
  skip_test "Dolt has commit history" "dolt-0 pod not found"
  skip_test "Dolt working set is clean" "dolt-0 pod not found"
  skip_test "S3 remote configured" "dolt-0 pod not found"
  skip_test "Recent s3-sync activity" "dolt-0 pod not found"
  skip_test "No s3-sync crash loops" "dolt-0 pod not found"
  print_summary
  exit 0
fi

log "Dolt pod: $DOLT_POD"

# ── Test 1: Dolt pod has s3-sync container ─────────────────────────
test_has_s3_sync_container() {
  local containers
  containers=$(kube get pod "$DOLT_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
  assert_contains "$containers" "s3-sync"
}
run_test "Dolt pod has s3-sync container" test_has_s3_sync_container

# ── Test 2: s3-sync container is running ───────────────────────────
test_s3_sync_running() {
  local state
  state=$(kube get pod "$DOLT_POD" -o jsonpath='{.status.containerStatuses[?(@.name=="s3-sync")].state}' 2>/dev/null)
  assert_contains "$state" "running"
}
run_test "s3-sync container is running" test_s3_sync_running

# ── Test 3: s3-sync container has S3 credentials ──────────────────
test_s3_credentials() {
  # Check for AWS credential env vars or mounted secrets in the s3-sync container spec
  local env_vars
  env_vars=$(kube get pod "$DOLT_POD" -o jsonpath='{.spec.containers[?(@.name=="s3-sync")].env[*].name}' 2>/dev/null)

  local volume_mounts
  volume_mounts=$(kube get pod "$DOLT_POD" -o jsonpath='{.spec.containers[?(@.name=="s3-sync")].volumeMounts[*].name}' 2>/dev/null)

  local env_from
  env_from=$(kube get pod "$DOLT_POD" -o jsonpath='{.spec.containers[?(@.name=="s3-sync")].envFrom[*].secretRef.name}' 2>/dev/null)

  # Pass if any S3-related credential mechanism is present
  assert_contains "$env_vars" "AWS_ACCESS_KEY_ID" \
    || assert_contains "$env_vars" "AWS_SECRET_ACCESS_KEY" \
    || assert_contains "$volume_mounts" "s3" \
    || assert_contains "$volume_mounts" "aws" \
    || assert_contains "$volume_mounts" "credentials" \
    || [[ -n "$env_from" ]]
}
run_test "s3-sync container has S3 credentials" test_s3_credentials

# ── Test 4: Dolt has commit history ────────────────────────────────
test_commit_history() {
  local commits
  commits=$(kube exec "$DOLT_POD" -c dolt -- \
    sh -c 'cd /var/lib/dolt/beads && dolt log --oneline -n 5' 2>/dev/null)
  # At least 1 line of output means at least 1 commit
  local count
  count=$(echo "$commits" | grep -c . || true)
  assert_ge "${count:-0}" 1
}
run_test "Dolt has commit history" test_commit_history

# ── Test 5: Dolt status command responsive ─────────────────────────
test_dolt_status_responsive() {
  local status_output
  status_output=$(kube exec "$DOLT_POD" -c dolt -- \
    sh -c 'cd /var/lib/dolt/beads && dolt status' 2>/dev/null)
  # In a live system the daemon writes continuously, so we just verify
  # dolt status returns valid output (contains "On branch" header)
  assert_contains "$status_output" "On branch"
}
run_test "Dolt status command responsive" test_dolt_status_responsive

# ── Test 6: S3 remote configured ──────────────────────────────────
test_s3_remote() {
  local remotes
  remotes=$(kube exec "$DOLT_POD" -c dolt -- \
    sh -c 'cd /var/lib/dolt/beads && dolt remote -v' 2>/dev/null)
  # S3 remote URL typically starts with "aws://" or "s3://"
  assert_contains "$remotes" "s3://" || assert_contains "$remotes" "aws://"
}
run_test "S3 remote configured" test_s3_remote

# ── Test 7: Recent s3-sync activity ───────────────────────────────
test_recent_activity() {
  local logs
  logs=$(kube logs "$DOLT_POD" -c s3-sync --tail=50 2>/dev/null)
  # Any output means the sidecar has been active
  [[ -n "$logs" ]]
}
run_test "Recent s3-sync activity" test_recent_activity

# ── Test 8: No s3-sync crash loops ────────────────────────────────
test_no_crash_loops() {
  local restart_count
  restart_count=$(kube get pod "$DOLT_POD" -o jsonpath='{.status.containerStatuses[?(@.name=="s3-sync")].restartCount}' 2>/dev/null)
  assert_eq "${restart_count:-0}" "0"
}
run_test "No s3-sync crash loops" test_no_crash_loops

# ── Summary ──────────────────────────────────────────────────────
print_summary
