#!/bin/bash
# Template for crew member .envrc files.
# Copy to a crew workspace directory and customize RIG/CREW/AGENT values.
# Requires direnv: https://direnv.net/
#
# Usage:
#   cp scripts/crew-envrc-template.sh /path/to/rig/crew/name/.envrc
#   cd /path/to/rig/crew/name && direnv allow
#
# Or generate automatically:
#   gt crew env <rig> <name> > /path/to/rig/crew/name/.envrc

# --- Customize these ---
RIG="${GT_CREW_RIG:-lora_forge}"
CREW="${GT_CREW_NAME:-forge}"
# Set to "claudecode-nvim" for neovim-embedded sessions,
# "claudecode" for standard CLI sessions.
AGENT="${GT_CREW_AGENT:-claudecode-nvim}"
# --- End customization ---

# Gas Town identity
export GT_ROLE="${RIG}/crew/${CREW}"
export GT_RIG="${RIG}"
export GT_CREW="${CREW}"
export GT_AGENT="${AGENT}"

# Beads attribution
export BD_ACTOR="${RIG}/crew/${CREW}"
export BEADS_AGENT_NAME="${RIG}/${CREW}"
export GIT_AUTHOR_NAME="${CREW}"

# Gas Town root (auto-detect by walking up to find .gt or settings/)
_gt_root="$(cd "${PWD}" && while [ "$PWD" != "/" ]; do
  if [ -d ".gt" ] || [ -d "settings" -a -f "settings/config.json" ]; then
    echo "$PWD"; break
  fi
  cd ..
done)"
if [ -n "$_gt_root" ]; then
  export GT_ROOT="$_gt_root"
fi

# OTel telemetry propagation (if GT telemetry is active)
if [ -n "${GT_OTEL_METRICS_URL:-}" ]; then
  export CLAUDE_CODE_ENABLE_TELEMETRY=1
  export OTEL_METRICS_EXPORTER=otlp
  export OTEL_METRIC_EXPORT_INTERVAL=1000
  export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT="${GT_OTEL_METRICS_URL}"
  export OTEL_EXPORTER_OTLP_METRICS_PROTOCOL="http/protobuf"

  # Build OTEL_RESOURCE_ATTRIBUTES from GT vars
  _otel_attrs="gt.role=${GT_ROLE},gt.rig=${GT_RIG},gt.agent=${GT_CREW},gt.harness=${GT_AGENT}"
  export OTEL_RESOURCE_ATTRIBUTES="${_otel_attrs}"
fi
