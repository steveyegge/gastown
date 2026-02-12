#!/usr/bin/env bash
# run-suite.sh — Orchestrate all E2E health test modules.
#
# Runs each health test module sequentially, collects results, and
# produces a summary report. Exit code 0 only if all modules pass.
#
# Usage:
#   ./scripts/run-suite.sh [NAMESPACE]
#   E2E_NAMESPACE=gastown-next ./scripts/run-suite.sh
#
# Options:
#   --namespace NAME    Target namespace (default: gastown-next)
#   --skip MODULE       Skip a module (can repeat, e.g. --skip git-mirror)
#   --only MODULE       Run only this module
#   --with-mux          Also run Playwright mux.spec.js tests
#   --json              Output results as JSON (for CI)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
E2E_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── Defaults ─────────────────────────────────────────────────────────
E2E_NAMESPACE="${E2E_NAMESPACE:-gastown-next}"
SKIP_MODULES=""
ONLY_MODULE=""
WITH_MUX=false
JSON_OUTPUT=false

# ── Colors ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# ── Parse args ───────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace)  E2E_NAMESPACE="$2"; shift 2 ;;
    --skip)       SKIP_MODULES="$SKIP_MODULES $2"; shift 2 ;;
    --only)       ONLY_MODULE="$2"; shift 2 ;;
    --with-mux)   WITH_MUX=true; shift ;;
    --json)       JSON_OUTPUT=true; shift ;;
    *)
      # Positional argument = namespace
      if [[ -z "${E2E_NAMESPACE_SET:-}" ]]; then
        E2E_NAMESPACE="$1"
        E2E_NAMESPACE_SET=1
      fi
      shift
      ;;
  esac
done

export E2E_NAMESPACE

# ── Module list ──────────────────────────────────────────────────────
# Order matters: foundational services first, then dependent services
MODULES=(
  # Phase 1: Infrastructure health
  "dolt-health"
  "redis-health"
  "daemon-health"
  "nats-health"
  "coop-broker-health"
  "controller-health"
  "git-mirror-health"
  # Phase 2: Agent capabilities
  "agent-spawn"
  "agent-state"
  "agent-io"
  "agent-credentials"
  "agent-claude-auth"
  "agent-roundtrip"
  "agent-interaction"
  "credential-lifecycle"
  "agent-resume"
  "agent-multi"
  "agent-coordination"
  "agent-cleanup"
  # Phase 2.5: Decision flow
  "decision-flow"
  # Phase 3: Advanced health & lifecycle
  "agent-lifecycle"
  "controller-create-pod"
  "session-persistence"
  "dolt-s3-sync"
  "controller-failsafe"
  "slack-bot-health"
)

should_skip() {
  local module="$1"
  if [[ -n "$ONLY_MODULE" && "$module" != "$ONLY_MODULE" ]]; then
    return 0
  fi
  case "$SKIP_MODULES" in
    *" $module"*|*"$module "*) return 0 ;;
  esac
  return 1
}

# ── Header ───────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  Gastown E2E Health Suite${NC}"
echo -e "${BOLD}  Namespace: ${BLUE}$E2E_NAMESPACE${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Verify cluster connectivity
if ! kubectl cluster-info >/dev/null 2>&1; then
  echo -e "${RED}Cannot connect to K8s cluster${NC}"
  exit 1
fi

# Verify namespace exists
if ! kubectl get ns "$E2E_NAMESPACE" >/dev/null 2>&1; then
  echo -e "${RED}Namespace $E2E_NAMESPACE does not exist${NC}"
  exit 1
fi

# ── Run modules ──────────────────────────────────────────────────────
# Use parallel arrays instead of associative arrays (bash 3.x compat)
RESULT_MODULES=()
RESULT_STATUS=()
TOTAL_PASS=0
TOTAL_FAIL=0
TOTAL_SKIP=0

