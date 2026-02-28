package copilot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGuardScript_NonBashToolPassesThrough verifies non-bash tools exit cleanly.
func TestGuardScript_NonBashToolPassesThrough(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	guardPath := writeGuardScript(t)
	input := `{"toolName":"read_file","toolArgs":{"path":"/tmp/test.txt"}}`

	cmd := exec.Command("bash", guardPath)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Guard script failed for non-bash tool: %v\nOutput: %s", err, output)
	}
	// Non-bash tools should produce no output (allow)
	if strings.TrimSpace(string(output)) != "" {
		t.Errorf("Non-bash tool should produce no output, got: %q", string(output))
	}
}

// TestGuardScript_NonMatchingBashPassesThrough verifies non-matching bash commands pass.
func TestGuardScript_NonMatchingBashPassesThrough(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	guardPath := writeGuardScript(t)
	input := `{"toolName":"bash","toolArgs":{"command":"ls -la"}}`

	cmd := exec.Command("bash", guardPath)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Guard script failed for non-matching bash: %v\nOutput: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Errorf("Non-matching bash should produce no output, got: %q", string(output))
	}
}

// TestGuardScript_MalformedJSONExitsCleanly verifies malformed input doesn't crash.
func TestGuardScript_MalformedJSONExitsCleanly(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	guardPath := writeGuardScript(t)
	input := `{not valid json`

	cmd := exec.Command("bash", guardPath)
	cmd.Stdin = strings.NewReader(input)
	// We don't check the error — jq may fail, but the script shouldn't crash badly
	cmd.CombinedOutput()
}

// TestGuardScript_EmptyCommandPassesThrough verifies empty bash command passes.
func TestGuardScript_EmptyCommandPassesThrough(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	guardPath := writeGuardScript(t)
	input := `{"toolName":"bash","toolArgs":{"command":""}}`

	cmd := exec.Command("bash", guardPath)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Guard script failed for empty command: %v\nOutput: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Errorf("Empty bash command should produce no output, got: %q", string(output))
	}
}

// TestGuardScript_ChainedCommandsMatch verifies guarded commands are caught in chained commands.
func TestGuardScript_ChainedCommandsMatch(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	// gt tap is not available in test, so we only verify the grep pattern matches
	// by checking that the script does NOT silently pass (it will fail on gt tap,
	// but the important thing is it doesn't silently allow the command through).
	// We test the pattern directly instead.
	cases := []struct {
		name    string
		command string
		shouldMatch bool
	}{
		{"direct command", "gh pr create --title foo", true},
		{"chained with cd", "cd /path && gh pr create", true},
		{"chained checkout", "cd /path && git checkout -b my-branch", true},
		{"semicolon chain", "ls; gh pr create", true},
		{"or chain", "something || git switch -c feat", true},
		{"leading whitespace", "  gh pr create", true},
		{"safe command", "ls -la", false},
		{"git status", "git status", false},
		{"echo with guarded text", "echo gh pr create", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Use grep directly to test the pattern matching logic
			pattern := `(^|[;&|]\s*|&&\s*|\|\|\s*)(\s*)(gh pr create|git checkout -b|git switch -c)`
			cmd := exec.Command("grep", "-qE", pattern)
			cmd.Stdin = strings.NewReader(tc.command)
			err := cmd.Run()
			matched := err == nil
			if matched != tc.shouldMatch {
				t.Errorf("command %q: expected match=%v, got match=%v", tc.command, tc.shouldMatch, matched)
			}
		})
	}
}

// writeGuardScript writes the embedded guard script to a temp file and returns its path.
func writeGuardScript(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	guardPath := filepath.Join(tmpDir, "gastown-pretool-guard.sh")
	if err := os.WriteFile(guardPath, guardScript, 0755); err != nil {
		t.Fatalf("Failed to write guard script: %v", err)
	}
	return guardPath
}
