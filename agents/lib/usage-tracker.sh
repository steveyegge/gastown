#!/bin/bash
# SPDX-License-Identifier: MIT
# Usage Tracking Library for Gastown Agents
#
# Provides functions to log and report token/cost usage across all agents.
# Data is stored in JSON format for easy parsing and reporting.
#
# Usage in agent scripts:
#   source "$(dirname "$0")/lib/usage-tracker.sh"
#   usage_start "gemini-flash" "task-123"
#   # ... run agent ...
#   usage_end "gemini-flash" "task-123" "$input_tokens" "$output_tokens"
#
# Environment Variables:
#   GT_USAGE_DIR    - Directory for usage logs (default: ~/.gastown/usage)
#   GT_RIG          - Current rig name (set by Gastown)
#   GT_CONVOY       - Current convoy ID (set by Gastown)
#
# Requires: jq, bc

# Configuration
USAGE_DIR="${GT_USAGE_DIR:-$HOME/.gastown/usage}"
USAGE_LOG="$USAGE_DIR/usage.jsonl"
SESSION_LOG="$USAGE_DIR/sessions.jsonl"

# Ensure usage directory exists
mkdir -p "$USAGE_DIR"

# Cost per 1M tokens (approximate, update as needed)
# Using simple variable names to avoid bash associative array issues
_get_input_cost() {
    local model="$1"
    case "$model" in
        claude) echo "15.00" ;;
        claude-sonnet|claudesonnet) echo "3.00" ;;
        claude-haiku|claudehaiku) echo "0.25" ;;
        gemini-2.5-pro|geminipro|gemini-pro) echo "1.25" ;;
        gemini-2.5-flash|geminiflash|gemini-flash) echo "0.075" ;;
        gpt-4o|gpt4o) echo "2.50" ;;
        gpt-4o-mini|gpt4o-mini|gpt4omini) echo "0.15" ;;
        perplexity) echo "0.00" ;;
        *) echo "1.00" ;;  # Default estimate
    esac
}

_get_output_cost() {
    local model="$1"
    case "$model" in
        claude) echo "75.00" ;;
        claude-sonnet|claudesonnet) echo "15.00" ;;
        claude-haiku|claudehaiku) echo "1.25" ;;
        gemini-2.5-pro|geminipro|gemini-pro) echo "5.00" ;;
        gemini-2.5-flash|geminiflash|gemini-flash) echo "0.30" ;;
        gpt-4o|gpt4o) echo "10.00" ;;
        gpt-4o-mini|gpt4o-mini|gpt4omini) echo "0.60" ;;
        perplexity) echo "0.00" ;;
        *) echo "3.00" ;;  # Default estimate
    esac
}

# Get current timestamp in ISO format
_timestamp() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# Get today's date
_today() {
    date +"%Y-%m-%d"
}

# Calculate cost from tokens
# Args: model, input_tokens, output_tokens
_calculate_cost() {
    local model="$1"
    local input_tokens="${2:-0}"
    local output_tokens="${3:-0}"

    local input_cost=$(_get_input_cost "$model")
    local output_cost=$(_get_output_cost "$model")

    # Calculate: (tokens / 1,000,000) * cost_per_1M
    local cost=$(echo "scale=6; ($input_tokens * $input_cost / 1000000) + ($output_tokens * $output_cost / 1000000)" | bc 2>/dev/null || echo "0")
    echo "$cost"
}

# Log the start of an agent session
# Args: agent_name, task_id
usage_start() {
    local agent="$1"
    local task_id="${2:-unknown}"
    local rig="${GT_RIG:-unknown}"
    local convoy="${GT_CONVOY:-}"

    local entry=$(cat <<EOF
{"event":"start","agent":"$agent","task":"$task_id","rig":"$rig","convoy":"$convoy","timestamp":"$(_timestamp)"}
EOF
)
    echo "$entry" >> "$SESSION_LOG"

    # Store start time for duration calculation
    export _GT_USAGE_START=$(date +%s)
    export _GT_USAGE_AGENT="$agent"
    export _GT_USAGE_TASK="$task_id"
}

