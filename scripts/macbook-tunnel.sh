#!/bin/bash
# MacBook → gt11 reverse tunnel script
# Run this from your MacBook to establish remote access from gt11

set -euo pipefail

# Configuration - adjust these for your setup
GT11_HOST="${GT11_HOST:-gt11.example.com}"
GT11_USER="${GT11_USER:-ubuntu}"
REVERSE_PORT="${REVERSE_PORT:-2222}"  # Port on gt11 that tunnels back to MacBook:22
LOCAL_SSH_PORT="${LOCAL_SSH_PORT:-22}"  # MacBook's SSH port
KEEPALIVE_INTERVAL="${KEEPALIVE_INTERVAL:-60}"

usage() {
    cat <<EOF
Usage: $(basename "$0") [options]

Establishes a reverse SSH tunnel from MacBook to gt11, allowing gt11 to SSH back.

Options:
  -h, --host HOST     gt11 hostname (default: \$GT11_HOST or gt11.example.com)
  -u, --user USER     gt11 username (default: \$GT11_USER or ubuntu)
  -p, --port PORT     Reverse tunnel port on gt11 (default: \$REVERSE_PORT or 2222)
  -l, --local PORT    Local SSH port on MacBook (default: 22)
  --help              Show this help

Environment variables:
  GT11_HOST           gt11 hostname
  GT11_USER           gt11 username
  REVERSE_PORT        Reverse tunnel port
  LOCAL_SSH_PORT      Local SSH port

Example:
  # Basic usage (uses defaults/env vars)
  ./macbook-tunnel.sh

  # With explicit options
  ./macbook-tunnel.sh -h my-gt11.aws.com -u myuser -p 2223

Once connected, gt11 can reach this MacBook via:
  ssh -p $REVERSE_PORT localhost

EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--host)
            GT11_HOST="$2"
            shift 2
            ;;
        -u|--user)
            GT11_USER="$2"
            shift 2
            ;;
        -p|--port)
            REVERSE_PORT="$2"
            shift 2
            ;;
        -l|--local)
            LOCAL_SSH_PORT="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

echo "=== MacBook → gt11 Reverse Tunnel ==="
echo "gt11 host:       $GT11_USER@$GT11_HOST"
echo "Reverse port:    $REVERSE_PORT (gt11:$REVERSE_PORT → MacBook:$LOCAL_SSH_PORT)"
echo "Keepalive:       ${KEEPALIVE_INTERVAL}s"
echo ""
echo "Once connected, gt11 can reach this MacBook via:"
echo "  ssh -p $REVERSE_PORT localhost"
echo ""
echo "Press Ctrl+C to disconnect"
echo "=================================="

# Establish the reverse tunnel
# -R creates the reverse tunnel: gt11's port $REVERSE_PORT → MacBook's port 22
# -N means no remote command (just tunnel)
# -o ServerAliveInterval keeps connection alive
# -o ExitOnForwardFailure exits if tunnel can't be established
exec ssh \
    -R "${REVERSE_PORT}:localhost:${LOCAL_SSH_PORT}" \
    -N \
    -o ServerAliveInterval="${KEEPALIVE_INTERVAL}" \
    -o ServerAliveCountMax=3 \
    -o ExitOnForwardFailure=yes \
    -o TCPKeepAlive=yes \
    "${GT11_USER}@${GT11_HOST}"
