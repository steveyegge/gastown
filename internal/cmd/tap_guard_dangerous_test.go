package cmd

import (
	"testing"
)

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid hook input",
			input: `{"tool_name":"Bash","tool_input":{"command":"rm -rf /tmp/foo"}}`,
			want:  "rm -rf /tmp/foo",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
		{
			name:  "invalid json",
			input: "not json",
			want:  "",
		},
		{
			name:  "no command field",
			input: `{"tool_name":"Write","tool_input":{"file_path":"/tmp/foo"}}`,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommand([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchesDangerous(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// Should block
		{"rm -rf absolute", "rm -rf /tmp/important", true},
		{"rm -rf root", "rm -rf /", true},
		{"git push force long", "git push --force origin main", true},
		{"git push force short", "git push -f origin main", true},
		{"git reset hard", "git reset --hard HEAD~1", true},
		{"git clean f", "git clean -f", true},
		{"git clean fd", "git clean -fd", true},

		// Should allow
		{"rm single file", "rm foo.txt", false},
		{"rm -r relative", "rm -r ./tmp", false},
		{"git push normal", "git push origin main", false},
		{"git reset soft", "git reset --soft HEAD~1", false},
		{"git status", "git status", false},
		{"ls", "ls -la", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked := false
			for _, pattern := range dangerousPatterns {
				if matchesDangerous(tt.command, pattern) {
					blocked = true
					break
				}
			}
			if blocked != tt.want {
				t.Errorf("matchesDangerous(%q) = %v, want %v", tt.command, blocked, tt.want)
			}
		})
	}
}
