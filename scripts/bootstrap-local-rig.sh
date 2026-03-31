#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bootstrap-local-rig.sh --town-root PATH --rig NAME --local-repo PATH [options]

Create a clean Gas Town rig from a local source repo by using `gt rig add` with
`--local-repo` instead of adopting a manually assembled rig directory.

Required:
  --town-root PATH       Gas Town town root
  --rig NAME             New rig name
  --local-repo PATH      Existing local repo to use as the object reference

Optional:
  --remote URL           Git URL to register for the rig
                         (default: file://<local-repo>)
  --gt-bin PATH          gt binary to use (default: ./gt if present, else gt)
  --prefix PREFIX        Beads prefix override
  --branch NAME          Default branch override
  --polecat-agent NAME   Write rig settings role_agents.polecat
  --witness-agent NAME   Write rig settings role_agents.witness
  --refinery-agent NAME  Write rig settings role_agents.refinery

Example:
  ./scripts/bootstrap-local-rig.sh \
    --town-root /gt \
    --rig nightrider_local \
    --local-repo /gt/nightRider \
    --prefix nr \
    --polecat-agent claude \
    --witness-agent codex \
    --refinery-agent codex
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_GT_BIN="gt"
if [[ -x "${SCRIPT_DIR}/../gt" ]]; then
  DEFAULT_GT_BIN="${SCRIPT_DIR}/../gt"
fi

TOWN_ROOT=""
RIG_NAME=""
REMOTE_URL=""
LOCAL_REPO=""
GT_BIN="${DEFAULT_GT_BIN}"
PREFIX=""
BRANCH=""
POLECAT_AGENT=""
WITNESS_AGENT=""
REFINERY_AGENT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --town-root)
      TOWN_ROOT="${2:-}"
      shift 2
      ;;
    --rig)
      RIG_NAME="${2:-}"
      shift 2
      ;;
    --remote)
      REMOTE_URL="${2:-}"
      shift 2
      ;;
    --local-repo)
      LOCAL_REPO="${2:-}"
      shift 2
      ;;
    --gt-bin)
      GT_BIN="${2:-}"
      shift 2
      ;;
    --prefix)
      PREFIX="${2:-}"
      shift 2
      ;;
    --branch)
      BRANCH="${2:-}"
      shift 2
      ;;
    --polecat-agent)
      POLECAT_AGENT="${2:-}"
      shift 2
      ;;
    --witness-agent)
      WITNESS_AGENT="${2:-}"
      shift 2
      ;;
    --refinery-agent)
      REFINERY_AGENT="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

for required in TOWN_ROOT RIG_NAME LOCAL_REPO; do
  if [[ -z "${!required}" ]]; then
    echo "Missing required argument: ${required}" >&2
    usage >&2
    exit 2
  fi
done

if [[ ! -d "${TOWN_ROOT}" ]]; then
  echo "Town root does not exist: ${TOWN_ROOT}" >&2
  exit 1
fi

if [[ ! -d "${LOCAL_REPO}" ]]; then
  echo "Local repo does not exist: ${LOCAL_REPO}" >&2
  exit 1
fi

if ! git -C "${LOCAL_REPO}" rev-parse --git-dir >/dev/null 2>&1; then
  echo "Local repo is not a git repository: ${LOCAL_REPO}" >&2
  exit 1
fi

if [[ -z "${REMOTE_URL}" ]]; then
  REMOTE_URL="file://${LOCAL_REPO}"
fi

RIG_PATH="${TOWN_ROOT}/${RIG_NAME}"
if [[ -e "${RIG_PATH}" ]]; then
  echo "Target rig already exists: ${RIG_PATH}" >&2
  exit 1
fi

GT_ARGS=("rig" "add" "${RIG_NAME}" "${REMOTE_URL}" "--local-repo" "${LOCAL_REPO}")
if [[ -n "${PREFIX}" ]]; then
  GT_ARGS+=("--prefix" "${PREFIX}")
fi
if [[ -n "${BRANCH}" ]]; then
  GT_ARGS+=("--branch" "${BRANCH}")
fi

(
  cd "${TOWN_ROOT}"
  "${GT_BIN}" "${GT_ARGS[@]}"
)

if [[ -n "${POLECAT_AGENT}" || -n "${WITNESS_AGENT}" || -n "${REFINERY_AGENT}" ]]; then
  SETTINGS_PATH="${RIG_PATH}/settings/config.json"
  mkdir -p "$(dirname "${SETTINGS_PATH}")"
  python3 - "${SETTINGS_PATH}" "${POLECAT_AGENT}" "${WITNESS_AGENT}" "${REFINERY_AGENT}" <<'PY'
import json
import os
import sys

settings_path, polecat, witness, refinery = sys.argv[1:]
data = {}
if os.path.exists(settings_path):
    with open(settings_path, "r", encoding="utf-8") as fh:
        data = json.load(fh)

data.setdefault("type", "rig-settings")
data.setdefault("version", 1)
role_agents = data.setdefault("role_agents", {})
if polecat:
    role_agents["polecat"] = polecat
if witness:
    role_agents["witness"] = witness
if refinery:
    role_agents["refinery"] = refinery

with open(settings_path, "w", encoding="utf-8") as fh:
    json.dump(data, fh, indent=2)
    fh.write("\n")
PY
fi

echo "Rig bootstrapped at ${RIG_PATH}"
