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
debug_log "Starting git signing key retrieval..."

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

info_log "Looking up signing key for repository: $REPO_NAME"

# Query 1Password for matching items — same vault as auth key
VAULT=$(get_repo_vault_name "$REPO_NAME")
TITLE_PATTERN="GitHub Signing Key $REPO_NAME"

debug_log "Searching 1Password vault '$VAULT' for items matching: $TITLE_PATTERN"

MATCHING_ITEMS=$(op item list --vault "$VAULT" --format json 2>&1 |
  jq -r --arg pattern "$TITLE_PATTERN" \
    '.[] | select(.title | startswith($pattern)) | .title' 2>/dev/null || true)

if [ -z "$MATCHING_ITEMS" ]; then
  error_log "No signing key found for repository: $REPO_NAME"
  error_log "Expected item title pattern: 'GitHub Signing Key $REPO_NAME YYYYMMDD'"
  error_log "Item type: SSH Key"
  error_log "Vault: $VAULT"
  exit 1
fi

# Sort by date in title (newest first) and take the first one
MOST_RECENT_ITEM=$(echo "$MATCHING_ITEMS" | sort -r | head -n 1)
debug_log "Found $(echo "$MATCHING_ITEMS" | wc -l | tr -d ' ') matching item(s)"
info_log "Using signing key: $MOST_RECENT_ITEM"

# Fetch the private key from the SSH Key item.
# NOTE: ?ssh-format=openssh is intentionally NOT used — that query parameter causes
# the op CLI to contact a separate key-conversion endpoint which hangs in this
# firewall-restricted environment. We convert PKCS#8 -> OpenSSH ourselves using node.
debug_log "Fetching private key from 1Password (PKCS#8 format)..."
PRIVATE_KEY=$(op read "op://$VAULT/$MOST_RECENT_ITEM/private key" 2>&1)
READ_EXIT=$?

if [ $READ_EXIT -ne 0 ]; then
  error_log "Failed to read signing key from 1Password using 'op read'"
  error_log "Output: $PRIVATE_KEY"
  error_log "Please verify this is an SSH Key item type"
  exit 1
fi

debug_log "Successfully retrieved private key (length: ${#PRIVATE_KEY})"

if [[ ! "$PRIVATE_KEY" =~ ^-----BEGIN ]]; then
  error_log "Retrieved key does not appear to be a valid SSH private key"
  error_log "Key starts with: $(echo "$PRIVATE_KEY" | head -c 50)"
  error_log "This may indicate an encoding issue. Please check the 1Password item."
  exit 1
fi

# Convert PKCS#8 PEM -> OpenSSH PEM using node.js (always available in this container).
# Constructs the OpenSSH wire format from JWK key material per RFC 8709 / openssh-key-v1.
debug_log "Converting PKCS#8 key to OpenSSH format via node..."
PKCS8_KEY="$PRIVATE_KEY" node -e "
const crypto = require('crypto');
const key = crypto.createPrivateKey(process.env.PKCS8_KEY);
const jwk = key.export({ format: 'jwk' });
const priv = Buffer.from(jwk.d, 'base64url');
const pub  = Buffer.from(jwk.x, 'base64url');
function writeStr(s) { const b = Buffer.from(s); const l = Buffer.alloc(4); l.writeUInt32BE(b.length); return Buffer.concat([l, b]); }
function writeBuf(b) { const l = Buffer.alloc(4); l.writeUInt32BE(b.length); return Buffer.concat([l, b]); }
const keyType    = writeStr('ssh-ed25519');
const pubBlock   = Buffer.concat([keyType, writeBuf(pub)]);
const checkInt   = crypto.randomInt(0, 0xffffffff);
const checkBuf   = Buffer.alloc(4); checkBuf.writeUInt32BE(checkInt);
const privBlob   = Buffer.concat([keyType, writeBuf(pub), writeBuf(Buffer.concat([priv, pub])), writeStr('')]);
const padLen     = (8 - ((privBlob.length + 8) % 8)) % 8;
const padding    = Buffer.from([...Array(padLen)].map((_, i) => i + 1));
const inner      = Buffer.concat([checkBuf, checkBuf, privBlob, padding]);
const outer      = Buffer.concat([
  Buffer.from('openssh-key-v1\0'),
  writeStr('none'), writeStr('none'), writeStr(''),
  Buffer.from([0, 0, 0, 1]),
  writeBuf(pubBlock),
  writeBuf(inner),
]);
const b64 = outer.toString('base64').match(/.{1,70}/g).join('\n');
console.log('-----BEGIN OPENSSH PRIVATE KEY-----');
console.log(b64);
console.log('-----END OPENSSH PRIVATE KEY-----');
"
CONVERT_EXIT=$?
if [ $CONVERT_EXIT -ne 0 ]; then
  error_log "Failed to convert private key from PKCS#8 to OpenSSH format"
  exit 1
fi
