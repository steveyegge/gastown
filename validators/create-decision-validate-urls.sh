#!/bin/bash
# Validate URLs in decision context
# Checks that any URLs in context are reachable (HTTP 200)
#
# Input: DecisionInput JSON on stdin
# Output: ValidationResult JSON on stdout (or just exit code)
# Exit codes: 0=pass, 1=blocking fail, 2=warning only

set -e

input=$(cat)
context_str=$(echo "$input" | jq -r 'if .context then (.context | tostring) else "" end')

# Exit early if no context
if [ -z "$context_str" ] || [ "$context_str" = "null" ]; then
  exit 0
fi

# Extract URLs from context (http:// and https://)
# Match URLs until we hit: quote, space, close brace, close bracket, comma
urls=$(echo "$context_str" | grep -oP 'https?://[^\s"'"'"'\}\],]+' | sort -u || true)

# Exit if no URLs found
if [ -z "$urls" ]; then
  exit 0
fi

# Check each URL
bad_urls=()
while IFS= read -r url; do
  # Skip empty lines
  [ -z "$url" ] && continue

  # Check URL status (5 second timeout)
  status=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$url" 2>/dev/null || echo "000")

  # Treat anything other than 2xx as potentially bad
  if [[ ! "$status" =~ ^2[0-9][0-9]$ ]]; then
    bad_urls+=("$url (HTTP $status)")
  fi
done <<< "$urls"

# If all URLs are good, pass
if [ ${#bad_urls[@]} -eq 0 ]; then
  exit 0
fi

# Build warnings list
warnings_json=$(printf '%s\n' "${bad_urls[@]}" | jq -R . | jq -s .)

# Output warning (non-blocking)
cat <<EOF
{
  "valid": true,
  "blocking": false,
  "errors": [],
  "warnings": [
    "Some URLs in context may be inaccessible:",
    $(echo "$warnings_json" | jq -c '.[]'),
    "URLs might be: private repos, not yet pushed, or temporarily unavailable.",
    "This is a warning only - decision creation will proceed."
  ]
}
EOF
exit 2  # Warning only, non-blocking
