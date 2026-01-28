package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCheckHelpFlag(t *testing.T) {
	// Create a test command
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
		Long:  "This is a test command for testing checkHelpFlag.",
	}

	tests := []struct {
		name        string
		args        []string
		wantHelped  bool
		description string
	}{
		{
			name:        "--help as first arg",
			args:        []string{"--help"},
			wantHelped:  true,
			description: "should show help when --help is first argument",
		},
		{
			name:        "-h as first arg",
			args:        []string{"-h"},
			wantHelped:  true,
			description: "should show help when -h is first argument",
		},
		{
			name:        "--help with other args after",
			args:        []string{"--help", "something"},
			wantHelped:  true,
			description: "should show help when --help is first, ignoring rest",
		},
		{
			name:        "no args",
			args:        []string{},
			wantHelped:  false,
			description: "should not show help with no args",
		},
		{
			name:        "regular args",
			args:        []string{"abc123", "--json"},
			wantHelped:  false,
			description: "should not show help with regular args",
		},
		{
			name:        "--help NOT first - false positive prevention",
			args:        []string{"-m", "--help"},
			wantHelped:  false,
			description: "should NOT show help when --help is not first (e.g., commit -m '--help')",
		},
		{
			name:        "-h NOT first - false positive prevention",
			args:        []string{"something", "-h"},
			wantHelped:  false,
			description: "should NOT show help when -h is not first",
		},
		{
			name:        "--help after -- separator",
			args:        []string{"--", "--help"},
			wantHelped:  false,
			description: "should NOT show help when --help is after -- (passed to underlying tool)",
		},
		{
			name:        "similar but not help flag",
			args:        []string{"--helper"},
			wantHelped:  false,
			description: "should not match --helper as help flag",
		},
		{
			name:        "help without dashes",
			args:        []string{"help"},
			wantHelped:  false,
			description: "should not match 'help' without dashes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helped, err := checkHelpFlag(testCmd, tt.args)
			if err != nil {
				t.Errorf("checkHelpFlag() returned error: %v", err)
			}
			if helped != tt.wantHelped {
				t.Errorf("checkHelpFlag(%v) helped = %v, want %v (%s)",
					tt.args, helped, tt.wantHelped, tt.description)
			}
		})
	}
}

func TestCheckHelpFlag_EdgeCases(t *testing.T) {
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}

	// Test that we correctly handle edge cases that could cause panics or unexpected behavior
	t.Run("nil-like empty slice", func(t *testing.T) {
		var args []string
		helped, err := checkHelpFlag(testCmd, args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if helped {
			t.Error("should not show help for nil/empty args")
		}
	})

	t.Run("single empty string arg", func(t *testing.T) {
		args := []string{""}
		helped, err := checkHelpFlag(testCmd, args)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if helped {
			t.Error("should not show help for empty string arg")
		}
	})
}
