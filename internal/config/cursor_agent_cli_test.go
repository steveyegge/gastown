package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// resolveCursorAgentForCLIPTest returns a real cursor-agent binary path. test_main_test.go prepends
// tiny PATH stubs that shadow the real CLI; we skip those dirs and prefer GT_CURSOR_AGENT_BIN when set.
func resolveCursorAgentForCLIPTest(t *testing.T) (string, []byte) {
	t.Helper()
	if p := os.Getenv("GT_CURSOR_AGENT_BIN"); p != "" {
		out, err := exec.Command(p, "--help").CombinedOutput()
		if err != nil {
			t.Fatalf("GT_CURSOR_AGENT_BIN --help: %v\n%s", err, out)
		}
		return p, out
	}
	stubDir := os.Getenv("GT_AGENT_STUB_BIN_DIR")
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if stubDir != "" && dir == stubDir {
			continue
		}
		p := filepath.Join(dir, "cursor-agent")
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		out, err := exec.Command(p, "--help").CombinedOutput()
		if err != nil {
			continue
		}
		if len(out) >= 200 && strings.Contains(string(out), "Usage:") {
			return p, out
		}
	}
	return "", nil
}

// TestCursorAgentCLIPresetMatchesHelp verifies the built-in AgentCursor preset stays aligned with
// the Cursor Agent CLI when `cursor-agent` is installed (curl https://cursor.com/install -fsSL | bash).
func TestCursorAgentCLIPresetMatchesHelp(t *testing.T) {
	path, out := resolveCursorAgentForCLIPTest(t)
	if path == "" {
		t.Skip("real cursor-agent not found outside test stubs; install Cursor CLI or set GT_CURSOR_AGENT_BIN")
	}

	t.Logf("cursor-agent CLI contract using %s", path)
	help := strings.ToLower(string(out))

	info := GetAgentPreset(AgentCursor)
	if info == nil {
		t.Fatal("cursor preset not found")
	}

	for _, needle := range []string{"--resume", "-f", "--force", "--print", "--output-format"} {
		if !strings.Contains(help, strings.ToLower(needle)) {
			t.Errorf("cursor-agent --help missing %q (preset may be stale vs CLI)", needle)
		}
	}
	// Require json as an output-format option, not merely the substring "json" elsewhere.
	if !strings.Contains(help, "output-format") || !(strings.Contains(help, "json") && strings.Contains(help, "stream-json")) {
		t.Errorf("cursor-agent --help should document --output-format with json and stream-json")
	}

	if info.ResumeFlag != "--resume" {
		t.Errorf("preset ResumeFlag = %q, want --resume", info.ResumeFlag)
	}
	if info.NonInteractive == nil {
		t.Fatal("preset NonInteractive is nil")
	}
	if info.NonInteractive.PromptFlag != "-p" || !strings.Contains(help, "--print") {
		t.Errorf("preset PromptFlag %q should match CLI -p/--print for headless use", info.NonInteractive.PromptFlag)
	}
	of := strings.Fields(info.NonInteractive.OutputFlag)
	if len(of) < 2 || of[0] != "--output-format" {
		t.Fatalf("preset OutputFlag = %q, want '--output-format …'", info.NonInteractive.OutputFlag)
	}
	if !strings.Contains(help, strings.TrimPrefix(of[0], "-")) {
		t.Errorf("cursor-agent --help should document %s", of[0])
	}

	if info.HooksDir != ".cursor" || info.HooksSettingsFile != "hooks.json" {
		t.Errorf("hooks path parts = %q + %q, want .cursor + hooks.json", info.HooksDir, info.HooksSettingsFile)
	}
}
