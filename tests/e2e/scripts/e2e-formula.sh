#!/usr/bin/env bash
# e2e-formula.sh — Full E2E lifecycle: provision → validate → teardown.
#
# Orchestrates the complete E2E test run:
#   Phase 1: Provision — create namespace and install gastown helm chart
#   Phase 2: Validate  — run the full E2E health test suite
#   Phase 3: Teardown  — delete namespace (unless --keep)
#
# Exit code 0 only if all phases succeed.
#
# Usage:
#   ./scripts/e2e-formula.sh [OPTIONS] [--set KEY=VAL ...]
#
# Options:
#   --namespace NAME    Namespace name (default: gastown-e2e-<timestamp>)
#   --values FILE       Helm values file (default: auto-detect values-e2e.yaml)
#   --chart-dir DIR     Helm chart directory (default: auto-detect)
#   --keep              Don't delete namespace on exit (for debugging)
#   --skip MODULE       Skip a test module (repeatable)
#   --only MODULE       Run only one test module
#   --with-mux          Include Playwright mux.spec.js tests
#   --timeout SECS      Provision timeout (default: 600)
#   --json              Output JSON results
#   --epic ID           Link auto-filed bugs to this beads epic
#   --set KEY=VAL       Passthrough to helm --set (repeatable)
#
# Required --set flags for ExternalSecrets:
#   --set bd-daemon.externalSecrets.doltRootPassword.remoteRef=shared-e2e-dolt-root-password
#   --set bd-daemon.externalSecrets.daemonToken.remoteRef=shared-e2e-bd-daemon-token
#   --set bd-daemon.externalSecrets.doltS3Credentials.remoteRef=shared-e2e-dolt-s3-credentials
#
# Examples:
#   # Full ephemeral E2E run (provision + validate + teardown):
#   ./scripts/e2e-formula.sh \
#     --set bd-daemon.externalSecrets.doltRootPassword.remoteRef=shared-e2e-dolt-root-password \
#     --set bd-daemon.externalSecrets.daemonToken.remoteRef=shared-e2e-bd-daemon-token \
#     --set bd-daemon.externalSecrets.doltS3Credentials.remoteRef=shared-e2e-dolt-s3-credentials
#
#   # Run against existing namespace (skip provision/teardown):
#   ./scripts/e2e-formula.sh --namespace gastown-next --keep
#
#   # Run only one module:
#   ./scripts/e2e-formula.sh --namespace gastown-next --keep --only daemon-health

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Defaults ─────────────────────────────────────────────────────────
NAMESPACE=""
VALUES_FILE=""
CHART_DIR=""
KEEP=false
TIMEOUT=600
JSON_OUTPUT=false
EPIC_ID=""
PROVISION_ARGS=()    # args for provision-namespace.sh
SUITE_ARGS=()        # args for run-suite.sh
HELM_SET_ARGS=()     # --set passthrough for helm

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${BLUE}[formula]${NC} $1"; }
ok()   { echo -e "${GREEN}[formula]${NC} $1"; }
warn() { echo -e "${YELLOW}[formula]${NC} $1"; }
err()  { echo -e "${RED}[formula]${NC} $1" >&2; }

# ── Parse args ───────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace)  NAMESPACE="$2"; shift 2 ;;
    --values)     VALUES_FILE="$2"; shift 2 ;;
    --chart-dir)  CHART_DIR="$2"; shift 2 ;;
    --keep)       KEEP=true; shift ;;
    --timeout)    TIMEOUT="$2"; shift 2 ;;
    --json)       JSON_OUTPUT=true; SUITE_ARGS+=("--json"); shift ;;
    --epic)       EPIC_ID="$2"; shift 2 ;;
    --skip)       SUITE_ARGS+=("--skip" "$2"); shift 2 ;;
    --only)       SUITE_ARGS+=("--only" "$2"); shift 2 ;;
    --with-mux)   SUITE_ARGS+=("--with-mux"); shift ;;
    --set)        HELM_SET_ARGS+=("--set" "$2"); shift 2 ;;
    -h|--help)
      echo "Usage: $0 [--namespace NAME] [--values FILE] [--keep] [--set KEY=VAL ...] [--skip MODULE] [--only MODULE]"
      exit 0
      ;;
    *) err "Unknown arg: $1"; exit 1 ;;
  esac
done

# ── Resolve defaults ────────────────────────────────────────────────
if [[ -z "$NAMESPACE" ]]; then
  NAMESPACE="gastown-e2e-$(date +%s)"
fi

FORMULA_NAME="e2e-${NAMESPACE}"
START_TIME=$SECONDS

export E2E_NAMESPACE="$NAMESPACE"
export E2E_EPIC_ID="$EPIC_ID"

# ── Banner ──────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  Gastown E2E Formula${NC}"
echo -e "${BOLD}  Namespace: ${BLUE}$NAMESPACE${NC}"
echo -e "${BOLD}  Keep:      ${KEEP}${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── Results tracking ─────────────────────────────────────────────────
PROVISION_OK=false
VALIDATE_OK=false
TEARDOWN_OK=false
VALIDATE_EXIT=0

