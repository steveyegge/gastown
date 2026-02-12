#!/usr/bin/env bash
# test-slack-bot-health.sh — Verify Slack bot sidecar health and NATS connection.
#
# Tests:
#   1. Daemon pod has slack-bot container
#   2. slack-bot container is running
#   3. slack-bot container has no crash restarts
#   4. slack-bot has NATS connection config
#   5. slack-bot has Slack credentials
#   6. slack-bot appears in NATS connections
#   7. slack-bot logs show NATS subscription
#   8. slack-bot logs show no fatal errors
#
# Usage:
#   ./scripts/test-slack-bot-health.sh [NAMESPACE]

MODULE_NAME="slack-bot-health"
source "$(dirname "$0")/lib.sh"

NS="$E2E_NAMESPACE"

log "Testing Slack bot health in $NS"

# ── Discover daemon pod ──────────────────────────────────────────────
DAEMON_POD=$(kube get pods --no-headers 2>/dev/null | grep "daemon" | grep -v "dolt\|nats\|clusterctl" | grep "Running" | head -1 | awk '{print $1}')

log "Daemon pod: ${DAEMON_POD:-none}"

# ── Early exit: skip all if slack-bot sidecar not deployed ────────────
if [[ -z "$DAEMON_POD" ]]; then
  skip_all "no daemon pod found"
  exit 0
fi

CONTAINERS=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
if [[ "$CONTAINERS" != *"slack-bot"* ]]; then
  skip_all "slack-bot sidecar not deployed"
  exit 0
fi

# ── Discover NATS service ───────────────────────────────────────────
NATS_SVC=$(kube get svc --no-headers 2>/dev/null | grep "nats" | head -1 | awk '{print $1}')

log "NATS service: ${NATS_SVC:-none}"

# ── Test 1: Daemon pod has slack-bot container ───────────────────────
test_has_slack_bot_container() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local containers
  containers=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[*].name}' 2>/dev/null)
  assert_contains "$containers" "slack-bot"
}
run_test "Daemon pod has slack-bot container" test_has_slack_bot_container

# ── Test 2: slack-bot container is running ───────────────────────────
test_slack_bot_running() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local state
  state=$(kube get pod "$DAEMON_POD" -o jsonpath='{.status.containerStatuses[?(@.name=="slack-bot")].state}' 2>/dev/null)
  assert_contains "$state" "running"
}
run_test "slack-bot container is running" test_slack_bot_running

# ── Test 3: slack-bot container has no crash restarts ────────────────
test_slack_bot_no_restarts() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local restarts
  restarts=$(kube get pod "$DAEMON_POD" -o jsonpath='{.status.containerStatuses[?(@.name=="slack-bot")].restartCount}' 2>/dev/null)
  assert_eq "${restarts:-0}" "0"
}
run_test "slack-bot container has no crash restarts" test_slack_bot_no_restarts

# ── Test 4: slack-bot has NATS connection config ─────────────────────
test_slack_bot_nats_config() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local env_vars
  env_vars=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[?(@.name=="slack-bot")].env[*].name}' 2>/dev/null)
  assert_contains "$env_vars" "BD_NATS_URL" || assert_contains "$env_vars" "BD_NATS_TOKEN"
}
run_test "slack-bot has NATS connection config" test_slack_bot_nats_config

# ── Test 5: slack-bot has Slack credentials ──────────────────────────
test_slack_bot_credentials() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local env_vars
  env_vars=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[?(@.name=="slack-bot")].env[*].name}' 2>/dev/null)
  local env_from
  env_from=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[?(@.name=="slack-bot")].envFrom[*].secretRef.name}' 2>/dev/null)
  local vol_mounts
  vol_mounts=$(kube get pod "$DAEMON_POD" -o jsonpath='{.spec.containers[?(@.name=="slack-bot")].volumeMounts[*].name}' 2>/dev/null)
  # Check for SLACK_BOT_TOKEN in direct env, or a secret reference, or a volume mount with "slack" in the name
  assert_contains "$env_vars" "SLACK_BOT_TOKEN" || \
    assert_contains "$env_from" "slack" || \
    assert_contains "$vol_mounts" "slack"
}
run_test "slack-bot has Slack credentials" test_slack_bot_credentials

# ── Test 6: slack-bot appears in NATS connections ────────────────────
NATS_PORT=""

test_slack_bot_nats_connection() {
  [[ -n "$NATS_SVC" ]] || return 1
  NATS_PORT=$(start_port_forward "svc/$NATS_SVC" 8222) || return 1
  local connz
  connz=$(curl -sf "http://localhost:${NATS_PORT}/connz" 2>/dev/null)
  [[ -n "$connz" ]] || return 1
  local names
  names=$(echo "$connz" | python3 -c "
import sys,json
d=json.load(sys.stdin)
for c in d.get('connections',[]):
    n=c.get('name','')
    if n: print(n)
" 2>/dev/null)
  assert_contains "$names" "beads-slack-bot"
}
run_test "slack-bot appears in NATS connections" test_slack_bot_nats_connection

# ── Test 7: slack-bot logs show NATS subscription ────────────────────
test_slack_bot_logs_nats() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local nats_lines
  nats_lines=$(kube logs "$DAEMON_POD" -c slack-bot --tail=100 2>/dev/null \
    | grep -ci "nats\|subscribe\|connected" || true)
  assert_gt "${nats_lines:-0}" 0
}
run_test "slack-bot logs show NATS subscription" test_slack_bot_logs_nats

# ── Test 8: slack-bot logs show no fatal errors ──────────────────────
test_slack_bot_no_fatal() {
  [[ -n "$DAEMON_POD" ]] || return 1
  local fatal_count
  fatal_count=$(kube logs "$DAEMON_POD" -c slack-bot --tail=100 2>/dev/null \
    | grep -ci "panic\|fatal" || true)
  assert_eq "${fatal_count:-0}" "0"
}
run_test "slack-bot logs show no fatal errors" test_slack_bot_no_fatal

# ── Summary ──────────────────────────────────────────────────────────
print_summary
