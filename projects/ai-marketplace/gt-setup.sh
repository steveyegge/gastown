#!/usr/bin/env bash
# gt-setup.sh — Bootstrap this project as a Gas Town rig (run on Linux/macOS or WSL)
# Usage: ./gt-setup.sh [workspace-path]
set -euo pipefail

WORKSPACE="${1:-$HOME/gt}"
PROJECT_NAME="ai-marketplace"
REPO_URL="https://github.com/your-org/ai-marketplace.git"

echo "=== Gas Town: AI Marketplace Setup ==="
echo "Workspace: $WORKSPACE"
echo ""

# 1. Check gt is installed
if ! command -v gt &> /dev/null; then
  echo "Installing gt from source..."
  go install github.com/steveyegge/gastown/cmd/gt@latest
fi

# 2. Check bd is installed
if ! command -v bd &> /dev/null; then
  echo "Installing beads (bd)..."
  go install github.com/steveyegge/beads/cmd/bd@latest
fi

# 3. Create workspace if needed
if [ ! -d "$WORKSPACE" ]; then
  echo "Creating Gas Town workspace at $WORKSPACE..."
  gt install "$WORKSPACE" --git
fi

cd "$WORKSPACE"

# 4. Add the marketplace rig
echo "Adding ai-marketplace rig..."
gt rig add "$PROJECT_NAME" "$REPO_URL" --prefix mkt || echo "Rig may already exist, skipping"

# 5. Create crew workspace
echo "Creating crew workspace..."
gt crew add dev --rig "$PROJECT_NAME" || echo "Crew may already exist, skipping"

# 6. Import beads from the pre-created issues file
echo "Importing PRD issues into beads..."
cd "$WORKSPACE/$PROJECT_NAME" || exit 1
if [ -f ".beads/issues.jsonl" ]; then
  bd import --file .beads/issues.jsonl 2>/dev/null || echo "  (beads import: some items may already exist)"
fi

# 7. Create Sprint 1 convoy
echo "Creating Sprint 1 convoy..."
gt convoy create "Sprint 1: Marketplace + Orchestrator Canvas" \
  mkt-00001 mkt-00002 mkt-00003 mkt-00004 mkt-00005 mkt-00006 \
  --notify mayor 2>/dev/null || echo "  (convoy may already exist)"

# 8. Launch Mayor
echo ""
echo "=== Setup complete! ==="
echo ""
echo "Next steps:"
echo "  cd $WORKSPACE"
echo "  gt mayor attach          # launch Mayor AI coordinator"
echo "  gt convoy list           # see active sprint convoys"
echo "  gt sling mkt-00006 --to polecats  # sling orchestrator canvas work to a polecat"
echo ""
echo "To develop locally:"
echo "  cd $WORKSPACE/$PROJECT_NAME/apps/web && npm install && npm run dev"
echo "  cd $WORKSPACE/$PROJECT_NAME/apps/api && npm install && npm run dev"
