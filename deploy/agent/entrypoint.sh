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

# ── Git credentials ────────────────────────────────────────────────────
# If GIT_USERNAME and GIT_TOKEN are set (from ExternalSecret), configure
# git credential-store so clone/push to github.com works automatically.
if [ -n "${GIT_USERNAME:-}" ] && [ -n "${GIT_TOKEN:-}" ]; then
    CRED_FILE="${HOME}/.git-credentials"
    echo "https://${GIT_USERNAME}:${GIT_TOKEN}@github.com" > "${CRED_FILE}"
    chmod 600 "${CRED_FILE}"
    git config --global credential.helper "store --file=${CRED_FILE}"
    echo "[entrypoint] Git credentials configured for ${GIT_USERNAME}@github.com"
fi

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

# Seed credentials from K8s secret mount if PVC doesn't have them yet.
# IMPORTANT: Don't overwrite PVC credentials on restart — the refresh loop
# rotates refresh tokens, so the PVC copy is newer than the K8s secret.
CREDS_STAGING="/tmp/claude-credentials/credentials.json"
CREDS_PVC="${CLAUDE_STATE}/.credentials.json"
if [ -f "${CREDS_STAGING}" ] && [ ! -f "${CREDS_PVC}" ]; then
    cp "${CREDS_STAGING}" "${CREDS_PVC}"
    echo "[entrypoint] Seeded Claude credentials from K8s secret"
elif [ -f "${CREDS_PVC}" ]; then
    echo "[entrypoint] Using existing PVC credentials (preserved from refresh)"
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
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook && (gt mail check --inject || true) && (gt nudge deacon session-started || true)"
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
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && (gt inject drain --quiet || true) && (gt nudge drain --quiet || true)"
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
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && gt prime --hook && (gt nudge deacon session-started || true)"
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
            "command": "export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\" && (gt inject drain --quiet || true) && (gt nudge drain --quiet || true)"
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

# Write CLAUDE.md with role-specific context if not already present.
# This is the static identity anchor — gt prime (via SessionStart hook) adds
# dynamic context (hooked work, advice, mail) on top of this.
if [ ! -f "${WORKSPACE}/CLAUDE.md" ]; then
    case "${ROLE}" in
        polecat)
            cat > "${WORKSPACE}/CLAUDE.md" <<CLAUDEMD
# Polecat Context

