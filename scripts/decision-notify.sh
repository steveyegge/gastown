#!/bin/bash
#
# decision-notify.sh - macOS notification-based decision prompts
#
# Shows macOS notifications with actionable buttons for yes/no decisions.
# Supports multiple notification backends with automatic fallback.
#
# Backends (in order of preference):
#   1. terminal-notifier - Native notifications with actions (brew install terminal-notifier)
#   2. alerter - Similar to terminal-notifier (brew install alerter)
#   3. osascript - Built-in AppleScript dialogs (always available on macOS)
#
# Usage:
#   decision-notify.sh "Approve changes to auth.ts?" [--timeout=30] [--default=yes]
#   decision-notify.sh --title="Claude Code" "Deploy to production?"
#
# Response format (JSON):
#   {"decision": "yes|no|timeout", "backend": "terminal-notifier|alerter|osascript"}

set -euo pipefail

# Configuration
DEFAULT_TITLE="Claude Code"
DEFAULT_TIMEOUT=0
DEFAULT_SOUND="default"

# Detect available notification backend
detect_backend() {
    if [[ "$(uname)" != "Darwin" ]]; then
        echo "none"
        return
    fi

    if command -v terminal-notifier &>/dev/null; then
        echo "terminal-notifier"
    elif command -v alerter &>/dev/null; then
        echo "alerter"
    else
        echo "osascript"
    fi
}

# Send notification via terminal-notifier
# Returns: clicked action or empty string
notify_terminal_notifier() {
    local title="$1"
    local message="$2"
    local timeout="$3"
    local sound="$4"

    local args=(
        -title "$title"
        -message "$message"
        -actions "Yes,No"
        -closeLabel "Dismiss"
        -sender "com.apple.Terminal"
    )

    if [[ -n "$sound" && "$sound" != "none" ]]; then
        args+=(-sound "$sound")
    fi

    if [[ "$timeout" -gt 0 ]]; then
        args+=(-timeout "$timeout")
    fi

    # terminal-notifier returns the clicked action on stdout
    local result
    result=$(terminal-notifier "${args[@]}" 2>/dev/null) || true

    case "$result" in
        "Yes") echo "yes" ;;
        "No") echo "no" ;;
        "@TIMEOUT") echo "timeout" ;;
        "@CLOSED"|"@CONTENTCLICKED"|"") echo "dismissed" ;;
        *) echo "dismissed" ;;
    esac
}

# Send notification via alerter
# Returns: clicked action or empty string
notify_alerter() {
    local title="$1"
    local message="$2"
    local timeout="$3"
    local sound="$4"

    local args=(
        -title "$title"
        -message "$message"
        -actions "Yes,No"
        -closeLabel "Dismiss"
    )

    if [[ -n "$sound" && "$sound" != "none" ]]; then
        args+=(-sound "$sound")
    fi

    if [[ "$timeout" -gt 0 ]]; then
        args+=(-timeout "$timeout")
    fi

    # alerter returns the clicked action on stdout
    local result
    result=$(alerter "${args[@]}" 2>/dev/null) || true

    case "$result" in
        "Yes") echo "yes" ;;
        "No") echo "no" ;;
        "@TIMEOUT") echo "timeout" ;;
        "@CLOSED"|"@CONTENTCLICKED"|"") echo "dismissed" ;;
        *) echo "dismissed" ;;
    esac
}

# Show dialog via osascript (AppleScript)
# This is the fallback - shows a modal dialog, not a notification
notify_osascript() {
    local title="$1"
    local message="$2"
    local timeout="$3"
    local default_button="${4:-}"

    local giving_up=""
    if [[ "$timeout" -gt 0 ]]; then
        giving_up="giving up after $timeout"
    fi

    local default_btn="\"Yes\""
    if [[ "$default_button" == "no" ]]; then
        default_btn="\"No\""
    fi

    # AppleScript to show dialog with Yes/No buttons
    local script="display dialog \"$message\" with title \"$title\" buttons {\"No\", \"Yes\"} default button $default_btn $giving_up"

    local result
    result=$(osascript -e "$script" 2>/dev/null) || {
        # User clicked a button (might return error on cancel)
        echo "dismissed"
        return
    }

    # Parse result: "button returned:Yes" or "button returned:No, gave up:true"
    if [[ "$result" == *"gave up:true"* ]]; then
        echo "timeout"
    elif [[ "$result" == *"button returned:Yes"* ]]; then
        echo "yes"
    elif [[ "$result" == *"button returned:No"* ]]; then
        echo "no"
    else
        echo "dismissed"
    fi
}

# Build JSON response
build_response() {
    local decision="$1"
    local backend="$2"
    local timedout="${3:-false}"

    if command -v jq &>/dev/null; then
        jq -n \
            --arg decision "$decision" \
            --arg backend "$backend" \
            --argjson timedout "$timedout" \
            '{decision: $decision, backend: $backend, timedout: $timedout}'
    else
        echo "{\"decision\":\"$decision\",\"backend\":\"$backend\",\"timedout\":$timedout}"
    fi
}

