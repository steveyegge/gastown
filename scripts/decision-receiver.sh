#!/bin/bash
#
# decision-receiver.sh - Receive and present decision points from agents
#
# IPC: Uses named pipes for bidirectional communication
#   - /tmp/gt-decisions.fifo     - Agent writes decision requests here
#   - /tmp/gt-decisions-out.fifo - Script writes responses here
#
# Decision Format (JSON):
#   {
#     "id": "unique-id",
#     "type": "yesno|choice|multiselect",
#     "context": "Description of the decision",
#     "options": ["opt1", "opt2", ...],  // for choice/multiselect
#     "default": "opt1",                  // optional default
#     "timeout": 30                       // optional timeout in seconds
#   }
#
# Response Format (JSON):
#   {
#     "id": "unique-id",
#     "decision": "yes|no|<selected option>|[selected options]",
#     "timestamp": "ISO8601",
#     "timedout": false
#   }

set -euo pipefail

# Configuration
FIFO_IN="${GT_DECISION_FIFO_IN:-/tmp/gt-decisions.fifo}"
FIFO_OUT="${GT_DECISION_FIFO_OUT:-/tmp/gt-decisions-out.fifo}"
POLL_INTERVAL=0.1
DECISION_MODE="${GT_DECISION_MODE:-terminal}"  # terminal or notify

# Script directory for finding decision-notify.sh
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # No Color

# Cleanup handler
cleanup() {
    # Remove FIFOs on exit
    rm -f "$FIFO_IN" "$FIFO_OUT" 2>/dev/null || true
    # Restore terminal settings if needed
    stty sane 2>/dev/null || true
}

trap cleanup EXIT INT TERM

# Setup FIFOs
setup_fifos() {
    # Create FIFOs if they don't exist
    if [[ ! -p "$FIFO_IN" ]]; then
        mkfifo "$FIFO_IN"
    fi
    if [[ ! -p "$FIFO_OUT" ]]; then
        mkfifo "$FIFO_OUT"
    fi
    echo -e "${DIM}FIFOs ready: $FIFO_IN -> $FIFO_OUT${NC}"
}

# Parse JSON field (basic jq-like extraction)
json_get() {
    local json="$1"
    local field="$2"
    # Use jq if available, otherwise basic grep/sed
    if command -v jq &>/dev/null; then
        echo "$json" | jq -r ".$field // empty" 2>/dev/null
    else
        # Fallback: basic extraction for simple cases
        echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | sed 's/.*: *"\([^"]*\)".*/\1/'
    fi
}

# Parse JSON array field
json_get_array() {
    local json="$1"
    local field="$2"
    if command -v jq &>/dev/null; then
        echo "$json" | jq -r ".$field[]? // empty" 2>/dev/null
    else
        # Fallback: basic array extraction
        echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\[[^]]*\]" | \
            sed 's/.*\[\(.*\)\].*/\1/' | tr ',' '\n' | sed 's/[" ]//g'
    fi
}

# Present yes/no decision (terminal mode)
present_yesno_terminal() {
    local context="$1"
    local default="${2:-}"
    local timeout="${3:-0}"

    echo "" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo -e "${BOLD}  DECISION REQUIRED${NC}" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo "" >&2
    echo -e "  $context" >&2
    echo "" >&2

    local prompt
    if [[ "$default" == "yes" ]]; then
        prompt="${GREEN}[Y]${NC}/n"
    elif [[ "$default" == "no" ]]; then
        prompt="y/${RED}[N]${NC}"
    else
        prompt="y/n"
    fi

    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2

    local answer
    if [[ "$timeout" -gt 0 ]]; then
        echo -ne "  ${prompt} ${DIM}(${timeout}s timeout)${NC}: " >&2
        read -t "$timeout" -n 1 answer || answer="timeout"
    else
        echo -ne "  ${prompt}: " >&2
        read -n 1 answer
    fi
    echo "" >&2

    case "${answer,,}" in
        y) echo "yes" ;;
        n) echo "no" ;;
        timeout) echo "timeout" ;;
        "") echo "${default:-no}" ;;
        *) echo "no" ;;
    esac
}

# Present yes/no decision (notification mode - macOS)
present_yesno_notify() {
    local context="$1"
    local default="${2:-no}"
    local timeout="${3:-0}"

    local notify_script="$SCRIPT_DIR/decision-notify.sh"
    if [[ ! -x "$notify_script" ]]; then
        echo -e "${YELLOW}Warning: decision-notify.sh not found, falling back to terminal${NC}" >&2
        present_yesno_terminal "$context" "$default" "$timeout"
        return
    fi

    local args=(
        "$context"
        "--default=$default"
    )
    if [[ "$timeout" -gt 0 ]]; then
        args+=("--timeout=$timeout")
    fi

    local result
    result=$("$notify_script" "${args[@]}" 2>/dev/null) || {
        echo -e "${YELLOW}Notification failed, falling back to terminal${NC}" >&2
        present_yesno_terminal "$context" "$default" "$timeout"
        return
    }

    # Extract decision from JSON response
    if command -v jq &>/dev/null; then
        echo "$result" | jq -r '.decision // "no"'
    else
        echo "$result" | grep -oP '"decision"\s*:\s*"\K[^"]+' || echo "no"
    fi
}

