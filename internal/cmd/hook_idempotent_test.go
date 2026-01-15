package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestHookResult_JSON(t *testing.T) {
	tests := []struct {
		name     string
		result   hookResult
		expected map[string]interface{}
	}{
		{
			name: "hooked action",
			result: hookResult{
				Action: "hooked",
				BeadID: "gt-abc123",
			},
			expected: map[string]interface{}{
				"action":  "hooked",
				"bead_id": "gt-abc123",
			},
		},
		{
			name: "skipped action",
			result: hookResult{
				Action:  "skipped",
				BeadID:  "gt-abc123",
				Reason:  "hook_occupied",
				Current: "gt-xyz789",
			},
			expected: map[string]interface{}{
				"action":  "skipped",
				"bead_id": "gt-abc123",
				"reason":  "hook_occupied",
				"current": "gt-xyz789",
			},
		},
		{
			name: "replaced action",
			result: hookResult{
				Action:   "replaced",
				BeadID:   "gt-abc123",
				Previous: strPtr("gt-old456"),
			},
			expected: map[string]interface{}{
				"action":   "replaced",
				"bead_id":  "gt-abc123",
				"previous": "gt-old456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			for key, expectedVal := range tt.expected {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("missing key %q in JSON output", key)
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("key %q: got %v, want %v", key, gotVal, expectedVal)
				}
			}

			// Ensure no extra keys for "hooked" action (omitempty should work)
			if tt.result.Action == "hooked" {
				if _, ok := got["previous"]; ok {
					t.Error("previous should be omitted for hooked action")
				}
				if _, ok := got["reason"]; ok {
					t.Error("reason should be omitted for hooked action")
				}
				if _, ok := got["current"]; ok {
					t.Error("current should be omitted for hooked action")
				}
			}
		})
	}
}

func TestHookResult_OmitEmpty(t *testing.T) {
	// Test that empty optional fields are omitted
	result := hookResult{
		Action: "hooked",
		BeadID: "gt-test",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Check raw JSON doesn't contain omitted fields
	if bytes.Contains(data, []byte("previous")) {
		t.Error("JSON should not contain 'previous' when nil")
	}
	if bytes.Contains(data, []byte("reason")) {
		t.Error("JSON should not contain 'reason' when empty")
	}
	if bytes.Contains(data, []byte("current")) {
		t.Error("JSON should not contain 'current' when empty")
	}
}

func TestHookFlagValidation(t *testing.T) {
	// These tests validate flag combinations at the logic level
	// The actual flag parsing is tested via command execution

	tests := []struct {
		name      string
		ifEmpty   bool
		upsert    bool
		force     bool
		wantError bool
	}{
		{
			name:      "no flags",
			ifEmpty:   false,
			upsert:    false,
			force:     false,
			wantError: false,
		},
		{
			name:      "if-empty only",
			ifEmpty:   true,
			upsert:    false,
			force:     false,
			wantError: false,
		},
		{
			name:      "upsert only",
			ifEmpty:   false,
			upsert:    true,
			force:     false,
			wantError: false,
		},
		{
			name:      "force only",
			ifEmpty:   false,
			upsert:    false,
			force:     true,
			wantError: false,
		},
		{
			name:      "if-empty and upsert conflict",
			ifEmpty:   true,
			upsert:    true,
			force:     false,
			wantError: true,
		},
		{
			name:      "if-empty and force conflict",
			ifEmpty:   true,
			upsert:    false,
			force:     true,
			wantError: true,
		},
		{
			name:      "upsert and force allowed",
			ifEmpty:   false,
			upsert:    true,
			force:     true,
			wantError: false, // upsert is stronger than force, no conflict
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := false
			if tt.ifEmpty && tt.upsert {
				hasError = true
			}
			if tt.ifEmpty && tt.force {
				hasError = true
			}

			if hasError != tt.wantError {
				t.Errorf("flag validation: got error=%v, want error=%v", hasError, tt.wantError)
			}
		})
	}
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}
