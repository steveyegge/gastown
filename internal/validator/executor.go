package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// ValidationResult represents the output of a validator.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Blocking bool     `json:"blocking"`
}

// DecisionInput is the JSON sent to validators on stdin.
type DecisionInput struct {
	ID            string                 `json:"id"`
	Prompt        string                 `json:"prompt"`
	Context       map[string]interface{} `json:"context,omitempty"`
	Options       []OptionInput          `json:"options"`
	ChosenIndex   int                    `json:"chosen_index,omitempty"`
	Event         string                 `json:"event"` // "create", "stop", "resolve"
	PredecessorID string                 `json:"predecessor_id,omitempty"`
	Type          string                 `json:"type,omitempty"`
}

// OptionInput represents a decision option.
type OptionInput struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// ExecuteResult contains the aggregated result of running validators.
type ExecuteResult struct {
	Passed    bool               // All validators passed (no blocking failures)
	Results   []ValidatorResult  // Individual results
	Errors    []string           // Aggregated blocking errors
	Warnings  []string           // Aggregated warnings
}

// ValidatorResult is the result of running a single validator.
type ValidatorResult struct {
	Validator Validator
	Result    ValidationResult
	ExitCode  int
	Error     error // Execution error (not validation error)
}

// DefaultTimeout is the default timeout for validator execution.
const DefaultTimeout = 30 * time.Second

// Execute runs all provided validators against the given input.
func Execute(validators []Validator, input DecisionInput, timeout time.Duration) ExecuteResult {
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	result := ExecuteResult{
		Passed:  true,
		Results: make([]ValidatorResult, 0, len(validators)),
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		result.Passed = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to marshal input: %v", err))
		return result
	}

	for _, v := range validators {
		vr := executeOne(v, inputJSON, timeout)
		result.Results = append(result.Results, vr)

		// Aggregate errors and warnings
		result.Warnings = append(result.Warnings, vr.Result.Warnings...)

		if vr.Error != nil {
			// Execution error - treat as blocking
			result.Passed = false
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] execution error: %v", v.Name, vr.Error))
		} else if !vr.Result.Valid && vr.Result.Blocking {
			// Validation failed with blocking
			result.Passed = false
			for _, e := range vr.Result.Errors {
				result.Errors = append(result.Errors, fmt.Sprintf("[%s] %s", v.Name, e))
			}
		}
	}

	return result
}

// executeOne runs a single validator and returns its result.
func executeOne(v Validator, inputJSON []byte, timeout time.Duration) ValidatorResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.Path) //nolint:gosec // validators are trusted
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	vr := ValidatorResult{
		Validator: v,
		ExitCode:  0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			vr.ExitCode = exitErr.ExitCode()
		} else {
			vr.Error = err
			return vr
		}
	}

	// Parse output as JSON
	if stdout.Len() > 0 {
		if parseErr := json.Unmarshal(stdout.Bytes(), &vr.Result); parseErr != nil {
			// If output isn't valid JSON, treat as plain error message
			vr.Result = ValidationResult{
				Valid:    false,
				Errors:   []string{stdout.String()},
				Blocking: vr.ExitCode == 1,
			}
		}
	} else {
		// No output - determine result from exit code
		vr.Result = resultFromExitCode(vr.ExitCode)
	}

	// Override blocking based on exit code
	switch vr.ExitCode {
	case 0:
		vr.Result.Valid = true
		vr.Result.Blocking = false
	case 1:
		vr.Result.Valid = false
		vr.Result.Blocking = true
	case 2:
		vr.Result.Valid = false
		vr.Result.Blocking = false // Warning only
	}

	return vr
}

// resultFromExitCode creates a ValidationResult based on exit code alone.
func resultFromExitCode(code int) ValidationResult {
	switch code {
	case 0:
		return ValidationResult{Valid: true, Blocking: false}
	case 1:
		return ValidationResult{Valid: false, Blocking: true, Errors: []string{"validator failed (exit 1)"}}
	case 2:
		return ValidationResult{Valid: false, Blocking: false, Warnings: []string{"validator warning (exit 2)"}}
	default:
		return ValidationResult{Valid: false, Blocking: true, Errors: []string{fmt.Sprintf("validator failed (exit %d)", code)}}
	}
}

// RunCreateValidators discovers and runs validators for decision creation.
func RunCreateValidators(townRoot string, input DecisionInput) ExecuteResult {
	input.Event = "create"
	validators, _ := DiscoverForScope(townRoot, "create", "decision")
	return Execute(validators, input, DefaultTimeout)
}

// RunStopValidators discovers and runs validators for stop/turn-check.
func RunStopValidators(townRoot string, input DecisionInput) ExecuteResult {
	input.Event = "stop"
	validators, _ := DiscoverForScope(townRoot, "stop", "decision")
	return Execute(validators, input, DefaultTimeout)
}

// RunResolveValidators discovers and runs validators for decision resolution.
func RunResolveValidators(townRoot string, input DecisionInput) ExecuteResult {
	input.Event = "resolve"
	validators, _ := DiscoverForScope(townRoot, "resolve", "decision")
	return Execute(validators, input, DefaultTimeout)
}
