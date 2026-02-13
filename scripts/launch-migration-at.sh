#!/usr/bin/env bash
# launch-migration-at.sh — Launch the Migration Hardening Agent
#
# Launches a solo Claude Code agent that works autonomously through the
# migration hardening mission. Pushes directly to main.
#
# Usage:
#   ./scripts/launch-migration-at.sh              # Launch (interactive)
#   ./scripts/launch-migration-at.sh --dry-run    # Show what would be launched
#   ./scripts/launch-migration-at.sh --bg         # Launch in tmux session

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BOLD='\033[1m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

DRY_RUN=false
BG=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run) DRY_RUN=true; shift ;;
        --bg) BG=true; shift ;;
        -h|--help)
            echo "Usage: $0 [--dry-run] [--bg]"
            echo "  --dry-run  Show what would be launched"
            echo "  --bg       Launch in background tmux session"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo -e "${BOLD}${BLUE}"
echo "=============================================="
echo "  Migration Hardening Agent (Solo)"
echo "=============================================="
echo -e "${NC}"

MISSION_FILE="$REPO_ROOT/.claude/agents/at-migration-mission.md"
AGENT_FILE="$REPO_ROOT/.claude/agents/migration-hardener.md"

for f in "$MISSION_FILE" "$AGENT_FILE"; do
    if [[ ! -f "$f" ]]; then
        echo "ERROR: Required file not found: $f"
        exit 1
    fi
done

echo -e "${GREEN}Repo:${NC}    $REPO_ROOT"
echo -e "${GREEN}Agent:${NC}   $AGENT_FILE"
echo -e "${GREEN}Mission:${NC} $MISSION_FILE ($(wc -l < "$MISSION_FILE" | tr -d ' ') lines)"
echo ""

command -v claude >/dev/null 2>&1 || { echo "ERROR: claude CLI not found"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "ERROR: go toolchain not found"; exit 1; }
command -v gcloud >/dev/null 2>&1 || { echo "WARN: gcloud not found — VM testing may not work"; }
echo -e "${GREEN}Prerequisites:${NC} OK"

# Verify VM access
echo -n "Checking VM access... "
if gcloud compute ssh migration-test-lab --zone=us-west1-b --command="echo ok" 2>/dev/null; then
    echo -e "${GREEN}VM reachable${NC}"
else
    echo -e "WARN: VM not reachable (continuing anyway)"
fi
echo ""

if $DRY_RUN; then
    echo "[DRY-RUN] Would launch Claude Code with:"
    echo "  Mode: Solo agent, acceptEdits permission"
    echo "  Agent: migration-hardener"
    echo "  System prompt: at-migration-mission.md"
    echo "  Working dir: $REPO_ROOT"
    exit 0
fi

LAUNCH_CMD=(
    claude
    --permission-mode acceptEdits
    --agent migration-hardener
    --system-prompt "$(cat "$MISSION_FILE")"
    --append-system-prompt "
You are a solo migration hardening agent. Work through the mission document
systematically, phase by phase. You have full autonomy.

Working directory: $REPO_ROOT
Push target: origin/main (direct push, no PRs)
VM: migration-test-lab (access via gcloud compute ssh)

WORKFLOW:
1. Start with Phase 1 (Audit) — read all migration code
2. Phase 2 — systematically work through the edge case matrix, writing Go tests
3. Phase 3 — fix bugs you discover
4. Phase 4 — run VM integration tests with multiple configurations
5. Phase 5 — document and report

CRITICAL REMINDERS:
- Run 'go test ./... && golangci-lint run ./...' before every push
- After EVERY migration test: check for zombie artifacts (bd daemons, SQLite files, JSONL files)
- Commit frequently, push to main regularly
- Use TaskCreate to track your progress
- If context gets full, commit+push everything, then handoff

START NOW. Begin with Phase 1."
)

cd "$REPO_ROOT"

if $BG; then
    SESSION_NAME="migration-hardener-$(date +%H%M)"
    echo -e "${BOLD}Launching in tmux session: ${SESSION_NAME}${NC}"
    tmux new-session -d -s "$SESSION_NAME" "${LAUNCH_CMD[*]}"
    echo "Attached to: tmux attach -t $SESSION_NAME"
else
    echo -e "${BOLD}Launching agent...${NC}"
    echo ""
    exec "${LAUNCH_CMD[@]}"
fi
