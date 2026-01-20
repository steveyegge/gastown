//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/testutil"
)

type E2EConfig struct {
	ClaudeModel   string
	OpenCodeModel string
	Timeout       time.Duration
}

func loadConfig() E2EConfig {
	return E2EConfig{
		ClaudeModel:   envOr("E2E_CLAUDE_MODEL", "haiku"),
		OpenCodeModel: envOr("E2E_OPENCODE_MODEL", "github-copilot/gpt-5-mini"),
		Timeout:       envDurationOr("E2E_TIMEOUT", 120*time.Second),
	}
}

func createTestOpencodeConfig(baseConfig string, pluginPath string, modelName string) string {
	baseConfig = strings.TrimSpace(baseConfig)

	lines := strings.Split(baseConfig, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	cleanConfig := strings.Join(cleanLines, "\n")

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(cleanConfig), &config); err != nil {
		config = make(map[string]interface{})
	}

	config["plugin"] = []string{
		"opencode-antigravity-auth@1.3.0",
		"file://" + pluginPath,
	}

	config["mcp"] = make(map[string]interface{})

	if modelName != "" {
		config["model"] = modelName
	}

	if _, ok := config["theme"]; !ok {
		config["theme"] = "opencode"
	}

	result, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"plugin": ["opencode-antigravity-auth@1.3.0", "file://%s"], "model": "%s"}`, pluginPath, modelName)
	}
	return string(result)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDurationOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

type E2ERunner struct {
	t            *testing.T
	config       E2EConfig
	fixture      *testutil.TownFixture
	runtime      string
	rigName      string
	rigDir       string
	beadName     string
	bdPath       string
	gtPath       string
	prompt       string
	spawnedPIDs  []int
	sessionName  string
	tmuxDir      string
	lastLogLines int
}

func NewE2ERunner(t *testing.T, runtime string) *E2ERunner {
	t.Helper()

	config := loadConfig()

	runner := &E2ERunner{
		t:       t,
		config:  config,
		runtime: runtime,
		rigName: "testrig",
	}

	bdPath, err := deps.BeadsPath()
	if err != nil {
		t.Fatalf("Cannot find beads: %v", err)
	}
	runner.bdPath = bdPath
	bdDir := filepath.Dir(bdPath)

	realRuntimePath, err := exec.LookPath(runtime)
	if err != nil {
		t.Skipf("%s not found", runtime)
	}

	binDir := filepath.Join(t.TempDir(), "bin")
	os.MkdirAll(binDir, 0755)

	gtPath := filepath.Join(binDir, "gt")
	runner.gtPath = gtPath
	projectRoot := testutil.FindProjectRoot()
	t.Logf("Compiling gt from %s to %s", projectRoot, gtPath)
	buildCmd := exec.Command("go", "build", "-o", gtPath, "./cmd/gt")
	buildCmd.Dir = projectRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build gt: %v\nOutput: %s", err, string(out))
	}

	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+string(os.PathListSeparator)+bdDir+string(os.PathListSeparator)+originalPath)

	originalTmuxDir := os.Getenv("TMUX_TMPDIR")
	testHash := fmt.Sprintf("%x", md5.Sum([]byte(t.Name())))
	tmuxDir := filepath.Join("/tmp", "gt-e2e-"+testHash[:8])
	os.RemoveAll(tmuxDir)
	os.MkdirAll(tmuxDir, 0700)
	os.Setenv("TMUX_TMPDIR", tmuxDir)

	if runtime == "opencode" {
		wrapperPath := filepath.Join(binDir, "opencode")
		logFile := filepath.Join(os.TempDir(), "gastown_opencode_debug.log")
		opencodeLogFile := filepath.Join(os.TempDir(), "opencode_internal.log")
		pluginLogFile := filepath.Join(os.TempDir(), "gastown_plugin.log")

		script := fmt.Sprintf(`#!/bin/sh
