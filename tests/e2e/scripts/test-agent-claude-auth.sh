#!/usr/bin/env bash
# test-agent-claude-auth.sh — Verify Claude Code authenticates through coop.
#
# Validates the full auth chain:
#   K8s secret → credential volume → entrypoint copies to ~/.claude →
#   coop starts Claude Code → Claude Code authenticates → agent reaches idle
#
# Tests:
#   1. K8s credential secret has expected keys (credentials.json)
#   2. Credential file copied to ~/.claude/.credentials.json on pod
#   3. Credential JSON has required OAuth fields
#   4. Coop reports agent type contains "claude"
#   5. Agent reached idle state (auth succeeded, not stuck at startup)
#   6. No auth/credential errors in coop logs
#   7. Startup bypass completed (not stuck at permission dialog)
#   8. Agent session is active (not exited)
#
# Usage:
#   ./scripts/test-agent-claude-auth.sh [NAMESPACE]

MODULE_NAME="agent-claude-auth"
source "$(dirname "$0")/lib.sh"

log "Testing Claude Code authentication through coop in namespace: $E2E_NAMESPACE"

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Credential secret has expected keys" "no running agent pods"
  skip_test "Credential file copied to ~/.claude" "no running agent pods"
  skip_test "Credential JSON has OAuth fields" "no running agent pods"
  skip_test "Coop reports agent type 'claude'" "no running agent pods"
  skip_test "Agent reached idle state (auth OK)" "no running agent pods"
  skip_test "No auth errors in coop logs" "no running agent pods"
  skip_test "Startup bypass completed" "no running agent pods"
  skip_test "Agent session is active" "no running agent pods"
  print_summary
  exit 0
fi

log "Using agent pod: $AGENT_POD"

# ── Test 1: K8s credential secret has expected structure ─────────────
test_secret_structure() {
  # Find the credential secret (may be named claude-credentials or similar)
  local secret_name
  secret_name=$(kube get secrets --no-headers 2>/dev/null | { grep -i "claude-credential" || true; } | head -1 | awk '{print $1}')
  [[ -n "$secret_name" ]] || return 1

  # Verify it has a credentials.json key
  local keys
  keys=$(kube get secret "$secret_name" -o jsonpath='{.data}' 2>/dev/null)
  assert_contains "$keys" "credentials.json"
}
run_test "Credential secret has expected keys (credentials.json)" test_secret_structure

# ── Test 2: Credential file copied to ~/.claude on pod ────────────────
test_credential_copied() {
  # The entrypoint copies from /tmp/claude-credentials/ to ~/.claude/
  kube exec "$AGENT_POD" -- test -f /home/agent/.claude/.credentials.json 2>/dev/null
}
run_test "Credential file exists at ~/.claude/.credentials.json" test_credential_copied

# ── Test 3: Credential JSON has required OAuth fields ─────────────────
test_credential_oauth_fields() {
  local cred_json
  cred_json=$(kube exec "$AGENT_POD" -- cat /home/agent/.claude/.credentials.json 2>/dev/null) || return 1
  [[ -z "$cred_json" ]] && return 1

  # Validate required OAuth fields are present
  local has_fields
  has_fields=$(echo "$cred_json" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    # Claude OAuth credentials need at minimum an access mechanism
    # Check for common OAuth fields
    has_token = 'accessToken' in d or 'access_token' in d
    has_refresh = 'refreshToken' in d or 'refresh_token' in d
    has_key = 'apiKey' in d or 'api_key' in d
    # Need at least one auth method
    if has_token or has_refresh or has_key:
        print('ok')
    else:
        print('missing')
except:
    print('invalid')
" 2>/dev/null)
  assert_eq "$has_fields" "ok"
}
run_test "Credential JSON has required OAuth/API fields" test_credential_oauth_fields

# ── Test 4: Coop reports agent type contains "claude" ─────────────────
COOP_PORT=""
COOP_MAIN_PORT=""

test_coop_agent_type() {
  # Health port (9090) for basic health check
  COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 9090) || return 1
  local health
  health=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/health" 2>/dev/null) || return 1
  # Health response should indicate this is a claude agent
  assert_contains "$health" "claude"
}
run_test "Coop health reports agent type 'claude'" test_coop_agent_type

# ── Test 5: Agent reached idle state (auth + startup succeeded) ───────
test_agent_idle() {
  # Use main API port (8080) for agent state
  COOP_MAIN_PORT=$(start_port_forward "pod/$AGENT_POD" 8080) || return 1
  local agent_resp
  agent_resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_MAIN_PORT}/api/v1/agent" 2>/dev/null) || return 1

  # Extract state — agent should be "idle" (meaning Claude Code started,
  # authenticated, and is waiting for input). Other acceptable states:
  # "working" (actively processing), "tool_use" (executing tool)
  # Bad states: "error", "starting" (stuck), "exited"
  local state
  state=$(echo "$agent_resp" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('state', d.get('status', 'unknown')))
except:
    print('parse_error')
" 2>/dev/null)

  log "Agent state: $state"
  # idle, working, tool_use all mean auth succeeded
  case "$state" in
    idle|working|tool_use|tool_input|prompt) return 0 ;;
    *) return 1 ;;
  esac
}
run_test "Agent reached active state (auth + startup succeeded)" test_agent_idle

# ── Test 6: No auth/credential errors in agent container logs ─────────
test_no_auth_errors() {
  local error_count
  error_count=$(kube logs "$AGENT_POD" --tail=500 2>/dev/null | \
    grep -ci "auth.*fail\|credential.*error\|credential.*invalid\|token.*expired\|unauthorized\|403.*forbidden\|login.*required" || true)
  # Allow 0 auth errors
  assert_eq "${error_count:-0}" "0"
}
run_test "No auth/credential errors in agent logs (last 500 lines)" test_no_auth_errors

# ── Test 7: Startup bypass completed (not stuck at permission dialog) ──
test_startup_bypass() {
  [[ -n "$COOP_MAIN_PORT" ]] || return 1
  local agent_resp
  agent_resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_MAIN_PORT}/api/v1/agent" 2>/dev/null) || return 1

  # If agent is in startup/dialog state, bypass hasn't completed
  local state
  state=$(echo "$agent_resp" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('state', d.get('status', 'unknown')))
except:
    print('parse_error')
" 2>/dev/null)

  # "setup" or "starting" means stuck at startup dialog
  case "$state" in
    setup|starting) return 1 ;;
    *) return 0 ;;
  esac
}
run_test "Startup bypass completed (not stuck at dialog)" test_startup_bypass

# ── Test 8: Agent session is active (not exited) ─────────────────────
test_session_active() {
  [[ -n "$COOP_MAIN_PORT" ]] || return 1
  local agent_resp
  agent_resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_MAIN_PORT}/api/v1/agent" 2>/dev/null) || return 1

  local state
  state=$(echo "$agent_resp" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('state', d.get('status', 'unknown')))
except:
    print('parse_error')
" 2>/dev/null)

  # "exited" means the agent process died — auth may have failed
  [[ "$state" != "exited" ]]
}
run_test "Agent session is active (not exited)" test_session_active

# ── Summary ──────────────────────────────────────────────────────────
print_summary
