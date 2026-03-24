#!/bin/bash
set -euo pipefail

# ============================================================================
# Source Shared Utilities
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_FILE="$SCRIPT_DIR/common.sh"
if [ ! -f "$COMMON_FILE" ]; then
  echo "ERROR: Could not find common.sh at expected location: $COMMON_FILE" >&2
  exit 1
fi
source "$COMMON_FILE"

# ============================================================================
# Main
# ============================================================================

main() {
  info_log "Configuring git commit signing..."

  local key_source
  key_source="op"

  if [ -z "$key_source" ]; then
    info_log "Git commit signing not enabled (set GIT_SIGNING_KEY_SOURCE=ssh-agent or op in config.local to enable)"
    return 0
  fi

  info_log "Signing key source: $key_source"

  SSH_DIR="$HOME/.ssh"
  mkdir -p "$SSH_DIR"
  chmod 700 "$SSH_DIR"

  SIGNING_KEY_FILE="$SSH_DIR/id_ed25519_signing"
  ALLOWED_SIGNERS_FILE="$SSH_DIR/allowed_signers"

  local public_key
  case "$key_source" in
  op) public_key=$(configure_from_op) ;;
  ssh-agent) public_key=$(configure_from_ssh_agent) || return 0 ;;
  none | NA)
    info_log "Git commit signing explicitly disabled (GIT_SIGNING_KEY_SOURCE=$key_source)"
    return 0
    ;;
  *)
    error_log "Unknown GIT_SIGNING_KEY_SOURCE: '$key_source'"
    error_log "Valid values: 'op', 'ssh-agent', 'none', 'NA'"
    return 1
    ;;
  esac

  configure_git_globals "$public_key"
  info_log "Git commit signing configured successfully (source: $key_source)"
}

# ============================================================================
# Key source: 1Password
# ============================================================================

configure_from_op() {
  local helper_script="$SCRIPT_DIR/op/retrieve-git-signing-key.sh"

  if [ ! -x "$helper_script" ]; then
    error_log "Signing key retriever script not found or not executable: $helper_script"
    return 1
  fi

  # Cache the private key on disk — keys rotate infrequently and fetching is slow.
  # To force a re-fetch: rm ~/.ssh/id_ed25519_signing
  if [ -f "$SIGNING_KEY_FILE" ]; then
    info_log "Signing key already on disk — skipping 1Password fetch"
  else
    debug_log "Fetching signing private key from 1Password..."
    local private_key
    private_key=$("$helper_script")

    if [ -z "$private_key" ]; then
      error_log "Retrieved signing key is empty"
      return 1
    fi

    echo "$private_key" >"$SIGNING_KEY_FILE"
    chmod 600 "$SIGNING_KEY_FILE"
    info_log "Signing private key written to: $SIGNING_KEY_FILE"
  fi

  # Derive public key from private key (always — ensures git config stays correct).
  local public_key
  public_key=$(ssh-keygen -y -f "$SIGNING_KEY_FILE" 2>&1)
  if [ $? -ne 0 ] || [ -z "$public_key" ]; then
    error_log "Failed to derive public key from signing private key"
    error_log "Output: $public_key"
    return 1
  fi

  git config --global user.signingKey "$SIGNING_KEY_FILE"
  echo "$public_key"
}

# ============================================================================
# Key source: host SSH agent
# ============================================================================

configure_from_ssh_agent() {
  if [ -z "${SSH_AUTH_SOCK:-}" ]; then
    warn_log "SSH_AUTH_SOCK is not set — skipping signing config"
    warn_log "This is expected if the IDE SSH agent socket is not yet forwarded (e.g. during postAttachCommand)"
    warn_log "Open a new terminal and run: .devcontainer/scripts/configure-git-signing.sh"
    return 1
  fi

  local agent_keys
  agent_keys=$(ssh-add -L 2>&1) || true
  if [ -z "$agent_keys" ] || echo "$agent_keys" | grep -qE "^(Could not open|The agent has no|Error connecting to agent)"; then
    warn_log "SSH agent is not accessible or contains no keys — skipping signing config"
    warn_log "This is expected if the IDE SSH agent socket is not yet forwarded (e.g. during postAttachCommand)"
    warn_log "Open a new terminal and run: .devcontainer/scripts/configure-git-signing.sh"
    return 1
  fi

  local key_count
  key_count=$(echo "$agent_keys" | wc -l | tr -d ' ')
  debug_log "SSH agent has $key_count key(s)"

  if [ "$key_count" -gt 1 ]; then
    warn_log "SSH agent has $key_count keys; using the first one for signing"
    warn_log "If this is wrong, ensure only your signing key is loaded in the agent"
  fi

  local public_key
  public_key=$(echo "$agent_keys" | head -n 1)

  # Write to a .pub file so git config is stable across agent restarts.
  echo "$public_key" >"${SIGNING_KEY_FILE}.pub"
  chmod 644 "${SIGNING_KEY_FILE}.pub"

  git config --global user.signingKey "${SIGNING_KEY_FILE}.pub"
  info_log "Signing key set from SSH agent (public key written to ${SIGNING_KEY_FILE}.pub)"
  echo "$public_key"
}

# ============================================================================
# Shared: write allowed_signers and set git globals
# ============================================================================

configure_git_globals() {
  local public_key="$1"

  local git_email
  git_email=$(git config --global user.email 2>/dev/null || echo "")
  if [ -z "$git_email" ]; then
    warn_log "user.email not set in git config — allowed_signers file will be incomplete"
    git_email="unknown"
  fi

  echo "$git_email $public_key" >"$ALLOWED_SIGNERS_FILE"
  chmod 644 "$ALLOWED_SIGNERS_FILE"
  debug_log "Wrote allowed_signers: $ALLOWED_SIGNERS_FILE"

  git config --global gpg.format ssh
  git config --global gpg.ssh.allowedSignersFile "$ALLOWED_SIGNERS_FILE"
  git config --global commit.gpgsign true
  git config --global tag.gpgsign true

  # also unset gpg ssh program because it comes through from the host (where it's useful) to here (where it's not)
  git config --global --unset gpg.ssh.program 2>/dev/null || true
}

# ============================================================================
main
