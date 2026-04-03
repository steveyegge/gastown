#!/usr/bin/env bash
# rate-limit-watchdog/run.sh — Rotate on rate limit, ESTOP only as last resort.
#
# No LLM calls — uses a malformed API request (empty messages array) that
# returns 400 (usable) or 429 (limited) with zero token cost.

set -euo pipefail

# --- Configuration -----------------------------------------------------------
TOWN_ROOT="${GT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
ESTOP_FILE="$TOWN_ROOT/ESTOP"
PROBE_MODEL="claude-haiku-4-5-20251001"

# --- Preflight ---------------------------------------------------------------
if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ANTHROPIC_API_KEY not set — cannot probe API"
    exit 1
fi

# --- Probe API (non-LLM: malformed request, zero tokens) --------------------
# Empty messages array triggers 400 if key works, 429 if rate-limited.
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "https://api.anthropic.com/v1/messages" \
    -H "x-api-key: $ANTHROPIC_API_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    -d "{\"model\":\"$PROBE_MODEL\",\"max_tokens\":1,\"messages\":[]}" \
    --connect-timeout 10 \
    --max-time 15 \
    2>/dev/null || echo "000")

echo "API probe: HTTP $HTTP_CODE"

# --- Decision ----------------------------------------------------------------
case "$HTTP_CODE" in
    400)
        # Malformed request accepted — API key works. Same as 200 for our purposes.
        if [ -f "$ESTOP_FILE" ]; then
            if grep -q "auto" "$ESTOP_FILE" 2>/dev/null || grep -q "rate limit" "$ESTOP_FILE" 2>/dev/null; then
                echo "API healthy — thawing (rate limit cleared)"
                gt thaw

                # Post-thaw: verify agents restarted (wait for SIGCONT to propagate)
                sleep 5
                DEAD_RIGS=$(gt rig list 2>/dev/null | grep -c "○ stopped" || true)
                if [ "$DEAD_RIGS" -gt 0 ]; then
                    echo "Post-thaw: $DEAD_RIGS rig(s) have stopped agents — restarting"
                    gt rig list 2>/dev/null | grep -B1 "○ stopped" | grep "^[^ ]" | while read -r RIG_LINE; do
                        RIG_NAME=$(echo "$RIG_LINE" | sed 's/^[🅿️🟢🔴 ]*//' | awk '{print $1}')
                        if [ -n "$RIG_NAME" ]; then
                            echo "  Restarting $RIG_NAME"
                            gt rig start "$RIG_NAME" 2>/dev/null || true
                        fi
                    done
                    echo "result:thawed-with-restart"
                else
                    echo "Post-thaw: all rigs healthy"
                    echo "result:thawed"
                fi
            else
                echo "API healthy — estop active but not rate-limit (skipping thaw)"
                echo "result:manual-estop-preserved"
            fi
        else
            echo "API healthy — no action needed"
            echo "result:healthy"
        fi
        ;;
    200|201)
        # Unexpected success (shouldn't happen with empty messages) — treat same as 400.
        echo "API healthy (unexpected 2xx) — treating as usable"
        if [ -f "$ESTOP_FILE" ]; then
            if grep -q "auto" "$ESTOP_FILE" 2>/dev/null || grep -q "rate limit" "$ESTOP_FILE" 2>/dev/null; then
                echo "Thawing (rate limit cleared)"
                gt thaw
                sleep 5
                echo "result:thawed"
            fi
        else
            echo "result:healthy"
        fi
        ;;
    429)
        # Rate limited — try rotation before ESTOP.
        if [ -f "$ESTOP_FILE" ]; then
            echo "Rate limit detected — estop already active"
            echo "result:already-frozen"
        else
            echo "Rate limit detected — attempting account rotation"

            # Check if any account is available (non-LLM probe of all accounts)
            AVAILABLE=$(gt quota status --probe --first-available 2>/dev/null || true)

            if [ -n "$AVAILABLE" ]; then
                echo "Found available account: $AVAILABLE — rotating"
                # gt quota rotate (no --from) targets rate-limited sessions detected by scan
                if gt quota rotate 2>/dev/null; then
                    echo "Rotation successful — ESTOP avoided"
                    echo "result:rotated"
                else
                    echo "Rotation failed — triggering estop"
                    TOTAL=$(gt quota status --json 2>/dev/null | grep -c '"handle"' || echo "?")
                    gt estop -r "All $TOTAL accounts rate-limited (auto-watchdog)"
                    echo "result:estop-triggered"
                fi
            else
                echo "No available accounts — triggering estop"
                TOTAL=$(gt quota status --json 2>/dev/null | grep -c '"handle"' || echo "?")
                gt estop -r "All $TOTAL accounts rate-limited (auto-watchdog)"
                echo "result:estop-triggered"
            fi
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
