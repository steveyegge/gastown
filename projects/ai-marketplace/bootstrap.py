import os, subprocess, shutil

home = os.path.expanduser('~')
script = home + '/bootstrap-gt.sh'

lines = [
    '#!/usr/bin/env bash',
    'set -euo pipefail',
    'export PATH="$PATH:$HOME/.local/go/bin:$HOME/go/bin"',
    'if ! command -v go &>/dev/null; then',
    '  echo "=== Installing Go 1.24 (no sudo) ==="',
    '  wget -q https://go.dev/dl/go1.24.0.linux-amd64.tar.gz -O /tmp/go.tar.gz',
    '  rm -rf "$HOME/.local/go" && mkdir -p "$HOME/.local"',
    '  tar -C "$HOME/.local" -xzf /tmp/go.tar.gz',
    '  export PATH="$PATH:$HOME/.local/go/bin"',
    "  grep -qxF 'export PATH=" + '"$PATH:$HOME/.local/go/bin:$HOME/go/bin"' + "' ~/.bashrc || echo 'export PATH=" + '"$PATH:$HOME/.local/go/bin:$HOME/go/bin"' + "' >> ~/.bashrc",
    'fi',
    'echo "Go: $(go version)"',
    'REPO=/mnt/c/gitrepos/gastown',
    'echo "=== Building gt from source ==="',
    'mkdir -p "$HOME/go/bin"',
    'cd "$REPO" && go build -o "$HOME/go/bin/gt" ./cmd/gt && echo "gt OK" || echo "WARN: gt build had issues"',
    'echo "=== Installing beads (bd) ==="',
    'go install github.com/steveyegge/beads/cmd/bd@latest && echo "bd OK" || echo "WARN: bd install failed"',
    'mkdir -p "$HOME/gt"',
    'echo ""',
    'echo "=== Gas Town workspace ready at ~/gt ==="',
    'echo ""',
    'echo "Sprint 1 convoy (mkt-cv-s1):"',
    'echo "  mkt-00001  FR-1: Asset catalog listing types"',
    'echo "  mkt-00002  FR-2: Marketplace search and filter"',
    'echo "  mkt-00003  FR-3: Asset detail page"',
    'echo "  mkt-00004  FR-4: Ratings and usage metrics"',
    'echo "  mkt-00005  FR-5: Add to workspace"',
    'echo "  mkt-00006  FR-9: Visual orchestration canvas  [critical]"',
    'echo ""',
    'if command -v gt &>/dev/null; then',
    '  echo "gt version: $(gt version 2>&1 || true)"',
    'else',
    '  echo "gt not in PATH yet - run: source ~/.bashrc"',
    'fi',
    'if command -v bd &>/dev/null; then',
    '  echo "bd OK"',
    'else',
    '  echo "bd not in PATH yet - run: source ~/.bashrc"',
    'fi',
    'echo ""',
    'echo "Next: source ~/.bashrc && gt mayor attach"',
]

with open(script, 'w', newline='\n') as f:
    f.write('\n'.join(lines) + '\n')

os.chmod(script, 0o755)
print('Script written:', script)
subprocess.run(['bash', script])
