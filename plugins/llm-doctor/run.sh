#!/usr/bin/env bash
# llm-doctor/run.sh — Diagnose LLM API failures using local Ollama.
#
# Probes the configured LLM provider. On failure, gathers diagnostics,
# optionally feeds them to a local Ollama model for classification,
# and escalates to the Overseer.
#
# Usage: ./run.sh [--force] [--dry-run]
#
# --force    Run diagnosis even if API is healthy (for testing)
# --dry-run  Print diagnosis but don't escalate
#
# Model selection: Set LLM_DOCTOR_OLLAMA_MODEL to override, or let
# resolve-model.sh auto-discover the best available model.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOWN_ROOT="${GT_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"

# --- Configuration -----------------------------------------------------------

PROBE_MODEL="${LLM_DOCTOR_PROBE_MODEL:-claude-haiku-4-5-20251001}"
OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"
# Model resolved dynamically — see resolve-model.sh
source "$SCRIPT_DIR/resolve-model.sh"
API_BASE="${ANTHROPIC_BASE_URL:-https://api.anthropic.com}"
PROMPT_FILE="$SCRIPT_DIR/prompts/diagnose.txt"
STATE_DIR="$TOWN_ROOT/.llm-doctor"
LAST_HEALTHY_FILE="$STATE_DIR/last-healthy"
LAST_DIAGNOSIS_FILE="$STATE_DIR/last-diagnosis"
CONSECUTIVE_FAIL_FILE="$STATE_DIR/consecutive-failures"

mkdir -p "$STATE_DIR"

# --- Argument parsing ---------------------------------------------------------

FORCE=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --force)   FORCE=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--force] [--dry-run]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# --- Helpers ------------------------------------------------------------------

log() { echo "[llm-doctor] $*"; }

timestamp() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }

increment_failures() {
    local count=0
    [[ -f "$CONSECUTIVE_FAIL_FILE" ]] && count=$(cat "$CONSECUTIVE_FAIL_FILE")
    count=$((count + 1))
    echo "$count" > "$CONSECUTIVE_FAIL_FILE"
    echo "$count"
}

reset_failures() {
    echo "0" > "$CONSECUTIVE_FAIL_FILE"
}

get_failure_count() {
    [[ -f "$CONSECUTIVE_FAIL_FILE" ]] && cat "$CONSECUTIVE_FAIL_FILE" || echo "0"
}

# --- Step 1: Probe the API ---------------------------------------------------

log "Probing $API_BASE ..."

# Determine auth method: API key (x-api-key) vs OAuth token (Bearer).
# Claude Max users have ANTHROPIC_AUTH_TOKEN; API users have ANTHROPIC_API_KEY.
AUTH_HEADERS=()
AUTH_METHOD="none"
if [[ -n "${ANTHROPIC_AUTH_TOKEN:-}" ]]; then
    AUTH_HEADERS=(-H "Authorization: Bearer ${ANTHROPIC_AUTH_TOKEN}")
    AUTH_METHOD="oauth"
elif [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
    AUTH_HEADERS=(-H "x-api-key: ${ANTHROPIC_API_KEY}")
    AUTH_METHOD="api-key"
else
    AUTH_HEADERS=(-H "x-api-key: missing")
    AUTH_METHOD="none"
fi

log "Auth method: $AUTH_METHOD"

# Capture both HTTP code and response body for error analysis.
PROBE_TMPFILE=$(mktemp)
trap "rm -f $PROBE_TMPFILE" EXIT

HTTP_CODE=$(curl -s -o "$PROBE_TMPFILE" -w '%{http_code}' \
    -X POST "${API_BASE}/v1/messages" \
    "${AUTH_HEADERS[@]}" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d "{\"model\":\"$PROBE_MODEL\",\"max_tokens\":1,\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}" \
    --connect-timeout 10 \
    --max-time 20 \
    2>/dev/null || echo "000")

RESPONSE_BODY=$(cat "$PROBE_TMPFILE" 2>/dev/null | head -c 2000 || true)

log "Probe result: HTTP $HTTP_CODE"

# --- Step 2: Decide if we need to diagnose ------------------------------------

NEEDS_DIAGNOSIS=false
FAILURE_TYPE=""

case "$HTTP_CODE" in
    200|201)
        log "API healthy"
        date -u +%s > "$LAST_HEALTHY_FILE"
        reset_failures
        if ! $FORCE; then
            exit 0
        fi
        log "--force: running diagnosis anyway"
        FAILURE_TYPE="forced-test"
        NEEDS_DIAGNOSIS=true
        ;;
    429)
        # Defer to rate-limit-watchdog — don't duplicate estop logic.
        log "Rate limited (429) — deferring to rate-limit-watchdog"
        exit 0
        ;;
    401)
        FAILURE_TYPE="auth-error"
        NEEDS_DIAGNOSIS=true
        ;;
    403)
        FAILURE_TYPE="forbidden"
        NEEDS_DIAGNOSIS=true
        ;;
    500|502|503|504)
        FAILURE_TYPE="api-server-error"
        NEEDS_DIAGNOSIS=true
        ;;
    000)
        FAILURE_TYPE="network-unreachable"
        NEEDS_DIAGNOSIS=true
        ;;
    *)
        FAILURE_TYPE="unexpected-$HTTP_CODE"
        NEEDS_DIAGNOSIS=true
        ;;
