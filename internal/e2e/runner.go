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
	"regexp"
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
		Timeout:       envDurationOr("E2E_TIMEOUT", 240*time.Second),
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

	agentConfig := make(map[string]interface{})
	agentConfig["polecat"] = map[string]interface{}{
		"permissions": map[string]interface{}{
			"gt_done": "allow",
		},
	}
	config["agent"] = agentConfig

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

type sessionInfo struct {
	Role      string
	AgentName string
	RigName   string
	StartTime time.Time
	LastEvent string
	IsTarget  bool
}

type E2ERunner struct {
	t              *testing.T
	config         E2EConfig
	fixture        *testutil.TownFixture
	runtime        string
	rigName        string
	rigDir         string
	beadName       string
	bdPath         string
	gtPath         string
	prompt         string
	spawnedPIDs    []int
	sessionName    string
	tmuxDir        string
	lastLogLines   int
	ansiRegex      *regexp.Regexp
	activePolecats map[string]string
	knownSessions  map[string]sessionInfo
	gtBuildDone    chan struct{}
	runID          string
}

func NewE2ERunner(t *testing.T, runtime string) *E2ERunner {
	t.Helper()

	config := loadConfig()
	testHash := fmt.Sprintf("%x", md5.Sum([]byte(t.Name())))
	runID := fmt.Sprintf("%s_%d", testHash[:4], time.Now().Unix()%10000)

	projectRoot := testutil.FindProjectRoot()
	binDir := filepath.Join(t.TempDir(), "bin")
	gtPath := filepath.Join(binDir, "gt")

	gtBuildDone := make(chan struct{})
	go func() {
		os.MkdirAll(binDir, 0755)
		buildCmd := exec.Command("go", "build", "-o", gtPath, "./cmd/gt")
		buildCmd.Dir = projectRoot
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Errorf("  [STATUS] FAILED: go build -o %s ./cmd/gt\n%s", gtPath, string(out))
		}
		close(gtBuildDone)
	}()

	runner := &E2ERunner{
		t:              t,
		config:         config,
		runtime:        runtime,
		rigName:        fmt.Sprintf("testrig_%s", runID),
		ansiRegex:      regexp.MustCompile(`[\x1b\x9b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]`),
		activePolecats: make(map[string]string),
		knownSessions:  make(map[string]sessionInfo),
		gtBuildDone:    gtBuildDone,
		gtPath:         gtPath,
		runID:          runID,
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

	t.Logf("\n"+
		"============================================================\n"+
		" [COMPONENT: GT BINARY] Background Compilation\n"+
		"============================================================\n"+
		"  PURPOSE: Build core binary in parallel with rig setup.\n"+
		"  SOURCE:  %s\n"+
		"  TARGET:  %s", projectRoot, gtPath)

	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", binDir+string(os.PathListSeparator)+bdDir+string(os.PathListSeparator)+originalPath)

	originalTmuxDir := os.Getenv("TMUX_TMPDIR")
	tmuxDir := filepath.Join("/tmp", "gt-e2e-"+testHash[:8])
	os.RemoveAll(tmuxDir)
	os.MkdirAll(tmuxDir, 0700)
	os.Setenv("TMUX_TMPDIR", tmuxDir)
	runner.tmuxDir = tmuxDir
	runner.sessionName = fmt.Sprintf("gt-%s-rust", runner.rigName)

	t.Logf("\n"+
		"============================================================\n"+
		" [CONSTANTS] Test Environment Details\n"+
		"============================================================\n"+
		"  GT_BINARY:   %s\n"+
		"  TEST_ROOT:   %s\n"+
		"  TARGET_RIG:  %s\n"+
		"  TMUX_TMPDIR: %s", gtPath, projectRoot, runner.rigName, tmuxDir)

	if runtime == "opencode" {
		wrapperPath := filepath.Join(binDir, "opencode")
		logFile := filepath.Join(os.TempDir(), fmt.Sprintf("gastown_opencode_debug_%s.log", testHash[:8]))
		opencodeLogFile := filepath.Join(os.TempDir(), fmt.Sprintf("opencode_internal_%s.log", testHash[:8]))

		os.Remove(logFile)
		os.Remove(opencodeLogFile)

		script := fmt.Sprintf(`#!/bin/sh
echo "--- $(date) ---" >> "%[1]s"
echo "CWD: $(pwd)" >> "%[1]s"
echo "ARGS: $@" >> "%[1]s"
echo "Running: %[2]s $@" >> "%[1]s"
export TMUX_TMPDIR="%[4]s"
echo "TMUX_TMPDIR: $TMUX_TMPDIR" >> "%[1]s"
export TERM=dumb
export CI=true
export GT_ISOLATED_BEADS=1
export GASTOWN_TEST_HASH="%[5]s"
export OPENCODE_LOG_LEVEL=debug
export OPENCODE_LOG_FILE="%[3]s"
export GASTOWN_PLUGIN_LOG="%[6]s/gastown_plugin_${GT_ROLE:-unknown}_%[7]s.log"
export GASTOWN_PLUGIN_LOG_EVENTS="%[6]s/gastown_plugin_events_${GT_ROLE:-unknown}_%[7]s.log"
export OPENCODE_CONFIG="${OPENCODE_CONFIG}"
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME}"
export XDG_DATA_HOME="${XDG_DATA_HOME}"
export GT_BINARY_PATH="${GT_BINARY_PATH}"
echo "OPENCODE_CONFIG=$OPENCODE_CONFIG" >> "%[1]s"
echo "XDG_CONFIG_HOME=$XDG_CONFIG_HOME" >> "%[1]s"
echo "OPENCODE_LOG_FILE=%[3]s" >> "%[1]s"
"%[2]s" "$@" >> "%[1]s" 2>&1
EXIT_CODE=$?
echo "Exit code: $EXIT_CODE" >> "%[1]s"
exit $EXIT_CODE
`, logFile, realRuntimePath, opencodeLogFile, tmuxDir, testHash[:8], os.TempDir(), runner.runID)

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

	t.Cleanup(func() {
		runner.cleanupTestProcesses()
		runner.killTmuxSession()
		if runner.tmuxDir != "" {
			socketPath := filepath.Join(runner.tmuxDir, "default")
			exec.Command("tmux", "-S", socketPath, "kill-server").Run()
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
	r.t.Logf("\n"+
		"============================================================\n"+
		" [STEP: DISPATCH] Assigning task to autonomous worker\n"+
		"============================================================\n"+
		"  COMMAND: gt sling %s %s --agent %s\n"+
		"  PURPOSE: Spawns a dedicated polecat agent and hooks the bead\n"+
		"           assignment for immediate execution.", r.beadName, r.rigName, r.runtime)

	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig != "" {
		promptFile := filepath.Join(xdgConfig, "gastown_prompt.txt")
		prompt := r.prompt
		if prompt == "" {
			prompt = fmt.Sprintf("Hook bead '%s' and execute the work immediately. Use 'gt hook %s' if needed.", r.beadName, r.beadName)
		}
		os.WriteFile(promptFile, []byte(prompt), 0644)
	}

	args := []string{"sling", r.beadName, r.rigName, "--agent", r.runtime}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	<-r.gtBuildDone
	cmd := exec.CommandContext(ctx, r.gtPath, args...)
	cmd.Dir = r.fixture.Root

	env := os.Environ()
	env = append(env, "TMUX_TMPDIR="+r.tmuxDir)
	env = append(env, "GT_ISOLATED_BEADS=1")
	cmd.Env = env

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()

	started := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			r.t.Logf("    [SLING OUT] %s", line)

			if strings.Contains(line, "Allocated polecat:") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					polecat := strings.TrimSpace(parts[1])
					r.sessionName = fmt.Sprintf("gt-%s-%s", r.rigName, polecat)
					r.t.Logf("    [STATUS] Target Session: %s", r.sessionName)
					r.rigDir = filepath.Join(r.fixture.Root, r.rigName, "polecats", polecat, r.rigName)
				}
			}

			if strings.Contains(line, "Starting session") {
				r.t.Logf("    [STATUS] Session starting, awaiting tmux ready...")
				for i := 0; i < 40; i++ {
					if r.checkTmuxSession() {
						r.t.Logf("    [STATUS] Session %s is ready", r.sessionName)
						close(started)
						return
					}
					time.Sleep(250 * time.Millisecond)
				}
				r.t.Logf("    [ERROR] Session %s timed out during startup", r.sessionName)
				close(started)
				return
			}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			r.t.Logf("    [SLING ERR] %s", scanner.Text())
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			r.t.Logf("  [RESULT] Dispatch failed: %v", err)
		} else {
			r.t.Logf("  [RESULT] Dispatch completed")
		}
	case <-started:
		r.t.Logf("  [RESULT] Agent session established")
	case <-ctx.Done():
		r.t.Logf("  [RESULT] Dispatch monitoring active (60s timeout)")
	}
}

