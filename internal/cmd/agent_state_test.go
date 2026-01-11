package cmd

import (
	"errors"
	"testing"
)

func TestParseStateLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		wantKeys []string
	}{
		{
			name:     "empty labels",
			labels:   []string{},
			wantKeys: []string{},
		},
		{
			name:     "only non-state labels",
			labels:   []string{"role_type", "urgent"},
			wantKeys: []string{},
		},
		{
			name:     "only state labels",
			labels:   []string{"idle:3", "backoff:2m"},
			wantKeys: []string{"idle", "backoff"},
		},
		{
			name:     "mixed labels",
			labels:   []string{"role_type", "idle:5", "urgent", "backoff:30s"},
			wantKeys: []string{"idle", "backoff"},
		},
		{
			name:     "label with multiple colons",
			labels:   []string{"last_activity:2025-01-01T12:00:00Z"},
			wantKeys: []string{"last_activity"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := parseStateLabels(tt.labels)
			if len(labels) != len(tt.wantKeys) {
				t.Errorf("got %d labels, want %d", len(labels), len(tt.wantKeys))
				return
			}
			for _, key := range tt.wantKeys {
				if _, ok := labels[key]; !ok {
					t.Errorf("missing expected key: %s", key)
				}
			}
		})
	}
}

func TestApplyLabelOperations(t *testing.T) {
	tests := []struct {
		name      string
		initial   map[string]string
		setOps    []string
		incrKey   string
		delKeys   []string
		wantKeys  map[string]string
		wantError bool
	}{
		{
			name:     "set new label",
			initial:  map[string]string{},
			setOps:   []string{"idle=0"},
			wantKeys: map[string]string{"idle": "0"},
		},
		{
			name:     "set overwrites existing",
			initial:  map[string]string{"idle": "5"},
			setOps:   []string{"idle=0"},
			wantKeys: map[string]string{"idle": "0"},
		},
		{
			name:     "increment missing key creates with 1",
			initial:  map[string]string{},
			incrKey:  "idle",
			wantKeys: map[string]string{"idle": "1"},
		},
		{
			name:     "increment existing key",
			initial:  map[string]string{"idle": "3"},
			incrKey:  "idle",
			wantKeys: map[string]string{"idle": "4"},
		},
		{
			name:     "delete existing key",
			initial:  map[string]string{"idle": "3", "backoff": "2m"},
			delKeys:  []string{"idle"},
			wantKeys: map[string]string{"backoff": "2m"},
		},
		{
			name:     "delete non-existent key is noop",
			initial:  map[string]string{"idle": "3"},
			delKeys:  []string{"nonexistent"},
			wantKeys: map[string]string{"idle": "3"},
		},
		{
			name:      "invalid set format",
			initial:   map[string]string{},
			setOps:    []string{"invalid"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := copyMap(tt.initial)
			err := applyLabelOperations(labels, tt.setOps, tt.incrKey, tt.delKeys)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(labels) != len(tt.wantKeys) {
				t.Errorf("got %d labels, want %d", len(labels), len(tt.wantKeys))
				return
			}

			for key, wantVal := range tt.wantKeys {
				if gotVal, ok := labels[key]; !ok {
					t.Errorf("missing expected key: %s", key)
				} else if gotVal != wantVal {
					t.Errorf("labels[%s] = %s, want %s", key, gotVal, wantVal)
				}
			}
		})
	}
}

// parseStateLabels extracts state labels (key:value format) from all labels.
// This is a helper for testing that mirrors the logic in getAgentLabels.
func parseStateLabels(allLabels []string) map[string]string {
	labels := make(map[string]string)
	for _, label := range allLabels {
		if idx := indexOf(label, ":"); idx > 0 {
			labels[label[:idx]] = label[idx+1:]
		}
	}
	return labels
}

// indexOf returns the index of the first occurrence of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// applyLabelOperations applies set, increment, and delete operations to a label map.
// This mirrors the logic in modifyAgentState.
func applyLabelOperations(labels map[string]string, setOps []string, incrKey string, delKeys []string) error {
	// Apply increment
	if incrKey != "" {
		currentValue := 0
		if valStr, ok := labels[incrKey]; ok {
			for i := 0; i < len(valStr); i++ {
				if valStr[i] >= '0' && valStr[i] <= '9' {
					currentValue = currentValue*10 + int(valStr[i]-'0')
				}
			}
		}
		labels[incrKey] = intToString(currentValue + 1)
	}

	// Apply set operations
	for _, setOp := range setOps {
		idx := indexOf(setOp, "=")
		if idx <= 0 {
			return errors.New("invalid set format: " + setOp)
		}
		labels[setOp[:idx]] = setOp[idx+1:]
	}

	// Apply delete operations
	for _, delKey := range delKeys {
		delete(labels, delKey)
	}

	return nil
}

// copyMap creates a shallow copy of a string map.
func copyMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// intToString converts an int to a string without using strconv.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