# Main notification function
send_notification() {
    local title="$1"
    local message="$2"
    local timeout="$3"
    local default="$4"
    local sound="$5"
    local backend

    backend=$(detect_backend)

    if [[ "$backend" == "none" ]]; then
        echo "Error: Not running on macOS. Notifications not available." >&2
        build_response "error" "none" "false"
        return 1
    fi

    local result timedout="false"

    case "$backend" in
        terminal-notifier)
            result=$(notify_terminal_notifier "$title" "$message" "$timeout" "$sound")
            ;;
        alerter)
            result=$(notify_alerter "$title" "$message" "$timeout" "$sound")
            ;;
        osascript)
            result=$(notify_osascript "$title" "$message" "$timeout" "$default")
            ;;
    esac

    # Handle timeout and dismissed states
    case "$result" in
        timeout)
            timedout="true"
            result="${default:-no}"
            ;;
        dismissed)
            # User dismissed without choosing - use default or "no"
            result="${default:-no}"
            ;;
    esac

    build_response "$result" "$backend" "$timedout"
}

# Usage
usage() {
    cat <<EOF
Usage: $(basename "$0") MESSAGE [OPTIONS]

Send a macOS notification with Yes/No action buttons.

Options:
    --title=TITLE       Notification title (default: "$DEFAULT_TITLE")
    --timeout=SECS      Timeout in seconds (default: no timeout)
    --default=yes|no    Default response for timeout/dismiss (default: no)
    --sound=SOUND       Notification sound (default: "$DEFAULT_SOUND", use "none" to disable)
    --backend=BACKEND   Force specific backend (terminal-notifier, alerter, osascript)
    --json              Parse MESSAGE as JSON decision request
    --detect            Print detected backend and exit
    --help, -h          Show this help

Backends (auto-detected in order):
    terminal-notifier   Native macOS notifications with actions (recommended)
    alerter             Similar to terminal-notifier
    osascript           Built-in AppleScript dialogs (fallback)

Examples:
    $(basename "$0") "Approve changes to auth.ts?"
    $(basename "$0") "Deploy to production?" --timeout=30 --default=no
    $(basename "$0") --title="Build System" "Run npm install?"

JSON mode:
    echo '{"context":"Approve?","timeout":30}' | $(basename "$0") --json

Response format (JSON):
    {"decision": "yes", "backend": "terminal-notifier", "timedout": false}
EOF
}

# Parse JSON input (for integration with decision-receiver)
parse_json_input() {
    local json="$1"

    if command -v jq &>/dev/null; then
        local context timeout default_val
        context=$(echo "$json" | jq -r '.context // .message // ""')
        timeout=$(echo "$json" | jq -r '.timeout // 0')
        default_val=$(echo "$json" | jq -r '.default // "no"')

        echo "$context"
        echo "$timeout"
        echo "$default_val"
    else
        # Basic parsing without jq
        local context timeout default_val
        context=$(echo "$json" | grep -oP '"context"\s*:\s*"\K[^"]+' || echo "")
        if [[ -z "$context" ]]; then
            context=$(echo "$json" | grep -oP '"message"\s*:\s*"\K[^"]+' || echo "")
        fi
        timeout=$(echo "$json" | grep -oP '"timeout"\s*:\s*\K\d+' || echo "0")
        default_val=$(echo "$json" | grep -oP '"default"\s*:\s*"\K[^"]+' || echo "no")

        echo "$context"
        echo "$timeout"
        echo "$default_val"
    fi
}

main() {
    local title="$DEFAULT_TITLE"
    local timeout="$DEFAULT_TIMEOUT"
    local default="no"
    local sound="$DEFAULT_SOUND"
    local json_mode=false
    local message=""
    local force_backend=""

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --title=*)
                title="${1#*=}"
                shift
                ;;
            --timeout=*)
                timeout="${1#*=}"
                shift
                ;;
            --default=*)
                default="${1#*=}"
                shift
                ;;
            --sound=*)
                sound="${1#*=}"
                shift
                ;;
            --backend=*)
                force_backend="${1#*=}"
                shift
                ;;
            --json)
                json_mode=true
                shift
                ;;
            --detect)
                detect_backend
                exit 0
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            -*)
                echo "Unknown option: $1" >&2
                usage
                exit 1
                ;;
            *)
                message="$1"
                shift
                ;;
        esac
    done

    # JSON mode: read from stdin or use message as JSON
    if [[ "$json_mode" == true ]]; then
        local json_input
        if [[ -n "$message" ]]; then
            json_input="$message"
        else
            json_input=$(cat)
        fi

        local parsed
        parsed=$(parse_json_input "$json_input")

        message=$(echo "$parsed" | sed -n '1p')
        timeout=$(echo "$parsed" | sed -n '2p')
        default=$(echo "$parsed" | sed -n '3p')
    fi

    if [[ -z "$message" ]]; then
        echo "Error: No message provided" >&2
        usage
        exit 1
    fi

    # Override backend detection if forced
    if [[ -n "$force_backend" ]]; then
        export GT_NOTIFY_BACKEND="$force_backend"
    fi

    send_notification "$title" "$message" "$timeout" "$default" "$sound"
}

main "$@"
