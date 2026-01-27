//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/testutil"
)

// TestUnit_GtBinaryBuild tests that the gt binary can be built and is available
func TestUnit_GtBinaryBuild(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0755)

	gtPath := filepath.Join(binDir, "gt")
	projectRoot := testutil.FindProjectRoot()
	t.Logf("Building gt from %s to %s", projectRoot, gtPath)

	buildCmd := exec.Command("go", "build", "-o", gtPath, "./cmd/gt")
	buildCmd.Dir = projectRoot
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build gt: %v\nOutput: %s", err, string(out))
	}

	// Verify binary exists and is executable
	info, err := os.Stat(gtPath)
	if err != nil {
		t.Fatalf("gt binary not found after build: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("gt binary is not executable: mode=%s", info.Mode())
	}

	// Test that it runs
	versionCmd := exec.Command(gtPath, "version")
	versionOut, err := versionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt version failed: %v\nOutput: %s", err, string(versionOut))
	}
	t.Logf("gt version: %s", strings.TrimSpace(string(versionOut)))

	if !strings.Contains(string(versionOut), "gt version") {
		t.Fatalf("Unexpected version output: %s", versionOut)
	}
}

// TestUnit_SourceRepoSetup tests that source repos can be created with files
func TestUnit_SourceRepoSetup(t *testing.T) {
	sourceRepo := filepath.Join(t.TempDir(), "source")
	os.MkdirAll(sourceRepo, 0755)

	// Initialize git repo
	runTestCmd(t, sourceRepo, "git", "init", "--initial-branch=main")
	runTestCmd(t, sourceRepo, "git", "config", "user.email", "test@test.com")
	runTestCmd(t, sourceRepo, "git", "config", "user.name", "Test")

	// Create test files
	mathGo := filepath.Join(sourceRepo, "math.go")
	os.WriteFile(mathGo, []byte(`package main

func subtract(a, b int) int {
	return a + b
}
`), 0644)

	testGo := filepath.Join(sourceRepo, "math_test.go")
	os.WriteFile(testGo, []byte(`package main

import "testing"

func TestSubtract(t *testing.T) {
	got := subtract(5, 3)
	want := 2
	if got != want {
		t.Errorf("subtract(5, 3) = %d, want %d", got, want)
	}
}
`), 0644)

	// Commit files
	runTestCmd(t, sourceRepo, "git", "add", "math.go", "math_test.go")
	runTestCmd(t, sourceRepo, "git", "commit", "-m", "add buggy code")

	// Verify files exist in repo
	files, err := os.ReadDir(sourceRepo)
	if err != nil {
		t.Fatalf("Cannot read source repo: %v", err)
	}

	foundMath := false
	foundTest := false
	for _, f := range files {
		t.Logf("Found file: %s", f.Name())
		if f.Name() == "math.go" {
			foundMath = true
		}
		if f.Name() == "math_test.go" {
			foundTest = true
		}
	}

	if !foundMath {
		t.Error("math.go not found in source repo")
	}
	if !foundTest {
		t.Error("math_test.go not found in source repo")
	}

	// Verify git log
	logCmd := exec.Command("git", "log", "--oneline", "-1")
	logCmd.Dir = sourceRepo
	logOut, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	t.Logf("Git log: %s", strings.TrimSpace(string(logOut)))

	if !strings.Contains(string(logOut), "add buggy code") {
		t.Fatalf("Commit message not found in git log")
	}
}

// TestUnit_RigCreation tests that a rig can be created from source repo
func TestUnit_RigCreation(t *testing.T) {
	// Build gt first
	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0755)
	gtPath := filepath.Join(binDir, "gt")
	projectRoot := testutil.FindProjectRoot()
	buildCmd := exec.Command("go", "build", "-o", gtPath, "./cmd/gt")
	buildCmd.Dir = projectRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build gt: %v\nOutput: %s", err, string(out))
	}

	// Find beads
	bdPaths := []string{
		os.ExpandEnv("$HOME/go/bin/bd"),
		"/usr/local/bin/bd",
	}
	var bdPath string
	for _, p := range bdPaths {
		if _, err := os.Stat(p); err == nil {
			bdPath = p
			break
		}
	}
	// Also try using which
	if bdPath == "" {
		if out, err := exec.Command("which", "bd").CombinedOutput(); err == nil {
			bdPath = strings.TrimSpace(string(out))
		}
	}
	if bdPath == "" {
		t.Skip("beads (bd) not found")
	}
	t.Logf("Using bd at: %s", bdPath)

	// Set up PATH
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(bdPath)+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

	// Create town workspace
	townDir := filepath.Join(t.TempDir(), "town")
	os.MkdirAll(townDir, 0755)

	// Initialize town
	initCmd := exec.Command(gtPath, "install", townDir, "--git")
	initCmd.Dir = townDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, string(out))
	}

	// Create source repo with files
	sourceRepo := filepath.Join(t.TempDir(), "source")
	os.MkdirAll(sourceRepo, 0755)
	runTestCmd(t, sourceRepo, "git", "init", "--initial-branch=main")
	runTestCmd(t, sourceRepo, "git", "config", "user.email", "test@test.com")
	runTestCmd(t, sourceRepo, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(sourceRepo, "math.go"), []byte("package main\n"), 0644)
	runTestCmd(t, sourceRepo, "git", "add", "math.go")
	runTestCmd(t, sourceRepo, "git", "commit", "-m", "initial")

	// Add rig
	rigCmd := exec.Command(gtPath, "rig", "add", "testrig", sourceRepo, "--prefix", "test")
	rigCmd.Dir = townDir
	if out, err := rigCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt rig add failed: %v\nOutput: %s", err, string(out))
	}

	// Verify rig directory exists
	rigDir := filepath.Join(townDir, "testrig")
	if _, err := os.Stat(rigDir); os.IsNotExist(err) {
		t.Fatalf("Rig directory not created: %s", rigDir)
	}

	// List rig contents
	listCmd := exec.Command("ls", "-la", rigDir)
	listOut, _ := listCmd.CombinedOutput()
	t.Logf("Rig directory contents:\n%s", string(listOut))
}

