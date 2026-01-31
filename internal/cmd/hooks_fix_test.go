package cmd

import (
	"testing"
)

func TestNeedsOutputFix(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "claude-flow pre-command hook",
			command:  `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command"`,
			expected: true,
		},
		{
			name:     "claude-flow post-command hook",
			command:  `npx @claude-flow/cli@latest hooks post-command --command "$TOOL_INPUT_command" --success "$TOOL_SUCCESS"`,
			expected: true,
		},
		{
			name:     "claude-flow pre-edit hook",
			command:  `npx @claude-flow/cli@latest hooks pre-edit --file "$TOOL_INPUT_file_path"`,
			expected: true,
		},
		{
			name:     "claude-flow post-task hook",
			command:  `npx @claude-flow/cli@latest hooks post-task --task-id "$TOOL_RESULT_agent_id"`,
			expected: true,
		},
		{
			name:     "already fixed with > /dev/null 2>&1",
			command:  `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command" > /dev/null 2>&1`,
			expected: false,
		},
		{
			name:     "already fixed with 2>&1 > /dev/null",
			command:  `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command" 2>&1 > /dev/null`,
			expected: false,
		},
		{
			name:     "gt command - no fix needed",
			command:  `gt inject drain --quiet`,
			expected: false,
		},
		{
			name:     "gt tap guard - no fix needed",
			command:  `gt tap guard pr-workflow`,
			expected: false,
		},
		{
			name:     "echo command - no fix needed",
			command:  `echo '{"continue": true}'`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NeedsOutputFix(tt.command)
			if result != tt.expected {
				t.Errorf("NeedsOutputFix(%q) = %v, want %v", tt.command, result, tt.expected)
			}
		})
	}
}

func TestFixCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "fixes claude-flow pre-command",
			input:    `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command"`,
			expected: `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command" > /dev/null 2>&1`,
		},
		{
			name:     "does not double-fix already fixed command",
			input:    `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command" > /dev/null 2>&1`,
			expected: `npx @claude-flow/cli@latest hooks pre-command --command "$TOOL_INPUT_command" > /dev/null 2>&1`,
		},
		{
			name:     "leaves non-problematic command unchanged",
			input:    `gt inject drain --quiet`,
			expected: `gt inject drain --quiet`,
		},
		{
			name:     "trims whitespace before appending",
			input:    `npx @claude-flow/cli@latest hooks post-command --command "$TOOL_INPUT_command"  `,
			expected: `npx @claude-flow/cli@latest hooks post-command --command "$TOOL_INPUT_command" > /dev/null 2>&1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixCommand(tt.input)
			if result != tt.expected {
				t.Errorf("FixCommand(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
