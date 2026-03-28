#!/usr/bin/env bash
# rate-limit-watchdog/run.sh — Auto-estop on API rate limit, auto-thaw when clear.
#
# No LLM calls — just a minimal API probe and shell commands.

set -euo pipefail

# --- Configuration -----------------------------------------------------------
TOWN_ROOT="${GT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
ESTOP_FILE="$TOWN_ROOT/ESTOP"
PROBE_MODEL="claude-haiku-4-5-20251001"
RATE_LIMIT_REASON="API rate limited (auto-watchdog)"

# --- Preflight ---------------------------------------------------------------
if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ANTHROPIC_API_KEY not set — cannot probe API"
    exit 1
fi

# --- Probe API ---------------------------------------------------------------
# Minimal request: 1 max_token to cheapest model.
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "https://api.anthropic.com/v1/messages" \
    -H "x-api-key: $ANTHROPIC_API_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d "{\"model\":\"$PROBE_MODEL\",\"max_tokens\":1,\"messages\":[{\"role\":\"user\",\"content\":\"ping\"}]}" \
    --connect-timeout 10 \
    --max-time 15 \
    2>/dev/null || echo "000")

echo "API probe: HTTP $HTTP_CODE"

# --- Decision ----------------------------------------------------------------
case "$HTTP_CODE" in
    429)
        # Rate limited — freeze if not already frozen.
        if [ ! -f "$ESTOP_FILE" ]; then
            echo "Rate limit detected — triggering estop"
            gt estop -r "$RATE_LIMIT_REASON"
            echo "result:estop-triggered"
        else
            echo "Rate limit detected — estop already active"
            echo "result:already-frozen"
        fi
        ;;
    200|201)
        # API healthy — thaw if we were the ones who froze it.
        if [ -f "$ESTOP_FILE" ]; then
            # Only thaw auto-triggered estops with our specific reason.
            if grep -q "auto" "$ESTOP_FILE" 2>/dev/null || grep -q "rate limit" "$ESTOP_FILE" 2>/dev/null; then
                echo "API healthy — thawing (rate limit cleared)"
                gt thaw
                echo "result:thawed"
            else
                echo "API healthy — estop active but not rate-limit (skipping thaw)"
                echo "result:manual-estop-preserved"
            fi
        else
            echo "API healthy — no action needed"
            echo "result:healthy"
        fi
        ;;
    000)
        # Network error — can't reach API. Don't estop (might be local network).
        echo "Warning: API unreachable (network error)"
        echo "result:network-error"
        ;;
    *)
        # Other error (500, 503, etc.) — log but don't estop.
        echo "Warning: API returned $HTTP_CODE"
        echo "result:api-error-$HTTP_CODE"
        ;;
esac
