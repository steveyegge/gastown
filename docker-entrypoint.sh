#!/bin/sh
set -e

# Re-apply git/dolt config on every start so env var changes take effect
# even when the home volume already exists from a previous run.
if [ -n "$GIT_USER" ] && [ -n "$GIT_EMAIL" ]; then
    git config --global user.name "$GIT_USER"
    git config --global user.email "$GIT_EMAIL"
    git config --global credential.helper store
    dolt config --global --add user.name "$GIT_USER"
    dolt config --global --add user.email "$GIT_EMAIL"
fi

# Fetch credentials from 1Password if service account token is available
if [ -n "$OP_SERVICE_ACCOUNT_TOKEN" ]; then
  # Fetch deploy key for git auth
  /app/gastown/scripts/op/configure-github-deploykey-auth.sh
  # Fetch signing key for commit signing
  /app/gastown/scripts/configure-git-signing.sh
  # Fetch gh CLI token
  /app/gastown/scripts/op/configure-gh-auth.sh
fi

if [ ! -f /gt/mayor/town.json ]; then
    echo "Initializing Gas Town workspace at /gt..."
    /app/gastown/gt install /gt --git
else
    echo "Refreshing Gas Town workspace at /gt..."
    /app/gastown/gt install /gt --git --force
fi

# Start the web dashboard (mapped to host via DASHBOARD_PORT, default 8080)
gt dashboard --bind 0.0.0.0 --port 8080 &

exec "$@"
