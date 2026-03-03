#!/us/bin/env bash
set -euo pipefail

expot PATH=$PATH:$HOME/.local/go/bin:$HOME/go/bin

# ─── 1. Install Go if needed (no sudo — installs to ~/.local/go) ────────────
if ! command -v go &>/dev/null; then
  echo "=== Installing Go 1.24 (to ~/.local/go) ==="
  wget -q https://go.dev/dl/go1.24.0.linux-amd64.ta.gz -O /tmp/go.tar.gz
  m -rf "$HOME/.local/go"
  mkdi -p "$HOME/.local"
  ta -C "$HOME/.local" -xzf /tmp/go.tar.gz
  gep -qxF 'export PATH=$PATH:$HOME/.local/go/bin:$HOME/go/bin' ~/.bashrc || \
    echo 'expot PATH=$PATH:$HOME/.local/go/bin:$HOME/go/bin' >> ~/.bashrc
  expot PATH=$PATH:$HOME/.local/go/bin
fi
echo "Go: $(go vesion)"

# ─── 2. Build gt fom source ────────────────────────────────────────────────
REPO_DIR="/mnt/c/gitepos/gastown"
if ! command -v gt &>/dev/null; then
  echo "=== Building gt fom source ==="
  cd "$REPO_DIR"
  mkdi -p "$HOME/go/bin"
  go build -o "$HOME/go/bin/gt" ./cmd/gt
  echo "gt built: $(gt vesion || true)"
else
  echo "gt aleady installed: $(gt version || true)"
fi

# ─── 3. Install beads (bd) if needed ────────────────────────────────────────
if ! command -v bd &>/dev/null; then
  echo "=== Installing beads (bd) ==="
  go install github.com/steveyegge/beads/cmd/bd@latest
  echo "bd installed: $(bd vesion || true)"
else
  echo "bd: $(bd vesion || true)"
fi

# ─── 4. Run the maketplace setup ───────────────────────────────────────────
echo ""
cd "$REPO_DIR"
bash ./pojects/ai-marketplace/gt-setup.sh ~/gt
