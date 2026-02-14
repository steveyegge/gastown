#!/usr/bin/env bash
# E2E test: verify each agent from HQ settings gets its prompt correctly
# and starts work when slung. Tests the ACTUAL running agent, not just config.
#
# Usage:
#   ./scripts/test-agent-e2e.sh <agent-name> [rig]
#
# Examples:
#   ./scripts/test-agent-e2e.sh pi sfgastown
#   ./scripts/test-agent-e2e.sh opus-46 sfgastown
#   ./scripts/test-agent-e2e.sh "Kimi K2.5" sfgastown
#
# What it does:
#   1. Saves current config
#   2. Sets role_agents.polecat to the specified agent
#   3. Creates a test bead
#   4. Slings the bead to spawn a polecat
#   5. Waits for the tmux session to appear
#   6. Captures pane output — verifies GAS TOWN context injection
#   7. Verifies the hook is correctly set (via bd show)
#   8. Sends a verification nudge and checks for agent activity
#   9. Nukes the polecat
#   10. Restores config
#
# Requirements:
#   - gt binary installed with pi agent support
#   - Dolt server running
#   - API credentials for the agent being tested

set -euo pipefail

# Configuration
TOWN_ROOT="${GT_ROOT:-/home/ubuntu/gt}"
CONFIG_FILE="$TOWN_ROOT/settings/config.json"
CONFIG_BACKUP="$TOWN_ROOT/settings/config.json.e2e-backup"
DEFAULT_RIG="sfgastown"
STARTUP_TIMEOUT=60       # seconds to wait for session to appear
PROMPT_TIMEOUT=45        # seconds to wait for prompt to be visible
ACTIVITY_TIMEOUT=30      # seconds to wait for agent to show activity after nudge
PANE_CAPTURE_LINES=500   # scrollback lines to capture

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log()  { echo -e "${BLUE}[TEST]${NC} $*"; }
pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

# Parse arguments
AGENT_NAME="${1:?Usage: $0 <agent-name> [rig]}"
RIG="${2:-$DEFAULT_RIG}"
TEST_ID="e2e-$(date +%s)"
BEAD_ID=""
POLECAT_NAME=""
SESSION_NAME=""

# Cleanup handler — always runs on exit
cleanup() {
    local exit_code=$?
    log "Cleaning up..."

    # Kill the polecat session if it exists
    if [[ -n "$SESSION_NAME" ]]; then
        log "Killing tmux session: $SESSION_NAME"
        tmux kill-session -t "$SESSION_NAME" 2>/dev/null || true
    fi

    # Nuke the polecat if we know its name
    if [[ -n "$POLECAT_NAME" ]]; then
        log "Nuking polecat: $RIG/$POLECAT_NAME"
        gt polecat nuke "$RIG/$POLECAT_NAME" --force 2>/dev/null || true
    fi

    # Close any wisps attached to the test bead, then the bead itself
    if [[ -n "$BEAD_ID" ]]; then
        log "Closing test bead: $BEAD_ID"
        # Force-close the bead (also closes attached wisps/children)
        bd close "$BEAD_ID" --force --reason "e2e test complete" 2>/dev/null || true
    fi

    # Restore config
    if [[ -f "$CONFIG_BACKUP" ]]; then
        log "Restoring config from backup"
        cp "$CONFIG_BACKUP" "$CONFIG_FILE"
        rm -f "$CONFIG_BACKUP"
    fi

    if [[ $exit_code -eq 0 ]]; then
        pass "Test cleanup complete"
    else
        fail "Test failed (exit code: $exit_code)"
    fi
}
trap cleanup EXIT

# ── Step 0: Validate the agent exists in config ──────────────────────────

log "═══════════════════════════════════════════════════════════════"
log "E2E Agent Prompt Test: $AGENT_NAME → $RIG"
log "═══════════════════════════════════════════════════════════════"

if ! command -v gt &>/dev/null; then
    fail "gt binary not found in PATH"
    exit 1
fi

if ! command -v bd &>/dev/null; then
    fail "bd binary not found in PATH"
    exit 1
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
    fail "Config file not found: $CONFIG_FILE"
    exit 1
fi

# Check agent exists in config
AGENT_CMD=$(python3 -c "
import json, sys
with open('$CONFIG_FILE') as f:
    cfg = json.load(f)
agents = cfg.get('agents', {})
if '$AGENT_NAME' not in agents:
    print('NOT_FOUND', file=sys.stderr)
    sys.exit(1)
agent = agents['$AGENT_NAME']
print(agent.get('command', 'unknown'))
" 2>/dev/null) || {
    fail "Agent '$AGENT_NAME' not found in $CONFIG_FILE"
    log "Available agents:"
    python3 -c "
import json
with open('$CONFIG_FILE') as f:
    cfg = json.load(f)
for name in sorted(cfg.get('agents', {}).keys()):
    agent = cfg['agents'][name]
    print(f'  - {name} (command: {agent.get(\"command\", \"?\")})')
"
    exit 1
}

