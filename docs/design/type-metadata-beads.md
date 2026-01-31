# Decision Validation Scripts Design

> Move all decision validation from Go code to scripts
> Part of epic: bd-epc-decision_type_templates_subtype

## Summary

**All decision validation becomes scripts.** Delete inline Go validation, reuse existing validator infrastructure.

| Before | After |
|--------|-------|
| `hasFailureContext()` Go function | `create-decision-fail-file.sh` |
| `validateSuccessorSchema()` Go function | `create-decision-successor.sh` |
| (new) type validation | `create-decision-type-{name}.sh` |

## Architecture

Reuse existing `internal/validator/` infrastructure:

```
gt decision request --type tradeoff --context '{...}'
        │
        ▼
    validator.RunCreateValidators(townRoot, input)
        │
        ▼
    Discovers scripts in:
      ~/.config/gt/validators/create-decision-*.sh
      .gt/validators/create-decision-*.sh
        │
        ▼
    Runs each script with DecisionInput JSON on stdin
        │
        ▼
    Scripts output JSON: {"valid": false, "errors": ["..."], "warnings": ["advice"]}
    Or just exit code: 0=pass, 1=block, 2=warn
```

## Script Output Format

Scripts can output rich JSON:

```json
{
  "valid": false,
  "blocking": true,
  "errors": ["tradeoff requires at least 2 options"],
  "warnings": ["Consider adding a 'deciding_factor' field for better context"]
}
```

Or just use exit codes:
- **Exit 0**: Pass
- **Exit 1**: Blocking failure (stderr shown as error)
- **Exit 2**: Warning only (stderr shown as warning)

## Scripts to Create

### 1. `create-decision-fail-file.sh`

Replaces `hasFailureContext()` + `hasFileOption()` (~70 lines Go).

```bash
#!/bin/bash
# Enforce "Fail then File" principle
# If prompt/context mentions failure, require a FILE option

input=$(cat)
prompt=$(echo "$input" | jq -r '.prompt // ""')
context=$(echo "$input" | jq -r '.context | tostring // ""')
options=$(echo "$input" | jq -r '.options[].label' | tr '[:upper:]' '[:lower:]')

combined=$(echo "$prompt $context" | tr '[:upper:]' '[:lower:]')

# Exclusions - design discussions, not actual failures
for excl in "vs" "tradeoff" "comparison" "versus" "hard fail" "soft fail"; do
  if [[ "$combined" == *"$excl"* ]]; then
    exit 0  # Skip validation for design discussions
  fi
done

# Check for failure keywords
failure_keywords="error errors failed fails failing failure bug bugs broke broken stuck crash crashed exception panic fatal cannot unable"
has_failure=false
for kw in $failure_keywords; do
  if [[ "$combined" =~ (^|[^a-z])$kw([^a-z]|$) ]]; then
    has_failure=true
    break
  fi
done

if [ "$has_failure" = false ]; then
  exit 0  # No failure context
fi

# Check for FILE option
file_keywords="file bug track issue create bead"
for kw in $file_keywords; do
  if [[ "$options" == *"$kw"* ]]; then
    exit 0  # Has FILE option
  fi
done

# Failure context without FILE option
cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Failure context detected but no FILE option provided"],
  "warnings": [
    "Per the 'Fail then File' principle, decisions about failures should include an option to file a tracking bug.",
    "Suggested option: --option \"File bug: Create tracking bead to investigate root cause\"",
    "Use --no-file-check to skip this validation."
  ]
}
EOF
exit 1
```

### 2. `create-decision-successor.sh`

Replaces `validateSuccessorSchema()` (~80 lines Go).

```bash
#!/bin/bash
# Validate successor schema from predecessor decision

input=$(cat)
predecessor_id=$(echo "$input" | jq -r '.predecessor_id // empty')

if [ -z "$predecessor_id" ]; then
  exit 0  # No predecessor, nothing to validate
fi

# Load predecessor via bd
pred_json=$(bd show "$predecessor_id" --json 2>/dev/null)
if [ $? -ne 0 ]; then
  echo '{"valid": false, "blocking": true, "errors": ["Failed to load predecessor decision"]}'
  exit 1
fi

chosen_index=$(echo "$pred_json" | jq -r '.chosen_index // 0')
if [ "$chosen_index" = "0" ]; then
  echo '{"valid": false, "blocking": true, "errors": ["Predecessor decision is not yet resolved"]}'
  exit 1
fi

# Get chosen option label
chosen_label=$(echo "$pred_json" | jq -r ".options[$((chosen_index - 1))].label // empty")

# Get successor schema for chosen option
schema=$(echo "$pred_json" | jq -r ".context.successor_schemas[\"$chosen_label\"] // empty")
if [ -z "$schema" ] || [ "$schema" = "null" ]; then
  exit 0  # No schema defined
fi

# Check required fields
required=$(echo "$schema" | jq -r '.required // [] | .[]')
context=$(echo "$input" | jq '.context // {}')

missing=""
for field in $required; do
  val=$(echo "$context" | jq -r ".[\"$field\"] // empty")
  if [ -z "$val" ]; then
    missing="$missing $field"
  fi
done

if [ -n "$missing" ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["Successor schema requires missing fields:$missing"],
  "warnings": ["Predecessor '$predecessor_id' chose '$chosen_label' which requires: $required"]
}
EOF
  exit 1
fi

exit 0
```

