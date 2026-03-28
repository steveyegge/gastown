package cmd

import "testing"

func TestShortHash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef0123456789", "abcdef01"},
		{"abcdef01", "abcdef01"},
		{"abcdef0", "abcdef0"},
		{"abc", "abc"},
		{"", ""},
		{"13bd088", "13bd088"},
	}
	for _, tt := range tests {
		if got := shortHash(tt.input); got != tt.want {
			t.Errorf("shortHash(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
