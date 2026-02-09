#!/usr/bin/env bash
# run-hardener.sh — Launch migration hardener agent in a tmux session
#
# Usage: ./scripts/run-hardener.sh
#
# Launches claude in a tmux session, then uses gt nudge to send the initial
# prompt (tmux send-keys + Enter doesn't work with Claude Code TUI).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SESSION_NAME="migration-hardener"

# Kill existing session if any
tmux kill-session -t "$SESSION_NAME" 2>/dev/null || true

# Launch claude (skip project hooks — gt prime --hook hangs without GT_ROLE)
tmux new-session -d -s "$SESSION_NAME" -c "$REPO_ROOT" \
    "claude --dangerously-skip-permissions --setting-sources user"

INITIAL_PROMPT="You are a solo migration hardening agent. Read .claude/agents/at-migration-mission.md for your full mission and .claude/agents/migration-hardener.md for your role context. Execute all 5 phases autonomously. Push directly to main. VM access: gcloud compute ssh migration-test-lab --zone=us-west1-b. Start Phase 1 now."

# Wait for session to become ready, then deliver the initial prompt with retry.
# Claude Code can take varying time to initialize depending on machine speed and
# project size, so we poll rather than using a fixed sleep.
MAX_ATTEMPTS=10
RETRY_DELAY=5

echo "Waiting for session to initialize..."

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
    # First check the tmux session still exists (claude may have crashed on startup)
    if ! tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        echo "ERROR: tmux session '$SESSION_NAME' died before nudge could be delivered"
        exit 1
    fi

    # Attempt to deliver the initial prompt
    if gt nudge "$SESSION_NAME" "$INITIAL_PROMPT" 2>/dev/null; then
        echo ""
        echo "Migration hardener launched in tmux session: $SESSION_NAME"
        echo "  Monitor: tmux attach -t $SESSION_NAME"
        echo "  Check:   tmux capture-pane -t $SESSION_NAME -p | tail -20"
        exit 0
    fi

    echo "  Attempt $attempt/$MAX_ATTEMPTS: nudge not delivered yet, retrying in ${RETRY_DELAY}s..."
    sleep "$RETRY_DELAY"
done

echo "ERROR: Failed to deliver initial prompt after $MAX_ATTEMPTS attempts"
echo "  Session may not have initialized. Check manually:"
echo "    tmux attach -t $SESSION_NAME"
exit 1
