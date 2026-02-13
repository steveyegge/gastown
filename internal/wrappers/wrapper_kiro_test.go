package wrappers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKiroWrapperScriptExists verifies the gt-kiro script is present in the
// embedded scripts filesystem.
func TestKiroWrapperScriptExists(t *testing.T) {
	t.Parallel()

	content, err := scriptsFS.ReadFile("scripts/gt-kiro")
	if err != nil {
		t.Fatalf("embedded gt-kiro script not found: %v", err)
	}
	if len(content) == 0 {
		t.Error("embedded gt-kiro script is empty")
	}
}

// TestKiroWrapperScriptContent verifies the gt-kiro wrapper script contains
// all the essential components: gastown_enabled check, gt prime call, and exec kiro.
func TestKiroWrapperScriptContent(t *testing.T) {
	t.Parallel()

	data, err := scriptsFS.ReadFile("scripts/gt-kiro")
	if err != nil {
		t.Fatalf("reading gt-kiro script: %v", err)
	}
	content := string(data)

	// Must be a bash script
	if !strings.HasPrefix(content, "#!/bin/bash") {
		t.Error("gt-kiro script should start with #!/bin/bash shebang")
	}

	// Must contain gastown_enabled function
	if !strings.Contains(content, "gastown_enabled") {
		t.Error("gt-kiro script should contain gastown_enabled function")
	}

	// gastown_enabled should check GASTOWN_DISABLED env var
	if !strings.Contains(content, "GASTOWN_DISABLED") {
		t.Error("gt-kiro script should check GASTOWN_DISABLED env var")
	}

	// gastown_enabled should check GASTOWN_ENABLED env var
	if !strings.Contains(content, "GASTOWN_ENABLED") {
		t.Error("gt-kiro script should check GASTOWN_ENABLED env var")
	}

	// Must call gt prime
	if !strings.Contains(content, "gt prime") {
		t.Error("gt-kiro script should call 'gt prime'")
	}

	// Must exec kiro (not codex, not opencode)
	if !strings.Contains(content, "exec kiro") {
		t.Error("gt-kiro script should contain 'exec kiro' to hand off to kiro")
	}

	// Should pass through arguments
	if !strings.Contains(content, `"$@"`) {
		t.Error("gt-kiro script should pass through arguments with \"$@\"")
	}
}

// TestKiroWrapperScriptContent_NotOtherAgents verifies the gt-kiro script does
// not accidentally exec a different agent.
func TestKiroWrapperScriptContent_NotOtherAgents(t *testing.T) {
	t.Parallel()

	data, err := scriptsFS.ReadFile("scripts/gt-kiro")
	if err != nil {
		t.Fatalf("reading gt-kiro script: %v", err)
	}
	content := string(data)

	// Should NOT exec other agents
	if strings.Contains(content, "exec codex") {
		t.Error("gt-kiro script should not contain 'exec codex'")
	}
	if strings.Contains(content, "exec opencode") {
		t.Error("gt-kiro script should not contain 'exec opencode'")
	}
	if strings.Contains(content, "exec claude") {
		t.Error("gt-kiro script should not contain 'exec claude'")
	}
}

// TestKiroWrapperInstallation verifies the wrapper can be installed by writing
// the embedded script to a temporary directory and checking file properties.
func TestKiroWrapperInstallation(t *testing.T) {
	t.Parallel()

	// Read the embedded script
	content, err := scriptsFS.ReadFile("scripts/gt-kiro")
	if err != nil {
		t.Fatalf("reading embedded gt-kiro: %v", err)
	}

	// Write it to a temp directory (simulating Install behavior)
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "gt-kiro")

	if err := os.WriteFile(destPath, content, 0755); err != nil {
		t.Fatalf("writing gt-kiro to temp dir: %v", err)
	}

	// Verify the file exists
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat gt-kiro: %v", err)
	}

	// Verify it is not a directory
	if info.IsDir() {
		t.Error("gt-kiro should be a file, not a directory")
	}

	// Verify it is executable (owner execute bit)
	perm := info.Mode().Perm()
	if perm&0100 == 0 {
		t.Errorf("gt-kiro should be executable, got permissions %o", perm)
	}

	// Verify the content matches what we wrote
	readBack, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading back gt-kiro: %v", err)
	}
	if string(readBack) != string(content) {
		t.Error("installed gt-kiro content does not match embedded content")
	}
}

// TestKiroWrapperInWrapperList verifies that gt-kiro is included in the list
// of wrappers managed by Install/Remove (by checking the embedded FS has it).
func TestKiroWrapperInWrapperList(t *testing.T) {
	t.Parallel()

	entries, err := scriptsFS.ReadDir("scripts")
	if err != nil {
		t.Fatalf("reading scripts directory: %v", err)
	}

	found := false
	for _, entry := range entries {
		if entry.Name() == "gt-kiro" {
			found = true
			break
		}
	}
	if !found {
		t.Error("gt-kiro not found in embedded scripts directory")
	}
}

// TestAllWrapperScriptsHaveConsistentStructure verifies that gt-kiro follows
// the same structural pattern as the other wrapper scripts (gt-codex, gt-opencode).
func TestAllWrapperScriptsHaveConsistentStructure(t *testing.T) {
	t.Parallel()

	scripts := []string{"gt-codex", "gt-opencode", "gt-kiro"}

	for _, name := range scripts {
		t.Run(name, func(t *testing.T) {
			data, err := scriptsFS.ReadFile("scripts/" + name)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			content := string(data)

			// All scripts should have the same structure
			if !strings.HasPrefix(content, "#!/bin/bash") {
				t.Errorf("%s should start with bash shebang", name)
			}
			if !strings.Contains(content, "gastown_enabled") {
				t.Errorf("%s should contain gastown_enabled function", name)
			}
			if !strings.Contains(content, "gt prime") {
				t.Errorf("%s should call gt prime", name)
			}
			if !strings.Contains(content, "set -e") {
				t.Errorf("%s should set -e for error handling", name)
			}
		})
	}
}
