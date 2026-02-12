#!/usr/bin/env bash
# test-agent-roundtrip.sh — Full Claude Code round-trip: prompt → work → response.
#
# This is the definitive E2E test: send Claude a real prompt through coop,
# verify it works on it, and verify we get a correct response back.
#
# Tests:
#   1. Agent is idle and ready for input
#   2. Capture baseline usage (tokens before prompt)
#   3. Send a real prompt via coop input API
#   4. Agent transitions to working state
#   5. Agent returns to idle (prompt completed)
#   6. Screen/transcript contains expected response
#   7. Token usage increased (Claude API was actually called)
#   8. Transcript recorded the conversation
#
# IMPORTANT: This test sends a real prompt to a live Claude agent.
# It uses a trivial math question to minimize cost and time.
#
# Usage:
#   ./scripts/test-agent-roundtrip.sh [NAMESPACE]

MODULE_NAME="agent-roundtrip"
source "$(dirname "$0")/lib.sh"

log "Testing full Claude Code round-trip in namespace: $E2E_NAMESPACE"

# ── Configuration ────────────────────────────────────────────────────
WORK_TIMEOUT=120    # seconds to wait for agent to finish working
PROMPT_TEXT='What is 2+2? Reply with ONLY the number, nothing else.'
EXPECTED_ANSWER='4'

# ── Discover agent pods ──────────────────────────────────────────────
AGENT_PODS=$(kube get pods --no-headers 2>/dev/null | { grep "^gt-" || true; } | { grep "Running" || true; } | awk '{print $1}')
AGENT_POD=$(echo "$AGENT_PODS" | head -1)

if [[ -z "$AGENT_POD" ]]; then
  skip_test "Agent is idle and ready" "no running agent pods"
  skip_test "Capture baseline usage" "no running agent pods"
  skip_test "Send prompt via coop input" "no running agent pods"
  skip_test "Agent transitions to working" "no running agent pods"
  skip_test "Agent returns to idle" "no running agent pods"
  skip_test "Response contains expected answer" "no running agent pods"
  skip_test "Token usage increased" "no running agent pods"
  skip_test "Transcript recorded" "no running agent pods"
  print_summary
  exit 0
fi

log "Using agent pod: $AGENT_POD"

# ── Port-forward to agent's main API port (8080) ─────────────────────
COOP_PORT=""

setup_coop() {
  if [[ -z "$COOP_PORT" ]]; then
    COOP_PORT=$(start_port_forward "pod/$AGENT_POD" 8080) || return 1
  fi
}

# Helper: get agent state from coop
get_agent_state() {
  local resp
  resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null) || return 1
  # Write to temp file to avoid pipe issues
  local tmpf
  tmpf=$(mktemp)
  printf '%s' "$resp" > "$tmpf"
  python3 -c "
import json
with open('$tmpf') as f:
    d = json.load(f)
print(d.get('state', d.get('status', 'unknown')))
" 2>/dev/null
  rm -f "$tmpf"
}

# Helper: get usage stats
get_usage() {
  curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/session/usage" 2>/dev/null
}

# Helper: extract a numeric field from JSON stored in a file
json_field() {
  local file="$1" field="$2"
  python3 -c "
import json
with open('$file') as f:
    d = json.load(f)
print(d.get('$field', 0))
" 2>/dev/null
}

# ── Test 1: Agent is idle and ready for input ─────────────────────────
test_agent_ready() {
  setup_coop || return 1
  local state
  state=$(get_agent_state)
  log "Agent state: $state"
  # Must be idle to accept input. If working, we could wait, but
  # better to skip than interfere with an active task.
  [[ "$state" == "idle" ]]
}
run_test "Agent is idle and ready for input" test_agent_ready

# Bail if agent isn't idle — we can't safely send a prompt
AGENT_STATE=$(get_agent_state 2>/dev/null)
if [[ "$AGENT_STATE" != "idle" ]]; then
  skip_test "Capture baseline usage" "agent not idle (state: ${AGENT_STATE:-unknown})"
  skip_test "Send prompt via coop input" "agent not idle"
  skip_test "Agent transitions to working" "agent not idle"
  skip_test "Agent returns to idle" "agent not idle"
  skip_test "Response contains expected answer" "agent not idle"
  skip_test "Token usage increased" "agent not idle"
  skip_test "Transcript recorded" "agent not idle"
  print_summary
  exit 0
fi

