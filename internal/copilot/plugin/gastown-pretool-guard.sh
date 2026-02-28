#!/bin/bash
# Gas Town preToolUse guard — filters tool invocations and enforces PR workflow policy.
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.toolName')
[ "$TOOL_NAME" = "bash" ] || exit 0

COMMAND=$(echo "$INPUT" | jq -r '.toolArgs' | jq -r '.command // empty')
[ -n "$COMMAND" ] || exit 0

if echo "$COMMAND" | grep -qE '(^|[;&|]\s*|&&\s*|\|\|\s*)(\s*)(gh pr create|git checkout -b|git switch -c)'; then
  RESULT=$(gt tap guard pr-workflow 2>&1)
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    jq -nc --arg reason "$RESULT" \
      '{"permissionDecision":"deny","permissionDecisionReason":$reason}'
  fi
fi