# Log the end of an agent session with token counts
# Args: agent_name, task_id, input_tokens, output_tokens, [status]
usage_end() {
    local agent="${1:-$_GT_USAGE_AGENT}"
    local task_id="${2:-$_GT_USAGE_TASK}"
    local input_tokens="${3:-0}"
    local output_tokens="${4:-0}"
    local status="${5:-completed}"
    local rig="${GT_RIG:-unknown}"
    local convoy="${GT_CONVOY:-}"

    # Calculate duration
    local end_time=$(date +%s)
    local start_time="${_GT_USAGE_START:-$end_time}"
    local duration=$((end_time - start_time))

    # Calculate cost
    local cost=$(_calculate_cost "$agent" "$input_tokens" "$output_tokens")

    local entry=$(cat <<EOF
{"event":"end","agent":"$agent","task":"$task_id","rig":"$rig","convoy":"$convoy","input_tokens":$input_tokens,"output_tokens":$output_tokens,"total_tokens":$((input_tokens + output_tokens)),"cost_usd":$cost,"duration_sec":$duration,"status":"$status","timestamp":"$(_timestamp)","date":"$(_today)"}
EOF
)
    echo "$entry" >> "$USAGE_LOG"

    # Print summary to stderr (visible in logs but not mixed with agent output)
    echo "[Usage] $agent: ${input_tokens}in/${output_tokens}out tokens, \$$cost, ${duration}s" >&2
}

# Log usage without start/end (for simpler tracking)
# Args: agent_name, task_id, input_tokens, output_tokens, [duration_sec]
usage_log() {
    local agent="$1"
    local task_id="${2:-unknown}"
    local input_tokens="${3:-0}"
    local output_tokens="${4:-0}"
    local duration="${5:-0}"
    local rig="${GT_RIG:-unknown}"
    local convoy="${GT_CONVOY:-}"

    local cost=$(_calculate_cost "$agent" "$input_tokens" "$output_tokens")

    local entry=$(cat <<EOF
{"event":"usage","agent":"$agent","task":"$task_id","rig":"$rig","convoy":"$convoy","input_tokens":$input_tokens,"output_tokens":$output_tokens,"total_tokens":$((input_tokens + output_tokens)),"cost_usd":$cost,"duration_sec":$duration,"status":"completed","timestamp":"$(_timestamp)","date":"$(_today)"}
EOF
)
    echo "$entry" >> "$USAGE_LOG"
}

# Get usage summary for today
# Args: [agent_filter]
usage_today() {
    local agent_filter="$1"
    local today=$(_today)

    if [[ ! -f "$USAGE_LOG" ]]; then
        echo "No usage data found."
        return
    fi

    if [[ -n "$agent_filter" ]]; then
        grep "\"date\":\"$today\"" "$USAGE_LOG" | grep "\"agent\":\"$agent_filter\"" | \
            jq -s '{
                agent: "'$agent_filter'",
                date: "'$today'",
                requests: length,
                input_tokens: (map(.input_tokens) | add),
                output_tokens: (map(.output_tokens) | add),
                total_tokens: (map(.total_tokens) | add),
                total_cost_usd: (map(.cost_usd) | add),
                total_duration_sec: (map(.duration_sec) | add)
            }' 2>/dev/null || echo "Error parsing usage data"
    else
        grep "\"date\":\"$today\"" "$USAGE_LOG" | \
            jq -s 'group_by(.agent) | map({
                agent: .[0].agent,
                requests: length,
                input_tokens: (map(.input_tokens) | add),
                output_tokens: (map(.output_tokens) | add),
                total_cost_usd: (map(.cost_usd) | add)
            })' 2>/dev/null || echo "Error parsing usage data"
    fi
}

# Get usage summary for a convoy
# Args: convoy_id
usage_convoy() {
    local convoy_id="$1"

    if [[ ! -f "$USAGE_LOG" ]]; then
        echo "No usage data found."
        return
    fi

    grep "\"convoy\":\"$convoy_id\"" "$USAGE_LOG" | \
        jq -s '{
            convoy: "'$convoy_id'",
            agents: (group_by(.agent) | map({agent: .[0].agent, requests: length, tokens: (map(.total_tokens) | add), cost: (map(.cost_usd) | add)})),
            summary: {
                total_requests: length,
                total_input_tokens: (map(.input_tokens) | add),
                total_output_tokens: (map(.output_tokens) | add),
                total_tokens: (map(.total_tokens) | add),
                total_cost_usd: (map(.cost_usd) | add),
                total_duration_sec: (map(.duration_sec) | add)
            }
        }' 2>/dev/null || echo "Error parsing usage data"
}

# Get usage summary for a rig
# Args: rig_name, [date]
usage_rig() {
    local rig_name="$1"
    local date_filter="${2:-}"

    if [[ ! -f "$USAGE_LOG" ]]; then
        echo "No usage data found."
        return
    fi

    local filter="\"rig\":\"$rig_name\""
    if [[ -n "$date_filter" ]]; then
        filter="$filter.*\"date\":\"$date_filter\""
    fi

    grep "$filter" "$USAGE_LOG" | \
        jq -s '{
            rig: "'$rig_name'",
            agents: (group_by(.agent) | map({agent: .[0].agent, requests: length, tokens: (map(.total_tokens) | add), cost: (map(.cost_usd) | add)})),
            summary: {
                total_requests: length,
                total_tokens: (map(.total_tokens) | add),
                total_cost_usd: (map(.cost_usd) | add)
            }
        }' 2>/dev/null || echo "Error parsing usage data"
}

