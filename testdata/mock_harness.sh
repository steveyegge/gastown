#!/bin/bash
# Mock agent harness for testing rate limit detection and swapping.
# Exits with a configurable exit code to simulate various scenarios.
#
# Usage:
#   mock_harness.sh [exit_code] [message]
#
# Exit codes:
#   0 - Success
#   1 - Generic error
#   2 - Rate limit (Claude Code convention)
#
# Examples:
#   mock_harness.sh 2 "Rate limit exceeded"
#   mock_harness.sh 0 "Work completed successfully"

EXIT_CODE="${1:-0}"
MESSAGE="${2:-Mock harness exiting}"

# Output to stderr for rate limit scenarios
if [ "$EXIT_CODE" -eq 2 ]; then
    echo "Error: 429 Too Many Requests - Rate limit exceeded" >&2
    echo "$MESSAGE" >&2
else
    echo "$MESSAGE"
fi

exit "$EXIT_CODE"
