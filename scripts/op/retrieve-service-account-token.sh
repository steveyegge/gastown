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
# Get service account token from config.local
# Returns: token value or empty string if not found
get_service_account_token() {
  local repo_name="$1"
  local token=""

  token=$(get_op_service_account_token)

  if [ -z "$token" ]; then
    error_log "Service account token not found for repository: $repo_name"
    error_log "Set OP_SERVICE_ACCOUNT_TOKEN as an env var or in config.local"
    return 1
  fi

  echo "$token"
  return 0
}