esac

if ! $NEEDS_DIAGNOSIS; then
    exit 0
fi

FAIL_COUNT=$(increment_failures)
log "Failure detected: $FAILURE_TYPE (consecutive: $FAIL_COUNT)"

# --- Step 3: Gather diagnostics -----------------------------------------------

DIAG=""

# 3a. Basic info
DIAG+="TIMESTAMP: $(timestamp)"$'\n'
DIAG+="FAILURE_TYPE: $FAILURE_TYPE"$'\n'
DIAG+="HTTP_CODE: $HTTP_CODE"$'\n'
DIAG+="API_BASE: $API_BASE"$'\n'
DIAG+="CONSECUTIVE_FAILURES: $FAIL_COUNT"$'\n'

# 3b. Last healthy timestamp
if [[ -f "$LAST_HEALTHY_FILE" ]]; then
    LAST_HEALTHY=$(cat "$LAST_HEALTHY_FILE")
    NOW=$(date -u +%s)
    MINUTES_AGO=$(( (NOW - LAST_HEALTHY) / 60 ))
    DIAG+="LAST_HEALTHY: ${MINUTES_AGO} minutes ago"$'\n'
else
    DIAG+="LAST_HEALTHY: unknown (never recorded)"$'\n'
fi

# 3c. Response body (truncated)
if [[ -n "$RESPONSE_BODY" ]]; then
    DIAG+="RESPONSE_BODY: $RESPONSE_BODY"$'\n'
fi

# 3d. DNS resolution
API_HOST=$(echo "$API_BASE" | sed -E 's|https?://||' | cut -d/ -f1 | cut -d: -f1)
DNS_RESULT=$(dig +short "$API_HOST" 2>/dev/null | head -3 || echo "dig failed")
DIAG+="DNS_LOOKUP ($API_HOST): $DNS_RESULT"$'\n'

# 3e. Network connectivity
PING_RESULT=$(ping -c 1 -W 3 8.8.8.8 2>&1 | tail -1 || echo "ping failed")
DIAG+="INTERNET_PING: $PING_RESULT"$'\n'

# 3f. TLS check
TLS_RESULT=$(echo | openssl s_client -connect "$API_HOST:443" -servername "$API_HOST" 2>&1 \
    | grep -E "Verify return code|subject=" | head -2 || echo "TLS check failed")
DIAG+="TLS_CHECK: $TLS_RESULT"$'\n'

# 3g. Auth check (redacted)
DIAG+="AUTH_METHOD: $AUTH_METHOD"$'\n'
if [[ "$AUTH_METHOD" == "oauth" ]]; then
    TOKEN_PREFIX="${ANTHROPIC_AUTH_TOKEN:0:12}..."
    DIAG+="AUTH_TOKEN: present ($TOKEN_PREFIX)"$'\n'
elif [[ "$AUTH_METHOD" == "api-key" ]]; then
    if [[ "${ANTHROPIC_API_KEY}" == sk-ant-* ]]; then
        KEY_PREFIX="${ANTHROPIC_API_KEY:0:12}..."
        DIAG+="API_KEY: present ($KEY_PREFIX)"$'\n'
    else
        DIAG+="API_KEY: present (non-standard prefix: ${ANTHROPIC_API_KEY:0:6}...)"$'\n'
    fi
else
    DIAG+="AUTH: NOT SET (no ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN)"$'\n'
fi

# 3h. Proxy configuration
if [[ -n "${HTTPS_PROXY:-}" ]]; then
    DIAG+="HTTPS_PROXY: $HTTPS_PROXY"$'\n'
fi
if [[ -n "${HTTP_PROXY:-}" ]]; then
    DIAG+="HTTP_PROXY: $HTTP_PROXY"$'\n'
fi

