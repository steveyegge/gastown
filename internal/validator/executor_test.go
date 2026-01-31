package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecuteOne(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple passing validator
	passScript := filepath.Join(tmpDir, "pass.sh")
	if err := os.WriteFile(passScript, []byte(`#!/bin/sh
echo '{"valid": true}'
exit 0
`), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a blocking failure validator
	failScript := filepath.Join(tmpDir, "fail.sh")
	if err := os.WriteFile(failScript, []byte(`#!/bin/sh
echo '{"valid": false, "errors": ["missing required field"], "blocking": true}'
exit 1
`), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a warning validator
	warnScript := filepath.Join(tmpDir, "warn.sh")
	if err := os.WriteFile(warnScript, []byte(`#!/bin/sh
echo '{"valid": false, "warnings": ["consider adding context"], "blocking": false}'
exit 2
`), 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		path         string
		wantValid    bool
		wantBlocking bool
		wantExitCode int
	}{
		{
			name:         "passing validator",
			path:         passScript,
			wantValid:    true,
			wantBlocking: false,
			wantExitCode: 0,
		},
		{
			name:         "blocking failure",
			path:         failScript,
			wantValid:    false,
			wantBlocking: true,
			wantExitCode: 1,
		},
		{
			name:         "warning only",
			path:         warnScript,
			wantValid:    false,
			wantBlocking: false,
			wantExitCode: 2,
		},
	}

	input := DecisionInput{
		ID:     "test-123",
		Prompt: "Test decision",
		Options: []OptionInput{
			{Label: "A", Description: "Option A"},
			{Label: "B", Description: "Option B"},
		},
		Event: "create",
	}
	inputJSON, _ := json.Marshal(input)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Validator{Path: tt.path, Name: "test"}
			result := executeOne(v, inputJSON, 5*time.Second)

			if result.ExitCode != tt.wantExitCode {
				t.Errorf("ExitCode = %d, want %d", result.ExitCode, tt.wantExitCode)
			}
			if result.Result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Result.Valid, tt.wantValid)
			}
			if result.Result.Blocking != tt.wantBlocking {
				t.Errorf("Blocking = %v, want %v", result.Result.Blocking, tt.wantBlocking)
			}
		})
	}
}

func TestExecuteAggregation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple validators
	pass1 := filepath.Join(tmpDir, "pass1.sh")
	os.WriteFile(pass1, []byte("#!/bin/sh\necho '{\"valid\": true}'\nexit 0"), 0755)

	warn1 := filepath.Join(tmpDir, "warn1.sh")
	os.WriteFile(warn1, []byte("#!/bin/sh\necho '{\"valid\": false, \"warnings\": [\"warning 1\"], \"blocking\": false}'\nexit 2"), 0755)

	pass2 := filepath.Join(tmpDir, "pass2.sh")
	os.WriteFile(pass2, []byte("#!/bin/sh\necho '{\"valid\": true}'\nexit 0"), 0755)

	validators := []Validator{
		{Path: pass1, Name: "pass1"},
		{Path: warn1, Name: "warn1"},
		{Path: pass2, Name: "pass2"},
	}

	input := DecisionInput{
		ID:     "test",
		Prompt: "Test",
		Options: []OptionInput{{Label: "A"}, {Label: "B"}},
		Event:  "create",
	}

	result := Execute(validators, input, 5*time.Second)

	// Should pass overall (warnings don't block)
	if !result.Passed {
		t.Error("Expected Passed=true with only warnings")
	}

	// Should have collected the warning
	if len(result.Warnings) != 1 {
		t.Errorf("got %d warnings, want 1", len(result.Warnings))
	}

	// All three validators should have run
	if len(result.Results) != 3 {
		t.Errorf("got %d results, want 3", len(result.Results))
	}
}

