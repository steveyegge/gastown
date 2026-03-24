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
debug_log "Starting GitHub SSH key retrieval..."

# Get service account token using convention-based resolution
REPO_NAME=$(get_repo_name)
if [ -z "$REPO_NAME" ]; then
  error_log "Failed to determine repository name"
  exit 1
fi

debug_log "Repository name: $REPO_NAME"

# Resolve service account token from file
SERVICE_ACCOUNT_TOKEN=$(get_service_account_token "$REPO_NAME")
if [ -z "$SERVICE_ACCOUNT_TOKEN" ]; then
  error_log "Service account token not found for repository: $REPO_NAME"
  error_log "Expected OP_SERVICE_ACCOUNT_TOKEN in /workspace/.devcontainer/config.local"
  exit 1
fi

debug_log "Service account token found (length: ${#SERVICE_ACCOUNT_TOKEN})"

# Configure 1Password CLI to use service account
export OP_SERVICE_ACCOUNT_TOKEN="$SERVICE_ACCOUNT_TOKEN"

info_log "Looking up SSH key for repository: $REPO_NAME"

# Query 1Password for matching items
VAULT=$(get_repo_vault_name "$REPO_NAME")
TITLE_PATTERN="GitHub SSH Key $REPO_NAME"

debug_log "Searching 1Password vault '$VAULT' for items matching: $TITLE_PATTERN"

# List all items in vault matching the title pattern (looking for SSH Key item type)
MATCHING_ITEMS=$(op item list --vault "$VAULT" --format json 2>&1 |
  jq -r --arg pattern "$TITLE_PATTERN" \
    '.[] | select(.title | startswith($pattern)) | .title' 2>/dev/null || true)

if [ -z "$MATCHING_ITEMS" ]; then
  error_log "No SSH key found for repository: $REPO_NAME"
  error_log "Expected item title pattern: 'GitHub SSH Key $REPO_NAME YYYYMMDD'"
  error_log "Item type: SSH Key"
  error_log "Vault: $VAULT"
  exit 1
fi

# Sort by date in title (newest first) and take the first one
MOST_RECENT_ITEM=$(echo "$MATCHING_ITEMS" | sort -r | head -n 1)
debug_log "Found $(echo "$MATCHING_ITEMS" | wc -l | tr -d ' ') matching item(s)"
info_log "Using SSH key: $MOST_RECENT_ITEM"

# Fetch the private key from the SSH Key item
# Use the recommended 'op read' command with ssh-format=openssh query parameter
# See: https://developer.1password.com/docs/cli/ssh-keys/#get-a-private-key
debug_log "Fetching private key using op read with OpenSSH format..."
PRIVATE_KEY=$(op read "op://$VAULT/$MOST_RECENT_ITEM/private key?ssh-format=openssh" 2>&1)
READ_EXIT=$?

if [ $READ_EXIT -ne 0 ]; then
  error_log "Failed to read private key from 1Password using 'op read'"
  error_log "Output: $PRIVATE_KEY"
  error_log "Please verify this is an SSH Key item type"
  exit 1
fi

debug_log "Successfully retrieved private key (length: ${#PRIVATE_KEY})"

# Verify the key looks valid (should start with -----BEGIN)
if [[ ! "$PRIVATE_KEY" =~ ^-----BEGIN ]]; then
  error_log "Retrieved key does not appear to be a valid SSH private key"
  error_log "Key starts with: $(echo "$PRIVATE_KEY" | head -c 50)"
  error_log "This may indicate an encoding issue. Please check the 1Password item."
  exit 1
fi

# Output the private key (should now be properly decoded with actual newlines)
echo "$PRIVATE_KEY"
