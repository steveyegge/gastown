#!/bin/bash
# Validate tradeoff decision type
# A tradeoff decision needs options being compared and a recommendation
#
# Input: DecisionInput JSON on stdin
# Output: ValidationResult JSON on stdout
# Exit codes: 0=pass, 1=blocking fail

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

# Only validate if type is "tradeoff"
if [ "$dtype" != "tradeoff" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

# Check options array (the alternatives being weighed)
options_count=$(echo "$context" | jq '.options | length // 0')
if [ "$options_count" -lt 2 ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["tradeoff type requires 'options' array with at least 2 items in context"],
  "warnings": [
    "A good tradeoff decision shows the alternatives being weighed.",
    "Example context: {\"options\": [\"Redis\", \"SQLite\"], \"recommendation\": \"Redis\", \"deciding_factor\": \"ops simplicity vs scalability\"}"
  ]
}
EOF
  exit 1
fi

# Check recommendation
rec=$(echo "$context" | jq -r '.recommendation // empty')
if [ -z "$rec" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["tradeoff type requires 'recommendation' field in context"],
  "warnings": [
    "Even if you're unsure, give your best recommendation.",
    "The human can override it, but your analysis is valuable.",
    "Example: \"recommendation\": \"Redis - better for our multi-node future\""
  ]
}
EOF
  exit 1
fi

exit 0
