#!/bin/bash
# Shared Common Utilities
# Provides common functions for repository management, token management, and logging
# Version: 1.0.0

# ============================================================================
# Logging Utilities
# ============================================================================

# Only initialize logging if not already initialized
if [ -z "${COLOR_DEBUG:-}" ]; then
  # Color codes - respect NO_COLOR environment variable
  if [ -t 2 ] && [ "${NO_COLOR:-}" = "" ]; then
    COLOR_DEBUG='\033[0;90m' # Grey (bright black, works in both dark/light themes)
    COLOR_INFO='\033[0;32m'  # Green
    COLOR_WARN='\033[0;33m'  # Yellow
    COLOR_ERROR='\033[0;31m' # Red
    COLOR_RESET='\033[0m'    # Reset
  else
    COLOR_DEBUG=''
    COLOR_INFO=''
    COLOR_WARN=''
    COLOR_ERROR=''
    COLOR_RESET=''
  fi

  # Single source of truth: DEBUG=true to enable debug logging
  OP_DEBUG=true

  # Debug logging - only shown if DEBUG=true
  debug_log() {
    if [ "${OP_DEBUG:-false}" = "true" ]; then
      echo -e "${COLOR_DEBUG}[DEBUG]${COLOR_RESET} $*" >&2
    fi
  }

  # Info logging - always shown
  info_log() {
    echo -e "${COLOR_INFO}[INFO]${COLOR_RESET} $*" >&2
  }

  # Warning logging - always shown
  warn_log() {
    echo -e "${COLOR_WARN}[WARN]${COLOR_RESET} $*" >&2
  }

  # Error logging - always shown
  error_log() {
    echo -e "${COLOR_ERROR}[ERROR]${COLOR_RESET} $*" >&2
  }
fi

# ============================================================================
# Config Resolution (env vars first, then config.local fallback)
# ============================================================================

CONFIG_LOCAL_FILE="/workspace/.devcontainer/config.local"

# Read a config value: checks env var first, then config.local file
# Usage: _read_config_value KEY
_read_config_value() {
  local key="$1"
  # Check env var first (supports gastown container where vars come via docker-compose)
  local env_val="${!key:-}"
  if [ -n "$env_val" ]; then
    echo "$env_val" | tr -d '[:space:]'
    return
  fi
  # Fall back to config.local file (devcontainer path)
  if [ -f "$CONFIG_LOCAL_FILE" ]; then
    grep -E "^${key}=" "$CONFIG_LOCAL_FILE" 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"'"'" | tr -d '[:space:]'
  fi
}

# Read a config value preserving internal whitespace: env var first, then config.local
# Usage: _read_config_value_raw KEY
_read_config_value_raw() {
  local key="$1"
  # Check env var first
  local env_val="${!key:-}"
  if [ -n "$env_val" ]; then
    echo "$env_val" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
    return
  fi
  # Fall back to config.local file
  if [ -f "$CONFIG_LOCAL_FILE" ]; then
    grep -E "^${key}=" "$CONFIG_LOCAL_FILE" 2>/dev/null | head -1 | cut -d= -f2- | tr -d '"'"'" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
  fi
}

# Get the git user name (env: GIT_USER_NAME or GIT_USER, then config.local)
# Returns: user name or empty string
get_git_user_name() {
  local name
  name=$(_read_config_value_raw "GIT_USER_NAME")
  # Also check GIT_USER (used by gastown docker-compose)
  if [ -z "$name" ]; then
    name="${GIT_USER:-}"
  fi
  if [ -n "$name" ]; then
    debug_log "Git user name: $name"
    echo "$name"
  fi
}

# Get the git user email (env: GIT_USER_EMAIL or GIT_EMAIL, then config.local)
# Returns: user email or empty string
get_git_user_email() {
  local email
  email=$(_read_config_value "GIT_USER_EMAIL")
  # Also check GIT_EMAIL (used by gastown docker-compose)
  if [ -z "$email" ]; then
    email="${GIT_EMAIL:-}"
  fi
  if [ -n "$email" ]; then
    debug_log "Git user email: $email"
    echo "$email"
  fi
}

# Get the Claude auth mode
# Returns: "browser" or "api-key"
get_claude_auth_mode() {
  local auth_mode
  auth_mode=$(_read_config_value "CLAUDE_AUTH_MODE")
  if [ -n "$auth_mode" ]; then
    debug_log "Claude auth mode: $auth_mode"
    echo "$auth_mode"
  else
    debug_log "No CLAUDE_AUTH_MODE configured — defaulting to browser"
    echo "browser"
  fi
}

# Get the devcontainer profile
# Returns: profile name (default: "default")
get_devcontainer_profile() {
  local profile
  profile=$(_read_config_value "PROFILE")
  if [ -n "$profile" ]; then
    debug_log "Profile: $profile"
    echo "$profile"
    return
  fi
  debug_log "No profile configured — defaulting to default"
  echo "default"
}

# Get the 1Password service account token (env var or config.local)
# Returns: token value or empty string
get_op_service_account_token() {
  local token
  token=$(_read_config_value "OP_SERVICE_ACCOUNT_TOKEN")
  if [ -n "$token" ]; then
    debug_log "OP service account token found (length: ${#token})"
    echo "$token"
  fi
}

# Get the git signing key source
# Returns: "op", "ssh-agent", or empty string (signing disabled)
get_git_signing_key_source() {
  local source
  source=$(_read_config_value "GIT_SIGNING_KEY_SOURCE")
  if [ -n "$source" ]; then
    debug_log "Git signing key source: $source"
    echo "$source"
  else
    debug_log "GIT_SIGNING_KEY_SOURCE not set — signing disabled"
  fi
}

# ============================================================================
# Repository Name Detection
# ============================================================================

# Returns the repository name for 1Password vault lookups.
# Hardcoded for gastown — this fork targets Carepatron-App.
get_repo_name() {
  echo "Carepatron-App"
}

# ============================================================================
# Vault Name Resolution
# ============================================================================

# Get repository-specific vault name
# Returns: vault name in format "Gastown {repo-name}"
# Example: "my-repo" -> "Gastown my-repo"
get_repo_vault_name() {
  local repo_name="$1"
  if [ -z "$repo_name" ]; then
    error_log "get_repo_vault_name: repo_name is required"
    return 1
  fi
  echo "Gastown ${repo_name}"
}