### 3. `create-decision-type-{name}.sh` (one per type)

Example for `tradeoff`:

```bash
#!/bin/bash
# Validate tradeoff decision type

input=$(cat)
context=$(echo "$input" | jq '.context // {}')

# Check options array
options_count=$(echo "$context" | jq '.options | length // 0')
if [ "$options_count" -lt 2 ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["tradeoff requires at least 2 options in context.options"],
  "warnings": [
    "A good tradeoff decision shows the alternatives being weighed.",
    "Example: {\"options\": [\"Redis\", \"SQLite\"], \"recommendation\": \"Redis\"}"
  ]
}
EOF
  exit 1
fi

# Check recommendation
rec=$(echo "$context" | jq -r '.recommendation // empty')
if [ -z "$rec" ]; then
  cat <<EOF
{
  "valid": false,
  "blocking": true,
  "errors": ["tradeoff requires a recommendation in context"],
  "warnings": [
    "Even if you're unsure, give your best recommendation.",
    "The human can override it, but your analysis is valuable."
  ]
}
EOF
  exit 1
fi

exit 0
```

## Go Code Changes

### Delete from `decision_impl.go`

```go
// DELETE these functions (~200 lines total):
- hasFailureContext()
- containsWholeWord()
- isWordChar()
- hasFileOption()
- suggestFileOption()
- validateSuccessorSchema()
- failureKeywords[]
- failureExclusions[]
- fileKeywords[]
```

### Modify in `decision_impl.go`

```go
func runDecisionRequest(cmd *cobra.Command, args []string) error {
    // ... existing validation ...

    // Build validator input
    input := validator.DecisionInput{
        Prompt:        decisionPrompt,
        Context:       contextMap,  // parsed JSON
        Options:       validatorOptions,
        PredecessorID: decisionPredecessor,  // NEW field
        Type:          decisionType,          // NEW field
    }

    // Run all create validators (replaces inline validation)
    result := validator.RunCreateValidators(townRoot, input)
    if !result.Passed {
        // Show errors
        for _, err := range result.Errors {
            style.PrintError("%s", err)
        }
        // Show warnings as advice
        for _, warn := range result.Warnings {
            style.PrintWarning("Advice: %s", warn)
        }
        return fmt.Errorf("validation failed")
    }

    // Show any warnings even on success
    for _, warn := range result.Warnings {
        style.PrintWarning("%s", warn)
    }

    // ... rest of function ...
}
```

### Add to `validator.DecisionInput`

```go
type DecisionInput struct {
    ID            string                 `json:"id"`
    Prompt        string                 `json:"prompt"`
    Context       map[string]interface{} `json:"context,omitempty"`
    Options       []OptionInput          `json:"options"`
    ChosenIndex   int                    `json:"chosen_index,omitempty"`
    Event         string                 `json:"event"`
    PredecessorID string                 `json:"predecessor_id,omitempty"`  // NEW
    Type          string                 `json:"type,omitempty"`            // NEW
}
```

## Implementation Estimate

| Action | Lines |
|--------|-------|
| Delete Go validation | -200 |
| Add `--type` flag | +10 |
| Add fields to DecisionInput | +5 |
| Update runDecisionRequest | +20 |
| `create-decision-fail-file.sh` | +50 |
| `create-decision-successor.sh` | +50 |
| 8 type validator scripts | +100 |
| **Net** | **+35** |

But more importantly: validation is now **editable without recompiling**.

## Migration

1. Add `--type` flag and `PredecessorID`/`Type` to DecisionInput
2. Create validator scripts in `~/.config/gt/validators/`
3. Delete Go validation functions
4. Test that scripts produce same behavior

## Type Scripts to Ship

| Type | Script | Required Fields |
|------|--------|-----------------|
| `confirmation` | `create-decision-type-confirmation.sh` | action, impact |
| `ambiguity` | `create-decision-type-ambiguity.sh` | interpretations |
| `tradeoff` | `create-decision-type-tradeoff.sh` | options, recommendation |
| `stuck` | `create-decision-type-stuck.sh` | blocker, tried |
| `checkpoint` | `create-decision-type-checkpoint.sh` | progress, next_steps |
| `quality` | `create-decision-type-quality.sh` | artifact, assessment |
| `exception` | `create-decision-type-exception.sh` | situation, recommendation |
| `prioritization` | `create-decision-type-prioritization.sh` | candidates, constraints |