# Present yes/no decision (auto-selects based on mode)
present_yesno() {
    if [[ "$DECISION_MODE" == "notify" ]]; then
        present_yesno_notify "$@"
    else
        present_yesno_terminal "$@"
    fi
}

# Present choice decision (single select)
present_choice() {
    local context="$1"
    local default="$2"
    local timeout="$3"
    shift 3
    local options=("$@")

    echo "" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo -e "${BOLD}  DECISION REQUIRED${NC}" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo "" >&2
    echo -e "  $context" >&2
    echo "" >&2

    local i=1
    local default_idx=0
    for opt in "${options[@]}"; do
        if [[ "$opt" == "$default" ]]; then
            echo -e "  ${GREEN}[$i]${NC} $opt ${DIM}(default)${NC}" >&2
            default_idx=$i
        else
            echo -e "  ${BLUE}[$i]${NC} $opt" >&2
        fi
        ((i++))
    done

    echo "" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2

    local choice
    if [[ "$timeout" -gt 0 ]]; then
        echo -ne "  Enter choice [1-${#options[@]}] ${DIM}(${timeout}s timeout)${NC}: " >&2
        read -t "$timeout" choice || choice="timeout"
    else
        echo -ne "  Enter choice [1-${#options[@]}]: " >&2
        read choice
    fi
    echo "" >&2

    if [[ "$choice" == "timeout" ]]; then
        echo "timeout"
    elif [[ -z "$choice" && $default_idx -gt 0 ]]; then
        echo "${options[$((default_idx-1))]}"
    elif [[ "$choice" =~ ^[0-9]+$ ]] && [[ "$choice" -ge 1 ]] && [[ "$choice" -le "${#options[@]}" ]]; then
        echo "${options[$((choice-1))]}"
    else
        echo "invalid"
    fi
}

# Present multiselect decision
present_multiselect() {
    local context="$1"
    local timeout="$2"
    shift 2
    local options=("$@")

    echo "" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo -e "${BOLD}  DECISION REQUIRED ${DIM}(multi-select)${NC}" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2
    echo "" >&2
    echo -e "  $context" >&2
    echo "" >&2

    local i=1
    for opt in "${options[@]}"; do
        echo -e "  ${BLUE}[$i]${NC} $opt" >&2
        ((i++))
    done

    echo "" >&2
    echo -e "${CYAN}-----------------------------------------------------${NC}" >&2

    local choices
    if [[ "$timeout" -gt 0 ]]; then
        echo -ne "  Enter choices (comma-separated, e.g., 1,3) ${DIM}(${timeout}s timeout)${NC}: " >&2
        read -t "$timeout" choices || choices="timeout"
    else
        echo -ne "  Enter choices (comma-separated, e.g., 1,3): " >&2
        read choices
    fi
    echo "" >&2

    if [[ "$choices" == "timeout" ]]; then
        echo "timeout"
        return
    fi

    # Parse comma-separated choices and return JSON array
    local selected=()
    IFS=',' read -ra choice_arr <<< "$choices"
    for c in "${choice_arr[@]}"; do
        c=$(echo "$c" | tr -d ' ')
        if [[ "$c" =~ ^[0-9]+$ ]] && [[ "$c" -ge 1 ]] && [[ "$c" -le "${#options[@]}" ]]; then
            selected+=("\"${options[$((c-1))]}\"")
        fi
    done

    echo "[$(IFS=,; echo "${selected[*]}")]"
}

# Build JSON response
build_response() {
    local id="$1"
    local decision="$2"
    local timedout="${3:-false}"

    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    if command -v jq &>/dev/null; then
        jq -n \
            --arg id "$id" \
            --arg decision "$decision" \
            --arg ts "$timestamp" \
            --argjson timedout "$timedout" \
            '{id: $id, decision: $decision, timestamp: $ts, timedout: $timedout}'
    else
        echo "{\"id\":\"$id\",\"decision\":\"$decision\",\"timestamp\":\"$timestamp\",\"timedout\":$timedout}"
    fi
}