> **Recovery**: Run \`gt prime\` after compaction, clear, or new session

## Your Role: POLECAT (Worker: ${AGENT} in ${RIG:-unknown})

You are polecat **${AGENT}** — a worker agent in the ${RIG:-unknown} rig.
You work on assigned issues and submit completed work to the merge queue.

## Polecat Lifecycle (EPHEMERAL)

\`\`\`
SPAWN → WORK → gt done → DEATH
\`\`\`

**Key insight**: You are born with work. You do ONE task. Then you die.
There is no "next assignment." When \`gt done\` runs, you cease to exist.

## Key Commands

### Session & Context
- \`gt prime\` — Load full context after compaction/clear/new session
- \`gt hook\` — Check your hooked molecule (primary work source)

### Your Work
- \`bd show <issue>\` — View specific issue details
- \`bd ready\` — See your workflow steps

### Progress
- \`bd update <id> --status=in_progress\` — Claim work
- \`bd close <step-id>\` — Mark molecule STEP complete (NOT your main issue!)

### Completion
- \`gt done\` — Signal work ready for merge queue

## Work Protocol

Your work follows the **mol-polecat-work** molecule.

**FIRST: Check your steps with \`bd ready\`.** Do NOT use Claude's internal task tools.

\`\`\`bash
bd ready                   # See your workflow steps — DO THIS FIRST
# ... work on current step ...
bd close <step-id>         # Mark step complete
bd ready                   # See next step
\`\`\`

When all steps are done, run \`gt done\`.

## Communication

\`\`\`bash
# To your Witness
gt mail send ${RIG:-unknown}/witness -s "Question" -m "..."

# To the Mayor (cross-rig issues)
gt mail send mayor/ -s "Need coordination" -m "..."
\`\`\`

---
Polecat: ${AGENT} | Rig: ${RIG:-unknown} | Working directory: ${WORKSPACE}
CLAUDEMD
            ;;
        mayor)
            cat > "${WORKSPACE}/CLAUDE.md" <<CLAUDEMD
# Mayor Context

> **Recovery**: Run \`gt prime\` after compaction, clear, or new session

Full context is injected by \`gt prime\` at session start.

## Quick Reference

- Check mail: \`gt mail inbox\`
- Check rigs: \`gt rig list\`
- Start patrol: \`gt patrol start\`
CLAUDEMD
            ;;
        *)
            cat > "${WORKSPACE}/CLAUDE.md" <<CLAUDEMD
# Gas Town Agent: ${ROLE}

> **Recovery**: Run \`gt prime\` after compaction, clear, or new session

You are the **${ROLE}** agent in a Gas Town rig${RIG:+ (rig: ${RIG})}.
Agent name: ${AGENT}

Full context is injected by \`gt prime\` at session start.

## Quick Reference

- \`gt prime\` — Load full context
- \`gt hook\` — Check hooked work
- \`gt mail inbox\` — Check messages
CLAUDEMD
            ;;
    esac
fi

# ── Skip Claude onboarding wizard ─────────────────────────────────────────

printf '{"hasCompletedOnboarding":true,"lastOnboardingVersion":"2.1.37","preferredTheme":"dark","bypassPermissionsModeAccepted":true}\n' > "${HOME}/.claude.json"

# ── Start coop + Claude ──────────────────────────────────────────────────
#
# We keep bash as PID 1 (no exec) so the pod survives if Claude/coop exit
# (e.g. user sends Ctrl+C which delivers SIGINT to Claude via the PTY).
# On child exit we clean up FIFO pipes and restart with --resume.
# SIGTERM from K8s is forwarded to coop for graceful shutdown.

cd "${WORKSPACE}"

COOP_CMD="coop --agent=claude --port 8080 --port-health 9090 --cols 200 --rows 50"

# Coop log level (overridable via pod env).
export COOP_LOG_LEVEL="${COOP_LOG_LEVEL:-info}"

# ── Auto-bypass startup prompts ────────────────────────────────────────
# Coop v0.4.0 no longer auto-dismisses setup prompts (bypass, trust, etc).
# This background function polls the coop /api/v1/agent endpoint and uses
# the high-level /api/v1/agent/respond API to accept setup prompts.
#
# IMPORTANT: Coop's screen parser can false-positive on "bypass permissions"
# text in the status bar after the agent is past setup. We only auto-respond
# during the first ~20s of startup, and verify the screen actually shows the
# bypass dialog (contains "No, exit" which is unique to the real prompt).
auto_bypass_startup() {
    false_positive_count=0
    for i in $(seq 1 30); do
        sleep 2
        state=$(curl -sf http://localhost:8080/api/v1/agent 2>/dev/null) || continue
        prompt_type=$(echo "${state}" | jq -r '.prompt.type // empty' 2>/dev/null)
        subtype=$(echo "${state}" | jq -r '.prompt.subtype // empty' 2>/dev/null)
        if [ "${prompt_type}" = "setup" ]; then
            # Verify this is a real setup prompt by checking the screen for
            # the actual dialog text, not just the status bar mention.
            screen=$(curl -sf http://localhost:8080/api/v1/screen 2>/dev/null)
            if echo "${screen}" | grep -q "No, exit"; then
                echo "[entrypoint] Auto-accepting setup prompt (subtype: ${subtype})"
                # Option 2 = "Yes, I accept" for bypass; option 1 = "No, exit"
                curl -sf -X POST http://localhost:8080/api/v1/agent/respond \
                    -H 'Content-Type: application/json' \
                    -d '{"option":2}' 2>&1 || true
                false_positive_count=0
                # Give agent time to process the response before next check
                sleep 5
                continue
            else
                false_positive_count=$((false_positive_count + 1))
                # If we see setup state without dialog 5+ times, it's a false positive
                # from the status bar text on a resumed session
                if [ "${false_positive_count}" -ge 5 ]; then
                    echo "[entrypoint] Skipping false-positive setup prompt (no dialog after ${false_positive_count} checks)"
                    return 0
                fi
                continue
            fi
        fi
        # If agent is past setup prompts, we're done
        agent_state=$(echo "${state}" | jq -r '.state // empty' 2>/dev/null)
        if [ "${agent_state}" = "idle" ] || [ "${agent_state}" = "working" ]; then
            return 0
        fi
    done
    echo "[entrypoint] WARNING: auto-bypass timed out after 60s"
}

# ── Inject initial work prompt ────────────────────────────────────────
# After auto-bypass completes and Claude is idle, send the initial work
# prompt via coop's nudge API. This is the coop equivalent of the tmux
# NudgeSession() call in session_manager.go:310-340.
#
# The nudge tells Claude to check its hook and begin working. Without this,
# K8s-spawned polecats boot to an empty welcome screen and sit idle.
#
# Uses POST /api/v1/agent/nudge (reliable delivery — coop queues the message
# and injects it when Claude is ready for input, unlike raw /api/v1/input).
inject_initial_prompt() {
    # Wait for agent to be past setup and idle
    for i in $(seq 1 60); do
        sleep 2
        state=$(curl -sf http://localhost:8080/api/v1/agent 2>/dev/null) || continue
        agent_state=$(echo "${state}" | jq -r '.state // empty' 2>/dev/null)
        if [ "${agent_state}" = "idle" ]; then
            break
        fi
        # If agent is already working (hook triggered it), no nudge needed
        if [ "${agent_state}" = "working" ]; then
            echo "[entrypoint] Agent already working, skipping initial prompt"
            return 0
        fi
    done

    # Build nudge message based on role
    local nudge_msg=""
    case "${ROLE}" in
        polecat)
            nudge_msg="Work is on your hook. Run \`gt hook\` now and begin immediately. If no hook is set, run \`bd ready\` to find available work."
            ;;
        mayor)
            nudge_msg="Run \`gt prime\` to load context, then check \`gt mail inbox\` and \`gt rig list\` to begin your patrol."
            ;;
        witness|refinery|deacon)
            nudge_msg="Run \`gt prime\` to load context, then check \`gt mail inbox\` for pending work."
            ;;
        *)
            nudge_msg="Run \`gt prime\` to load your context and begin working."
            ;;
    esac

    echo "[entrypoint] Injecting initial work prompt (role: ${ROLE})"
    response=$(curl -sf -X POST http://localhost:8080/api/v1/agent/nudge \
        -H 'Content-Type: application/json' \
        -d "{\"message\": \"${nudge_msg}\"}" 2>&1) || {
        echo "[entrypoint] WARNING: nudge failed: ${response}"
        return 0
    }

    delivered=$(echo "${response}" | jq -r '.delivered // false' 2>/dev/null)
    if [ "${delivered}" = "true" ]; then
        echo "[entrypoint] Initial prompt delivered successfully"
    else
        reason=$(echo "${response}" | jq -r '.reason // "unknown"' 2>/dev/null)
        echo "[entrypoint] WARNING: nudge not delivered: ${reason}"
    fi
}

# ── OAuth credential refresh ────────────────────────────────────────────
# Claude OAuth access tokens expire after ~8 hours. This background loop
# uses the refresh_token to obtain a fresh access_token before expiry.
# Runs every 5 minutes, refreshes when within 1 hour of expiry.
#
# Token endpoint: https://platform.claude.com/v1/oauth/token
# Client ID: from Claude Code source (public OAuth client).
OAUTH_TOKEN_URL="https://platform.claude.com/v1/oauth/token"
OAUTH_CLIENT_ID="9d1c250a-e61b-44d9-88ed-5944d1962f5e"
CREDS_FILE="${CLAUDE_STATE}/.credentials.json"

refresh_credentials() {
    sleep 30  # Let Claude start first
    while true; do
        sleep 300  # Check every 5 minutes

        # Read current credentials
        if [ ! -f "${CREDS_FILE}" ]; then
            continue
        fi

        expires_at=$(jq -r '.claudeAiOauth.expiresAt // 0' "${CREDS_FILE}" 2>/dev/null)
        refresh_token=$(jq -r '.claudeAiOauth.refreshToken // empty' "${CREDS_FILE}" 2>/dev/null)

        if [ -z "${refresh_token}" ] || [ "${expires_at}" = "0" ]; then
            continue
        fi

        # Check if within 1 hour of expiry (3600000ms)
        now_ms=$(date +%s)000
        remaining_ms=$((expires_at - now_ms))
        if [ "${remaining_ms}" -gt 3600000 ]; then
            continue  # More than 1 hour left, skip
        fi

        echo "[entrypoint] OAuth token expires in $((remaining_ms / 60000))m, refreshing..."

        response=$(curl -sf "${OAUTH_TOKEN_URL}" \
            -H 'Content-Type: application/json' \
            -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"${refresh_token}\",\"client_id\":\"${OAUTH_CLIENT_ID}\"}" 2>/dev/null) || {
            echo "[entrypoint] WARNING: OAuth refresh request failed"
            continue
        }

        # Validate response has required fields
        new_access_token=$(echo "${response}" | jq -r '.access_token // empty' 2>/dev/null)
        new_refresh_token=$(echo "${response}" | jq -r '.refresh_token // empty' 2>/dev/null)
        expires_in=$(echo "${response}" | jq -r '.expires_in // 0' 2>/dev/null)

        if [ -z "${new_access_token}" ] || [ -z "${new_refresh_token}" ]; then
            echo "[entrypoint] WARNING: OAuth refresh returned invalid response"
            continue
        fi

        # Compute new expiresAt (current time + expires_in seconds, in ms)
        new_expires_at=$(( $(date +%s) * 1000 + expires_in * 1000 ))

        # Read current creds and update tokens (preserves other fields like scopes)
        jq --arg at "${new_access_token}" \
           --arg rt "${new_refresh_token}" \
           --argjson ea "${new_expires_at}" \
           '.claudeAiOauth.accessToken = $at | .claudeAiOauth.refreshToken = $rt | .claudeAiOauth.expiresAt = $ea' \
           "${CREDS_FILE}" > "${CREDS_FILE}.tmp" && mv "${CREDS_FILE}.tmp" "${CREDS_FILE}"

        echo "[entrypoint] OAuth credentials refreshed (expires in $((expires_in / 3600))h)"
    done
}

# ── Monitor agent exit and shut down coop ──────────────────────────────
# Coop v0.4.0 stays alive in "awaiting shutdown" after the agent exits.
# This monitor polls the agent state and sends a shutdown request when
# the agent is in "exited" state, so the entrypoint's `wait` can return.
monitor_agent_exit() {
    # Wait for agent to start first
    sleep 10
    while true; do
        sleep 5
        state=$(curl -sf http://localhost:8080/api/v1/agent 2>/dev/null) || break
        agent_state=$(echo "${state}" | jq -r '.state // empty' 2>/dev/null)
        if [ "${agent_state}" = "exited" ]; then
            echo "[entrypoint] Agent exited, requesting coop shutdown"
            curl -sf -X POST http://localhost:8080/api/v1/shutdown 2>/dev/null || true
            return 0
        fi
    done
}

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

# Start credential refresh in background (survives coop restarts).
refresh_credentials &

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
        (auto_bypass_startup && inject_initial_prompt) &
        monitor_agent_exit &
        wait "${COOP_PID}" 2>/dev/null && exit_code=0 || exit_code=$?
        COOP_PID=""

        # If resume failed, retire the stale session log so the next iteration
        # starts fresh.  The log is renamed (not deleted) so the agent can still
        # review it at <path>.stale if needed.
        if [ "${exit_code}" -ne 0 ] && [ -n "${LATEST_LOG}" ] && [ -f "${LATEST_LOG}" ]; then
            echo "[entrypoint] Resume failed (exit ${exit_code}), retiring stale session log"
            mv "${LATEST_LOG}" "${LATEST_LOG}.stale"
            echo "[entrypoint]   renamed: ${LATEST_LOG} -> ${LATEST_LOG}.stale"
        fi
    else
        echo "[entrypoint] Starting coop + claude (${ROLE}/${AGENT})"
        ${COOP_CMD} -- claude --dangerously-skip-permissions &
        COOP_PID=$!
        (auto_bypass_startup && inject_initial_prompt) &
        monitor_agent_exit &
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
