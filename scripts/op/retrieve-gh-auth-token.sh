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

TOKEN_RETRIEVER_FILE="$SCRIPT_DIR/retrieve-service-account-token.sh"
if [ ! -f "$TOKEN_RETRIEVER_FILE" ]; then
  error_log "Could not find retrieve-service-account-token.sh at expected location: $TOKEN_RETRIEVER_FILE"
  exit 1
fi
source "$TOKEN_RETRIEVER_FILE"

# ============================================================================
# Main Script
# ============================================================================
debug_log "Starting GitHub PAT retrieval..."

# Get service account token using convention-based resolution
REPO_NAME=$(get_repo_name)
if [ -z "$REPO_NAME" ]; then
  error_log "Failed to determine repository name"
  exit 1
fi

debug_log "Repository name: $REPO_NAME"

# Resolve service account token from config.local
SERVICE_ACCOUNT_TOKEN=$(get_service_account_token "$REPO_NAME")
if [ -z "$SERVICE_ACCOUNT_TOKEN" ]; then
  error_log "Service account token not found for repository: $REPO_NAME"
  error_log "Expected OP_SERVICE_ACCOUNT_TOKEN in /workspace/.devcontainer/config.local"
  exit 1
fi

debug_log "Service account token found (length: ${#SERVICE_ACCOUNT_TOKEN})"

# Configure 1Password CLI to use service account
export OP_SERVICE_ACCOUNT_TOKEN="$SERVICE_ACCOUNT_TOKEN"
VAULT=$(get_repo_vault_name "$REPO_NAME")

info_log "Looking up GitHub PAT for repository: $REPO_NAME in vault: $VAULT"

# List all items in the vault
ALL_ITEMS=$(op item list --vault "$VAULT" --format json 2>/dev/null)

if [ -z "$ALL_ITEMS" ]; then
  error_log "Failed to list items in vault: $VAULT"
  error_log "Please verify:"
  error_log "  1. Service account has access to vault '$VAULT'"
  error_log "  2. Vault name is correct"
  error_log "  3. Service account token is valid"
  exit 1
fi

debug_log "Retrieved $(echo "$ALL_ITEMS" | jq 'length') items from vault"

# Find items matching "GitHub PAT {REPO_NAME} YYYYMMDD"
ITEM_NAME_PATTERN="GitHub PAT ${REPO_NAME}"
debug_log "Searching for items matching pattern: '$ITEM_NAME_PATTERN *'"

MATCHING_ITEMS=$(echo "$ALL_ITEMS" | jq -r --arg pattern "$ITEM_NAME_PATTERN" \
  '.[] | select(.title | startswith($pattern)) | .title' | sort -r)

if [ -z "$MATCHING_ITEMS" ]; then
  error_log "No GitHub PAT found for repository: $REPO_NAME"
  error_log "Expected item name pattern: '$ITEM_NAME_PATTERN YYYYMMDD'"
  error_log "Vault: $VAULT"
  error_log ""
  error_log "To create a PAT:"
  error_log "  1. Generate a fine-grained PAT at https://github.com/settings/personal-access-tokens"
  error_log "  2. Create an API Credential item in 1Password vault '$VAULT' with:"
  error_log "     - Title: '$ITEM_NAME_PATTERN YYYYMMDD' (e.g., '$ITEM_NAME_PATTERN $(date +%Y%m%d)')"
  error_log "     - Field 'credential': The PAT value"
  exit 1
fi

# Get the most recent item (first after reverse sort)
MOST_RECENT_ITEM=$(echo "$MATCHING_ITEMS" | head -n 1)
debug_log "Found $(echo "$MATCHING_ITEMS" | wc -l | tr -d ' ') matching item(s)"
info_log "Using PAT: $MOST_RECENT_ITEM"

# Fetch the credential field from the item
debug_log "Fetching credential from item..."
PAT=$(op item get "$MOST_RECENT_ITEM" --vault "$VAULT" --fields label=credential --reveal 2>/dev/null)

if [ -z "$PAT" ]; then
  error_log "Failed to retrieve credential from item: $MOST_RECENT_ITEM"
  error_log "Please verify the item has a field labeled 'credential'"
  exit 1
fi

debug_log "Successfully retrieved PAT (length: ${#PAT})"

# Strip quotes and whitespace
PAT=$(echo "$PAT" | tr -d '"\n\r ')

debug_log "After cleaning (length: ${#PAT})"

# Output the PAT
echo "$PAT"
