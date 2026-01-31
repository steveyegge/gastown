#!/bin/bash
# Validate ambiguity decision type
# An ambiguity decision clarifies between multiple valid interpretations

input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "ambiguity" ]; then
  exit 0
fi

context=$(echo "$input" | jq '.context // {}')

interp_count=$(echo "$context" | jq '.interpretations | length // 0')
if [ "$interp_count" -lt 2 ]; then
  cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["ambiguity type requires 'interpretations' array with at least 2 items"],
  "warnings": ["List the different ways the requirement could be interpreted. Example: \"interpretations\": [\"A: Validate on keystroke\", \"B: Validate on submit\"]"]
}
EOF
  exit 1
fi

exit 0