func TestExecuteWithBlockingFailure(t *testing.T) {
	tmpDir := t.TempDir()

	pass := filepath.Join(tmpDir, "pass.sh")
	os.WriteFile(pass, []byte("#!/bin/sh\necho '{\"valid\": true}'\nexit 0"), 0755)

	fail := filepath.Join(tmpDir, "fail.sh")
	os.WriteFile(fail, []byte("#!/bin/sh\necho '{\"valid\": false, \"errors\": [\"blocked!\"], \"blocking\": true}'\nexit 1"), 0755)

	validators := []Validator{
		{Path: pass, Name: "pass"},
		{Path: fail, Name: "fail"},
	}

	input := DecisionInput{
		ID:     "test",
		Prompt: "Test",
		Options: []OptionInput{{Label: "A"}, {Label: "B"}},
		Event:  "create",
	}

	result := Execute(validators, input, 5*time.Second)

	if result.Passed {
		t.Error("Expected Passed=false with blocking failure")
	}

	if len(result.Errors) == 0 {
		t.Error("Expected errors from blocking failure")
	}
}

func TestResultFromExitCode(t *testing.T) {
	tests := []struct {
		code         int
		wantValid    bool
		wantBlocking bool
	}{
		{0, true, false},
		{1, false, true},
		{2, false, false},
		{127, false, true},
	}

	for _, tt := range tests {
		r := resultFromExitCode(tt.code)
		if r.Valid != tt.wantValid {
			t.Errorf("exit %d: Valid = %v, want %v", tt.code, r.Valid, tt.wantValid)
		}
		if r.Blocking != tt.wantBlocking {
			t.Errorf("exit %d: Blocking = %v, want %v", tt.code, r.Blocking, tt.wantBlocking)
		}
	}
}

func TestDecisionInputTypeAndPredecessor(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a type validator that checks for type field
	typeScript := filepath.Join(tmpDir, "type-check.sh")
	if err := os.WriteFile(typeScript, []byte(`#!/bin/sh
input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')
pred=$(echo "$input" | jq -r '.predecessor_id // empty')

if [ "$dtype" = "tradeoff" ]; then
  echo '{"valid": true}'
  exit 0
else
  echo '{"valid": false, "errors": ["expected type=tradeoff"], "blocking": true}'
  exit 1
fi
`), 0755); err != nil {
		t.Fatal(err)
	}

	v := Validator{Path: typeScript, Name: "type-check"}

	// Test with Type field set
	input := DecisionInput{
		ID:            "test-123",
		Prompt:        "Test decision",
		Options:       []OptionInput{{Label: "A"}, {Label: "B"}},
		Event:         "create",
		Type:          "tradeoff",
		PredecessorID: "dec-predecessor",
	}

	inputJSON, _ := json.Marshal(input)
	result := executeOne(v, inputJSON, 5*time.Second)

	if !result.Result.Valid {
		t.Errorf("Expected Valid=true for type=tradeoff, got errors: %v", result.Result.Errors)
	}

	// Test with wrong type
	input.Type = "wrong"
	inputJSON, _ = json.Marshal(input)
	result = executeOne(v, inputJSON, 5*time.Second)

	if result.Result.Valid {
		t.Error("Expected Valid=false for type=wrong")
	}
}

