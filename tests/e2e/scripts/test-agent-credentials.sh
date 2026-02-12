#!/usr/bin/env bash
# test-agent-credentials.sh — Verify credential mounting and persistence.
#
# Tests:
#   1. K8s secret for claude-credentials exists in namespace
#   2. Pod has credentials volume
#   3. Pod has credentials volume mount on container
#   4. Credential file exists on pod (/tmp/claude-credentials/credentials.json)
#   5. Credential file contains valid JSON
#   6. Coop health endpoint reports agent type as "claude"
#   7. Workspace PVC is mounted for session persistence
#   8. Claude config directory exists (~/.claude)
#
# Usage:
#   ./scripts/test-agent-credentials.sh [NAMESPACE]

MODULE_NAME="agent-credentials"
source "$(dirname "$0")/lib.sh"

log "Testing credential mounting and persistence in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Credential secret exists" "no running agent pods"
  skip_test "Credential volume mounted" "no running agent pods"
  skip_test "Credential file exists" "no running agent pods"
  print_summary
  exit 0
fi

log "Using agent pod: $AGENT_POD"

# ── Test 1: K8s secret for credentials exists ─────────────────────────
test_credential_secret_exists() {
  local secrets
  secrets=$(kube get secrets --no-headers 2>/dev/null | { grep "claude-credentials" || true; })
  [[ -n "$secrets" ]]
}
run_test "K8s secret 'claude-credentials' exists in namespace" test_credential_secret_exists

# ── Test 2: Pod has credentials volume ────────────────────────────────
test_credential_volume() {
  local volumes
  volumes=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.volumes[*].name}' 2>/dev/null)
  assert_contains "$volumes" "credential" || assert_contains "$volumes" "claude"
}
run_test "Agent pod has credentials volume" test_credential_volume

# ── Test 3: Pod container has credentials volume mount ────────────────
test_credential_volume_mount() {
  local mounts
  mounts=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.containers[0].volumeMounts[*].name}' 2>/dev/null)
  assert_contains "$mounts" "credential" || assert_contains "$mounts" "claude"
}
run_test "Agent pod container has credentials volume mount" test_credential_volume_mount

# ── Test 4: Credential file exists on pod ─────────────────────────────
test_credential_file_exists() {
  kube exec "$AGENT_POD" -- ls /tmp/claude-credentials/credentials.json >/dev/null 2>&1
}
run_test "Credential file exists at /tmp/claude-credentials/credentials.json" test_credential_file_exists

# ── Test 5: Credential file has valid JSON ────────────────────────────
test_credential_json_valid() {
  local content
  content=$(kube exec "$AGENT_POD" -- cat /tmp/claude-credentials/credentials.json 2>/dev/null || echo "")
  [[ -z "$content" ]] && return 1
  echo "$content" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null
}
run_test "Credential file contains valid JSON" test_credential_json_valid

# ── Test 6: Coop health reports agent type ────────────────────────────
COOP_PORT=""

test_coop_reports_agent_type() {
  COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 9090) || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/health" 2>/dev/null)
  # Health endpoint should indicate agent type is "claude"
  assert_contains "$resp" "claude"
}
run_test "Coop health endpoint reports agent type 'claude'" test_coop_reports_agent_type

# ── Test 7: Workspace volume exists (PVC or emptyDir) ────────────────
test_workspace_volume() {
  local volumes
  volumes=$(kube get pod "$AGENT_POD" -o jsonpath='{.spec.volumes[*].name}' 2>/dev/null)
  assert_contains "$volumes" "workspace"
}
run_test "Workspace volume exists (PVC or emptyDir)" test_workspace_volume

# ── Test 8: Claude config directory exists ────────────────────────────
test_claude_config_dir() {
  kube exec "$AGENT_POD" -- test -d /home/agent/.claude 2>/dev/null
}
run_test "Claude config directory exists (~/.claude)" test_claude_config_dir

# ── Summary ──────────────────────────────────────────────────────────
print_summary