func (r *E2ERunner) WaitForCompletion() bool {
	r.t.Helper()
	suffix := r.runID

	r.t.Logf("\n"+
		"============================================================\n"+
		" [PHASE: MONITOR] Tracking Cluster Activity\n"+
		"============================================================\n"+
		"  PURPOSE: Tracks real-time telemetry from agents and system\n"+
		"           lifecycle signals (DONE/IDLE).\n"+
		"\n"+
		"  CONTEXT:\n"+
		"    The log captures shared telemetry from the cluster.\n"+
		"    We identify the TARGET activity for our assigned rig\n"+
		"    and agent while filtering background noise.\n"+
		"\n"+
		"  TARGETS:\n"+
		"    RIG:   %s\n"+
		"    AGENT: %s\n"+
		"    LOGS:  /tmp/gastown_plugin_*_%s.log\n", r.rigName, r.sessionName, suffix)

	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	seenBusy := false
	seenDone := false

	for {
		select {
		case <-ctx.Done():
			r.t.Logf("\n[RESULT] MONITORING TIMEOUT EXCEEDED")
			r.dumpOpencodeLogs()
			return false
		case <-ticker.C:
			// 1. Process agent telemetry from plugin logs
			roles := []string{"polecat", "unknown", "witness", "refinery"}
			var combinedContent strings.Builder
			for _, role := range roles {
				pluginLogFile := filepath.Join(os.TempDir(), fmt.Sprintf("gastown_plugin_%s_%s.log", role, suffix))
				if logContent, err := os.ReadFile(pluginLogFile); err == nil && len(logContent) > 0 {
					contentStr := string(logContent)
					combinedContent.WriteString(contentStr)

					lines := strings.Split(contentStr, "\n")
					if len(lines) > r.lastLogLines {
						newLines := lines[r.lastLogLines:]

						for _, line := range newLines {
							line = strings.TrimSpace(line)
							if line == "" {
								continue
							}

							isTarget := strings.Contains(line, r.rigName) || strings.Contains(line, r.sessionName)

							if isTarget {
								r.t.Logf("    [TARGET ACTIVITY] %s", line)
							} else {
								if strings.Contains(line, "[INFO]") || strings.Contains(line, "GASTOWN") || strings.Contains(line, "[SESSION START]") {
									r.t.Logf("    [CLUSTER NOISE]   %s", line)
								}
							}
						}
						r.lastLogLines = len(lines)
					}
				}
			}

			fullContent := combinedContent.String()
			lowerContent := strings.ToLower(fullContent)
			if strings.Contains(fullContent, "GASTOWN_TASK_COMPLETE") ||
				strings.Contains(fullContent, "Commit Summary") ||
				strings.Contains(lowerContent, "fixed subtract") ||
				strings.Contains(lowerContent, "bug fix complete") ||
				strings.Contains(lowerContent, "tests pass") ||
				(strings.Contains(lowerContent, "fixed") && strings.Contains(lowerContent, "committed")) {
				r.t.Logf("\n  [COMPLETION] Success: task finalized via telemetry signal")
				r.exitTmuxSession()
				return true
			}

			// 2. Check system-level cluster lifecycle
			output, _ := r.runCmdOutput(r.fixture.Root, "gt", "polecat", "list", "--all")
			cleanOutput := r.ansiRegex.ReplaceAllString(output, "")
			lowerOutput := strings.ToLower(cleanOutput)

			polecatStatus := "unknown"
			currentPolecats := make(map[string]string)

			if !strings.Contains(lowerOutput, "no active polecats") && !strings.Contains(lowerOutput, "no polecats found") {
				lines := strings.Split(cleanOutput, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					isBulletLine := strings.HasPrefix(line, "●") || strings.HasPrefix(line, "○")
					if line == "" || strings.HasPrefix(line, "Active") || (isBulletLine && len(strings.Fields(line)) < 2) {
						continue
					}
					parts := strings.Fields(line)
					name := ""
					status := ""
					if (parts[0] == "●" || parts[0] == "○") && len(parts) >= 3 {
						name = parts[1]
						status = parts[len(parts)-1]
					} else if len(parts) >= 2 {
						name = strings.TrimPrefix(strings.TrimPrefix(parts[0], "●"), "○")
						status = parts[len(parts)-1]
					}

					if name != "" {
						currentPolecats[name] = status
						if name == r.rigName || strings.Contains(name, r.rigName) {
							polecatStatus = status
						}
					}
				}
			}

			changed := false
			if len(currentPolecats) != len(r.activePolecats) {
				changed = true
			} else {
				for n, s := range currentPolecats {
					if r.activePolecats[n] != s {
						changed = true
						break
					}
				}
			}

			if changed {
				r.t.Log("\n  [LIFECYCLE] Cluster state updated:")
				for n, s := range currentPolecats {
					category := "[BACKGROUND]"
					if strings.Contains(n, r.rigName) {
						category = "[TARGET WORKER]"
					}
					r.t.Logf("    %-16s %s: %s", category, n, s)
				}
				if len(currentPolecats) == 0 {
					r.t.Log("    [empty cluster]")
				}
				r.activePolecats = currentPolecats
			}

			if polecatStatus == "busy" || polecatStatus == "active" {
				seenBusy = true
			}
			if polecatStatus == "done" {
				seenDone = true
			}

			if seenDone && time.Since(startTime) > 10*time.Second {
				r.t.Logf("\n  [COMPLETION] Success: cluster reports task 'done'")
				r.exitTmuxSession()
				return true
			}

			if seenBusy && (len(currentPolecats) == 0 || polecatStatus == "unknown") && time.Since(startTime) > 30*time.Second {
				r.t.Logf("\n  [COMPLETION] Success: agent process exited naturally")
				return true
			}

			if !r.checkTmuxSession() && time.Since(startTime) > 30*time.Second {
				if seenBusy {
					r.t.Logf("\n  [COMPLETION] Success: agent session closed")
					return true
				}
			}
		}
	}
}

