#!/bin/bash
# Gas Town preToolUse guard — filters tool invocations and enforces PR workflow policy.
INPUT=$(cat)
TOOL_NAME=$(printf '%s\n' "$INPUT" | jq -r '.toolName')
[ "$TOOL_NAME" = "bash" ] || exit 0

COMMAND=$(printf '%s\n' "$INPUT" | jq -r '.toolArgs' | jq -r '.command // empty')
[ -n "$COMMAND" ] || exit 0

if echo "$COMMAND" | grep -qE '(^|[;&|]\s*|&&\s*|\|\|\s*)(\s*)(gh pr create|git checkout -b|git switch -c)'; then
  if ! command -v gt >/dev/null 2>&1; then
    jq -nc --arg reason "gt not found — denying guarded command (fail-closed)" \
      '{"permissionDecision":"deny","permissionDecisionReason":$reason}'
    exit 0
  fi
  RESULT=$(gt tap guard pr-workflow 2>&1)
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    jq -nc --arg reason "$RESULT" \
      '{"permissionDecision":"deny","permissionDecisionReason":$reason}'
  fi
fi