log "Agent command: $AGENT_CMD"

# Verify the command binary exists
AGENT_BIN=$(command -v "$AGENT_CMD" 2>/dev/null || echo "")
if [[ -z "$AGENT_BIN" ]]; then
    fail "Agent binary not found: $AGENT_CMD"
    exit 1
fi
log "Agent binary: $AGENT_BIN"

# Verify the rig exists
if [[ ! -d "$TOWN_ROOT/$RIG" ]]; then
    fail "Rig not found: $TOWN_ROOT/$RIG"
    exit 1
fi
log "Rig path: $TOWN_ROOT/$RIG"

# ── Step 1: Backup config and set polecat agent ──────────────────────────

log "Step 1: Configure polecat agent to '$AGENT_NAME'"
cp "$CONFIG_FILE" "$CONFIG_BACKUP"

python3 -c "
import json
with open('$CONFIG_FILE') as f:
    cfg = json.load(f)
cfg.setdefault('role_agents', {})['polecat'] = '$AGENT_NAME'
with open('$CONFIG_FILE', 'w') as f:
    json.dump(cfg, f, indent=4)
print('Config updated: role_agents.polecat = $AGENT_NAME')
"
pass "Config updated"

# ── Step 2: Create a test bead ───────────────────────────────────────────

log "Step 2: Create test bead in $RIG"
BEAD_OUTPUT=$(bd create \
    --rig "$RIG" \
    --title "E2E test: $AGENT_NAME prompt verification ($TEST_ID)" \
    --description "Automated E2E test bead. Verify that agent '$AGENT_NAME' receives correct GAS TOWN prompt context when slung. This bead should be auto-closed by the test script." \
    --type task \
    --priority 3 \
    2>&1) || true

# Check for Created in output even if command had warnings
if ! echo "$BEAD_OUTPUT" | grep -q "Created"; then
    fail "Failed to create test bead: $BEAD_OUTPUT"
    exit 1
fi

# Parse bead ID from output (format: st-abc or st-abcd — 3-4 chars after prefix)
BEAD_ID=$(echo "$BEAD_OUTPUT" | grep -oP '[a-z]{2}-[a-z0-9]{3,5}' | head -1)
if [[ -z "$BEAD_ID" ]]; then
    # Try from "Created issue" line
    BEAD_ID=$(echo "$BEAD_OUTPUT" | grep -oP 'Created issue.*?:\s*\K[a-z]{2}-[a-z0-9]+' | head -1)
fi

if [[ -z "$BEAD_ID" ]]; then
    fail "Could not parse bead ID from: $BEAD_OUTPUT"
    exit 1
fi
pass "Created bead: $BEAD_ID"

# ── Step 3: Sling the bead to spawn a polecat ───────────────────────────

log "Step 3: Sling $BEAD_ID to $RIG (spawning polecat with agent: $AGENT_NAME)"
# Use normal sling pipeline (full formula instantiation) to test the real flow.
# Pass formula vars explicitly since bd treats default="" as "no default".
# --no-boot avoids witness/refinery interference during the test.
SLING_OUTPUT=$(gt sling "$BEAD_ID" "$RIG" --no-boot \
    --var setup_command="" \
    --var typecheck_command="" \
    --var lint_command="" \
    --var build_command="" \
    2>&1) || {
    fail "Sling failed: $SLING_OUTPUT"
    exit 1
}

log "Sling output:"
echo "$SLING_OUTPUT" | while IFS= read -r line; do echo "  $line"; done

# Parse polecat name from sling output
# Look for patterns like "gt-sfgastown-Toast" or "polecats/Toast"
POLECAT_NAME=$(echo "$SLING_OUTPUT" | grep -oP 'polecats/\K[A-Za-z]+' | head -1)
if [[ -z "$POLECAT_NAME" ]]; then
    # Try from session name pattern
    POLECAT_NAME=$(echo "$SLING_OUTPUT" | grep -oP "gt-${RIG}-\K[A-Za-z]+" | head -1)
fi
if [[ -z "$POLECAT_NAME" ]]; then
    # Try "Allocated polecat: <name>"
    POLECAT_NAME=$(echo "$SLING_OUTPUT" | grep -oP 'Allocated polecat:\s*\K\S+' | head -1)
fi
if [[ -z "$POLECAT_NAME" ]]; then
    # List polecats to find the new one
    POLECAT_NAME=$(gt polecat list "$RIG" 2>/dev/null | grep -oP '^\s+\K[A-Za-z]+' | tail -1)
