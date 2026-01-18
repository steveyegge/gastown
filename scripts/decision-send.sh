#!/bin/bash
#
# decision-send.sh - Send decision requests to the decision receiver
#
# Usage:
#   decision-send.sh yesno "Approve changes?" [--default=yes] [--timeout=30]
#   decision-send.sh choice "Select target:" staging production dev [--default=staging]
#   decision-send.sh multiselect "Select files:" file1 file2 file3 [--timeout=60]

set -euo pipefail

FIFO_IN="${GT_DECISION_FIFO_IN:-/tmp/gt-decisions.fifo}"
FIFO_OUT="${GT_DECISION_FIFO_OUT:-/tmp/gt-decisions-out.fifo}"

usage() {
    cat <<EOF
Usage: $(basename "$0") TYPE CONTEXT [OPTIONS...] [-- OPTION1 OPTION2 ...]

Types:
    yesno       Yes/No decision
    choice      Single selection from options
    multiselect Multiple selection from options

Options:
    --default=VALUE   Default selection (for timeout or empty input)
    --timeout=SECS    Timeout in seconds
    --id=ID           Custom request ID (default: auto-generated)

Examples:
    $(basename "$0") yesno "Continue with changes?"
    $(basename "$0") yesno "Deploy to production?" --default=no --timeout=30
    $(basename "$0") choice "Select environment:" -- staging production development
    $(basename "$0") multiselect "Select modules to build:" --timeout=60 -- api web cli
EOF
}

# Generate unique ID
generate_id() {
    echo "dec-$(date +%s)-$$"
}

main() {
    if [[ $# -lt 2 ]]; then
        usage
        exit 1
    fi

    local type="$1"
    local context="$2"
    shift 2

    local default=""
    local timeout="0"
    local id=""
    local options=()
    local parsing_options=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --)
                parsing_options=true
                shift
                ;;
            --default=*)
                default="${1#*=}"
                shift
                ;;
            --timeout=*)
                timeout="${1#*=}"
                shift
                ;;
            --id=*)
                id="${1#*=}"
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                if [[ "$parsing_options" == true ]]; then
                    options+=("$1")
                else
                    # For choice/multiselect, remaining args before -- are options
                    options+=("$1")
                fi
                shift
                ;;
        esac
    done

    # Generate ID if not provided
    id="${id:-$(generate_id)}"

    # Build JSON request
    local json
    if command -v jq &>/dev/null; then
        local options_json="[]"
        if [[ ${#options[@]} -gt 0 ]]; then
            options_json=$(printf '%s\n' "${options[@]}" | jq -R . | jq -s .)
        fi

        json=$(jq -n \
            --arg id "$id" \
            --arg type "$type" \
            --arg context "$context" \
            --arg default "$default" \
            --argjson timeout "$timeout" \
            --argjson options "$options_json" \
            '{id: $id, type: $type, context: $context, default: $default, timeout: $timeout, options: $options}')
    else
        # Fallback: manual JSON construction
        local opts_str=""
        if [[ ${#options[@]} -gt 0 ]]; then
            opts_str=$(printf '"%s",' "${options[@]}")
            opts_str="[${opts_str%,}]"
        else
            opts_str="[]"
        fi
        json="{\"id\":\"$id\",\"type\":\"$type\",\"context\":\"$context\",\"default\":\"$default\",\"timeout\":$timeout,\"options\":$opts_str}"
    fi

    # Check if FIFOs exist
    if [[ ! -p "$FIFO_IN" ]]; then
        echo "Error: Decision receiver not running (FIFO not found: $FIFO_IN)" >&2
        echo "Start the receiver with: decision-receiver.sh listen" >&2
        exit 1
    fi

    # Send request
    echo "$json" > "$FIFO_IN"

    # Read response
    local response
    response=$(cat "$FIFO_OUT")
    echo "$response"
}

main "$@"
