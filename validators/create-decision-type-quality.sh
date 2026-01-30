#!/bin/bash
# Validate quality decision type
# A quality decision asks "is this good enough?"

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "quality" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

artifact=$(echo "$context" | jq -r '.artifact // empty')
if [ -z "$artifact" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["quality type requires 'artifact' field - what's being evaluated"],
  "warnings": ["Example: \"artifact\": \"PR #123: Add user authentication\""]
}
EOF
  exit 1
fi

assessment=$(echo "$context" | jq -r '.assessment // empty')
if [ -z "$assessment" ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["quality type requires 'assessment' field - your quality evaluation"],
  "warnings": ["Example: \"assessment\": \"Functional and tested, but error messages are generic\""]
}
EOF
  exit 1
fi

exit 0