# ── Phase 1: Provision ──────────────────────────────────────────────
echo -e "${BOLD}━━━ Phase 1: Provision ━━━${NC}"

# Check if namespace already exists (skip-install mode)
NS_EXISTS=false
if kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
  NS_EXISTS=true
  log "Namespace $NAMESPACE already exists — skipping install"
fi

if [[ "$NS_EXISTS" == "true" ]]; then
  PROVISION_ARGS+=("--skip-install")
  PROVISION_OK=true
  log "Using existing namespace $NAMESPACE"
else
  # Build provision args
  PROVISION_ARGS+=("--namespace" "$NAMESPACE" "--timeout" "$TIMEOUT")
  [[ -n "$VALUES_FILE" ]] && PROVISION_ARGS+=("--values" "$VALUES_FILE")
  [[ -n "$CHART_DIR" ]] && PROVISION_ARGS+=("--chart-dir" "$CHART_DIR")
  for arg in "${HELM_SET_ARGS[@]}"; do
    PROVISION_ARGS+=("$arg")
  done

  log "Provisioning namespace $NAMESPACE..."
  if "$SCRIPT_DIR/provision-namespace.sh" "${PROVISION_ARGS[@]}"; then
    PROVISION_OK=true
    ok "Provision completed"
  else
    err "Provision failed"
    # If provision fails and we created the namespace, clean up
    if [[ "$KEEP" != "true" ]]; then
      warn "Cleaning up failed provision..."
      "$SCRIPT_DIR/teardown-namespace.sh" --namespace "$NAMESPACE" --force 2>/dev/null || true
    fi
    exit 1
  fi
fi

PROVISION_ELAPSED=$((SECONDS - START_TIME))
echo ""

# ── Phase 2: Validate ──────────────────────────────────────────────
echo -e "${BOLD}━━━ Phase 2: Validate ━━━${NC}"
VALIDATE_START=$SECONDS

log "Running E2E suite against $NAMESPACE..."
SUITE_ARGS+=("--namespace" "$NAMESPACE")

if "$SCRIPT_DIR/run-suite.sh" "${SUITE_ARGS[@]}"; then
  VALIDATE_OK=true
  ok "Validation passed"
else
  VALIDATE_EXIT=$?
  err "Validation failed (exit code $VALIDATE_EXIT)"
fi

VALIDATE_ELAPSED=$((SECONDS - VALIDATE_START))
echo ""

# ── Phase 3: Teardown ──────────────────────────────────────────────
echo -e "${BOLD}━━━ Phase 3: Teardown ━━━${NC}"
TEARDOWN_START=$SECONDS

if [[ "$KEEP" == "true" ]]; then
  warn "Keeping namespace $NAMESPACE (--keep specified)"
  TEARDOWN_OK=true
elif [[ "$NS_EXISTS" == "true" ]]; then
  warn "Keeping pre-existing namespace $NAMESPACE"
  TEARDOWN_OK=true
else
  log "Tearing down namespace $NAMESPACE..."
  if "$SCRIPT_DIR/teardown-namespace.sh" --namespace "$NAMESPACE" --force; then
    TEARDOWN_OK=true
    ok "Teardown completed"
  else
    err "Teardown failed — namespace may be stuck"
    TEARDOWN_OK=false
  fi
fi

TEARDOWN_ELAPSED=$((SECONDS - TEARDOWN_START))
TOTAL_ELAPSED=$((SECONDS - START_TIME))
echo ""

# ── Summary ─────────────────────────────────────────────────────────
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  E2E Formula Results${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

phase_icon() { [[ "$1" == "true" ]] && echo -e "${GREEN}✓${NC}" || echo -e "${RED}✗${NC}"; }

echo -e "  $(phase_icon "$PROVISION_OK") Provision  (${PROVISION_ELAPSED}s)"
echo -e "  $(phase_icon "$VALIDATE_OK") Validate   (${VALIDATE_ELAPSED}s)"
echo -e "  $(phase_icon "$TEARDOWN_OK") Teardown   (${TEARDOWN_ELAPSED}s)"
echo ""
echo -e "  Total time: ${TOTAL_ELAPSED}s"
echo ""

# ── JSON output ─────────────────────────────────────────────────────
if [[ "$JSON_OUTPUT" == "true" ]]; then
  cat <<EOJSON
{
  "formula": "$FORMULA_NAME",
  "namespace": "$NAMESPACE",
  "phases": {
    "provision": {"ok": $PROVISION_OK, "elapsed_secs": $PROVISION_ELAPSED},
    "validate": {"ok": $VALIDATE_OK, "elapsed_secs": $VALIDATE_ELAPSED, "exit_code": $VALIDATE_EXIT},
    "teardown": {"ok": $TEARDOWN_OK, "elapsed_secs": $TEARDOWN_ELAPSED}
  },
  "total_elapsed_secs": $TOTAL_ELAPSED
}
EOJSON
fi

# ── Exit code ───────────────────────────────────────────────────────
if [[ "$PROVISION_OK" == "true" && "$VALIDATE_OK" == "true" && "$TEARDOWN_OK" == "true" ]]; then
  ok "All phases passed"
  exit 0
else
  err "Formula failed"
  exit 1
fi
