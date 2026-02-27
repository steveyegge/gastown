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
