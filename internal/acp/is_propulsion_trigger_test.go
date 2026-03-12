package acp

import (
	"testing"
)

func TestIsPropulsionTrigger(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"Exact match", "AUTONOMOUS WORK MODE", true},
		{"With prefix/suffix", "Some prefix AUTONOMOUS WORK MODE some suffix", true},
		{"Hook trigger", "PROPULSION PRINCIPLE: Work is on your hook. RUN IT.", true},
		{"Step trigger", "EXECUTE THIS STEP NOW.", true},
		{"Case mismatch (now desired to be case-insensitive)", "autonomous work mode", true},
		{"Partial match (incomplete trigger)", "AUTONOMOUS WORK", false},
		{"Split across multiple lines (this will be handled by sliding window)", "AUTONOMOUS\nWORK MODE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPropulsionTrigger(tt.line); got != tt.expected {
				t.Errorf("isPropulsionTrigger(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}