// TestUnit_BeadCreation tests that beads can be created
func TestUnit_BeadCreation(t *testing.T) {
	// Build gt first
	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0755)
	gtPath := filepath.Join(binDir, "gt")
	projectRoot := testutil.FindProjectRoot()
	buildCmd := exec.Command("go", "build", "-o", gtPath, "./cmd/gt")
	buildCmd.Dir = projectRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build gt: %v\nOutput: %s", err, string(out))
	}

	// Find beads
	var bdPath string
	if out, err := exec.Command("which", "bd").CombinedOutput(); err == nil {
		bdPath = strings.TrimSpace(string(out))
	}
	if bdPath == "" {
		t.Skip("beads (bd) not found")
	}

	// Set up PATH
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(bdPath)+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

	// Create town workspace
	townDir := filepath.Join(t.TempDir(), "town")
	os.MkdirAll(townDir, 0755)

	// Initialize town
	initCmd := exec.Command(gtPath, "install", townDir, "--git")
	initCmd.Dir = townDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, string(out))
	}

	// Create source repo
	sourceRepo := filepath.Join(t.TempDir(), "source")
	os.MkdirAll(sourceRepo, 0755)
	runTestCmd(t, sourceRepo, "git", "init", "--initial-branch=main")
	runTestCmd(t, sourceRepo, "git", "config", "user.email", "test@test.com")
	runTestCmd(t, sourceRepo, "git", "config", "user.name", "Test")
	runTestCmd(t, sourceRepo, "git", "commit", "--allow-empty", "-m", "initial")

	// Add rig
	rigCmd := exec.Command(gtPath, "rig", "add", "testrig", sourceRepo, "--prefix", "test")
	rigCmd.Dir = townDir
	if out, err := rigCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt rig add failed: %v\nOutput: %s", err, string(out))
	}

	// Create bead
	rigDir := filepath.Join(townDir, "testrig", "refinery", "rig")
	beadCmd := exec.Command(gtPath, "bead", "add", "fix-test")
	beadCmd.Dir = rigDir
	if out, err := beadCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt bead add failed: %v\nOutput: %s", err, string(out))
	}

	t.Log("Bead created successfully")

	// Verify bead exists using bd
	listCmd := exec.Command(bdPath, "list")
	listCmd.Dir = rigDir
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Logf("bd list output (may have errors): %s", string(listOut))
	} else {
		t.Logf("Beads list:\n%s", string(listOut))
	}
}

// TestUnit_PluginSyntax tests that the gastown.js plugin has valid syntax
func TestUnit_PluginSyntax(t *testing.T) {
	projectRoot := testutil.FindProjectRoot()
	pluginPath := filepath.Join(projectRoot, "internal", "opencode", "plugin", "gastown.js")

	// Check file exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		t.Fatalf("Plugin file not found: %s", pluginPath)
	}

	// Use node to check syntax
	checkCmd := exec.Command("node", "--check", pluginPath)
	out, err := checkCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Plugin has syntax errors:\n%s", string(out))
	}
	t.Log("Plugin syntax is valid")
}