# Process a single decision request
process_decision() {
    local request="$1"

    local id type context default timeout
    id=$(json_get "$request" "id")
    type=$(json_get "$request" "type")
    context=$(json_get "$request" "context")
    default=$(json_get "$request" "default")
    timeout=$(json_get "$request" "timeout")
    timeout="${timeout:-0}"

    local decision timedout=false

    case "$type" in
        yesno)
            decision=$(present_yesno "$context" "$default" "$timeout")
            ;;
        choice)
            mapfile -t options < <(json_get_array "$request" "options")
            decision=$(present_choice "$context" "$default" "$timeout" "${options[@]}")
            ;;
        multiselect)
            mapfile -t options < <(json_get_array "$request" "options")
            decision=$(present_multiselect "$context" "$timeout" "${options[@]}")
            ;;
        *)
            echo -e "${RED}Unknown decision type: $type${NC}" >&2
            decision="error"
            ;;
    esac

    if [[ "$decision" == "timeout" ]]; then
        timedout=true
        decision="${default:-}"
    fi

    build_response "$id" "$decision" "$timedout"
}

# Main loop - listen for decisions
main_loop() {
    echo -e "${GREEN}Decision Receiver started${NC}"
    echo -e "${DIM}Listening on: $FIFO_IN${NC}"
    echo -e "${DIM}Responding to: $FIFO_OUT${NC}"
    echo ""

    while true; do
        # Open FIFO for reading (blocks until writer connects)
        if read -r request < "$FIFO_IN"; then
            if [[ -n "$request" ]]; then
                # Process the decision
                response=$(process_decision "$request")

                # Write response to output FIFO
                echo "$response" > "$FIFO_OUT"

                echo -e "${DIM}Response sent${NC}"
            fi
        fi
    done
}

# One-shot mode - process single decision from stdin
oneshot_mode() {
    local request
    request=$(cat)
    response=$(process_decision "$request")
    echo "$response"
}

# Test mode - run sample decisions
test_mode() {
    echo -e "${YELLOW}Running test decisions...${NC}"
    echo ""

    # Test yes/no
    local test1='{"id":"test-1","type":"yesno","context":"Approve changes to auth.ts?","default":"yes"}'
    echo -e "${DIM}Test 1: Yes/No with default${NC}"
    process_decision "$test1"
    echo ""

    # Test choice
    local test2='{"id":"test-2","type":"choice","context":"Select deployment target:","options":["staging","production","development"],"default":"staging"}'
    echo -e "${DIM}Test 2: Single choice${NC}"
    process_decision "$test2"
    echo ""

    # Test multiselect
    local test3='{"id":"test-3","type":"multiselect","context":"Select files to include:","options":["README.md","CHANGELOG.md","LICENSE","package.json"]}'
    echo -e "${DIM}Test 3: Multi-select${NC}"
    process_decision "$test3"
}

# Usage
usage() {
    cat <<EOF
Usage: $(basename "$0") [COMMAND] [OPTIONS]

Commands:
    listen      Start listening for decisions on FIFO (default)
    oneshot     Process single decision from stdin
    test        Run interactive test with sample decisions
    help        Show this help

Options:
    --mode=MODE     Decision presentation mode: terminal (default) or notify
                    In notify mode, uses macOS notifications for yes/no decisions

Environment:
    GT_DECISION_FIFO_IN   Input FIFO path (default: /tmp/gt-decisions.fifo)
    GT_DECISION_FIFO_OUT  Output FIFO path (default: /tmp/gt-decisions-out.fifo)
    GT_DECISION_MODE      Default presentation mode: terminal or notify

Notification Mode (macOS):
    When mode=notify, yes/no decisions are shown as macOS notifications
    with actionable Yes/No buttons. Requires one of:
    - terminal-notifier (brew install terminal-notifier) [recommended]
    - alerter (brew install alerter)
    - osascript (built-in, uses modal dialogs)

    Falls back to terminal mode if notifications unavailable.

Example (agent side):
    # Send decision request
    echo '{"id":"1","type":"yesno","context":"Continue?"}' > /tmp/gt-decisions.fifo
    # Read response
    read response < /tmp/gt-decisions-out.fifo

Example (oneshot):
    echo '{"id":"1","type":"yesno","context":"Continue?"}' | $0 oneshot

Example (notification mode):
    GT_DECISION_MODE=notify $0 listen
    $0 listen --mode=notify
EOF
}

# Parse global options
parse_global_opts() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --mode=*)
                DECISION_MODE="${1#*=}"
                shift
                ;;
            *)
                echo "$1"
                shift
                ;;
        esac
    done
}

# Main entry point
# First, extract any --mode= options
args=()
for arg in "$@"; do
    case "$arg" in
        --mode=*)
            DECISION_MODE="${arg#*=}"
            ;;
        *)
            args+=("$arg")
            ;;
    esac
done

case "${args[0]:-listen}" in
    listen)
        setup_fifos
        echo -e "${DIM}Mode: $DECISION_MODE${NC}"
        main_loop
        ;;
    oneshot)
        oneshot_mode
        ;;
    test)
        test_mode
        ;;
    help|--help|-h)
        usage
        ;;
    *)
        echo "Unknown command: ${args[0]}" >&2
        usage
        exit 1
        ;;
esac
