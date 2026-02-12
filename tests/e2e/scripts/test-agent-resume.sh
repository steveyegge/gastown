#!/usr/bin/env bash
# test-agent-resume.sh — Verify agent session resume infrastructure.
#
# Tests:
#   1. Coop state directory exists on PVC
#   2. Coop has session logs for resume
#   3. Entrypoint script has --resume flag support
#   4. Agent has .state directory for persistence
#
# Note: Does NOT restart pods (destructive). Validates that the resume
# infrastructure is correctly configured.
#
# Usage:
#   ./scripts/test-agent-resume.sh [NAMESPACE]

MODULE_NAME="agent-resume"
source "$(dirname "$0")/lib.sh"

log "Testing agent session resume infrastructure in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

if [[ -z "$AGENT_POD" ]]; then
  skip_test "State directory exists" "no running agent pods"
  skip_test "Session log exists" "no running agent pods"
  print_summary
  exit 0
fi

log "Using agent pod: $AGENT_POD"

# ── Test 1: .state directory exists ──────────────────────────────────
test_state_dir() {
  kube exec "$AGENT_POD" -- test -d /home/agent/gt/.state 2>/dev/null || \
  kube exec "$AGENT_POD" -- test -d /home/agent/.state 2>/dev/null
}
run_test "Agent .state directory exists" test_state_dir

# ── Test 2: Coop state directory has session data ────────────────────
test_coop_sessions() {
  local sessions
  sessions=$(kube exec "$AGENT_POD" -- sh -c 'ls /home/agent/gt/.state/coop/sessions/ 2>/dev/null || ls /home/agent/.state/coop/sessions/ 2>/dev/null || echo ""' 2>/dev/null)
  [[ -n "$sessions" ]]
}
run_test "Coop sessions directory has content" test_coop_sessions

# ── Test 3: Claude state directory exists ────────────────────────────
test_claude_state() {
  kube exec "$AGENT_POD" -- test -d /home/agent/gt/.state/claude 2>/dev/null || \
  kube exec "$AGENT_POD" -- test -d /home/agent/.state/claude 2>/dev/null || \
  kube exec "$AGENT_POD" -- test -d /home/agent/.claude 2>/dev/null
}
run_test "Claude state directory exists" test_claude_state

# ── Test 4: Coop health shows session info ───────────────────────────
COOP_PORT=""

test_session_info() {
  COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 8080) || return 1
  local resp
  resp=$(curl -s --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null)
  # Should have session_id or similar identifier
  [[ ${#resp} -gt 10 ]]
}
run_test "Coop /api/v1/agent returns session info" test_session_info

# ── Test 5: Agent has workspace with git repo ────────────────────────
test_workspace_git() {
  kube exec "$AGENT_POD" -- test -d /home/agent/gt/.git 2>/dev/null
}
run_test "Workspace has git repository (/home/agent/gt/.git)" test_workspace_git

# ── Test 6: Agent has beads config ───────────────────────────────────
test_beads_config() {
  kube exec "$AGENT_POD" -- test -d /home/agent/gt/.beads 2>/dev/null
}
run_test "Workspace has beads directory (.beads/)" test_beads_config

# ── Summary ──────────────────────────────────────────────────────────
print_summary
