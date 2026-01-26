#!/bin/bash
# gt11 → MacBook connection script
# Connects to MacBook through the reverse tunnel

set -euo pipefail

TUNNEL_PORT="${TUNNEL_PORT:-2222}"
MACBOOK_USER="${MACBOOK_USER:-$(whoami)}"

usage() {
    cat <<EOF
Usage: $(basename "$0") [options] [command]

Connects to MacBook through the reverse SSH tunnel.

Options:
  -p, --port PORT     Tunnel port (default: \$TUNNEL_PORT or 2222)
  -u, --user USER     MacBook username (default: \$MACBOOK_USER or current user)
  --help              Show this help

Environment:
  TUNNEL_PORT         Port for reverse tunnel
  MACBOOK_USER        Username on MacBook

Examples:
  # Interactive shell
  ./gt11-to-macbook.sh

  # Run a command
  ./gt11-to-macbook.sh ls -la ~/gt11

  # Specify port and user
  ./gt11-to-macbook.sh -p 2223 -u steve

EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--port)
            TUNNEL_PORT="$2"
            shift 2
            ;;
        -u|--user)
            MACBOOK_USER="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            break
            ;;
    esac
done

# Check if tunnel is available
if ! nc -z localhost "$TUNNEL_PORT" 2>/dev/null; then
    echo "Error: Tunnel not available on localhost:$TUNNEL_PORT"
    echo "Make sure the MacBook has established the reverse tunnel."
    exit 1
fi

# Connect (remaining args become the command, if any)
if [[ $# -gt 0 ]]; then
    # Run command
    exec ssh -p "$TUNNEL_PORT" -o StrictHostKeyChecking=accept-new \
        "$MACBOOK_USER@localhost" "$@"
else
    # Interactive shell
    exec ssh -p "$TUNNEL_PORT" -o StrictHostKeyChecking=accept-new \
        "$MACBOOK_USER@localhost"
fi
