#!/usr/bin/env bash
# resolve-model.sh — Find or pull the best available Ollama model for diagnosis.
#
# Walks a preference list from smallest to largest. Returns the first model
# that is already pulled, or pulls the smallest one if none are available.
#
# Usage:
#   source resolve-model.sh
#   MODEL=$(resolve_ollama_model)        # Returns model name or empty string
#   MODEL=$(resolve_ollama_model quiet)  # Suppress log output
#
# Environment:
#   LLM_DOCTOR_OLLAMA_MODEL  — Override: skip discovery entirely
#   OLLAMA_URL               — Ollama endpoint (default: http://localhost:11434)
#   LLM_DOCTOR_MODEL_PREFS   — Colon-separated override of preference list

# Model preference list: smallest → largest.
# The doctor only needs basic classification — a 1B model suffices.
# Larger models produce better analysis but use more RAM and respond slower.
DEFAULT_MODEL_PREFS=(
    "llama3.2:1b"       # 1.3GB — minimal, fast, good enough for classification
    "phi4-mini:3.8b"    # 2.4GB — strong reasoning for size
    "llama3.2:3b"       # 2.0GB — good balance
    "llama3.1:8b"       # 4.7GB — better analysis, heavier
    "qwen2.5:7b"        # 4.7GB — strong multilingual
    "gemma2:9b"         # 5.4GB — google's compact model
    "qwen2.5:32b"       # 20GB  — overkill but if it's all you have
)

_resolve_log() {
    [[ "${1:-}" == "quiet" ]] && return
    echo "[llm-doctor:model] $2" >&2
}

resolve_ollama_model() {
    local quiet="${1:-}"
    local ollama_url="${OLLAMA_URL:-http://localhost:11434}"

    # If explicit override is set, just use it (user knows what they want)
    if [[ -n "${LLM_DOCTOR_OLLAMA_MODEL:-}" ]]; then
        _resolve_log "$quiet" "Using explicit model: $LLM_DOCTOR_OLLAMA_MODEL"
        echo "$LLM_DOCTOR_OLLAMA_MODEL"
        return 0
    fi

    # Check if Ollama is reachable
    local http_rc
    http_rc=$(curl -s -o /dev/null -w '%{http_code}' "$ollama_url/api/tags" \
        --connect-timeout 3 --max-time 5 2>/dev/null || echo "000")

    if [[ "$http_rc" != "200" ]]; then
        _resolve_log "$quiet" "Ollama not reachable (HTTP $http_rc)"
        return 1
    fi

    # Get list of pulled models
    local tags_json
    tags_json=$(curl -s "$ollama_url/api/tags" --max-time 5 2>/dev/null || echo "{}")

    local pulled_models
    pulled_models=$(echo "$tags_json" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
    for m in data.get("models", []):
        print(m["name"])
except:
    pass
' 2>/dev/null)

    # Build preference list (allow override via env)
    local prefs=()
    if [[ -n "${LLM_DOCTOR_MODEL_PREFS:-}" ]]; then
        IFS=':' read -ra prefs <<< "$LLM_DOCTOR_MODEL_PREFS"
    else
        prefs=("${DEFAULT_MODEL_PREFS[@]}")
    fi

    # Phase 1: Check if any preferred model is already pulled
    for model in "${prefs[@]}"; do
        # Match both exact name and name without tag (e.g., "llama3.2:1b" matches "llama3.2:1b")
        if echo "$pulled_models" | grep -qF "$model"; then
            _resolve_log "$quiet" "Found preferred model: $model (already pulled)"
            echo "$model"
            return 0
        fi
    done

    # Phase 2: Check if ANY pulled model could work (not in prefs but available)
    local first_available
    first_available=$(echo "$pulled_models" | head -1)
    if [[ -n "$first_available" ]]; then
        _resolve_log "$quiet" "No preferred model found, using available: $first_available"
        echo "$first_available"
        return 0
    fi

    # Phase 3: No models at all — try to pull the smallest preferred model
    _resolve_log "$quiet" "No models available — pulling ${prefs[0]}..."
    local pull_result
    pull_result=$(curl -s -X POST "$ollama_url/api/pull" \
        -d "{\"name\":\"${prefs[0]}\",\"stream\":false}" \
        --max-time 600 2>/dev/null || echo "")

    if [[ -n "$pull_result" ]] && echo "$pull_result" | python3 -c '
import json, sys
r = json.load(sys.stdin)
sys.exit(0 if r.get("status") == "success" else 1)
' 2>/dev/null; then
        _resolve_log "$quiet" "Pulled ${prefs[0]} successfully"
        echo "${prefs[0]}"
        return 0
    fi

    _resolve_log "$quiet" "Failed to pull ${prefs[0]} — no model available"
    return 1
}

# If run directly (not sourced), execute and print result
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    MODEL=$(resolve_ollama_model "$@")
    RC=$?
    if [[ $RC -eq 0 ]]; then
        echo "$MODEL"
    else
        echo "No model available" >&2
        exit 1
    fi
fi
