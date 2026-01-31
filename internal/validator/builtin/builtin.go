// Package builtin provides built-in validators for decision validation.
package builtin

import (
	"encoding/json"
	"fmt"

	"github.com/steveyegge/gastown/internal/validator"
)

// ValidateJSONContext verifies that context is valid JSON (if present).
// This is largely redundant with the --context flag validation but serves
// as an example and catches edge cases.
func ValidateJSONContext(input validator.DecisionInput) validator.ValidationResult {
	result := validator.ValidationResult{Valid: true}

	if input.Context == nil {
		// No context is valid
		return result
	}

	// Re-marshal to verify it's valid JSON structure
	_, err := json.Marshal(input.Context)
	if err != nil {
		result.Valid = false
		result.Blocking = true
		result.Errors = append(result.Errors, fmt.Sprintf("context is not valid JSON: %v", err))
	}

	return result
}

// ValidateHasOptions verifies that 2-4 options are present.
func ValidateHasOptions(input validator.DecisionInput) validator.ValidationResult {
	result := validator.ValidationResult{Valid: true}

	n := len(input.Options)
	if n < 2 {
		result.Valid = false
		result.Blocking = true
		result.Errors = append(result.Errors, fmt.Sprintf("at least 2 options required, got %d", n))
	} else if n > 4 {
		result.Valid = false
		result.Blocking = true
		result.Errors = append(result.Errors, fmt.Sprintf("at most 4 options allowed, got %d", n))
	}

	return result
}

// ValidateSuccessorSchema verifies that successor_schemas is present when required.
// This is only enforced if the context indicates chaining is enabled.
func ValidateSuccessorSchema(input validator.DecisionInput) validator.ValidationResult {
	result := validator.ValidationResult{Valid: true}

	if input.Context == nil {
		// No context, can't have successor schemas
		return result
	}

	// Check if chaining_enabled is true
	chainingEnabled, ok := input.Context["chaining_enabled"].(bool)
	if !ok || !chainingEnabled {
		// Chaining not enabled, skip validation
		return result
	}

	// If chaining is enabled, successor_schemas should be present
	if _, ok := input.Context["successor_schemas"]; !ok {
		result.Valid = false
		result.Blocking = false // Warning only
		result.Warnings = append(result.Warnings, "chaining_enabled=true but no successor_schemas defined")
	}

	return result
}

// ValidateOptionLabels ensures all options have non-empty labels.
func ValidateOptionLabels(input validator.DecisionInput) validator.ValidationResult {
	result := validator.ValidationResult{Valid: true}

	for i, opt := range input.Options {
		if opt.Label == "" {
			result.Valid = false
			result.Blocking = true
			result.Errors = append(result.Errors, fmt.Sprintf("option %d has empty label", i+1))
		}
	}

	return result
}

// BuiltinValidators returns all built-in validators as functions.
type BuiltinValidator struct {
	Name     string
	When     string
	Scope    string
	Validate func(validator.DecisionInput) validator.ValidationResult
}

// GetBuiltinValidators returns the list of built-in validators.
func GetBuiltinValidators() []BuiltinValidator {
	return []BuiltinValidator{
		{
			Name:     "json-context",
			When:     "create",
			Scope:    "decision",
			Validate: ValidateJSONContext,
		},
		{
			Name:     "has-options",
			When:     "create",
			Scope:    "decision",
			Validate: ValidateHasOptions,
		},
		{
			Name:     "successor-schema",
			When:     "stop",
			Scope:    "decision",
			Validate: ValidateSuccessorSchema,
		},
		{
			Name:     "option-labels",
			When:     "create",
			Scope:    "decision",
			Validate: ValidateOptionLabels,
		},
	}
}

// RunBuiltinValidators runs all applicable built-in validators.
func RunBuiltinValidators(when, scope string, input validator.DecisionInput) validator.ExecuteResult {
	builtins := GetBuiltinValidators()
	result := validator.ExecuteResult{Passed: true}

	for _, b := range builtins {
		if b.When != when && b.When != "any" {
			continue
		}
		if b.Scope != scope && b.Scope != "any" {
			continue
		}

		vr := b.Validate(input)
		result.Results = append(result.Results, validator.ValidatorResult{
			Validator: validator.Validator{
				Name:   b.Name,
				When:   b.When,
				Scope:  b.Scope,
				Source: "builtin",
			},
			Result: vr,
		})

		result.Warnings = append(result.Warnings, vr.Warnings...)

		if !vr.Valid && vr.Blocking {
			result.Passed = false
			for _, e := range vr.Errors {
				result.Errors = append(result.Errors, fmt.Sprintf("[builtin:%s] %s", b.Name, e))
			}
		}
	}

	return result
}
