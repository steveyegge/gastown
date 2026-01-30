#!/bin/bash
# Validate exception decision type
# An exception decision handles unexpected situations

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "exception" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

situation=$(echo "$context" | jq -r '.situation // empty')
if [ -z "$situation" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["exception type requires 'situation' field - what unexpected thing happened"],
  "warnings": ["Example: \"situation\": \"Found 3 orphaned polecats with uncommitted work\""]
}
EOF
  exit 1
fi

recommendation=$(echo "$context" | jq -r '.recommendation // empty')
if [ -z "$recommendation" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["exception type requires 'recommendation' field - your suggested action"],
  "warnings": ["Example: \"recommendation\": \"RECOVER - the git log shows meaningful commits\""]
}
EOF
  exit 1
fi

exit 0
