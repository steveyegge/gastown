#!/usr/bin/env bash
# resource-monitor/run.sh — Periodic system health check.
#
# Checks disk, memory, and polecat session count.
# Logs a quiet summary bead; escalates if thresholds are breached.
#
# Thresholds:
#   Disk usage    > 80%   → HIGH escalation
#   Memory avail  < 10Gi  → HIGH escalation
#   Polecat count > 22    → MEDIUM escalation (you hit issues at 29)
#
# Usage: ./run.sh [--rig <rig>]

set -euo pipefail

RIG="${RESOURCE_MONITOR_RIG:-monorepo}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --rig) RIG="$2"; shift 2 ;;
    --help|-h) echo "Usage: $0 [--rig <rig>]"; exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

log() { echo "[resource-monitor] $*"; }

# --- Disk ---
DISK_PCT=$(df -h / | awk 'NR==2 {gsub(/%/,"",$5); print $5}')
DISK_AVAIL=$(df -h / | awk 'NR==2 {print $4}')
DISK_USED=$(df -h / | awk 'NR==2 {print $3}')
DISK_TOTAL=$(df -h / | awk 'NR==2 {print $2}')

# --- Memory ---
MEM_AVAIL_GI=$(free -g | awk '/^Mem:/ {print $7}')
MEM_USED_GI=$(free -g | awk '/^Mem:/ {print $3}')
MEM_TOTAL_GI=$(free -g | awk '/^Mem:/ {print $2}')

# --- Sessions (polecat proxy) ---
SESSION_COUNT=$(tmux ls 2>/dev/null | wc -l || echo 0)

POLECAT_COUNT=0
if command -v gt &>/dev/null; then
  POLECAT_COUNT=$(gt polecat list "$RIG" --json 2>/dev/null | python3 -c "
import json,sys
try:
    data=json.load(sys.stdin)
    working=[p for p in data if p['state']=='working']
    print(len(working))
except: print(0)" 2>/dev/null || echo 0)
fi

log "Disk: ${DISK_USED}/${DISK_TOTAL} (${DISK_PCT}% used, ${DISK_AVAIL} avail)"
log "Memory: ${MEM_USED_GI}Gi/${MEM_TOTAL_GI}Gi used (${MEM_AVAIL_GI}Gi avail)"
log "Sessions: ${SESSION_COUNT} tmux, ${POLECAT_COUNT} working polecats (${RIG})"

SUMMARY="resource-monitor: disk=${DISK_PCT}% mem_avail=${MEM_AVAIL_GI}Gi sessions=${SESSION_COUNT} polecats=${POLECAT_COUNT}"
RESULT="success"
ESCALATIONS=""

# --- Threshold checks ---
if [[ "$DISK_PCT" -ge 80 ]]; then
  RESULT="warning"
  ESCALATIONS="${ESCALATIONS}DISK_HIGH(${DISK_PCT}%) "
  gt escalate "resource-monitor: disk at ${DISK_PCT}% (${DISK_USED}/${DISK_TOTAL})" \
    -s HIGH \
    --reason "Disk usage at ${DISK_PCT}%. ${DISK_AVAIL} available. Polecats generate worktrees and logs — may fill up quickly." \
    2>/dev/null || true
fi

if [[ "$MEM_AVAIL_GI" -lt 10 ]]; then
  RESULT="warning"
  ESCALATIONS="${ESCALATIONS}MEM_LOW(${MEM_AVAIL_GI}Gi) "
  gt escalate "resource-monitor: memory low — only ${MEM_AVAIL_GI}Gi available" \
    -s HIGH \
    --reason "Only ${MEM_AVAIL_GI}Gi RAM available (${MEM_USED_GI}Gi/${MEM_TOTAL_GI}Gi used). Too many concurrent polecats may cause OOM." \
    2>/dev/null || true
fi

if [[ "$POLECAT_COUNT" -gt 22 ]]; then
  RESULT="warning"
  ESCALATIONS="${ESCALATIONS}POLECAT_HIGH(${POLECAT_COUNT}) "
  gt escalate "resource-monitor: ${POLECAT_COUNT} polecats working simultaneously — approaching limit" \
    -s MEDIUM \
    --reason "${POLECAT_COUNT} working polecats in ${RIG} rig. Previous run with 29 caused resource issues. Consider nuking done polecats or pausing new slings." \
    2>/dev/null || true
fi

# --- Record result ---
bd create --title "resource-monitor: ${SUMMARY}" \
  -t chore --ephemeral \
  -l "type:plugin-run,plugin:resource-monitor,result:${RESULT}" \
  -d "${SUMMARY}${ESCALATIONS:+ ALERTS: $ESCALATIONS}" \
  --silent 2>/dev/null || true

log "Done. Result: ${RESULT}${ESCALATIONS:+ — ALERTS: $ESCALATIONS}"