# 3i. Active agent count
# Count agent sessions across all gt tmux sockets
AGENT_COUNT=0
for sock in /tmp/tmux-$(id -u)/gt-*; do
    [[ -S "$sock" ]] || continue
    SOCK_NAME=$(basename "$sock")
    COUNT=$(tmux -L "$SOCK_NAME" list-sessions 2>/dev/null | wc -l | tr -d ' ' || echo 0)
    AGENT_COUNT=$((AGENT_COUNT + COUNT))
done
DIAG+="ACTIVE_AGENT_SESSIONS: $AGENT_COUNT"$'\n'

# 3j. Estop status
if [[ -f "$TOWN_ROOT/ESTOP" ]]; then
    DIAG+="ESTOP: ACTIVE — $(cat "$TOWN_ROOT/ESTOP" | head -1)"$'\n'
else
    DIAG+="ESTOP: not active"$'\n'
fi

log "Diagnostics gathered"

# --- Step 4: Diagnose with Ollama (if available) ------------------------------

DIAGNOSIS=""
USED_OLLAMA=false

# Resolve best available Ollama model
OLLAMA_MODEL=""
if OLLAMA_MODEL=$(resolve_ollama_model 2>/dev/null); then
    log "Ollama available — running diagnosis with $OLLAMA_MODEL"

    PROMPT=$(cat "$PROMPT_FILE")
    FULL_PROMPT="${PROMPT}"$'\n'"${DIAG}"

    # Escape for JSON
    ESCAPED_PROMPT=$(printf '%s' "$FULL_PROMPT" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')

    OLLAMA_RESPONSE=$(curl -s -X POST "$OLLAMA_URL/api/generate" \
        -H "content-type: application/json" \
        -d "{\"model\":\"$OLLAMA_MODEL\",\"prompt\":$ESCAPED_PROMPT,\"stream\":false,\"options\":{\"temperature\":0.1,\"num_predict\":300}}" \
        --max-time 90 2>/dev/null || echo "")

    if [[ -n "$OLLAMA_RESPONSE" ]]; then
        DIAGNOSIS=$(echo "$OLLAMA_RESPONSE" | python3 -c 'import json,sys; r=json.load(sys.stdin); print(r.get("response",""))' 2>/dev/null || echo "")
        if [[ -n "$DIAGNOSIS" ]]; then
            USED_OLLAMA=true
            log "Ollama diagnosis complete"
        fi
    fi
fi

# Fallback: shell-based classification if Ollama unavailable or failed
if ! $USED_OLLAMA; then
    log "Ollama unavailable — using shell-based diagnosis"
    case "$FAILURE_TYPE" in
        network-unreachable)
            if echo "$PING_RESULT" | grep -q "fail"; then
                DIAGNOSIS="CLASSIFICATION: network-down
ROOT CAUSE: No internet connectivity — ping to 8.8.8.8 failed.
SUGGESTED FIX:
- Check network connection and router
- Check VPN status if applicable
URGENCY: critical"
            elif echo "$DNS_RESULT" | grep -q "fail"; then
                DIAGNOSIS="CLASSIFICATION: dns-failure
ROOT CAUSE: DNS resolution failed for $API_HOST.
SUGGESTED FIX:
- Check DNS settings (try: dig $API_HOST)
- Try alternate DNS (8.8.8.8, 1.1.1.1)
URGENCY: critical"
            else
                DIAGNOSIS="CLASSIFICATION: network-down
ROOT CAUSE: Cannot reach $API_BASE — network or firewall issue.
SUGGESTED FIX:
- Check firewall rules
- Test: curl -v $API_BASE/v1/messages
URGENCY: critical"
            fi
            ;;
        auth-error)
            if [[ "$AUTH_METHOD" == "oauth" ]]; then
                DIAGNOSIS="CLASSIFICATION: auth-invalid
ROOT CAUSE: API returned 401 — OAuth token is invalid or expired.
SUGGESTED FIX:
- Re-authenticate: claude login
- Check ANTHROPIC_AUTH_TOKEN is current
- Claude Max tokens expire — may need refresh
URGENCY: critical"
            elif [[ "$AUTH_METHOD" == "api-key" ]]; then
                DIAGNOSIS="CLASSIFICATION: auth-invalid
ROOT CAUSE: API returned 401 — API key is invalid or expired.
SUGGESTED FIX:
- Check ANTHROPIC_API_KEY in environment
- Rotate key at console.anthropic.com
- Verify key has not been revoked
URGENCY: critical"
            else
                DIAGNOSIS="CLASSIFICATION: auth-invalid
ROOT CAUSE: API returned 401 — no credentials configured.
SUGGESTED FIX:
- Set ANTHROPIC_API_KEY (API users) or ANTHROPIC_AUTH_TOKEN (Claude Max)
- Claude Max: run 'claude login' to authenticate
URGENCY: critical"
            fi
            ;;
        forbidden)
            DIAGNOSIS="CLASSIFICATION: auth-expired
