#!/usr/bin/env bash
#
# context-budget-guard.sh — Monitor context window usage, enforce handoff thresholds.
#
# A standalone guard script for Claude Code hooks. Reads the active session's
# transcript to track token usage and enforces configurable thresholds.
#
# Exit codes:
#   0 — Allow (under threshold, disabled, or fail-open on any error)
#   2 — Block (hard-gate threshold exceeded for a hard-gated role)
#
# Configuration (environment variables):
#   GT_CONTEXT_BUDGET_DISABLE=1                — Disable the guard entirely
#   GT_CONTEXT_BUDGET_WARN=0.75                — Warning threshold (default: 0.75)
#   GT_CONTEXT_BUDGET_SOFT_GATE=0.85           — Soft gate threshold (default: 0.85)
#   GT_CONTEXT_BUDGET_HARD_GATE=0.92           — Hard gate threshold (default: 0.92)
#   GT_CONTEXT_BUDGET_MAX_TOKENS=200000        — Max context tokens (default: 200000)
#   GT_CONTEXT_BUDGET_HARD_GATE_ROLES=mayor,deacon,witness,refinery
#                                              — Comma-separated roles that get blocked
#                                                at hard gate (default shown above)
#
# Hook configuration example (UserPromptSubmit — fires once per user turn):
#   {
#     "UserPromptSubmit": [{
#       "matcher": "",
#       "hooks": [{"type": "command", "command": "/path/to/context-budget-guard.sh"}]
#     }]
#   }
#
# Dependencies: jq (fails open if not found)
#
# Known limitation: Transcript JSONL format is a Claude Code implementation
# detail that may change without notice. Token counts are extracted from the
# last assistant message's usage object. If the format changes, the guard
# fails open (exit 0). To override the token source, set GT_CONTEXT_BUDGET_TOKENS
# to a pre-computed token count and the guard will skip transcript parsing.
#
set -euo pipefail

# ── Fail open: any unhandled error → allow ──────────────────────────────────
trap 'exit 0' ERR

# ── Check if disabled ───────────────────────────────────────────────────────
[[ "${GT_CONTEXT_BUDGET_DISABLE:-}" == "1" ]] && exit 0

# ── Configuration with defaults ─────────────────────────────────────────────
WARN="${GT_CONTEXT_BUDGET_WARN:-0.75}"
SOFT_GATE="${GT_CONTEXT_BUDGET_SOFT_GATE:-0.85}"
HARD_GATE="${GT_CONTEXT_BUDGET_HARD_GATE:-0.92}"
MAX_TOKENS="${GT_CONTEXT_BUDGET_MAX_TOKENS:-200000}"
HARD_GATE_ROLES="${GT_CONTEXT_BUDGET_HARD_GATE_ROLES:-mayor,deacon,witness,refinery}"

# ── Threshold ordering validation ───────────────────────────────────────────
# If thresholds are inverted (e.g., WARN=0.95, HARD_GATE=0.70), reset to defaults.
if awk "BEGIN { exit !($WARN >= $SOFT_GATE || $SOFT_GATE >= $HARD_GATE) }"; then
    WARN=0.75
    SOFT_GATE=0.85
    HARD_GATE=0.92
    echo "context-budget-guard: thresholds inverted, reset to defaults (0.75/0.85/0.92)" >&2
fi

# ── Resolve token count ─────────────────────────────────────────────────────
# If GT_CONTEXT_BUDGET_TOKENS is set, use it directly (allows abstracting the
# token source away from transcript parsing).
if [[ -n "${GT_CONTEXT_BUDGET_TOKENS:-}" ]]; then
    INPUT_TOKENS="$GT_CONTEXT_BUDGET_TOKENS"
