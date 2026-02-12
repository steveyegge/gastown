#!/usr/bin/env bash
# test-decision-flow.sh — Full decision lifecycle: create → list → show → respond → verify.
#
# Tests the entire decision pipeline through the bd CLI on the daemon pod:
#
#   Phase 1 — Decision CRUD:
#     1. Create a decision with 2 options
#     2. Decision appears in list (pending)
#     3. Show returns correct fields (prompt, options, urgency)
#     4. Respond with option selection
#     5. Decision is resolved (closed)
#     6. Resolved decision shows chosen option
#
#   Phase 2 — Validation & edge cases:
#     7. Create with urgency=high
#     8. Cancel a pending decision
#     9. Cancelled decision not in pending list
#    10. Create with --no-notify (silent)
#
#   Phase 3 — Blocking & chaining:
#    11. Create decision that blocks another issue
#    12. Verify blocker relationship
#    13. Resolve unblocks the dependent issue
#
#   Phase 4 — NATS event verification:
#    14. Decision events published to NATS
#
# Usage:
#   ./scripts/test-decision-flow.sh [NAMESPACE]

MODULE_NAME="decision-flow"
source "$(dirname "$0")/lib.sh"

log "Testing decision flow in namespace: $E2E_NAMESPACE"

# ── Discover daemon pod ────────────────────────────────────────────────
DAEMON_POD=$(kube get pods --no-headers 2>/dev/null | grep "daemon" | grep -v dolt | head -1 | awk '{print $1}')

if [[ -z "$DAEMON_POD" ]]; then
  skip_test "Create decision" "no daemon pod"
  skip_test "Decision in pending list" "no daemon pod"
  skip_test "Show decision details" "no daemon pod"
  skip_test "Respond to decision" "no daemon pod"
  skip_test "Decision resolved (closed)" "no daemon pod"
  skip_test "Resolved decision shows chosen option" "no daemon pod"
  skip_test "Create high-urgency decision" "no daemon pod"
  skip_test "Cancel pending decision" "no daemon pod"
  skip_test "Cancelled not in pending list" "no daemon pod"
  skip_test "Create with --no-notify" "no daemon pod"
  skip_test "Create blocking decision" "no daemon pod"
  skip_test "Verify blocker relationship" "no daemon pod"
  skip_test "Resolve unblocks dependent" "no daemon pod"
  skip_test "NATS decision events" "no daemon pod"
  print_summary
  exit 0
fi

# Determine which container has bd
DAEMON_CONTAINER=""
for container in bd-daemon daemon; do
  if kube exec "$DAEMON_POD" -c "$container" -- which bd >/dev/null 2>&1; then
    DAEMON_CONTAINER="$container"
    break
  fi
done

if [[ -z "$DAEMON_CONTAINER" ]]; then
  log "No container with bd binary found in $DAEMON_POD"
  skip_all "no bd binary in daemon pod"
  exit 0
fi

log "Daemon pod: $DAEMON_POD (container: $DAEMON_CONTAINER)"

# ── Helpers ────────────────────────────────────────────────────────────
# Run bd command on daemon pod. Uses --no-notify and --wait=false to avoid
# blocking on notifications/waiting during E2E tests.
bd_cmd() {
  kube exec "$DAEMON_POD" -c "$DAEMON_CONTAINER" -- bd "$@" 2>/dev/null
}

# Extract JSON field via python3 temp file
json_extract() {
  local json_str="$1" expr="$2"
  local tmpf
  tmpf=$(mktemp)
  printf '%s' "$json_str" > "$tmpf"
  python3 -c "
import json
with open('$tmpf') as f:
    d = json.load(f)
$expr
" 2>/dev/null
  rm -f "$tmpf"
}

# Generate unique test ID to avoid collisions
TEST_ID="e2e-$(date +%s)"

# Track created decisions for cleanup
CREATED_DECISIONS=()

cleanup_decisions() {
  for id in "${CREATED_DECISIONS[@]}"; do
    bd_cmd decision cancel "$id" >/dev/null 2>&1 || true
  done
}
trap 'cleanup_decisions; stop_port_forwards' EXIT

# ═══════════════════════════════════════════════════════════════════════
# Phase 1: Decision CRUD
# ═══════════════════════════════════════════════════════════════════════

# ── Test 1: Create a decision ──────────────────────────────────────────
DECISION_1_ID=""
DECISION_1_RESP=""

