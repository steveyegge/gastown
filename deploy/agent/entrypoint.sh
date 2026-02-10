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

# Set global git config FIRST so safe.directory is set before any repo ops.
# The workspace volume mount is owned by root (EmptyDir/PVC) but we run as
# UID 1000 — git's dubious-ownership check would block all operations without this.
git config --global user.name "${GIT_AUTHOR_NAME:-${ROLE}}"
git config --global user.email "${ROLE}@gastown.local"
git config --global --add safe.directory '*'

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

# ── Gas Town workspace structure ───────────────────────────────────────
#
# gt prime detects the agent role from directory structure.
# The minimal required layout for a town-level workspace:
#
#   /home/agent/gt/              ← town root (WORKSPACE)
#   ├── mayor/town.json          ← primary workspace marker
#   ├── mayor/rigs.json          ← rig registry
#   ├── CLAUDE.md                ← town root identity anchor
#   └── .beads/config.yaml       ← daemon connection config

TOWN_NAME="${GT_TOWN_NAME:-town}"

# Create workspace marker (idempotent — skip if already exists on PVC).
if [ ! -f "${WORKSPACE}/mayor/town.json" ]; then
    echo "[entrypoint] Creating Gas Town workspace structure"
    mkdir -p "${WORKSPACE}/mayor"
    cat > "${WORKSPACE}/mayor/town.json" <<TOWNJSON
{"type":"town","version":2,"name":"${TOWN_NAME}","created_at":"$(date -u +%Y-%m-%dT%H:%M:%SZ)"}
TOWNJSON
    cat > "${WORKSPACE}/mayor/rigs.json" <<RIGSJSON
{"version":1,"rigs":{}}
RIGSJSON
fi

# Create role-specific directories.
case "${ROLE}" in
    mayor|deacon)
        echo "[entrypoint] Town-level singleton: ${ROLE}"
        mkdir -p "${WORKSPACE}/${ROLE}"
        ;;
    crew)
        echo "[entrypoint] Crew member: ${AGENT}"
        mkdir -p "${WORKSPACE}/crew/${AGENT}"
        ;;
    polecat)
        echo "[entrypoint] Polecat: ${AGENT} (ephemeral)"
        ;;
    witness|refinery)
        echo "[entrypoint] Singleton: ${ROLE}"
        mkdir -p "${WORKSPACE}/${ROLE}"
        ;;
    *)
        echo "[entrypoint] WARNING: Unknown role '${ROLE}', proceeding with defaults"
        ;;
esac

# ── Daemon connection via gt connect ────────────────────────────────────
#
# If BD_DAEMON_HOST is set and .beads/config.yaml doesn't exist yet,
# use gt connect --url to persist the daemon connection config.
# This lets bd and gt commands talk to the remote daemon.

if [ -n "${BD_DAEMON_HOST:-}" ]; then
    # Use the HTTP port for daemon connection (bd CLI auto-detects HTTP URLs).
    DAEMON_HTTP_PORT="${BD_DAEMON_HTTP_PORT:-9080}"
    DAEMON_URL="http://${BD_DAEMON_HOST}:${DAEMON_HTTP_PORT}"
    echo "[entrypoint] Connecting to daemon at ${DAEMON_URL}"
    # gt connect needs to be in the workspace dir
    cd "${WORKSPACE}"
    gt connect --url "${DAEMON_URL}" --token "${BD_DAEMON_TOKEN:-}" 2>&1 || {
        echo "[entrypoint] WARNING: gt connect failed, creating config manually"
        mkdir -p "${WORKSPACE}/.beads"
        cat > "${WORKSPACE}/.beads/config.yaml" <<BEADSCFG
daemon-host: "${DAEMON_URL}"
daemon-token: "${BD_DAEMON_TOKEN:-}"
BEADSCFG
    }
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
#
# User-level settings (permissions) always written to ~/.claude/settings.json.
# Hooks come from config bead materialization if available, otherwise static.

cat > "${CLAUDE_DIR}/settings.json" <<'PERMISSIONS'
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
PERMISSIONS

# Try config bead materialization (writes to workspace .claude/settings.json).
# This queries the daemon for claude-hooks config beads and merges them by
# specificity (global → role → agent). Falls back to static hooks if no
# config beads exist or daemon is unreachable.
MATERIALIZE_SCOPE="${GT_TOWN_NAME:-town}/${GT_RIG:-}/${ROLE}/${AGENT}"
MATERIALIZED=0

if command -v gt &>/dev/null; then
    echo "[entrypoint] Materializing hooks from config beads (scope: ${MATERIALIZE_SCOPE})"
    cd "${WORKSPACE}"
    if gt config materialize --hooks --scope="${MATERIALIZE_SCOPE}" 2>&1; then
        # Verify the file was written with actual hooks content
        if grep -q '"hooks"' "${WORKSPACE}/.claude/settings.json" 2>/dev/null; then
            MATERIALIZED=1
            echo "[entrypoint] Hooks materialized from config beads"
        fi
    fi
fi

