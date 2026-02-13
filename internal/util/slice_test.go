package util

import (
	"testing"
)

func TestRemoveFromSlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected []string
	}{
		{
			name:     "remove single occurrence",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove multiple occurrences",
			slice:    []string{"a", "b", "b", "c"},
			item:     "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove first element",
			slice:    []string{"a", "b", "c"},
			item:     "a",
			expected: []string{"b", "c"},
		},
		{
			name:     "remove last element",
			slice:    []string{"a", "b", "c"},
			item:     "c",
			expected: []string{"a", "b"},
		},
		{
			name:     "item not found",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: []string{},
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "a",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveFromSlice(tt.slice, tt.item)
			if len(result) != len(tt.expected) {
				t.Errorf("RemoveFromSlice() = %v, want %v", result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("RemoveFromSlice() = %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "contains item",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "does not contain item",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "a",
			expected: false,
		},
		{
			name:     "first element",
			slice:    []string{"a", "b", "c"},
			item:     "a",
			expected: true,
		},
		{
			name:     "last element",
			slice:    []string{"a", "b", "c"},
			item:     "c",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsString(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("ContainsString() = %v, want %v", result, tt.expected)
			}
		})
	}
}