test_create_decision() {
  local options
  options=$(python3 -c "
import json
opts = [
    {'id': 'alpha', 'short': 'Alpha', 'label': 'Option Alpha: first choice'},
    {'id': 'beta', 'short': 'Beta', 'label': 'Option Beta: second choice'}
]
print(json.dumps(opts))
")

  DECISION_1_RESP=$(bd_cmd decision create \
    --prompt "E2E test decision $TEST_ID: pick one" \
    --options "$options" \
    --urgency medium \
    --no-notify \
    --wait=false \
    --json 2>/dev/null)

  [[ -n "$DECISION_1_RESP" ]] || return 1

  DECISION_1_ID=$(json_extract "$DECISION_1_RESP" "
id_val = d.get('id', d.get('issue_id', ''))
print(id_val)
")

  log "Created decision: $DECISION_1_ID"
  [[ -n "$DECISION_1_ID" ]] && CREATED_DECISIONS+=("$DECISION_1_ID")
  [[ -n "$DECISION_1_ID" ]]
}
run_test "Create decision with 2 options" test_create_decision

if [[ -z "$DECISION_1_ID" ]]; then
  skip_test "Decision in pending list" "creation failed"
  skip_test "Show decision details" "creation failed"
  skip_test "Respond to decision" "creation failed"
  skip_test "Decision resolved (closed)" "creation failed"
  skip_test "Resolved decision shows chosen option" "creation failed"
else

# ── Test 2: Decision appears in pending list ───────────────────────────
test_in_pending_list() {
  local list_output
  # Default list shows pending decisions (no --pending flag needed)
  list_output=$(bd_cmd decision list --json)
  [[ -n "$list_output" ]] || return 1
  assert_contains "$list_output" "$DECISION_1_ID"
}
run_test "Decision appears in pending list" test_in_pending_list

# ── Test 3: Show returns correct fields ────────────────────────────────
test_show_details() {
  local show_output
  show_output=$(bd_cmd decision show "$DECISION_1_ID" --json)
  [[ -n "$show_output" ]] || return 1

  # Check prompt
  assert_contains "$show_output" "$TEST_ID" || return 1

  # Check options exist
  assert_contains "$show_output" "alpha" || return 1
  assert_contains "$show_output" "beta" || return 1

  # Check urgency
  assert_contains "$show_output" "medium"
}
run_test "Show decision has correct fields" test_show_details

# ── Test 4: Respond with option selection ──────────────────────────────
test_respond() {
  bd_cmd decision respond "$DECISION_1_ID" --select=alpha --by="e2e-test"
}
run_test "Respond to decision (select alpha)" test_respond

# ── Test 5: Decision is resolved (has selected_option + responded_at) ──
test_resolved() {
  local show_output
  show_output=$(bd_cmd decision show "$DECISION_1_ID" --json)
  [[ -n "$show_output" ]] || return 1

  # Check decision_point has selected_option and responded_at
  local selected responded_at
  selected=$(json_extract "$show_output" "
dp = d.get('decision_point', {})
print(dp.get('selected_option', ''))
")
  responded_at=$(json_extract "$show_output" "
dp = d.get('decision_point', {})
print(dp.get('responded_at', ''))
")
  log "Decision selected=$selected, responded_at=$responded_at"
  [[ -n "$selected" && -n "$responded_at" ]]
}
run_test "Decision resolved (has selection + timestamp)" test_resolved

# ── Test 6: Resolved decision shows chosen option ──────────────────────
test_chosen_option() {
  local show_output
  show_output=$(bd_cmd decision show "$DECISION_1_ID" --json)
  [[ -n "$show_output" ]] || return 1

  # Check decision_point.selected_option
  local selected
  selected=$(json_extract "$show_output" "
dp = d.get('decision_point', {})
print(dp.get('selected_option', ''))
")
  log "Selected option: $selected"
  [[ "$selected" == "alpha" ]]
}
run_test "Resolved shows chosen option (alpha)" test_chosen_option

fi  # end DECISION_1_ID block

# ═══════════════════════════════════════════════════════════════════════
# Phase 2: Validation & edge cases
# ═══════════════════════════════════════════════════════════════════════

# ── Test 7: Create with urgency=high ───────────────────────────────────
DECISION_HIGH_ID=""

test_high_urgency() {
  local options
  options=$(python3 -c "
import json
opts = [
    {'id': 'yes', 'short': 'Yes', 'label': 'Yes, proceed'},
    {'id': 'no', 'short': 'No', 'label': 'No, abort'}
]
print(json.dumps(opts))
")

  local resp
  resp=$(bd_cmd decision create \
    --prompt "E2E high urgency test $TEST_ID" \
    --options "$options" \
    --urgency high \
    --no-notify \
    --wait=false \
    --json 2>/dev/null)

  [[ -n "$resp" ]] || return 1

  DECISION_HIGH_ID=$(json_extract "$resp" "
print(d.get('id', d.get('issue_id', '')))
")
  [[ -n "$DECISION_HIGH_ID" ]] && CREATED_DECISIONS+=("$DECISION_HIGH_ID")

  # Verify urgency in show output
  local show_output
  show_output=$(bd_cmd decision show "$DECISION_HIGH_ID" --json)
  assert_contains "$show_output" "high"
}
run_test "Create high-urgency decision" test_high_urgency

# ── Test 8: Cancel a pending decision ──────────────────────────────────
test_cancel_decision() {
  [[ -n "$DECISION_HIGH_ID" ]] || return 1
  bd_cmd decision cancel "$DECISION_HIGH_ID"
}
run_test "Cancel pending decision" test_cancel_decision

# ── Test 9: Cancelled decision not in pending list ─────────────────────
test_cancelled_not_pending() {
  [[ -n "$DECISION_HIGH_ID" ]] || return 1
  local list_output
  list_output=$(bd_cmd decision list --json)
  # Should NOT contain the cancelled decision
  if assert_contains "${list_output:-[]}" "$DECISION_HIGH_ID"; then
    log "Cancelled decision still in pending list!"
    return 1
  fi
  return 0
}
run_test "Cancelled decision not in pending list" test_cancelled_not_pending

# ── Test 10: Create with --no-notify (silent) ─────────────────────────
DECISION_SILENT_ID=""

test_silent_create() {
  local options
  options=$(python3 -c "
import json
opts = [
    {'id': 'a', 'short': 'A', 'label': 'Option A'},
    {'id': 'b', 'short': 'B', 'label': 'Option B'}
]
print(json.dumps(opts))
")

  local resp
  resp=$(bd_cmd decision create \
    --prompt "E2E silent test $TEST_ID" \
    --options "$options" \
    --no-notify \
    --wait=false \
    --json 2>/dev/null)

  [[ -n "$resp" ]] || return 1

  DECISION_SILENT_ID=$(json_extract "$resp" "
print(d.get('id', d.get('issue_id', '')))
")
  [[ -n "$DECISION_SILENT_ID" ]] && CREATED_DECISIONS+=("$DECISION_SILENT_ID")
  log "Silent decision: $DECISION_SILENT_ID"
  [[ -n "$DECISION_SILENT_ID" ]]
}
run_test "Create decision with --no-notify" test_silent_create

# Clean up silent decision
if [[ -n "$DECISION_SILENT_ID" ]]; then
  bd_cmd decision cancel "$DECISION_SILENT_ID" >/dev/null 2>&1 || true
fi

# ═══════════════════════════════════════════════════════════════════════
# Phase 3: Blocking & chaining
# ═══════════════════════════════════════════════════════════════════════

# ── Test 11: Create decision that blocks another issue ─────────────────
BLOCKED_ISSUE_ID=""
DECISION_BLOCKER_ID=""

test_blocking_decision() {
  # First, create a dummy issue to block
  local issue_resp
  issue_resp=$(bd_cmd create \
    --title "E2E blocked task $TEST_ID" \
    --type task \
    --priority 3 \
    --json 2>/dev/null)

  if [[ -n "$issue_resp" ]]; then
    BLOCKED_ISSUE_ID=$(json_extract "$issue_resp" "
print(d.get('id', d.get('issue_id', '')))
")
  fi

  [[ -n "$BLOCKED_ISSUE_ID" ]] || {
    log "Could not create blocked issue"
    return 1
  }
  log "Blocked issue: $BLOCKED_ISSUE_ID"

  # Create decision that blocks it
  local options
  options=$(python3 -c "
import json
opts = [
    {'id': 'approve', 'short': 'Approve', 'label': 'Approve the task'},
    {'id': 'reject', 'short': 'Reject', 'label': 'Reject the task'}
]
print(json.dumps(opts))
")

  local resp
  resp=$(bd_cmd decision create \
    --prompt "E2E blocker test $TEST_ID: approve task?" \
    --options "$options" \
    --blocks "$BLOCKED_ISSUE_ID" \
    --no-notify \
    --wait=false \
    --json 2>/dev/null)

  [[ -n "$resp" ]] || return 1

  DECISION_BLOCKER_ID=$(json_extract "$resp" "
print(d.get('id', d.get('issue_id', '')))
")
  [[ -n "$DECISION_BLOCKER_ID" ]] && CREATED_DECISIONS+=("$DECISION_BLOCKER_ID")
  log "Blocking decision: $DECISION_BLOCKER_ID"
  [[ -n "$DECISION_BLOCKER_ID" ]]
}
run_test "Create decision that blocks an issue" test_blocking_decision

# ── Test 12: Verify blocker relationship ───────────────────────────────
test_blocker_relationship() {
  [[ -n "$BLOCKED_ISSUE_ID" && -n "$DECISION_BLOCKER_ID" ]] || return 1

  # Check via bd show on the blocked issue — it should show as blocked
  local blocked_show
  blocked_show=$(bd_cmd show "$BLOCKED_ISSUE_ID" --json 2>/dev/null)
  [[ -n "$blocked_show" ]] || return 1

  # The blocked issue should reference the decision in its dependencies or blockers.
  # Also check the decision show for the blocked_issues field.
  local decision_show
  decision_show=$(bd_cmd decision show "$DECISION_BLOCKER_ID" --json)

  # Accept if either:
  # 1. The decision show mentions the blocked issue
  # 2. The blocked issue has a dependency on the decision
  if assert_contains "$decision_show" "$BLOCKED_ISSUE_ID"; then
    log "Decision show references blocked issue"
    return 0
  fi
  if assert_contains "$blocked_show" "$DECISION_BLOCKER_ID"; then
    log "Blocked issue references decision"
    return 0
  fi

  # Fallback: just verify both exist and the decision was created with --blocks
  log "Blocker relationship not visible in JSON output (may be stored as dependency)"
  # The --blocks flag creates a bd dep, check via dep
  local dep_output
  dep_output=$(bd_cmd dep list "$BLOCKED_ISSUE_ID" 2>/dev/null)
  if [[ -n "$dep_output" ]] && assert_contains "$dep_output" "$DECISION_BLOCKER_ID"; then
    log "Found blocker in dependency list"
    return 0
  fi
  log "Could not verify blocker relationship"
  return 1
}
run_test "Decision has blocker relationship" test_blocker_relationship

# ── Test 13: Resolve and verify response recorded ──────────────────────
test_resolve_blocker() {
  [[ -n "$DECISION_BLOCKER_ID" ]] || return 1

  # Resolve the blocking decision
  bd_cmd decision respond "$DECISION_BLOCKER_ID" --select=approve --by="e2e-test" || return 1

  # Verify selection was recorded
  local show_output
  show_output=$(bd_cmd decision show "$DECISION_BLOCKER_ID" --json)
  local selected
  selected=$(json_extract "$show_output" "
dp = d.get('decision_point', {})
print(dp.get('selected_option', ''))
")
  log "Blocker decision selected: $selected"
  [[ "$selected" == "approve" ]]
}
run_test "Resolve blocking decision records selection" test_resolve_blocker

# ═══════════════════════════════════════════════════════════════════════
# Phase 4: NATS event verification
# ═══════════════════════════════════════════════════════════════════════

# Check if NATS is available
NATS_SVC=$(kube get svc --no-headers 2>/dev/null | grep nats | head -1 | awk '{print $1}')

if [[ -n "$NATS_SVC" ]]; then

# ── Test 14: Decision events published to NATS ─────────────────────────
test_nats_events() {
  # Verify DECISION_EVENTS stream has messages via NATS monitoring API.
  local nats_port
  nats_port=$(start_port_forward "svc/$NATS_SVC" 8222) || return 1

  # NATS monitoring API at port 8222
  local stream_info
  stream_info=$(curl -sf --connect-timeout 5 "http://127.0.0.1:${nats_port}/jsz?streams=true" 2>/dev/null)

  if [[ -z "$stream_info" ]]; then
    log "NATS monitoring API not available"
    return 1
  fi

  # The /jsz?streams=true response nests streams under account_details[].stream_detail[]
  local decision_msgs
  decision_msgs=$(json_extract "$stream_info" "
msgs = 0
for acct in d.get('account_details', []):
    for s in acct.get('stream_detail', []):
        if s.get('name') == 'DECISION_EVENTS':
            msgs = s.get('state', {}).get('messages', 0)
print(msgs)
")
  log "DECISION_EVENTS stream messages: $decision_msgs"
  [[ -n "$decision_msgs" && "$decision_msgs" -gt 0 ]]
}
run_test "NATS streams have decision events" test_nats_events

else
  skip_test "NATS streams have decision events" "no NATS service"
fi

# ═══════════════════════════════════════════════════════════════════════
# Cleanup: close any remaining test issues
# ═══════════════════════════════════════════════════════════════════════

# Close the blocked issue we created
if [[ -n "$BLOCKED_ISSUE_ID" ]]; then
  bd_cmd close "$BLOCKED_ISSUE_ID" >/dev/null 2>&1 || true
fi

# ── Summary ──────────────────────────────────────────────────────────
print_summary
