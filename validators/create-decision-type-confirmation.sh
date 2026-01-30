#!/bin/bash
# Validate confirmation decision type
# A confirmation is for high-stakes actions needing human sign-off

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "confirmation" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

action=$(echo "$context" | jq -r '.action // empty')
if [ -z "$action" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["confirmation type requires 'action' field in context"],
  "warnings": ["Describe what action you're about to take. Example: \"action\": \"Delete all test databases\""]
}
EOF
  exit 1
fi

impact=$(echo "$context" | jq -r '.impact // empty')
if [ -z "$impact" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["confirmation type requires 'impact' field in context"],
  "warnings": ["Describe the impact if this proceeds. Example: \"impact\": \"All test data will be permanently lost\""]
}
EOF
  exit 1
fi

exit 0