if [ "${MATERIALIZED}" = "0" ]; then
    echo "[entrypoint] No config beads found, writing static hooks"
    # Write project-level settings with hooks to workspace .claude/settings.json.
    # These must match the canonical templates in internal/claude/config/.
    # Interactive roles (mayor, crew) check mail on UserPromptSubmit.
    # Autonomous roles (polecat, witness, refinery, deacon) check mail on SessionStart.
    mkdir -p "${WORKSPACE}/.claude"

    case "${ROLE}" in
        polecat|witness|refinery|deacon|boot)
            HOOK_TYPE="autonomous"
            ;;
        *)
            HOOK_TYPE="interactive"
            ;;
    esac

    if [ "${HOOK_TYPE}" = "autonomous" ]; then
        cat > "${WORKSPACE}/.claude/settings.json" <<'HOOKS'
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook && gt mail check --inject && gt nudge deacon session-started"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && _stdin=$(cat) && (gt decision auto-close --inject || true) && (gt mail check --inject || true) && (echo \"$_stdin\" | bd decision check --inject || true) && (echo \"$_stdin\" | gt decision turn-clear || true)"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt inject drain --quiet && gt nudge drain --quiet"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && _stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check --soft"
          }
        ]
      }
    ]
  }
}
HOOKS
    else
        cat > "${WORKSPACE}/.claude/settings.json" <<'HOOKS'
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook && gt nudge deacon session-started"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && _stdin=$(cat) && (gt decision auto-close --inject || true) && (gt mail check --inject || true) && (echo \"$_stdin\" | bd decision check --inject || true) && (echo \"$_stdin\" | gt decision turn-clear || true)"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt inject drain --quiet && gt nudge drain --quiet"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && _stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check"
          }
        ]
      }
    ]
  }
}
HOOKS
    fi
fi

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

# ── Start coop + Claude ──────────────────────────────────────────────────
#
# We keep bash as PID 1 (no exec) so the pod survives if Claude/coop exit
# (e.g. user sends Ctrl+C which delivers SIGINT to Claude via the PTY).
# On child exit we clean up FIFO pipes and restart with --resume.
# SIGTERM from K8s is forwarded to coop for graceful shutdown.

cd "${WORKSPACE}"

COOP_CMD="coop --agent=claude --port 8080 --health-port 9090 --cols 200 --rows 50"

# Coop log level (overridable via pod env).
export COOP_LOG_LEVEL="${COOP_LOG_LEVEL:-info}"

# ── Signal forwarding ─────────────────────────────────────────────────────
# Forward SIGTERM from K8s to coop so it can do graceful shutdown.
COOP_PID=""
forward_signal() {
    if [ -n "${COOP_PID}" ]; then
        echo "[entrypoint] Forwarding $1 to coop (pid ${COOP_PID})"
        kill -"$1" "${COOP_PID}" 2>/dev/null || true
        wait "${COOP_PID}" 2>/dev/null || true
    fi
    exit 0
}
trap 'forward_signal TERM' TERM
trap 'forward_signal INT' INT

# ── Restart loop ──────────────────────────────────────────────────────────
# Max restarts to avoid infinite crash loop. Reset on successful long-lived run.
MAX_RESTARTS="${COOP_MAX_RESTARTS:-10}"
restart_count=0
MIN_RUNTIME_SECS=30  # If coop runs longer than this, reset the restart counter.

while true; do
    if [ "${restart_count}" -ge "${MAX_RESTARTS}" ]; then
        echo "[entrypoint] Max restarts (${MAX_RESTARTS}) reached, exiting"
        exit 1
    fi

    # Clean up stale FIFO pipes before each start (coop creates them per session).
    if [ -d "${COOP_STATE}/sessions" ]; then
        find "${COOP_STATE}/sessions" -name 'hook.pipe' -delete 2>/dev/null || true
    fi

    # Find latest session log for resume (respects GT_SESSION_RESUME=0 to disable).
    RESUME_FLAG=""
    if [ "${SESSION_RESUME}" = "1" ] && [ -d "${CLAUDE_STATE}/projects" ]; then
        LATEST_LOG=$(find "${CLAUDE_STATE}/projects" -name '*.jsonl' -type f -printf '%T@ %p\n' 2>/dev/null \
            | sort -rn | head -1 | cut -d' ' -f2-)
        if [ -n "${LATEST_LOG}" ]; then
            RESUME_FLAG="--resume ${LATEST_LOG}"
        fi
    fi

    start_time=$(date +%s)

    if [ -n "${RESUME_FLAG}" ]; then
        echo "[entrypoint] Starting coop + claude (${ROLE}/${AGENT}) with resume"
        ${COOP_CMD} ${RESUME_FLAG} -- claude --dangerously-skip-permissions &
        COOP_PID=$!
        wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
        COOP_PID=""

        # If resume failed quickly, try fresh start instead.
        elapsed=$(( $(date +%s) - start_time ))
        if [ "${elapsed}" -lt 5 ] && [ "${exit_code}" -ne 0 ]; then
            echo "[entrypoint] Resume failed (exit ${exit_code}), trying fresh start"
            ${COOP_CMD} -- claude --dangerously-skip-permissions &
            COOP_PID=$!
            start_time=$(date +%s)
            wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
            COOP_PID=""
        fi
    else
        echo "[entrypoint] Starting coop + claude (${ROLE}/${AGENT})"
        ${COOP_CMD} -- claude --dangerously-skip-permissions &
        COOP_PID=$!
        wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
        COOP_PID=""
    fi

    elapsed=$(( $(date +%s) - start_time ))
    echo "[entrypoint] Coop exited with code ${exit_code} after ${elapsed}s"

    # If coop ran long enough, reset the restart counter.
    if [ "${elapsed}" -ge "${MIN_RUNTIME_SECS}" ]; then
        restart_count=0
    fi

    restart_count=$((restart_count + 1))
    echo "[entrypoint] Restarting (attempt ${restart_count}/${MAX_RESTARTS}) in 2s..."
    sleep 2
done
