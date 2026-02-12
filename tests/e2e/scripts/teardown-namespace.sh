#!/usr/bin/env bash
# teardown-namespace.sh — Delete a K8s namespace and all its resources.
#
# Usage:
#   ./scripts/teardown-namespace.sh [--namespace NAME] [--timeout SECS] [--force]
#
# Options:
#   --namespace NAME    Namespace to delete (required, or E2E_NAMESPACE env)
#   --timeout SECS      Max wait for namespace deletion (default: 120)
#   --force             Skip confirmation prompt

set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────────
NAMESPACE="${E2E_NAMESPACE:-}"
TIMEOUT=120
FORCE=false

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()  { echo -e "${BLUE}[teardown]${NC} $1"; }
ok()   { echo -e "${GREEN}[teardown]${NC} $1"; }
warn() { echo -e "${YELLOW}[teardown]${NC} $1"; }
err()  { echo -e "${RED}[teardown]${NC} $1" >&2; }
die()  { err "$1"; exit 1; }

# ── Parse args ───────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace)  NAMESPACE="$2"; shift 2 ;;
    --timeout)    TIMEOUT="$2"; shift 2 ;;
    --force)      FORCE=true; shift ;;
    -h|--help)
      echo "Usage: $0 --namespace NAME [--timeout SECS] [--force]"
      exit 0
      ;;
    *) die "Unknown arg: $1" ;;
  esac
done

[[ -n "$NAMESPACE" ]] || die "Namespace required (--namespace or E2E_NAMESPACE env)"

# ── Safety check ─────────────────────────────────────────────────────
# Never delete production-like namespaces
case "$NAMESPACE" in
  gastown-uat|gastown-ha|default|kube-system|kube-public)
    die "Refusing to delete protected namespace: $NAMESPACE"
    ;;
esac

# Verify namespace exists
if ! kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
  warn "Namespace $NAMESPACE does not exist — nothing to tear down"
  exit 0
fi

# ── Confirm ──────────────────────────────────────────────────────────
if [[ "$FORCE" != "true" ]]; then
  pod_count=$(kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
  pvc_count=$(kubectl get pvc -n "$NAMESPACE" --no-headers 2>/dev/null | wc -l | tr -d ' ')
  warn "Will delete namespace $NAMESPACE ($pod_count pods, $pvc_count PVCs)"
  echo -n "Continue? [y/N] "
  read -r confirm
  [[ "$confirm" =~ ^[Yy] ]] || die "Aborted"
fi

# ── Uninstall Helm releases ─────────────────────────────────────────
RELEASES=$(helm list -n "$NAMESPACE" -q 2>/dev/null || true)
if [[ -n "$RELEASES" ]]; then
  for release in $RELEASES; do
    log "Uninstalling helm release: $release"
    helm uninstall "$release" -n "$NAMESPACE" --wait --timeout 60s 2>/dev/null || \
      warn "helm uninstall $release failed (continuing)"
  done
fi

# ── Delete namespace ─────────────────────────────────────────────────
log "Deleting namespace $NAMESPACE..."
kubectl delete namespace "$NAMESPACE" --wait=false 2>/dev/null || true

# Wait for namespace to be fully removed
deadline=$((SECONDS + TIMEOUT))
while [[ $SECONDS -lt $deadline ]]; do
  if ! kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
    ok "Namespace $NAMESPACE deleted"
    exit 0
  fi
  phase=$(kubectl get ns "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "gone")
  if [[ "$phase" == "gone" ]]; then
    ok "Namespace $NAMESPACE deleted"
    exit 0
  fi
  log "  Waiting for namespace deletion (phase: $phase)..."
  sleep 5
done

err "Timeout waiting for namespace $NAMESPACE deletion after ${TIMEOUT}s"
warn "Namespace may be stuck in Terminating — check for finalizers"
exit 1
