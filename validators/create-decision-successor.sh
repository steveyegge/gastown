#!/bin/bash
# Validate successor schema from predecessor decision
# If a decision has a predecessor, check that the context conforms to
# the schema defined by the predecessor's chosen option.
#
# Input: DecisionInput JSON on stdin (with predecessor_id field)
# Output: ValidationResult JSON on stdout
# Exit codes: 0=pass, 1=blocking fail

set -e

input=$(cat)
predecessor_id=$(echo "$input" | jq -r '.predecessor_id // empty')

# No predecessor = nothing to validate
if [ -z "$predecessor_id" ]; then
  exit 0
fi

# Load predecessor decision via bd (handle array output)
pred_raw=$(bd show "$predecessor_id" --json 2>/dev/null) || {
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Failed to load predecessor decision: $predecessor_id"]
}
EOF
  exit 1
}

# bd show returns array, extract first element
pred_json=$(echo "$pred_raw" | jq '.[0] // .')

# Check if predecessor is resolved (for decisions, check close_reason or labels)
status=$(echo "$pred_json" | jq -r '.status // "open"')
if [ "$status" != "closed" ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Predecessor decision $predecessor_id is not yet resolved"],
  "warnings": ["You must wait for the predecessor to be resolved before creating a follow-up decision."]
}
EOF
  exit 1
fi

# For newer decisions, chosen_index may not be stored in the bead itself
# Just pass validation if it's closed - the successor type check is handled in code
chosen_index=$(echo "$pred_json" | jq -r '.chosen_index // 1')
if [ "$chosen_index" = "0" ] || [ "$chosen_index" = "null" ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Predecessor decision $predecessor_id is not yet resolved"],
  "warnings": ["You must wait for the predecessor to be resolved before creating a follow-up decision."]
}
EOF
  exit 1
fi

# Get chosen option label (1-indexed to 0-indexed)
idx=$((chosen_index - 1))
chosen_label=$(echo "$pred_json" | jq -r ".options[$idx].label // empty")

# Get successor schema for chosen option from predecessor's context
schema=$(echo "$pred_json" | jq -r ".context.successor_schemas[\"$chosen_label\"] // empty" 2>/dev/null)
if [ -z "$schema" ] || [ "$schema" = "null" ]; then
  exit 0  # No schema defined for this option
fi

# Get required fields from schema
required=$(echo "$schema" | jq -r '.required // [] | .[]' 2>/dev/null)
if [ -z "$required" ]; then
  exit 0  # No required fields
fi

# Check current decision's context for required fields
context=$(echo "$input" | jq '.context // {}')
missing=""

for field in $required; do
  val=$(echo "$context" | jq -r ".[\"$field\"] // empty")
  if [ -z "$val" ]; then
    missing="$missing \"$field\""
  fi
done

if [ -n "$missing" ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Successor schema requires missing context fields:$missing"],
  "warnings": [
    "Predecessor '$predecessor_id' chose '$chosen_label'",
    "That option defines a successor schema requiring:$missing",
    "Add these fields to your --context JSON."
  ]
}
EOF
  exit 1
fi

exit 0
