#!/usr/bin/env bash
# Run E2E tests for ALL available agents from HQ settings.
# Skips agents whose binaries are not installed.
#
# Usage:
#   ./scripts/test-all-agents-e2e.sh [rig]
#
# Generates a summary report at the end.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOWN_ROOT="${GT_ROOT:-/home/ubuntu/gt}"
CONFIG_FILE="$TOWN_ROOT/settings/config.json"
RIG="${1:-sfgastown}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  E2E Agent Prompt Test Suite — All Agents${NC}"
echo -e "${BLUE}  Rig: $RIG${NC}"
echo -e "${BLUE}  Config: $CONFIG_FILE${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

# Get all agents and check which are testable
declare -A RESULTS
TESTED=0
PASSED=0
SKIPPED=0
FAILED=0

# Read agents from config
AGENTS=$(python3 -c "
import json, shutil
with open('$CONFIG_FILE') as f:
    cfg = json.load(f)
for name, agent in sorted(cfg.get('agents', {}).items()):
    cmd = agent.get('command', '?')
    found = 'yes' if shutil.which(cmd) else 'no'
    print(f'{name}|{cmd}|{found}')
")

echo "Agent inventory:"
while IFS='|' read -r name cmd available; do
    if [[ "$available" == "yes" ]]; then
        echo -e "  ${GREEN}●${NC} $name (command: $cmd)"
    else
        echo -e "  ${YELLOW}○${NC} $name (command: $cmd) — SKIPPED (binary not found)"
    fi
done <<< "$AGENTS"
echo ""

# Run tests for each available agent
while IFS='|' read -r name cmd available; do
    if [[ "$available" != "yes" ]]; then
        SKIPPED=$((SKIPPED + 1))
        RESULTS["$name"]="SKIPPED"
        continue
    fi

    # Skip stream-mode agents (different invocation pattern, not standard polecat flow)
    if [[ "$name" == *"-stream"* ]]; then
        echo -e "${YELLOW}[SKIP]${NC} $name — stream-mode agent (not standard polecat flow)"
        SKIPPED=$((SKIPPED + 1))
        RESULTS["$name"]="SKIPPED (stream)"
        continue
    fi

    echo -e "${BLUE}─────────────────────────────────────────────────────────────${NC}"
    echo -e "${BLUE}Testing: $name${NC}"
    echo -e "${BLUE}─────────────────────────────────────────────────────────────${NC}"

    TESTED=$((TESTED + 1))

    if "$SCRIPT_DIR/test-agent-e2e.sh" "$name" "$RIG" 2>&1; then
        PASSED=$((PASSED + 1))
        RESULTS["$name"]="PASS"
    else
        FAILED=$((FAILED + 1))
        RESULTS["$name"]="FAIL"
    fi

    echo ""
    # Brief pause between tests to avoid resource contention
    sleep 3
done <<< "$AGENTS"

# Summary report
echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  SUMMARY REPORT${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""

while IFS='|' read -r name cmd available; do
    result="${RESULTS[$name]:-UNKNOWN}"
    case "$result" in
        PASS)    echo -e "  ${GREEN}✓ PASS${NC}    $name" ;;
        FAIL)    echo -e "  ${RED}✗ FAIL${NC}    $name" ;;
        SKIP*)   echo -e "  ${YELLOW}○ SKIP${NC}    $name ($result)" ;;
        *)       echo -e "  ? ???     $name" ;;
    esac
done <<< "$AGENTS"

echo ""
echo -e "  Tested: $TESTED  Passed: ${GREEN}$PASSED${NC}  Failed: ${RED}$FAILED${NC}  Skipped: ${YELLOW}$SKIPPED${NC}"
echo ""

if [[ $FAILED -eq 0 && $TESTED -gt 0 ]]; then
    echo -e "${GREEN}ALL TESTED AGENTS PASSED ($PASSED/$TESTED)${NC}"
    exit 0
else
    echo -e "${RED}SOME TESTS FAILED ($FAILED/$TESTED)${NC}"
    exit 1
fi
