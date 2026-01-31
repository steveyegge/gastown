package builtin

import (
	"testing"

	"github.com/steveyegge/gastown/internal/validator"
)

func TestValidateHasOptions(t *testing.T) {
	tests := []struct {
		name       string
		numOptions int
		wantValid  bool
	}{
		{"zero options", 0, false},
		{"one option", 1, false},
		{"two options", 2, true},
		{"three options", 3, true},
		{"four options", 4, true},
		{"five options", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validator.DecisionInput{
				Options: make([]validator.OptionInput, tt.numOptions),
			}
			for i := range input.Options {
				input.Options[i].Label = "Option"
			}

			result := ValidateHasOptions(input)
			if result.Valid != tt.wantValid {
				t.Errorf("ValidateHasOptions() with %d options: Valid = %v, want %v",
					tt.numOptions, result.Valid, tt.wantValid)
			}
		})
	}
}

func TestValidateOptionLabels(t *testing.T) {
	tests := []struct {
		name      string
		labels    []string
		wantValid bool
	}{
		{"all valid", []string{"A", "B"}, true},
		{"one empty", []string{"A", ""}, false},
		{"all empty", []string{"", ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validator.DecisionInput{
				Options: make([]validator.OptionInput, len(tt.labels)),
			}
			for i, label := range tt.labels {
				input.Options[i].Label = label
			}

			result := ValidateOptionLabels(input)
			if result.Valid != tt.wantValid {
				t.Errorf("ValidateOptionLabels() Valid = %v, want %v", result.Valid, tt.wantValid)
			}
		})
	}
}

func TestValidateJSONContext(t *testing.T) {
	tests := []struct {
		name      string
		context   map[string]interface{}
		wantValid bool
	}{
		{"nil context", nil, true},
		{"empty context", map[string]interface{}{}, true},
		{"valid context", map[string]interface{}{"key": "value"}, true},
		{"nested context", map[string]interface{}{
			"data": map[string]interface{}{"nested": true},
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validator.DecisionInput{
				Context: tt.context,
			}

			result := ValidateJSONContext(input)
			if result.Valid != tt.wantValid {
				t.Errorf("ValidateJSONContext() Valid = %v, want %v", result.Valid, tt.wantValid)
			}
		})
	}
}

func TestValidateSuccessorSchema(t *testing.T) {
	tests := []struct {
		name         string
		context      map[string]interface{}
		wantValid    bool
		wantWarnings int
	}{
		{
			name:      "no context",
			context:   nil,
			wantValid: true,
		},
		{
			name:      "chaining disabled",
			context:   map[string]interface{}{"chaining_enabled": false},
			wantValid: true,
		},
		{
			name: "chaining enabled with schemas",
			context: map[string]interface{}{
				"chaining_enabled":  true,
				"successor_schemas": map[string]interface{}{},
			},
			wantValid: true,
		},
		{
			name: "chaining enabled without schemas",
			context: map[string]interface{}{
				"chaining_enabled": true,
			},
			wantValid:    false,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validator.DecisionInput{
				Context: tt.context,
			}

			result := ValidateSuccessorSchema(input)
			if result.Valid != tt.wantValid {
				t.Errorf("ValidateSuccessorSchema() Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d", len(result.Warnings), tt.wantWarnings)
			}
		})
	}
}

func TestRunBuiltinValidators(t *testing.T) {
	// Valid input should pass all validators
	validInput := validator.DecisionInput{
		Prompt: "Test decision",
		Options: []validator.OptionInput{
			{Label: "A", Description: "Option A"},
			{Label: "B", Description: "Option B"},
		},
		Context: map[string]interface{}{"key": "value"},
	}

	result := RunBuiltinValidators("create", "decision", validInput)
	if !result.Passed {
		t.Errorf("Valid input should pass: errors = %v", result.Errors)
	}

	// Invalid input (no options) should fail
	invalidInput := validator.DecisionInput{
		Prompt:  "Test",
		Options: []validator.OptionInput{},
	}

	result = RunBuiltinValidators("create", "decision", invalidInput)
	if result.Passed {
		t.Error("Invalid input (no options) should fail")
	}
}

func TestGetBuiltinValidators(t *testing.T) {
	validators := GetBuiltinValidators()
	if len(validators) < 3 {
		t.Errorf("Expected at least 3 built-in validators, got %d", len(validators))
	}

	// Verify required validators exist
	names := make(map[string]bool)
	for _, v := range validators {
		names[v.Name] = true
	}

	required := []string{"json-context", "has-options", "successor-schema"}
	for _, name := range required {
		if !names[name] {
			t.Errorf("Missing required validator: %s", name)
		}
	}
}