// TestUnit_OpencodeConfigCreation tests the opencode config creation
func TestUnit_OpencodeConfigCreation(t *testing.T) {
	baseConfig := `{
  "theme": "catppuccin",
  "model": "old-model",
  "mcp": {
    "some-server": {}
  }
}`
	pluginPath := "/path/to/plugin.js"
	modelName := "google/antigravity-gemini-3-flash"

	result := createTestOpencodeConfig(baseConfig, pluginPath, modelName)

	// Check that result contains expected fields
	if !strings.Contains(result, `"opencode-antigravity-auth@1.3.0"`) {
		t.Error("Config should include antigravity-auth plugin")
	}
	if !strings.Contains(result, `"file:///path/to/plugin.js"`) {
		t.Error("Config should include gastown plugin path")
	}
	if !strings.Contains(result, `"google/antigravity-gemini-3-flash"`) {
		t.Error("Config should include model name")
	}
	if strings.Contains(result, `"some-server"`) {
		t.Error("Config should have disabled MCP servers")
	}

	t.Logf("Generated config:\n%s", result)
}

// TestUnit_TmuxIsolation tests tmux session isolation
func TestUnit_TmuxIsolation(t *testing.T) {
	// Use /tmp with short name to avoid Unix socket path length limits (~104 chars)
	// t.TempDir() paths are too long for tmux sockets
	tmuxDir := filepath.Join("/tmp", fmt.Sprintf("gt-tmux-%d", os.Getpid()))
	os.MkdirAll(tmuxDir, 0700)
	t.Cleanup(func() {
		os.RemoveAll(tmuxDir)
	})

	originalTmuxDir := os.Getenv("TMUX_TMPDIR")
	os.Setenv("TMUX_TMPDIR", tmuxDir)
	defer func() {
		if originalTmuxDir != "" {
			os.Setenv("TMUX_TMPDIR", originalTmuxDir)
		} else {
			os.Unsetenv("TMUX_TMPDIR")
		}
	}()

	// Create a test session
	sessionName := "test-isolation-" + t.Name()
	createCmd := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "echo hello && sleep 1")
	createCmd.Env = os.Environ()
	if out, err := createCmd.CombinedOutput(); err != nil {
		t.Skipf("Cannot create tmux session (tmux may not be available): %v\n%s", err, out)
	}

	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	})

	// Verify session exists
	listCmd := exec.Command("tmux", "list-sessions")
	listCmd.Env = os.Environ()
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Cannot list sessions: %v", err)
	}

	if !strings.Contains(string(listOut), sessionName) {
		t.Fatalf("Session not found in list:\n%s", listOut)
	}

	t.Logf("Tmux sessions in isolated env:\n%s", listOut)
}

// TestUnit_PolecatListParsing tests the regex and logic for parsing gt polecat list output
func TestUnit_PolecatListParsing(t *testing.T) {
	runner := &E2ERunner{
		t:         t,
		rigName:   "testrig_5b4d_123",
		ansiRegex: regexp.MustCompile(`[\x1b\x9b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]`),
	}

	tests := []struct {
		name           string
		output         string
		expectedStatus string
		expectedBusy   bool
	}{
		{
			name: "Simple active",
			output: "Active Polecats\n" +
				"  ● testrig_5b4d_123/rust  busy",
			expectedStatus: "busy",
			expectedBusy:   true,
		},
		{
			name: "With ANSI and noise",
			output: "\x1b[32mActive Polecats\x1b[0m\n" +
				"  \x1b[1m●\x1b[0m testrig_5b4d_123/furiosa  \x1b[33mactive\x1b[0m",
			expectedStatus: "active",
			expectedBusy:   true,
		},
		{
			name: "Done state",
			output: "Active Polecats\n" +
				"  ● testrig_5b4d_123/obsidian  done",
			expectedStatus: "done",
			expectedBusy:   false,
		},
		{
			name: "Multiple polecats",
			output: "Active Polecats\n" +
				"  ● other_rig/rust  busy\n" +
				"  ● testrig_5b4d_123/rust  idle",
			expectedStatus: "idle",
			expectedBusy:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanOutput := runner.ansiRegex.ReplaceAllString(tt.output, "")
			polecatStatus := "unknown"
			seenBusy := false

			lines := strings.Split(cleanOutput, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "Active") || (strings.HasPrefix(line, "●") && len(strings.Fields(line)) < 2) {
					continue
				}
				parts := strings.Fields(line)
				name := ""
				status := ""
				if parts[0] == "●" && len(parts) >= 3 {
					name = parts[1]
					status = parts[len(parts)-1]
				} else if len(parts) >= 2 {
					name = strings.TrimPrefix(parts[0], "●")
					status = parts[len(parts)-1]
				}

				if name != "" && strings.Contains(name, runner.rigName) {
					polecatStatus = status
				}
			}

			if polecatStatus == "busy" || polecatStatus == "active" {
				seenBusy = true
			}

			if polecatStatus != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, polecatStatus)
			}
			if seenBusy != tt.expectedBusy {
				t.Errorf("Expected seenBusy %v, got %v", tt.expectedBusy, seenBusy)
			}
		})
	}
}

func runTestCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\nOutput: %s", name, args, err, string(out))
	}
}