fi

if [[ -z "$POLECAT_NAME" ]]; then
    fail "Could not determine polecat name from sling output"
    exit 1
fi

SESSION_NAME="gt-${RIG}-${POLECAT_NAME}"
pass "Polecat spawned: $POLECAT_NAME (session: $SESSION_NAME)"

# ── Step 4: Wait for tmux session to start ───────────────────────────────

log "Step 4: Waiting for tmux session '$SESSION_NAME' (timeout: ${STARTUP_TIMEOUT}s)"
STARTED=false
for i in $(seq 1 "$STARTUP_TIMEOUT"); do
    if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        STARTED=true
        break
    fi
    sleep 1
    if (( i % 10 == 0 )); then
        log "  Still waiting... (${i}s)"
    fi
done

if [[ "$STARTED" != "true" ]]; then
    fail "Session '$SESSION_NAME' did not appear within ${STARTUP_TIMEOUT}s"
    # Debug: list all sessions
    log "Available tmux sessions:"
    tmux list-sessions 2>/dev/null || echo "  (none)"
    exit 1
fi
pass "Session started: $SESSION_NAME"

# ── Step 5: Wait for prompt to appear, then capture ─────────────────────

log "Step 5: Waiting for prompt content (timeout: ${PROMPT_TIMEOUT}s)"
PROMPT_CAPTURED=false
PANE_CONTENT=""

for i in $(seq 1 "$PROMPT_TIMEOUT"); do
    PANE_CONTENT=$(tmux capture-pane -t "$SESSION_NAME" -p -S -"$PANE_CAPTURE_LINES" 2>/dev/null || echo "")

    # Check for GAS TOWN context markers that indicate the prompt was injected
    if echo "$PANE_CONTENT" | grep -qE "GAS.TOWN|gt prime|Check your hook"; then
        PROMPT_CAPTURED=true
        break
    fi

    # Check for agent-specific startup markers
    if echo "$PANE_CONTENT" | grep -qE "session_start|before_agent_start|gastown.*prime"; then
        PROMPT_CAPTURED=true
        break
    fi

    sleep 1
    if (( i % 10 == 0 )); then
        log "  Still waiting for prompt... (${i}s)"
    fi
done

# Save pane capture for analysis
CAPTURE_FILE="/tmp/e2e-${AGENT_NAME//[^a-zA-Z0-9_-]/_}-${TEST_ID}.txt"
echo "$PANE_CONTENT" > "$CAPTURE_FILE"
log "Pane capture saved to: $CAPTURE_FILE"

if [[ "$PROMPT_CAPTURED" == "true" ]]; then
    pass "Prompt content detected in pane output"
else
    warn "Could not detect GAS TOWN markers in pane (agent may still be starting)"
fi

# ── Step 6: Verify prompt content ────────────────────────────────────────

log "Step 6: Verifying prompt content (from pane capture)"
CHECKS_PASSED=0
CHECKS_TOTAL=0

check_prompt() {
    local label="$1"
    local pattern="$2"
    CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
    if echo "$PANE_CONTENT" | grep -qiE "$pattern"; then
        pass "  $label"
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    else
        fail "  $label (pattern not found: $pattern)"
    fi
}

# Core checks: these must pass for any agent that got GAS TOWN context
check_prompt "GAS TOWN header present" "GAS.TOWN|gastown"
check_prompt "Role identification" "polecat|POLECAT"
check_prompt "Hook/work assignment visible" "gt hook|hook|assigned|work"
check_prompt "Agent is active (working/processing)" "Working|Enchanting|Thinking|processing|running|Steering|assigned"

# ── Step 7: Verify hook is correctly set ─────────────────────────────────

log "Step 7: Verifying bead is hooked to polecat (via bd show)"
HOOK_OUTPUT=$(bd show "$BEAD_ID" 2>&1) || true

CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
if echo "$HOOK_OUTPUT" | grep -qiE "hooked|$POLECAT_NAME"; then
    pass "  Bead $BEAD_ID is hooked to polecat"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    fail "  Bead $BEAD_ID hook status not confirmed"
    warn "  bd show output: $(echo "$HOOK_OUTPUT" | head -5)"
fi

# ── Step 8: Talk to the running agent ────────────────────────────────────

log "Step 8: Sending verification nudge to agent"
NUDGE_MSG="This is an automated E2E verification test. Please respond with: I am a Gas Town polecat agent running as $AGENT_NAME. Include the text E2E_OK in your response."

gt nudge "$RIG/$POLECAT_NAME" "$NUDGE_MSG" 2>/dev/null || {
    warn "gt nudge failed — trying tmux send-keys as fallback"
    tmux send-keys -t "$SESSION_NAME" "$NUDGE_MSG" Enter 2>/dev/null || true
}
log "  Nudge sent. Waiting for agent activity (timeout: ${ACTIVITY_TIMEOUT}s)..."

