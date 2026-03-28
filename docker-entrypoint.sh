#!/bin/sh
set -e

# --- SSH key: generate on first run, persists in volume ---
SSH_DIR="/gt/.ssh"
if [ ! -f "$SSH_DIR/id_ed25519" ]; then
    mkdir -p "$SSH_DIR" && chmod 700 "$SSH_DIR"
    ssh-keygen -t ed25519 -C "gastown-server" -f "$SSH_DIR/id_ed25519" -N ""
    echo ""
    echo "========================================="
    echo "ADD THIS PUBLIC KEY TO GITHUB:"
    echo "========================================="
    cat "$SSH_DIR/id_ed25519.pub"
    echo "========================================="
    echo ""
fi

# Symlink SSH keys to home so git/ssh find them
mkdir -p ~/.ssh && chmod 700 ~/.ssh
ln -sf "$SSH_DIR/id_ed25519" ~/.ssh/id_ed25519
ln -sf "$SSH_DIR/id_ed25519.pub" ~/.ssh/id_ed25519.pub
ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null
git config --global url."git@github.com:".insteadOf "https://github.com/"

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
    echo "Initializing Gas Town workspace at /gt..."
    /app/gastown/gt install /gt --git
else
    echo "Refreshing Gas Town workspace at /gt..."
    /app/gastown/gt install /gt --git --force
fi

# --- Write .env for nakedapi from env vars ---
if [ -d /gt/nakedapi/refinery/rig ]; then
    cat > /gt/nakedapi/refinery/rig/.env << ENVFILE
OPENAI_API_KEY=${OPENAI_API_KEY:-}
KIE_API_KEY=${KIE_API_KEY:-}
XAI_API_KEY=${XAI_API_KEY:-}
FAL_API_KEY=${FAL_API_KEY:-}
KIMI_CODING_API_KEY=${KIMI_CODING_API_KEY:-}
ENVFILE
fi

# --- Pull latest nakedapi ---
if [ -d /gt/nakedapi/refinery/rig/.git ]; then
    echo "Pulling latest nakedapi main..."
    cd /gt/nakedapi/refinery/rig
    git fetch origin main && git reset --hard origin/main
    cd /gt
fi

# --- Start daemon in background (production) ---
if [ -f /gt/mayor/town.json ]; then
    /app/gastown/gt daemon run &
    echo "Daemon started in background (PID $!)"
fi

exec "$@"
