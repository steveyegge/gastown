#!/bin/bash
# Validate stuck decision type
# A stuck decision means the agent is blocked and needs help
#
# Input: DecisionInput JSON on stdin
# Output: ValidationResult JSON on stdout
# Exit codes: 0=pass, 1=blocking fail

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

# Only validate if type is "stuck"
if [ "$dtype" != "stuck" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

# Check blocker field
blocker=$(echo "$context" | jq -r '.blocker // empty')
if [ -z "$blocker" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["stuck type requires 'blocker' field in context"],
  "warnings": [
    "Describe what's blocking your progress.",
    "Example: \"blocker\": \"Need AWS credentials to test S3 integration\""
  ]
}
EOF
  exit 1
fi

# Check tried field (what you already attempted)
tried_count=$(echo "$context" | jq '.tried | length // 0')
if [ "$tried_count" -lt 1 ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["stuck type requires 'tried' array with at least 1 item in context"],
  "warnings": [
    "List what you've already tried before asking for help.",
    "This helps the human understand the situation and avoid suggesting things you've done.",
    "Example: \"tried\": [\"Checked .env files\", \"Asked in #infra channel\"]"
  ]
}
EOF
  exit 1
fi

exit 0