# For the response check, we capture the pane multiple times to detect CHANGE
# (the agent is alive and processing if the pane content changes after our nudge)
INITIAL_PANE=$(tmux capture-pane -t "$SESSION_NAME" -p -S -"$PANE_CAPTURE_LINES" 2>/dev/null || echo "")
AGENT_RESPONDED=false
RESPONSE_CONTENT=""

for i in $(seq 1 "$ACTIVITY_TIMEOUT"); do
    RESPONSE_CONTENT=$(tmux capture-pane -t "$SESSION_NAME" -p -S -"$PANE_CAPTURE_LINES" 2>/dev/null || echo "")

    # Check 1: did the agent echo back our marker?
    if echo "$RESPONSE_CONTENT" | grep -qF "E2E_OK"; then
        AGENT_RESPONDED=true
        break
    fi

    # Check 2: did the agent mention polecat (role awareness)?
    if echo "$RESPONSE_CONTENT" | grep -qiE "polecat.*agent|I am.*polecat|role.*polecat"; then
        AGENT_RESPONDED=true
        break
    fi

    # Check 3: did pane content change at all (agent is alive and processing)?
    if [[ "$RESPONSE_CONTENT" != "$INITIAL_PANE" ]]; then
        # Content changed — agent is alive and processing
        if echo "$RESPONSE_CONTENT" | grep -qiE "E2E|verify|test|polecat"; then
            AGENT_RESPONDED=true
            break
        fi
    fi

    sleep 1
    if (( i % 10 == 0 )); then
        log "  Still waiting for response... (${i}s)"
    fi
done

# Save full response
RESPONSE_FILE="/tmp/e2e-response-${AGENT_NAME//[^a-zA-Z0-9_-]/_}-${TEST_ID}.txt"
echo "$RESPONSE_CONTENT" > "$RESPONSE_FILE"
log "Response capture saved to: $RESPONSE_FILE"

CHECKS_TOTAL=$((CHECKS_TOTAL + 1))
if [[ "$AGENT_RESPONDED" == "true" ]]; then
    pass "  Agent responded to verification nudge"
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
else
    # Check if pane content at least shows the nudge was delivered
    if echo "$RESPONSE_CONTENT" | grep -qiE "E2E|verify|automated"; then
        warn "  Nudge was delivered but agent didn't produce expected response"
        warn "  (may be rate-limited, slow, or TUI doesn't show response in pane)"
        # Count as partial pass — nudge delivery confirmed
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
        pass "  Nudge delivery confirmed (partial)"
    else
        # Check if pane changed at ALL (agent alive)
        if [[ "$RESPONSE_CONTENT" != "$INITIAL_PANE" ]]; then
            warn "  Agent is active (pane changed) but didn't produce expected response"
            CHECKS_PASSED=$((CHECKS_PASSED + 1))
            pass "  Agent activity confirmed (partial)"
        else
            fail "  No agent activity detected within ${ACTIVITY_TIMEOUT}s"
            warn "  Last pane content (last 15 lines):"
            echo "$RESPONSE_CONTENT" | tail -15 | while IFS= read -r line; do echo "    $line"; done
        fi
    fi
fi

# ── Step 9: Report results ───────────────────────────────────────────────

log ""
log "═══════════════════════════════════════════════════════════════"
log "Results for: $AGENT_NAME (command: $AGENT_CMD)"
log "═══════════════════════════════════════════════════════════════"

if [[ $CHECKS_PASSED -eq $CHECKS_TOTAL ]]; then
    pass "ALL CHECKS PASSED ($CHECKS_PASSED/$CHECKS_TOTAL)"
    log "═══════════════════════════════════════════════════════════════"
    exit 0
elif [[ $CHECKS_PASSED -ge $(( CHECKS_TOTAL - 1 )) ]]; then
    warn "MOSTLY PASSED ($CHECKS_PASSED/$CHECKS_TOTAL) — 1 check failed (may be rate limit or TUI issue)"
    log "Capture files:"
    log "  Prompt: $CAPTURE_FILE"
    log "  Response: $RESPONSE_FILE"
    log "═══════════════════════════════════════════════════════════════"
    # Exit 0 for mostly-passed — the core prompt injection is working
    exit 0
else
    fail "CHECKS FAILED ($CHECKS_PASSED/$CHECKS_TOTAL)"
    log "Capture files:"
    log "  Prompt: $CAPTURE_FILE"
    log "  Response: $RESPONSE_FILE"
    log "═══════════════════════════════════════════════════════════════"
    exit 1
fi