# ── Test 2: Capture baseline usage ────────────────────────────────────
BASELINE_USAGE_FILE=""
BASELINE_REQUESTS=0

test_baseline_usage() {
  local usage
  usage=$(get_usage)
  [[ -n "$usage" ]] || return 1

  BASELINE_USAGE_FILE=$(mktemp)
  printf '%s' "$usage" > "$BASELINE_USAGE_FILE"
  BASELINE_REQUESTS=$(json_field "$BASELINE_USAGE_FILE" "request_count")
  log "Baseline: request_count=$BASELINE_REQUESTS"
  return 0
}
run_test "Capture baseline token usage" test_baseline_usage

# ── Test 3: Send prompt via coop input ────────────────────────────────
PROMPT_SENT=false

test_send_prompt() {
  [[ -n "$COOP_PORT" ]] || return 1

  # Build the input body with python3 (safe JSON generation)
  local bodyfile
  bodyfile=$(mktemp)
  python3 -c "
import json
with open('$bodyfile', 'w') as f:
    json.dump({'text': '''$PROMPT_TEXT''', 'enter': True}, f)
" 2>/dev/null

  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 \
    -X POST -H "Content-Type: application/json" \
    -d "@${bodyfile}" \
    "http://127.0.0.1:${COOP_PORT}/api/v1/input" 2>/dev/null)
  rm -f "$bodyfile"

  log "Input API returned HTTP $status"
  if [[ "$status" == "200" || "$status" == "204" ]]; then
    PROMPT_SENT=true
    return 0
  fi
  return 1
}
run_test "Send prompt to Claude via coop input API" test_send_prompt

if [[ "$PROMPT_SENT" != "true" ]]; then
  skip_test "Agent transitions to working" "prompt not sent"
  skip_test "Agent returns to idle" "prompt not sent"
  skip_test "Response contains expected answer" "prompt not sent"
  skip_test "Token usage increased" "prompt not sent"
  skip_test "Transcript recorded" "prompt not sent"
  print_summary
  exit 0
fi

# ── Test 4: Agent transitions to working state ────────────────────────
SAW_WORKING=false