# Print a formatted usage report
# Args: [period: today|week|month|all] [rig_filter]
usage_report() {
    local period="${1:-today}"
    local rig_filter="$2"

    if [[ ! -f "$USAGE_LOG" ]]; then
        echo "No usage data found."
        return
    fi

    local date_filter=""
    case "$period" in
        today)
            date_filter=$(_today)
            ;;
        week)
            date_filter=$(date -v-7d +"%Y-%m-%d" 2>/dev/null || date -d "7 days ago" +"%Y-%m-%d")
            ;;
        month)
            date_filter=$(date -v-30d +"%Y-%m-%d" 2>/dev/null || date -d "30 days ago" +"%Y-%m-%d")
            ;;
        all)
            date_filter=""
            ;;
    esac

    echo "=== Gastown Usage Report ($period) ==="
    echo ""

    local data
    if [[ -n "$date_filter" && "$period" == "today" ]]; then
        data=$(grep "\"date\":\"$date_filter\"" "$USAGE_LOG")
    elif [[ -n "$date_filter" ]]; then
        # Filter for dates >= date_filter (simplified, just gets recent)
        data=$(cat "$USAGE_LOG")
    else
        data=$(cat "$USAGE_LOG")
    fi

    if [[ -n "$rig_filter" ]]; then
        data=$(echo "$data" | grep "\"rig\":\"$rig_filter\"")
    fi

    if [[ -z "$data" ]]; then
        echo "No usage data for this period."
        return
    fi

    echo "$data" | jq -s '
        {
            "By Agent": (group_by(.agent) | map({
                Agent: .[0].agent,
                Requests: length,
                "Input Tokens": (map(.input_tokens) | add),
                "Output Tokens": (map(.output_tokens) | add),
                "Cost (USD)": (map(.cost_usd) | add | . * 100 | round / 100)
            }) | sort_by(.["Cost (USD)"]) | reverse),
            "By Rig": (group_by(.rig) | map({
                Rig: .[0].rig,
                Requests: length,
                "Total Tokens": (map(.total_tokens) | add),
                "Cost (USD)": (map(.cost_usd) | add | . * 100 | round / 100)
            })),
            "Totals": {
                "Total Requests": length,
                "Total Input Tokens": (map(.input_tokens) | add),
                "Total Output Tokens": (map(.output_tokens) | add),
                "Total Cost (USD)": (map(.cost_usd) | add | . * 100 | round / 100)
            }
        }
    ' 2>/dev/null || echo "Error generating report"
}

# Generate a completion message with usage stats (for Mayor to include)
# Args: task_id, [convoy_id]
usage_completion_message() {
    local task_id="$1"
    local convoy_id="$2"

    if [[ ! -f "$USAGE_LOG" ]]; then
        echo "Task completed. (No usage tracking data)"
        return
    fi

    local task_data=$(grep "\"task\":\"$task_id\"" "$USAGE_LOG" | tail -1)

    if [[ -z "$task_data" ]]; then
        echo "Task completed. (No usage data for this task)"
        return
    fi

    local agent=$(echo "$task_data" | jq -r '.agent')
    local tokens=$(echo "$task_data" | jq -r '.total_tokens')
    local cost=$(echo "$task_data" | jq -r '.cost_usd')
    local duration=$(echo "$task_data" | jq -r '.duration_sec')

    echo "Task completed via $agent: ${tokens} tokens, \$${cost}, ${duration}s"

    # If convoy provided, show convoy totals
    if [[ -n "$convoy_id" ]]; then
        local convoy_data=$(grep "\"convoy\":\"$convoy_id\"" "$USAGE_LOG")
        if [[ -n "$convoy_data" ]]; then
            local convoy_tokens=$(echo "$convoy_data" | jq -s 'map(.total_tokens) | add')
            local convoy_cost=$(echo "$convoy_data" | jq -s 'map(.cost_usd) | add | . * 100 | round / 100')
            local convoy_requests=$(echo "$convoy_data" | jq -s 'length')
            echo "Convoy total: ${convoy_requests} tasks, ${convoy_tokens} tokens, \$${convoy_cost}"
        fi
    fi
}
