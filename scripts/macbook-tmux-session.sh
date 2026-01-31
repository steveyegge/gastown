#!/bin/bash
# MacBook tmux session for gt11 remote access
# Creates/attaches to a tmux session that runs the persistent tunnel

set -euo pipefail

SESSION_NAME="${SESSION_NAME:-gt11-tunnel}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    cat <<EOF
Usage: $(basename "$0") [command] [options]

Manages a tmux session for the gt11 reverse tunnel.

Commands:
  start     Create session and start tunnel (default)
  attach    Attach to existing session
  stop      Stop the tunnel and kill session
  status    Check session status

Options passed to tunnel script:
  -h, --host HOST     gt11 hostname
  -u, --user USER     gt11 username
  -p, --port PORT     Reverse tunnel port

Environment:
  SESSION_NAME        tmux session name (default: gt11-tunnel)

Examples:
  # Start new session with tunnel
  ./macbook-tmux-session.sh start -h my-gt11.aws.com

  # Attach to running session
  ./macbook-tmux-session.sh attach

  # Check if tunnel is running
  ./macbook-tmux-session.sh status

EOF
    exit 0
}

cmd_start() {
    if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        echo "Session '$SESSION_NAME' already exists."
        echo "Use 'attach' to connect or 'stop' to restart."
        exit 1
    fi

    echo "Creating tmux session '$SESSION_NAME'..."

    # Create new detached session running the persistent tunnel
    tmux new-session -d -s "$SESSION_NAME" \
        "$SCRIPT_DIR/macbook-tunnel-persistent.sh $*"

    echo "Session started. Use 'attach' to view or 'status' to check."
    tmux list-sessions | grep "$SESSION_NAME"
}

cmd_attach() {
    if ! tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        echo "No session '$SESSION_NAME' found. Use 'start' first."
        exit 1
    fi

    tmux attach-session -t "$SESSION_NAME"
}

cmd_stop() {
    if ! tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        echo "No session '$SESSION_NAME' found."
        exit 0
    fi

    echo "Stopping session '$SESSION_NAME'..."
    tmux kill-session -t "$SESSION_NAME"
    echo "Session stopped."
}

cmd_status() {
    if tmux has-session -t "$SESSION_NAME" 2>/dev/null; then
        echo "Session '$SESSION_NAME' is RUNNING"
        echo ""
        tmux list-sessions | grep "$SESSION_NAME"
        echo ""
        echo "Recent output:"
        tmux capture-pane -t "$SESSION_NAME" -p | tail -10
    else
        echo "Session '$SESSION_NAME' is NOT RUNNING"
        exit 1
    fi
}

# Parse command
case "${1:-start}" in
    start)
        shift || true
        cmd_start "$@"
        ;;
    attach)
        cmd_attach
        ;;
    stop)
        cmd_stop
        ;;
    status)
        cmd_status
        ;;
    --help|-h)
        usage
        ;;
    *)
        echo "Unknown command: $1"
        usage
        ;;
esac
