#!/bin/bash
# Validate prioritization decision type
# A prioritization decision asks what to work on first

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "prioritization" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

candidates_count=$(echo "$context" | jq '.candidates | length // 0')
if [ "$candidates_count" -lt 2 ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["prioritization type requires 'candidates' array with at least 2 items"],
  "warnings": ["List the competing work items. Example: \"candidates\": [{\"id\": \"gt-123\", \"title\": \"Fix crash\"}, {\"id\": \"gt-456\", \"title\": \"Add feature\"}]"]
}
EOF
  exit 1
fi

constraints=$(echo "$context" | jq -r '.constraints // empty')
if [ -z "$constraints" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["prioritization type requires 'constraints' field - time/resource limits"],
  "warnings": ["Example: \"constraints\": \"Can only finish 2 items before EOD\""]
}
EOF
  exit 1
fi

exit 0