for module in "${MODULES[@]}"; do
  script="$SCRIPT_DIR/test-${module}.sh"

  if should_skip "$module"; then
    echo -e "${YELLOW}[SKIP]${NC} $module"
    RESULT_MODULES+=("$module")
    RESULT_STATUS+=("skip")
    TOTAL_SKIP=$((TOTAL_SKIP + 1))
    continue
  fi

  if [[ ! -x "$script" ]]; then
    echo -e "${RED}[ERR]${NC} $module — script not found or not executable: $script"
    RESULT_MODULES+=("$module")
    RESULT_STATUS+=("error")
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    continue
  fi

  echo -e "${BLUE}━━━ Running: $module ━━━${NC}"
  if "$script" "$E2E_NAMESPACE"; then
    RESULT_MODULES+=("$module")
    RESULT_STATUS+=("pass")
    TOTAL_PASS=$((TOTAL_PASS + 1))
  else
    RESULT_MODULES+=("$module")
    RESULT_STATUS+=("fail")
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
  fi
  echo ""
done

# ── Playwright mux tests (optional) ─────────────────────────────────
if [[ "$WITH_MUX" == "true" ]]; then
  echo -e "${BLUE}━━━ Running: mux (Playwright) ━━━${NC}"

  # Set up port-forward for mux tests
  MUX_SVC=$(kubectl get svc -n "$E2E_NAMESPACE" --no-headers 2>/dev/null | grep "coop-broker" | head -1 | awk '{print $1}')
  if [[ -n "$MUX_SVC" ]]; then
    kubectl port-forward -n "$E2E_NAMESPACE" "svc/$MUX_SVC" 18080:8080 >/dev/null 2>&1 &
    MUX_PF_PID=$!
    sleep 3

    if (cd "$E2E_DIR" && npx playwright test mux.spec.js 2>&1); then
      RESULT_MODULES+=("mux")
      RESULT_STATUS+=("pass")
      TOTAL_PASS=$((TOTAL_PASS + 1))
    else
      RESULT_MODULES+=("mux")
      RESULT_STATUS+=("fail")
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
    fi

    kill "$MUX_PF_PID" 2>/dev/null || true
  else
    echo -e "${YELLOW}[SKIP]${NC} mux — no coop-broker service found"
    RESULT_MODULES+=("mux")
    RESULT_STATUS+=("skip")
    TOTAL_SKIP=$((TOTAL_SKIP + 1))
  fi
  echo ""
fi

# ── Summary ──────────────────────────────────────────────────────────
TOTAL=$((TOTAL_PASS + TOTAL_FAIL + TOTAL_SKIP))

echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  E2E Suite Results${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

for i in "${!RESULT_MODULES[@]}"; do
  module="${RESULT_MODULES[$i]}"
  status="${RESULT_STATUS[$i]}"
  case "$status" in
    pass)  echo -e "  ${GREEN}✓${NC} $module" ;;
    fail)  echo -e "  ${RED}✗${NC} $module" ;;
    skip)  echo -e "  ${YELLOW}○${NC} $module (skipped)" ;;
    error) echo -e "  ${RED}!${NC} $module (error)" ;;
  esac
done

echo ""
echo -e "  Total:   $TOTAL"
echo -e "  ${GREEN}Passed:  $TOTAL_PASS${NC}"
if [[ $TOTAL_FAIL -gt 0 ]]; then
  echo -e "  ${RED}Failed:  $TOTAL_FAIL${NC}"
fi
if [[ $TOTAL_SKIP -gt 0 ]]; then
  echo -e "  ${YELLOW}Skipped: $TOTAL_SKIP${NC}"
fi
echo ""

# ── JSON output ──────────────────────────────────────────────────────
if [[ "$JSON_OUTPUT" == "true" ]]; then
  echo "{"
  echo "  \"namespace\": \"$E2E_NAMESPACE\","
  echo "  \"total\": $TOTAL,"
  echo "  \"passed\": $TOTAL_PASS,"
  echo "  \"failed\": $TOTAL_FAIL,"
  echo "  \"skipped\": $TOTAL_SKIP,"
  echo "  \"modules\": {"
  first=true
  for i in "${!RESULT_MODULES[@]}"; do
    if [[ "$first" == "true" ]]; then
      first=false
    else
      echo ","
    fi
    echo -n "    \"${RESULT_MODULES[$i]}\": \"${RESULT_STATUS[$i]}\""
  done
  echo ""
  echo "  }"
  echo "}"
fi

# ── Exit code ────────────────────────────────────────────────────────
if [[ $TOTAL_FAIL -gt 0 ]]; then
  exit 1
fi
exit 0
