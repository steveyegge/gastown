#!/bin/bash
set -euo pipefail

# ============================================================================
# Source Shared Utilities
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_FILE="$SCRIPT_DIR/../common.sh"
if [ ! -f "$COMMON_FILE" ]; then
  echo "ERROR: Could not find common.sh at expected location: $COMMON_FILE" >&2
  exit 1
fi
source "$COMMON_FILE"

# ============================================================================
# Main Script
# ============================================================================
info_log "Configuring GitHub CLI authentication..."

if gh auth status --hostname github.com &>/dev/null; then
  info_log "GitHub CLI already authenticated — skipping 1Password fetch"
else
  HELPER_SCRIPT="$SCRIPT_DIR/retrieve-gh-auth-token.sh"

  if [ ! -x "$HELPER_SCRIPT" ]; then
    error_log "GitHub PAT helper script not found or not executable: $HELPER_SCRIPT"
    exit 1
  fi

  debug_log "Fetching GitHub PAT from 1Password..."
  PAT=$("$HELPER_SCRIPT")
  HELPER_EXIT=$?

  if [ $HELPER_EXIT -ne 0 ]; then
    error_log "Failed to fetch GitHub PAT from 1Password"
    exit 1
  fi

  if [ -z "$PAT" ]; then
    error_log "GitHub PAT is empty"
    exit 1
  fi

  debug_log "PAT retrieved successfully (length: ${#PAT})"

  echo "$PAT" | gh auth login --with-token --hostname github.com
  GH_EXIT=$?

  if [ $GH_EXIT -ne 0 ]; then
    error_log "Failed to authenticate GitHub CLI with PAT"
    exit 1
  fi

  gh auth status --hostname github.com 2>&1 | while IFS= read -r line; do info_log "$line"; done

  info_log "GitHub CLI authenticated successfully"
fi
