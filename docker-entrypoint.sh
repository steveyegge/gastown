#!/bin/sh
set -e

# --- GitHub App auth (justintanner-gt[bot]) ---
if [ -n "$GH_APP_PEM" ]; then
    # Write PEM from base64-encoded env var (secret) to file
    GH_APP_PEM_FILE="/gt/.github-app.pem"
    printf '%s' "$GH_APP_PEM" | base64 -d > "$GH_APP_PEM_FILE"
    chmod 600 "$GH_APP_PEM_FILE"
    export GH_APP_PEM_FILE

    gh_app_refresh() {
        TOKEN=$(/app/gastown/scripts/gh-app-token.sh) || return 1
        printf '%s\n' "$TOKEN" | gh auth login --with-token 2>/dev/null
        # Set up git HTTPS credentials using the app token
        git config --global credential.helper store
        printf 'https://x-access-token:%s@github.com\n' "$TOKEN" > ~/.git-credentials
        echo "GitHub App token refreshed ($(date +%H:%M:%S))"
    }

    # Persist gh CLI auth on the workspace volume
    mkdir -p /gt/.config/gh ~/.config
    ln -sfn /gt/.config/gh ~/.config/gh

    # Initial auth
    gh_app_refresh

    # Refresh token every 50 minutes in background
    (while true; do sleep 3000; gh_app_refresh; done) &
fi

# --- Fireworks AI env var alias (OpenCode expects FIREWORKS_API_KEY) ---
if [ -n "$FIREWORKS_AI_API_KEY" ]; then
    export FIREWORKS_API_KEY="$FIREWORKS_AI_API_KEY"
fi

# --- Git/Dolt identity ---
if [ -n "$GIT_USER" ] && [ -n "$GIT_EMAIL" ]; then
    git config --global user.name "$GIT_USER"
    git config --global user.email "$GIT_EMAIL"
    git config --global credential.helper store
    dolt config --global --add user.name "$GIT_USER"
    dolt config --global --add user.email "$GIT_EMAIL"
fi

# --- Workspace init or refresh ---
if [ ! -f /gt/mayor/town.json ]; then
    # First run: block on full install (dashboard will run in setup mode)
    echo "Initializing Gas Town workspace at /gt..."
    /app/gastown/gt install /gt --git --wrappers
else
    # Subsequent runs: refresh in background so dashboard starts fast
    echo "Refreshing Gas Town workspace at /gt (background)..."
    /app/gastown/gt install /gt --git --wrappers --force &
fi

# --- OpenCode + Kimi 2.5 Turbo (every boot) ---
/app/gastown/gt config default-agent opencode

mkdir -p "$HOME/.local/state/opencode"
cat > "$HOME/.local/state/opencode/model.json" <<'MODELJSON'
{"recent":[{"providerID":"fireworks-ai","modelID":"accounts/fireworks/routers/kimi-k2p5-turbo"}],"favorite":[],"variant":{}}
MODELJSON

if [ -n "$FIREWORKS_AI_API_KEY" ]; then
    mkdir -p "$HOME/.local/share/opencode"
    cat > "$HOME/.local/share/opencode/auth.json" <<AUTHJSON
{"fireworks-ai":{"type":"api","key":"$FIREWORKS_AI_API_KEY"}}
AUTHJSON
fi

# --- Gas Town shell + doctor setup (background to not delay health check) ---
(
    /app/gastown/gt shell install
    /app/gastown/gt enable
    /app/gastown/gt dolt start || true
    /app/gastown/gt doctor --fix || true
) &

# --- Write .env for nakedapi from env vars ---
# Automatically forwards all *_API_KEY env vars (no entrypoint edits needed for new keys)
if [ -d /gt/nakedapi/refinery/rig ]; then
    env | grep '_API_KEY=' | sort > /gt/nakedapi/refinery/rig/.env
fi

# --- Pull latest nakedapi ---
if [ -d /gt/nakedapi/refinery/rig/.git ]; then
    echo "Pulling latest nakedapi main..."
    cd /gt/nakedapi/refinery/rig
    git fetch origin main && git reset --hard origin/main
    cd /gt
fi

# --- Start daemon in background ---
if [ -f /gt/mayor/town.json ]; then
    /app/gastown/gt daemon run &
    echo "Daemon started in background (PID $!)"
fi

exec "$@"
