#!/bin/bash
# Validate checkpoint decision type
# A checkpoint is a mid-work check-in to verify direction
#
# Input: DecisionInput JSON on stdin
# Output: ValidationResult JSON on stdout
# Exit codes: 0=pass, 1=blocking fail

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

# Only validate if type is "checkpoint"
if [ "$dtype" != "checkpoint" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

# Check progress field
progress=$(echo "$context" | jq -r '.progress // empty')
if [ -z "$progress" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["checkpoint type requires 'progress' field in context"],
  "warnings": [
    "Summarize what you've accomplished so far.",
    "Example: \"progress\": \"Completed API design and data model. Tests passing.\""
  ]
}
EOF
  exit 1
fi

# Check next_steps field
next_steps=$(echo "$context" | jq -r '.next_steps // empty')
if [ -z "$next_steps" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["checkpoint type requires 'next_steps' field in context"],
  "warnings": [
    "Describe what you plan to do next.",
    "This helps the human decide if you're on the right track.",
    "Example: \"next_steps\": \"Implement CLI commands and integration tests\""
  ]
}
EOF
  exit 1
fi

exit 0