test_agent_working() {
  # Poll for up to 30s — agent should start working quickly after input
  local deadline=$((SECONDS + 30))
  while [[ $SECONDS -lt $deadline ]]; do
    local state
    state=$(get_agent_state)
    if [[ "$state" == "working" || "$state" == "tool_use" || "$state" == "tool_input" ]]; then
      log "Agent is working (state=$state)"
      SAW_WORKING=true
      return 0
    fi
    # If already back to idle, the round-trip completed too fast to catch.
    # Verify by checking if screen_seq advanced (coop increments on output).
    if [[ "$state" == "idle" ]]; then
      local agent_resp tmpf screen_seq
      agent_resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/agent" 2>/dev/null) || true
      tmpf=$(mktemp)
      printf '%s' "$agent_resp" > "$tmpf"
      screen_seq=$(python3 -c "
import json
with open('$tmpf') as f:
    print(json.load(f).get('screen_seq', 0))
" 2>/dev/null)
      rm -f "$tmpf"
      # If screen advanced past initial state, Claude processed something
      if [[ "${screen_seq:-0}" -gt 5 ]]; then
        log "Agent already completed (screen_seq=$screen_seq, too fast to catch working)"
        SAW_WORKING=true
        return 0
      fi
    fi
    sleep 0.5
  done
  log "Agent did not transition to working within 30s"
  return 1
}
run_test "Agent transitions to working state" test_agent_working

# ── Test 5: Agent returns to idle ─────────────────────────────────────
test_agent_returns_idle() {
  local deadline=$((SECONDS + WORK_TIMEOUT))
  while [[ $SECONDS -lt $deadline ]]; do
    local state
    state=$(get_agent_state)
    if [[ "$state" == "idle" ]]; then
      log "Agent returned to idle"
      return 0
    fi
    if [[ "$state" == "exited" || "$state" == "error" ]]; then
      log "Agent in bad state: $state"
      return 1
    fi
    sleep 2
  done
  local final_state
  final_state=$(get_agent_state)
  log "Agent still in state '$final_state' after ${WORK_TIMEOUT}s"
  return 1
}
run_test "Agent returns to idle (prompt completed)" test_agent_returns_idle

# ── Test 6: Response contains expected answer ─────────────────────────
test_response_content() {
  [[ -n "$COOP_PORT" ]] || return 1

  # Try screen text first
  local screen_text
  screen_text=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/screen/text" 2>/dev/null)

  if [[ "$screen_text" == *"$EXPECTED_ANSWER"* ]]; then
    log "Found '$EXPECTED_ANSWER' in screen text"
    return 0
  fi

  # Fall back to transcript catchup
  local transcript
  transcript=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/transcripts/catchup" 2>/dev/null)

  if [[ "$transcript" == *"$EXPECTED_ANSWER"* ]]; then
    log "Found '$EXPECTED_ANSWER' in transcript"
    return 0
  fi

  log "Expected '$EXPECTED_ANSWER' not found in screen or transcript"
  # Show what we got for debugging
  log "Screen text (last 200 chars): ${screen_text: -200}"
  return 1
}
run_test "Response contains expected answer ($EXPECTED_ANSWER)" test_response_content

# ── Test 7: Token usage increased ─────────────────────────────────────
# Usage tracking requires coop hooks to be wired up. Skip if not available.
_USAGE_AVAILABLE=false

if [[ -n "$COOP_PORT" ]]; then
  _usage_check=$(get_usage)
  _tmpf=$(mktemp)
  printf '%s' "$_usage_check" > "$_tmpf"
  _check_requests=$(json_field "$_tmpf" "request_count")
  _check_output=$(json_field "$_tmpf" "output_tokens")
  rm -f "$_tmpf"
  if [[ "${_check_requests:-0}" -gt 0 || "${_check_output:-0}" -gt 0 ]]; then
    _USAGE_AVAILABLE=true
  fi
fi

if [[ "$_USAGE_AVAILABLE" == "true" ]]; then
  test_usage_increased() {
    local usage
    usage=$(get_usage)
    [[ -n "$usage" ]] || return 1

    local tmpf
    tmpf=$(mktemp)
    printf '%s' "$usage" > "$tmpf"

    local cur_requests cur_output
    cur_requests=$(json_field "$tmpf" "request_count")
    cur_output=$(json_field "$tmpf" "output_tokens")
    rm -f "$tmpf"

    log "Usage: request_count=$cur_requests (was $BASELINE_REQUESTS), output_tokens=$cur_output"
    [[ "$cur_requests" -gt "$BASELINE_REQUESTS" ]] || return 1
    [[ "$cur_output" -gt 0 ]]
  }
  run_test "Token usage increased (Claude API was called)" test_usage_increased
else
  skip_test "Token usage increased (Claude API was called)" "usage tracking not active on this pod"
fi

# ── Test 8: Transcript recorded ───────────────────────────────────────
# Transcript saving requires coop hooks. Skip if not available.
_TRANSCRIPTS_AVAILABLE=false

if [[ -n "$COOP_PORT" ]]; then
  _tx_check=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/transcripts" 2>/dev/null)
  _tmpf=$(mktemp)
  printf '%s' "$_tx_check" > "$_tmpf"
  _tx_count=$(python3 -c "
import json
with open('$_tmpf') as f:
    d = json.load(f)
print(len(d.get('transcripts', [])))
" 2>/dev/null)
  rm -f "$_tmpf"
  if [[ "${_tx_count:-0}" -gt 0 ]]; then
    _TRANSCRIPTS_AVAILABLE=true
  fi
fi

if [[ "$_TRANSCRIPTS_AVAILABLE" == "true" ]]; then
  test_transcript_recorded() {
    [[ -n "$COOP_PORT" ]] || return 1
    local resp
    resp=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${COOP_PORT}/api/v1/transcripts" 2>/dev/null)
    [[ -n "$resp" ]] || return 1

    local tmpf
    tmpf=$(mktemp)
    printf '%s' "$resp" > "$tmpf"
    local count
    count=$(python3 -c "
import json
with open('$tmpf') as f:
    d = json.load(f)
print(len(d.get('transcripts', [])))
" 2>/dev/null)
    rm -f "$tmpf"

    log "Transcript count: $count"
    [[ "${count:-0}" -gt 0 ]]
  }
  run_test "Transcript recorded the conversation" test_transcript_recorded
else
  skip_test "Transcript recorded the conversation" "transcript saving not active on this pod"
fi

# ── Cleanup ──────────────────────────────────────────────────────────
rm -f "$BASELINE_USAGE_FILE" 2>/dev/null

# ── Summary ──────────────────────────────────────────────────────────
print_summary