func TestTypeValidatorScript(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tradeoff type validator (simulates the real one)
	tradeoffScript := filepath.Join(tmpDir, "create-decision-type-tradeoff.sh")
	if err := os.WriteFile(tradeoffScript, []byte(`#!/bin/bash
input=$(cat)
dtype=$(echo "$input" | jq -r '.type // empty')

if [ "$dtype" != "tradeoff" ]; then
  exit 0  # Not our type, skip
fi

context=$(echo "$input" | jq '.context // {}')
options_count=$(echo "$context" | jq '.options | length // 0')

if [ "$options_count" -lt 2 ]; then
  echo '{"valid": false, "blocking": true, "errors": ["tradeoff requires 2+ options in context"]}'
  exit 1
fi

rec=$(echo "$context" | jq -r '.recommendation // empty')
if [ -z "$rec" ]; then
  echo '{"valid": false, "blocking": true, "errors": ["tradeoff requires recommendation"]}'
  exit 1
fi

echo '{"valid": true}'
exit 0
`), 0755); err != nil {
		t.Fatal(err)
	}

	v := Validator{Path: tradeoffScript, Name: "type-tradeoff"}

	tests := []struct {
		name      string
		input     DecisionInput
		wantValid bool
	}{
		{
			name: "valid tradeoff",
			input: DecisionInput{
				Type: "tradeoff",
				Context: map[string]interface{}{
					"options":        []string{"Redis", "SQLite"},
					"recommendation": "Redis",
				},
				Options: []OptionInput{{Label: "A"}, {Label: "B"}},
			},
			wantValid: true,
		},
		{
			name: "missing options",
			input: DecisionInput{
				Type: "tradeoff",
				Context: map[string]interface{}{
					"options":        []string{"Redis"}, // Only 1
					"recommendation": "Redis",
				},
				Options: []OptionInput{{Label: "A"}, {Label: "B"}},
			},
			wantValid: false,
		},
		{
			name: "missing recommendation",
			input: DecisionInput{
				Type: "tradeoff",
				Context: map[string]interface{}{
					"options": []string{"Redis", "SQLite"},
				},
				Options: []OptionInput{{Label: "A"}, {Label: "B"}},
			},
			wantValid: false,
		},
		{
			name: "different type (should skip)",
			input: DecisionInput{
				Type:    "checkpoint",
				Context: map[string]interface{}{}, // Empty context OK for other types
				Options: []OptionInput{{Label: "A"}, {Label: "B"}},
			},
			wantValid: true, // Skips validation, passes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, _ := json.Marshal(tt.input)
			result := executeOne(v, inputJSON, 5*time.Second)

			if result.Result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v (errors: %v)", result.Result.Valid, tt.wantValid, result.Result.Errors)
			}
		})
	}
}

func TestFailFileValidator(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fail-file validator (simulates the real one)
	failFileScript := filepath.Join(tmpDir, "create-decision-fail-file.sh")
	if err := os.WriteFile(failFileScript, []byte(`#!/bin/bash
input=$(cat)
prompt=$(echo "$input" | jq -r '.prompt // ""' | tr '[:upper:]' '[:lower:]')
options=$(echo "$input" | jq -r '.options[]?.label // empty' | tr '[:upper:]' '[:lower:]')

# Check for failure keywords
if [[ "$prompt" == *"error"* ]] || [[ "$prompt" == *"failed"* ]] || [[ "$prompt" == *"bug"* ]]; then
  # Check for FILE option
  if [[ "$options" == *"file"* ]] || [[ "$options" == *"bug"* ]] || [[ "$options" == *"track"* ]]; then
    exit 0  # Has FILE option
  fi
  echo '{"valid": false, "blocking": true, "errors": ["Failure context without FILE option"]}'
  exit 1
fi

exit 0  # No failure context
`), 0755); err != nil {
		t.Fatal(err)
	}

	v := Validator{Path: failFileScript, Name: "fail-file"}

	tests := []struct {
		name      string
		prompt    string
		options   []OptionInput
		wantValid bool
	}{
		{
			name:      "no failure context",
			prompt:    "Which cache strategy?",
			options:   []OptionInput{{Label: "Redis"}, {Label: "SQLite"}},
			wantValid: true,
		},
		{
			name:      "failure without FILE option",
			prompt:    "How to fix this error?",
			options:   []OptionInput{{Label: "Retry"}, {Label: "Skip"}},
			wantValid: false,
		},
		{
			name:      "failure with FILE option",
			prompt:    "How to fix this error?",
			options:   []OptionInput{{Label: "Retry"}, {Label: "File bug"}},
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := DecisionInput{
				Prompt:  tt.prompt,
				Options: tt.options,
			}
			inputJSON, _ := json.Marshal(input)
			result := executeOne(v, inputJSON, 5*time.Second)

			if result.Result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v (errors: %v)", result.Result.Valid, tt.wantValid, result.Result.Errors)
			}
		})
	}
}
