package refinery

import (
	"testing"
)

func TestValidateTestCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{
			name:    "valid command",
			cmd:     "go test ./...",
			wantErr: false,
		},
		{
			name:    "valid command with pipes",
			cmd:     "make test | tee results.log",
			wantErr: false,
		},
		{
			name:    "valid command with env vars",
			cmd:     "CI=true go test -v ./...",
			wantErr: false,
		},
		{
			name:    "empty string",
			cmd:     "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			cmd:     "   ",
			wantErr: true,
		},
		{
			name:    "tab and newline only",
			cmd:     "\t\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTestCommand(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTestCommand(%q) error = %v, wantErr %v", tt.cmd, err, tt.wantErr)
			}
		})
	}
}

func TestRunTests_EmptyCommand(t *testing.T) {
	// Verify that runTests returns a failure when TestCommand is empty,
	// rather than silently succeeding or executing a blank shell command.
	e := &Engineer{
		config: &MergeQueueConfig{
			TestCommand: "",
		},
	}

	result := e.runTests(nil)
	if result.Success {
		t.Error("expected failure for empty test command, got success")
	}
	if result.Error == "" {
		t.Error("expected error message for empty test command")
	}
}

func TestRunTests_WhitespaceCommand(t *testing.T) {
	e := &Engineer{
		config: &MergeQueueConfig{
			TestCommand: "   ",
		},
	}

	result := e.runTests(nil)
	if result.Success {
		t.Error("expected failure for whitespace-only test command, got success")
	}
}