echo "--- $(date) ---" >> "%[1]s"
echo "CWD: $(pwd)" >> "%[1]s"
echo "ARGS: $@" >> "%[1]s"
echo "Running: %[2]s $@" >> "%[1]s"
export TERM=dumb
export CI=true
export OPENCODE_LOG_LEVEL=debug
export OPENCODE_LOG_FILE="%[3]s"
export GASTOWN_PLUGIN_LOG="%[4]s"
echo "OPENCODE_CONFIG=$OPENCODE_CONFIG" >> "%[1]s"
echo "OPENCODE_LOG_FILE=%[3]s" >> "%[1]s"
"%[2]s" "$@" 2>&1 | tee -a "%[1]s"
EXIT_CODE=${PIPESTATUS[0]}
echo "Exit code: $EXIT_CODE" >> "%[1]s"
exit $EXIT_CODE
`, logFile, realRuntimePath, opencodeLogFile, pluginLogFile)

		os.WriteFile(wrapperPath, []byte(script), 0755)

		xdgHome := filepath.Join(t.TempDir(), "xdg")
		opencodeConfigDir := filepath.Join(xdgHome, "opencode")
		os.MkdirAll(opencodeConfigDir, 0755)

		pluginSrc := filepath.Join(projectRoot, "internal", "opencode", "plugin", "gastown.js")
		pluginContent, _ := os.ReadFile(pluginSrc)
		pluginDestDir := filepath.Join(opencodeConfigDir, "plugin")
		os.MkdirAll(pluginDestDir, 0755)
		pluginDest := filepath.Join(pluginDestDir, "gastown.js")
		os.WriteFile(pluginDest, pluginContent, 0644)

		globalConfigPath := filepath.Join(os.Getenv("HOME"), ".config", "opencode", "opencode.jsonc")
		var baseConfig string
		if content, err := os.ReadFile(globalConfigPath); err == nil {
			baseConfig = string(content)
		} else {
			baseConfig = "{}"
		}

		testConfigPath := filepath.Join(opencodeConfigDir, "opencode-test.jsonc")
		testConfig := createTestOpencodeConfig(baseConfig, pluginDest, config.OpenCodeModel)
		os.WriteFile(testConfigPath, []byte(testConfig), 0644)

		os.Setenv("OPENCODE_CONFIG", testConfigPath)
		os.Setenv("XDG_CONFIG_HOME", xdgHome)
		os.Setenv("GT_BINARY_PATH", gtPath)

		xdgDataHome := filepath.Join(xdgHome, ".local", "share")
		os.MkdirAll(xdgDataHome, 0755)
		os.Setenv("XDG_DATA_HOME", xdgDataHome)
		os.Symlink(filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode"), filepath.Join(xdgDataHome, "opencode"))
		os.MkdirAll(filepath.Join(xdgHome, ".local"), 0755)
		os.Symlink(filepath.Join(os.Getenv("HOME"), ".local", "opencode"), filepath.Join(xdgHome, ".local", "opencode"))
	}

	runner.tmuxDir = tmuxDir

	t.Cleanup(func() {
		runner.cleanupTestProcesses()
		if runner.sessionName != "" {
			exec.Command("tmux", "kill-session", "-t", runner.sessionName).Run()
		}
		if runner.tmuxDir != "" {
			killCmd := exec.Command("tmux", "-S", filepath.Join(runner.tmuxDir, "default"), "kill-server")
			killCmd.Run()
			os.RemoveAll(runner.tmuxDir)
		}
		os.Setenv("PATH", originalPath)
		if originalTmuxDir != "" {
			os.Setenv("TMUX_TMPDIR", originalTmuxDir)
		} else {
			os.Unsetenv("TMUX_TMPDIR")
		}
		if runtime == "opencode" && t.Failed() {
			runner.dumpOpencodeLogs()
		}
	})

	runner.fixture = testutil.NewTownFixture(t, runtime)
	return runner
}

func (r *E2ERunner) SetupSourceRepo(fn func(sourceDir string)) {
	sourceRepo := filepath.Join(r.fixture.Root, "source")
	os.MkdirAll(sourceRepo, 0755)
	r.runCmd(sourceRepo, "git", "init", "--initial-branch=main")
	r.runCmd(sourceRepo, "git", "config", "user.email", "test@test.com")
	r.runCmd(sourceRepo, "git", "config", "user.name", "Test")

	if fn != nil {
		fn(sourceRepo)
	}
}

func (r *E2ERunner) writeFileTo(dir, name, content string) {
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}

func (r *E2ERunner) CreateRig() {
	r.t.Helper()
	sourceRepo := filepath.Join(r.fixture.Root, "source")
	if _, err := os.Stat(sourceRepo); os.IsNotExist(err) {
		r.SetupSourceRepo(func(dir string) {
			r.runCmd(dir, "git", "commit", "--allow-empty", "-m", "initial commit")
		})
	}
	r.runCmd(r.fixture.Root, "gt", "rig", "add", r.rigName, sourceRepo, "--prefix", "test")
	r.rigDir = filepath.Join(r.fixture.Root, r.rigName, "refinery", "rig")
}

func (r *E2ERunner) CreateBead(name string, prompt string) {
	r.t.Helper()
	r.beadName = name
	r.prompt = prompt
	r.runCmd(r.rigDir, "gt", "bead", "add", name)
}

func (r *E2ERunner) SlingWork() {
	r.t.Helper()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig != "" {
		promptFile := filepath.Join(xdgConfig, "gastown_prompt.txt")
		prompt := r.prompt
		if prompt == "" {
			prompt = "Do the work assigned in the bead."
		}
		os.WriteFile(promptFile, []byte(prompt), 0644)
	}

	r.sessionName = fmt.Sprintf("gt-%s-rust", r.rigName)

	args := []string{"sling", r.beadName, r.rigName, "--agent", r.runtime}
	r.t.Logf("[SLING] gt %s", strings.Join(args, " "))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.gtPath, args...)
	cmd.Dir = r.fixture.Root
	cmd.Env = os.Environ()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			r.t.Logf("[SLING OUT] %s", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			r.t.Logf("[SLING ERR] %s", scanner.Text())
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			r.t.Logf("[SLING] Command completed with: %v", err)
		}
	case <-ctx.Done():
		r.t.Logf("[SLING] Timed out waiting for sling command, continuing to monitor polecat")
	}
}

func (r *E2ERunner) WaitForCompletion() bool {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	tmuxFailures := 0
	maxTmuxFailures := 3
	r.sessionName = fmt.Sprintf("gt-%s-rust", r.rigName)

	for {
		select {
		case <-ctx.Done():
			r.t.Logf("[WAIT] TIMEOUT after %v", r.config.Timeout)
			r.dumpOpencodeLogs()
			cmd := exec.Command("find", r.fixture.Root, "-name", "math.go")
			out, _ := cmd.CombinedOutput()
			r.t.Logf("[DEBUG] Locations of math.go:\n%s", string(out))
			return false
		case <-ticker.C:
			output, err := r.runCmdOutput(r.fixture.Root, "gt", "polecat", "list", "--all")
			if err != nil {
				r.t.Logf("[WAIT] gt polecat list failed: %v", err)
			}
			if strings.Contains(strings.ToLower(output), "no polecats") || strings.Contains(strings.ToLower(output), "0 active") || len(strings.TrimSpace(output)) == 0 {
				r.t.Logf("[WAIT] No active polecats, considering complete")
				return true
			}

			hasSession := r.checkTmuxSession()
			if !hasSession {
				tmuxFailures++
				r.t.Logf("[WAIT] tmux session %s not found (failure %d/%d)", r.sessionName, tmuxFailures, maxTmuxFailures)
				if tmuxFailures >= maxTmuxFailures {
					r.t.Logf("[WAIT] tmux session gone, assuming process died")
					r.dumpOpencodeLogs()
					return false
				}
				continue
			}
			tmuxFailures = 0

			pluginLogFile := filepath.Join(os.TempDir(), "gastown_plugin.log")
			if logContent, err := os.ReadFile(pluginLogFile); err == nil && len(logContent) > 0 {
				lines := strings.Split(string(logContent), "\n")
				if len(lines) > r.lastLogLines {
					newLines := lines[r.lastLogLines:]
					if len(newLines) > 0 && strings.TrimSpace(strings.Join(newLines, "")) != "" {
						r.t.Logf("[PLUGIN LOG] (+%d lines)\n%s", len(newLines), strings.Join(newLines, "\n"))
					}
					r.lastLogLines = len(lines)
				}

				if strings.Contains(string(logContent), "GASTOWN_TASK_COMPLETE") {
					r.t.Logf("[WAIT] Task completion detected in plugin log")
					exec.Command("tmux", "send-keys", "-t", r.sessionName, "/exit", "Enter").Run()
					time.Sleep(2 * time.Second)
					exec.Command("tmux", "kill-session", "-t", r.sessionName).Run()
					return true
				}
			}
		}
	}
}

func (r *E2ERunner) checkTmuxSession() bool {
	cmd := exec.Command("tmux", "has-session", "-t", r.sessionName)
	cmd.Env = os.Environ()
	return cmd.Run() == nil
}

func (r *E2ERunner) Verify(checks ...func() bool) bool {
	for _, check := range checks {
		if !check() {
			return false
		}
	}
	return true
}

func (r *E2ERunner) FileExists(name string) func() bool {
	return func() bool {
		_, err := os.Stat(filepath.Join(r.rigDir, name))
		return err == nil
	}
}

func (r *E2ERunner) FileContains(name, substr string) func() bool {
	return func() bool {
		content, _ := os.ReadFile(filepath.Join(r.rigDir, name))
		return strings.Contains(string(content), substr)
	}
}

func (r *E2ERunner) BuildsSuccessfully() func() bool {
	return func() bool {
		_, err := r.runCmdOutput(r.rigDir, "go", "build", "-o", "testbinary", ".")
		return err == nil
	}
}

func (r *E2ERunner) TestsPass() func() bool {
	return func() bool {
		_, err := r.runCmdOutput(r.rigDir, "go", "test", "./...")
		return err == nil
	}
}

func (r *E2ERunner) RunOutputContains(binary, expected string) func() bool {
	return func() bool {
		output, err := r.runCmdOutput(r.rigDir, "./"+binary)
		return err == nil && strings.Contains(strings.ToLower(output), strings.ToLower(expected))
	}
}

func (r *E2ERunner) writeFile(name, content string) {
	os.WriteFile(filepath.Join(r.rigDir, name), []byte(content), 0644)
}

func (r *E2ERunner) runCmd(dir, name string, args ...string) {
	r.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Logf("[CMD] %s %v failed in %s: %v\nOutput: %s", name, args, dir, err, string(out))
	} else {
		r.t.Logf("[CMD] %s %v succeeded in %s", name, args, dir)
	}
}

func (r *E2ERunner) runCmdOutput(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *E2ERunner) dumpOpencodeLogs() {
	tmpDir := os.TempDir()
	logFiles := []string{
		filepath.Join(tmpDir, "gastown_opencode_debug.log"),
		filepath.Join(tmpDir, "opencode_internal.log"),
		filepath.Join(tmpDir, "gastown_plugin.log"),
	}
	for _, logFile := range logFiles {
		content, err := os.ReadFile(logFile)
		if err == nil && len(content) > 0 {
			r.t.Logf("[DEBUG] %s:\n%s", filepath.Base(logFile), strings.ReplaceAll(string(content), "\x1b", "^["))
		}
	}
}

func (r *E2ERunner) killSpawnedProcesses() {
	for _, pid := range r.spawnedPIDs {
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Signal(os.Interrupt)
			time.Sleep(100 * time.Millisecond)
			proc.Kill()
			r.t.Logf("[CLEANUP] Killed spawned process PID %d", pid)
		}
	}
	r.spawnedPIDs = nil
}

func (r *E2ERunner) cleanupTestProcesses() {
	r.killSpawnedProcesses()

	if r.sessionName != "" {
		pidsCmd := exec.Command("pgrep", "-f", r.sessionName)
		if out, err := pidsCmd.Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if pid := strings.TrimSpace(line); pid != "" {
					killCmd := exec.Command("kill", "-9", pid)
					killCmd.Run()
					r.t.Logf("[CLEANUP] Killed process matching session %s: PID %s", r.sessionName, pid)
				}
			}
		}
	}

	if r.tmuxDir != "" {
		pidsCmd := exec.Command("pgrep", "-f", r.tmuxDir)
		if out, err := pidsCmd.Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if pid := strings.TrimSpace(line); pid != "" {
					killCmd := exec.Command("kill", "-9", pid)
					killCmd.Run()
					r.t.Logf("[CLEANUP] Killed process using tmuxDir: PID %s", pid)
				}
			}
		}
	}
}

func (r *E2ERunner) trackProcess(pid int) {
	r.spawnedPIDs = append(r.spawnedPIDs, pid)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
