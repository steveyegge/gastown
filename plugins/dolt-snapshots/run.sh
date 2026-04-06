#!/usr/bin/env bash
# dolt-snapshots/run.sh — Build and run the dolt-snapshots Go binary.
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$PLUGIN_DIR/dolt-snapshots"

# Build if binary doesn't exist or source is newer
if [ ! -f "$BINARY" ] || [ "$PLUGIN_DIR/main.go" -nt "$BINARY" ]; then
  echo "[dolt-snapshots] Building binary..."
  (cd "$PLUGIN_DIR" && go build -o dolt-snapshots .) || {
    echo "[dolt-snapshots] Build failed"
    exit 1
  }
fi

exec "$BINARY" "$@"