func (r *E2ERunner) checkTmuxSession() bool {
	socketPath := filepath.Join(r.tmuxDir, "default")
	cmd := exec.Command("tmux", "-S", socketPath, "has-session", "-t", r.sessionName)
	if cmd.Run() == nil {
		return true
	}

	pidsCmd := exec.Command("pgrep", "-f", r.sessionName)
	if out, err := pidsCmd.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true
	}

	return false
}

func (r *E2ERunner) Verify(checks ...func() bool) bool {
	for i, check := range checks {
		if !check() {
			r.t.Logf("[VERIFY] Check %d failed", i)
			return false
		}
		r.t.Logf("[VERIFY] Check %d passed", i)
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
		path := filepath.Join(r.rigDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			r.t.Logf("[VERIFY] FileContains: failed to read %s: %v", path, err)
			return false
		}
		result := strings.Contains(string(content), substr)
		if !result {
			r.t.Logf("[VERIFY] FileContains(%s, %q): NOT FOUND. Content:\n%s", name, substr, string(content))
		}
		return result
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
	if name == "gt" {
		<-r.gtBuildDone
		name = r.gtPath
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()

	displayDir := dir
	if r.fixture != nil && strings.HasPrefix(dir, r.fixture.Root) {
		displayDir = "." + strings.TrimPrefix(dir, r.fixture.Root)
	}

	if err != nil {
		r.t.Logf("[CMD] %s %v failed in %s: %v\nOutput: %s", name, args, displayDir, err, string(out))
	} else {
		r.t.Logf("[CMD] %s %v succeeded in %s", name, args, displayDir)
	}
}

func (r *E2ERunner) runCmdOutput(dir, name string, args ...string) (string, error) {
	if name == "gt" {
		<-r.gtBuildDone
		name = r.gtPath
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *E2ERunner) dumpOpencodeLogs() {
	tmpDir := os.TempDir()
	suffix := r.runID

	roles := []string{"polecat", "witness", "refinery", "unknown"}

	var logFiles []string
	logFiles = append(logFiles, filepath.Join(tmpDir, fmt.Sprintf("gastown_opencode_debug_%s.log", suffix)))
	logFiles = append(logFiles, filepath.Join(tmpDir, fmt.Sprintf("opencode_internal_%s.log", suffix)))

	for _, role := range roles {
		logFiles = append(logFiles, filepath.Join(tmpDir, fmt.Sprintf("gastown_plugin_%s_%s.log", role, suffix)))
		logFiles = append(logFiles, filepath.Join(tmpDir, fmt.Sprintf("gastown_plugin_events_%s_%s.log", role, suffix)))
	}

	for _, logFile := range logFiles {
		content, err := os.ReadFile(logFile)
		if err == nil && len(content) > 0 {
			cleanContent := r.ansiRegex.ReplaceAllString(string(content), "")

			graphicalChars := []string{"█", "▄", "▀", "┃", "╹", "┏", "┓", "┗", "┛", "┣", "┫", "┳", "┻", "╋", "─", "│"}
			for _, char := range graphicalChars {
				cleanContent = strings.ReplaceAll(cleanContent, char, "")
			}

			r.t.Logf("[DEBUG] %s:\n%s", filepath.Base(logFile), cleanContent)
		}
	}
}

func (r *E2ERunner) killTmuxSession() {
	if r.sessionName == "" || r.tmuxDir == "" {
		return
	}
	socketPath := filepath.Join(r.tmuxDir, "default")
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", r.sessionName).Run()
}

func (r *E2ERunner) sendTmuxKeys(keys ...string) {
	if r.sessionName == "" || r.tmuxDir == "" {
		return
	}
	socketPath := filepath.Join(r.tmuxDir, "default")
	args := []string{"-S", socketPath, "send-keys", "-t", r.sessionName}
	args = append(args, keys...)
	exec.Command("tmux", args...).Run()
}

func (r *E2ERunner) exitTmuxSession() {
	r.sendTmuxKeys("/exit", "Enter")
	time.Sleep(2 * time.Second)
	r.killTmuxSession()
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
