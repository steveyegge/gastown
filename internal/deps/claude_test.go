package deps

import "testing"

func TestParseClaudeCodeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2.1.101 (Claude Code)", "2.1.101"},
		{"2.1.101 (Claude Code)\n", "2.1.101"},
		{"2.1.101", "2.1.101"},
		{"2.0.20", "2.0.20"},
		{"1.0.128", "1.0.128"},
		{"10.20.30 (Claude Code)", "10.20.30"},
		{"some other output", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := parseClaudeCodeVersion(tt.input)
		if result != tt.expected {
			t.Errorf("parseClaudeCodeVersion(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCheckClaudeCode(t *testing.T) {
	status, version := CheckClaudeCode()

	if status == ClaudeCodeNotFound {
		t.Skip("claude not installed, skipping integration test")
	}

	if status == ClaudeCodeOK && version == "" {
		t.Error("CheckClaudeCode returned ClaudeCodeOK but empty version")
	}

	t.Logf("CheckClaudeCode: status=%d, version=%s", status, version)
}