else
    # Require jq for transcript parsing; fail open if missing
    command -v jq &>/dev/null || exit 0

    # Find Claude Code project directory for current working directory.
    # Claude Code stores transcripts in ~/.claude/projects/<path-with-dashes>/
    PROJECT_DIR="$HOME/.claude/projects/$(pwd | tr '/' '-')"
    [[ -d "$PROJECT_DIR" ]] || exit 0

    # Find the most recently modified .jsonl transcript (non-recursive)
    TRANSCRIPT=""
    LATEST_MTIME=0
    for f in "$PROJECT_DIR"/*.jsonl; do
        [[ -f "$f" ]] || continue
        MTIME=$(stat -c %Y "$f" 2>/dev/null || stat -f %m "$f" 2>/dev/null || echo 0)
        if [[ "$MTIME" -gt "$LATEST_MTIME" ]]; then
            LATEST_MTIME="$MTIME"
            TRANSCRIPT="$f"
        fi
    done
    [[ -n "$TRANSCRIPT" ]] || exit 0

    # Parse last assistant message's token usage from the transcript.
    # Read from the end for efficiency, find first assistant message with usage,
    # and sum all input token types: input_tokens + cache_creation + cache_read.
    #
    # Known limitation: This JSONL format is a Claude Code implementation detail.
    # If the format changes, jq will fail to match and the guard exits 0 (fail open).
    #
    # Portability: tac is GNU coreutils (Linux); macOS has tail -r instead.
    _reverse() { if command -v tac &>/dev/null; then tac "$1"; else tail -r "$1"; fi; }
    INPUT_TOKENS=$(_reverse "$TRANSCRIPT" \
        | jq -r 'select(.type == "assistant" and .message.usage != null)
                  | .message.usage
                  | ((.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0))' \
        2>/dev/null \
        | head -1)
    [[ -n "$INPUT_TOKENS" ]] && [[ "$INPUT_TOKENS" -gt 0 ]] 2>/dev/null || exit 0
fi

# ── Calculate usage ratio ───────────────────────────────────────────────────
# Use awk for portable floating-point math (no bc dependency)
RATIO=$(awk "BEGIN { printf \"%.4f\", $INPUT_TOKENS / $MAX_TOKENS }")
PCT=$(awk "BEGIN { printf \"%d\", $INPUT_TOKENS / $MAX_TOKENS * 100 }")
USED_K=$(( INPUT_TOKENS / 1000 ))
MAX_K=$(( MAX_TOKENS / 1000 ))

# ── Determine current role ──────────────────────────────────────────────────
ROLE="${GT_ROLE:-}"
[[ -z "$ROLE" ]] && [[ -n "${GT_POLECAT:-}" ]]   && ROLE="polecat"
[[ -z "$ROLE" ]] && [[ -n "${GT_CREW:-}" ]]       && ROLE="crew"
[[ -z "$ROLE" ]] && [[ -n "${GT_MAYOR:-}" ]]      && ROLE="mayor"
[[ -z "$ROLE" ]] && [[ -n "${GT_DEACON:-}" ]]     && ROLE="deacon"
[[ -z "$ROLE" ]] && [[ -n "${GT_WITNESS:-}" ]]    && ROLE="witness"
[[ -z "$ROLE" ]] && [[ -n "${GT_REFINERY:-}" ]]   && ROLE="refinery"
ROLE=$(echo "$ROLE" | tr '[:upper:]' '[:lower:]')

# Check if this role is hard-gated
IS_HARD_GATED=false
if [[ -n "$ROLE" ]] && echo ",$HARD_GATE_ROLES," | grep -qi ",$ROLE,"; then
    IS_HARD_GATED=true
elif [[ -z "$ROLE" ]]; then
    # Unknown role gets hard-gated for safety
    IS_HARD_GATED=true
fi

# ── Evaluate thresholds ─────────────────────────────────────────────────────
if awk "BEGIN { exit !($RATIO >= $HARD_GATE) }"; then
    echo "" >&2
    echo "CONTEXT BUDGET EXCEEDED (${PCT}%) — ${USED_K}k/${MAX_K}k tokens" >&2
    echo "   You MUST hand off remaining work NOW." >&2
    echo "   Run: gt handoff -s \"Context exhausted\" -m \"<remaining work>\"" >&2
    echo "   Or:  gt done  (if work is complete)" >&2
    echo "" >&2
    if [[ "$IS_HARD_GATED" == "true" ]]; then
        exit 2
    fi
elif awk "BEGIN { exit !($RATIO >= $SOFT_GATE) }"; then
    echo "" >&2
    echo "CONTEXT BUDGET AT ${PCT}% — HANDOFF RECOMMENDED — ${USED_K}k/${MAX_K}k tokens" >&2
    echo "   Run: gt handoff -s \"Context budget\" -m \"<what remains>\"" >&2
    echo "   Or:  gt done  (if work is complete)" >&2
    echo "" >&2
elif awk "BEGIN { exit !($RATIO >= $WARN) }"; then
    REMAINING_K=$(( (MAX_TOKENS - INPUT_TOKENS) / 1000 ))
    echo "" >&2
    echo "Context budget at ${PCT}% (${USED_K}k/${MAX_K}k tokens, ~${REMAINING_K}k remaining)" >&2
    echo "   Consider using gt handoff to pass remaining work to a fresh session." >&2
    echo "" >&2
fi

exit 0