ROOT CAUSE: API returned 403 — key lacks required permissions or account issue.
SUGGESTED FIX:
- Check account status at console.anthropic.com
- Verify API key permissions and billing
URGENCY: high"
            ;;
        api-server-error)
            DIAGNOSIS="CLASSIFICATION: api-outage
ROOT CAUSE: API returned $HTTP_CODE — Anthropic service is experiencing issues.
SUGGESTED FIX:
- Check status.anthropic.com for incidents
- Wait and retry — server errors are typically transient
- If persistent (>15 min), consider switching to Bedrock/Vertex provider
URGENCY: high"
            ;;
        *)
            DIAGNOSIS="CLASSIFICATION: unknown
ROOT CAUSE: Unexpected HTTP $HTTP_CODE from API.
SUGGESTED FIX:
- Check response body for details
- Test manually: curl -v $API_BASE/v1/messages
URGENCY: medium"
            ;;
    esac
fi

# --- Step 5: Save diagnosis ---------------------------------------------------

{
    echo "--- LLM Doctor Diagnosis ---"
    echo "Time: $(timestamp)"
    echo "Diagnosed by: $(if $USED_OLLAMA; then echo "Ollama ($OLLAMA_MODEL)"; else echo "shell fallback"; fi)"
    echo ""
    echo "$DIAGNOSIS"
    echo ""
    echo "--- Raw Diagnostics ---"
    echo "$DIAG"
} > "$LAST_DIAGNOSIS_FILE"

log "Diagnosis saved to $LAST_DIAGNOSIS_FILE"

# --- Step 6: Escalate ---------------------------------------------------------

if $DRY_RUN; then
    log "DRY RUN — would escalate:"
    cat "$LAST_DIAGNOSIS_FILE"
    exit 0
fi

# Determine severity from diagnosis
ESCALATION_SEVERITY="high"
if echo "$DIAGNOSIS" | grep -q "URGENCY: critical"; then
    ESCALATION_SEVERITY="critical"
elif echo "$DIAGNOSIS" | grep -q "URGENCY: medium"; then
    ESCALATION_SEVERITY="medium"
fi

# Extract classification line for subject
CLASSIFICATION=$(echo "$DIAGNOSIS" | grep "^CLASSIFICATION:" | head -1 | sed 's/CLASSIFICATION: //')
CLASSIFICATION="${CLASSIFICATION:-unknown}"

# Only escalate on first failure or every 3rd consecutive failure (avoid spam)
if [[ "$FAIL_COUNT" -eq 1 ]] || [[ $((FAIL_COUNT % 3)) -eq 0 ]]; then
    log "Escalating ($ESCALATION_SEVERITY): $CLASSIFICATION"

    gt escalate "LLM API: $CLASSIFICATION ($FAILURE_TYPE)" \
        --severity "$ESCALATION_SEVERITY" \
        --reason "$(cat "$LAST_DIAGNOSIS_FILE")" 2>/dev/null || true

    # Also mail the Overseer directly for critical issues
    if [[ "$ESCALATION_SEVERITY" == "critical" ]]; then
        gt mail send --human -s "LLM DOWN: $CLASSIFICATION" --stdin <<MAILBODY
LLM Doctor detected an API failure requiring attention.

$DIAGNOSIS

Consecutive failures: $FAIL_COUNT
Last healthy: $(if [[ -f "$LAST_HEALTHY_FILE" ]]; then echo "${MINUTES_AGO:-?} minutes ago"; else echo "unknown"; fi)
Diagnosed by: $(if $USED_OLLAMA; then echo "Ollama ($OLLAMA_MODEL)"; else echo "shell fallback"; fi)

Run \`cat $LAST_DIAGNOSIS_FILE\` for full diagnostics.
MAILBODY
    fi
else
    log "Suppressing escalation (failure $FAIL_COUNT — escalates on 1st and every 3rd)"
fi

# --- Step 7: Record plugin run ------------------------------------------------

RESULT_LABEL="failure"
[[ "$FAILURE_TYPE" == "forced-test" ]] && RESULT_LABEL="success"

bd create --title "llm-doctor: $CLASSIFICATION ($FAILURE_TYPE)" -t chore --ephemeral \
    -l "type:plugin-run,plugin:llm-doctor,result:$RESULT_LABEL" \
    -d "HTTP $HTTP_CODE | $CLASSIFICATION | failures: $FAIL_COUNT" \
    --silent 2>/dev/null || true

log "Done"
