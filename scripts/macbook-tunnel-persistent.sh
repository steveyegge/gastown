#!/bin/bash
# Persistent reverse tunnel with auto-reconnect
# Wraps macbook-tunnel.sh with automatic reconnection on failure

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECONNECT_DELAY="${RECONNECT_DELAY:-5}"  # Seconds between reconnect attempts
MAX_RETRIES="${MAX_RETRIES:-0}"          # 0 = infinite retries

retry_count=0

cleanup() {
    echo ""
    echo "[$(date '+%H:%M:%S')] Shutting down persistent tunnel..."
    exit 0
}

trap cleanup SIGINT SIGTERM

echo "=== Persistent Reverse Tunnel ==="
echo "Auto-reconnect: enabled"
echo "Reconnect delay: ${RECONNECT_DELAY}s"
echo "Max retries: ${MAX_RETRIES:-unlimited}"
echo "=================================="
echo ""

while true; do
    echo "[$(date '+%H:%M:%S')] Starting tunnel..."

    # Run the tunnel script (pass through all arguments)
    "$SCRIPT_DIR/macbook-tunnel.sh" "$@"
    exit_code=$?

    ((retry_count++))

    if [[ $MAX_RETRIES -gt 0 ]] && [[ $retry_count -ge $MAX_RETRIES ]]; then
        echo "[$(date '+%H:%M:%S')] Max retries ($MAX_RETRIES) reached. Exiting."
        exit 1
    fi

    echo "[$(date '+%H:%M:%S')] Tunnel disconnected (exit code: $exit_code). Reconnecting in ${RECONNECT_DELAY}s..."
    echo "                    (Attempt $retry_count${MAX_RETRIES:+ of $MAX_RETRIES})"
    sleep "$RECONNECT_DELAY"
done
