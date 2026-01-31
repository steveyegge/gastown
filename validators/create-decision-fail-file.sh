#!/bin/bash
# Enforce "Fail then File" principle
# If prompt/context mentions failure, require a FILE option
#
# Input: DecisionInput JSON on stdin
# Output: ValidationResult JSON on stdout (or just exit code)
# Exit codes: 0=pass, 1=blocking fail, 2=warning only

set -e

input=$(cat)
prompt=$(echo "$input" | jq -r '.prompt // ""')
context_str=$(echo "$input" | jq -r 'if .context then (.context | tostring) else "" end')
options=$(echo "$input" | jq -r '.options[]?.label // empty' | tr '[:upper:]' '[:lower:]')

combined=$(echo "$prompt $context_str" | tr '[:upper:]' '[:lower:]')

# Exclusions - design discussions, not actual failures
exclusions=("vs" "tradeoff" "comparison" "versus" "hard fail" "soft fail" "failure mode" "error handling")
for excl in "${exclusions[@]}"; do
  if [[ "$combined" == *"$excl"* ]]; then
    exit 0  # Skip validation for design discussions
  fi
done

# Check for failure keywords (word boundaries to avoid false positives)
failure_keywords=("error" "errors" "failed" "fails" "failing" "failure" "bug" "bugs" "broke" "broken" "stuck" "crash" "crashed" "crashing" "exception" "panic" "panicked" "fatal" "cannot" "unable")

has_failure=false
for kw in "${failure_keywords[@]}"; do
  # Word boundary check: keyword surrounded by non-alpha or at start/end
  if [[ "$combined" =~ (^|[^a-z])$kw([^a-z]|$) ]]; then
    has_failure=true
    break
  fi
done

if [ "$has_failure" = false ]; then
  exit 0  # No failure context detected
fi

# Check for FILE option keywords
file_keywords=("file" "bug" "track" "issue" "create" "bead")
for kw in "${file_keywords[@]}"; do
  if [[ "$options" == *"$kw"* ]]; then
    exit 0  # Has FILE option
  fi
done

# Failure context without FILE option - provide helpful feedback
cat <<'EOF'
{
  "valid": false,
  "blocking": true,
  "errors": ["Failure context detected but no FILE option provided"],
  "warnings": [
    "Per the 'Fail then File' principle, decisions about failures should include an option to file a tracking bug.",
    "Suggested option: --option \"File bug: Create tracking bead to investigate root cause\"",
    "Use --no-file-check to skip this validation (disables this validator)."
  ]
}
EOF
exit 1
