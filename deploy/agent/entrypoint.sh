#!/bin/bash
# Generic Gas Town agent entrypoint: starts a screen session with Claude.
#
# This entrypoint handles all agent roles (mayor, deacon, crew, polecat,
# witness, refinery). The controller sets role-specific env vars before
# the pod starts; this script reads GT_ROLE to configure the workspace
# and launch Claude with the correct context.
#
# Required environment variables (set by pod manager):
#   GT_ROLE       - agent role (mayor, deacon, crew, polecat, witness, refinery)
#   GT_RIG        - rig name (empty for town-level roles)
#   GT_AGENT      - agent name
#
# Optional:
#   GT_COMMAND    - command to run in screen (default: "claude --dangerously-skip-permissions")
#   BD_DAEMON_HOST - beads daemon URL
#   BD_DAEMON_PORT - beads daemon port
#   GT_SESSION_RESUME - set to "1" to auto-resume previous Claude session on restart

set -euo pipefail

ROLE="${GT_ROLE:-unknown}"
RIG="${GT_RIG:-}"
AGENT="${GT_AGENT:-unknown}"
WORKSPACE="/home/agent/gt"
SESSION_RESUME="${GT_SESSION_RESUME:-1}"

echo "[entrypoint] Starting ${ROLE} agent: ${AGENT} (rig: ${RIG:-none})"

# ── Workspace setup ──────────────────────────────────────────────────────

# Initialize git repo in workspace if not already present.
# Persistent roles (mayor, crew, etc.) keep state across restarts via PVC.
if [ ! -d "${WORKSPACE}/.git" ]; then
    echo "[entrypoint] Initializing git repo in ${WORKSPACE}"
    cd "${WORKSPACE}"
    git init -q
    git config user.name "${GIT_AUTHOR_NAME:-${ROLE}}"
    git config user.email "${ROLE}@gastown.local"
else
    echo "[entrypoint] Git repo already exists in ${WORKSPACE}"
    cd "${WORKSPACE}"
fi

# Initialize beads if not already present and bd binary exists.
if [ ! -d "${WORKSPACE}/.beads" ] && command -v bd &>/dev/null; then
    echo "[entrypoint] Initializing beads in ${WORKSPACE}"
    bd init --non-interactive 2>/dev/null || true
fi

# ── Role-specific setup ──────────────────────────────────────────────────

case "${ROLE}" in
    mayor|deacon)
        echo "[entrypoint] Town-level singleton: ${ROLE}"
        # Mayor/deacon maintain persistent state in the PVC workspace.
        # Create role-specific working directory.
        mkdir -p "${WORKSPACE}/${ROLE}"
        ;;
    crew)
        echo "[entrypoint] Crew member: ${AGENT}"
        mkdir -p "${WORKSPACE}/crew/${AGENT}"
        ;;
    polecat)
        echo "[entrypoint] Polecat: ${AGENT} (ephemeral)"
        # Polecats use EmptyDir — no persistent state.
        ;;
    witness|refinery)
        echo "[entrypoint] Singleton: ${ROLE}"
        mkdir -p "${WORKSPACE}/${ROLE}"
        ;;
    *)
        echo "[entrypoint] WARNING: Unknown role '${ROLE}', proceeding with defaults"
        ;;
esac

# ── Read agent config from ConfigMap mount if present ────────────────────

CONFIG_DIR="/etc/agent-pod"
if [ -f "${CONFIG_DIR}/prompt" ]; then
    STARTUP_PROMPT="$(cat "${CONFIG_DIR}/prompt")"
    echo "[entrypoint] Loaded startup prompt from ConfigMap"
fi

# ── Session persistence ──────────────────────────────────────────────────
#
# Persist Claude state (~/.claude) and coop session artifacts on the
# workspace PVC so they survive pod restarts.  The PVC is already mounted
# at /home/agent/gt.  We store session state under .state/ on the PVC
# and symlink the ephemeral home-directory paths into it.
#
#   PVC layout:
#     /home/agent/gt/.state/claude/     →  symlinked from ~/.claude
#     /home/agent/gt/.state/coop/       →  symlinked from $XDG_STATE_HOME/coop

STATE_DIR="${WORKSPACE}/.state"
CLAUDE_STATE="${STATE_DIR}/claude"
COOP_STATE="${STATE_DIR}/coop"

mkdir -p "${CLAUDE_STATE}" "${COOP_STATE}"

# Symlink ~/.claude → PVC-backed directory.
CLAUDE_DIR="${HOME}/.claude"
# Remove the ephemeral dir (or stale symlink) and replace with symlink.
rm -rf "${CLAUDE_DIR}"
ln -sfn "${CLAUDE_STATE}" "${CLAUDE_DIR}"
echo "[entrypoint] Linked ${CLAUDE_DIR} → ${CLAUDE_STATE} (PVC-backed)"

# Copy credentials from staging mount into PVC dir (K8s secret mount lives
# at /tmp/claude-credentials/credentials.json, set by helm chart).
CREDS_STAGING="/tmp/claude-credentials/credentials.json"
if [ -f "${CREDS_STAGING}" ]; then
    cp "${CREDS_STAGING}" "${CLAUDE_STATE}/.credentials.json"
    echo "[entrypoint] Copied Claude credentials to PVC state dir"
fi

# Set XDG_STATE_HOME so coop writes session artifacts to the PVC.
export XDG_STATE_HOME="${STATE_DIR}"
echo "[entrypoint] XDG_STATE_HOME=${XDG_STATE_HOME}"

# ── Claude settings ──────────────────────────────────────────────────────

# Write minimal settings.json for bypass permissions (idempotent).
cat > "${CLAUDE_DIR}/settings.json" <<'SETTINGS'
{
  "permissions": {
    "allow": [
      "Bash(*)",
      "Read(*)",
      "Write(*)",
      "Edit(*)",
      "Glob(*)",
      "Grep(*)",
      "WebFetch(*)",
      "WebSearch(*)"
    ],
    "deny": []
  }
}
SETTINGS

# Write CLAUDE.md with role context if not already present
if [ ! -f "${WORKSPACE}/CLAUDE.md" ]; then
    cat > "${WORKSPACE}/CLAUDE.md" <<CLAUDEMD
# Gas Town Agent: ${ROLE}

You are the **${ROLE}** agent in a Gas Town rig${RIG:+ (rig: ${RIG})}.
Agent name: ${AGENT}

Run \`gt prime\` for full context.
CLAUDEMD
fi

# ── Skip Claude onboarding wizard ─────────────────────────────────────────

printf '{"hasCompletedOnboarding":true,"lastOnboardingVersion":"2.1.37","preferredTheme":"dark"}\n' > "${HOME}/.claude.json"

# ── Session resume detection ─────────────────────────────────────────────
#
# If this is a restart (PVC already has Claude session logs from a previous
# run), use coop --resume to let coop discover and resume the session.
# Coop handles extracting the conversation ID from the log and passing
# --resume <id> or --continue to Claude (avoids --session-id conflict).

COOP_RESUME_FLAG=""
if [ "${SESSION_RESUME}" = "1" ]; then
    # Find the most recent session log. Use -print0/sort to avoid issues
    # when find returns no results (plain xargs runs ls with no args → bad).
    LATEST_LOG=""
    if [ -d "${CLAUDE_STATE}/projects" ]; then
        LATEST_LOG=$(find "${CLAUDE_STATE}/projects" -name '*.jsonl' -type f -printf '%T@ %p\n' 2>/dev/null \
            | sort -rn | head -1 | cut -d' ' -f2-)
    fi

    if [ -n "${LATEST_LOG}" ]; then
        COOP_RESUME_FLAG="--resume ${LATEST_LOG}"
        echo "[entrypoint] Will attempt to resume from: ${LATEST_LOG}"
    else
        echo "[entrypoint] No previous session logs found — starting fresh"
    fi
fi

# ── Start coop + Claude ──────────────────────────────────────────────────

cd "${WORKSPACE}"

COOP_CMD="coop --agent=claude --port 8080 --health-port 9090 --cols 200 --rows 50"

echo "[entrypoint] Starting coop + claude (${ROLE}/${AGENT})"

if [ -n "${COOP_RESUME_FLAG}" ]; then
    echo "[entrypoint] Trying resume..."
    # Use || to prevent set -e from killing the script on resume failure
    ${COOP_CMD} ${COOP_RESUME_FLAG} -- claude --dangerously-skip-permissions || {
        echo "[entrypoint] Resume failed (exit $?), starting fresh"
        exec ${COOP_CMD} -- claude --dangerously-skip-permissions
    }
else
    exec ${COOP_CMD} -- claude --dangerously-skip-permissions
fi
