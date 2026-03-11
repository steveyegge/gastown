package config

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Sandbox integration tests verify that exec-wrapper plugins using
// macOS sandbox-exec actually enforce filesystem and network restrictions.
//
// These tests use sandbox-exec directly (the underlying mechanism that
// exitbox wraps) to validate that the gastown-polecat sandbox profile
// enforces the intended security boundaries:
//
//   - File writes restricted to PROJECT_DIR and /private/tmp
//   - File reads allowed broadly (system libs, homebrew, etc.)
//   - Network restricted to loopback only (for Dolt SQL access)
//   - External network access denied

// testSandboxProfile is a sandbox-exec SBPL profile modeled after the
// gastown-polecat profile described in docs/design/sandboxed-polecat-execution.md.
//
// Key restrictions:
//   - Writes: only PROJECT_DIR + /private/tmp + /dev devices
//   - Network: loopback only (bind, inbound, outbound to localhost)
//   - Reads: broad (needed for system libs, binaries, homebrew)
const testSandboxProfile = `(version 1)
(deny default)

;; Process control
(allow process*)
(allow signal)
(allow sysctl-read)
(allow mach-lookup)

;; File: read broadly, write restricted to project dir + tmp
(allow file-read*)
(allow file-ioctl)
(allow file-write* (subpath (param "PROJECT_DIR")))
(allow file-write* (subpath "/private/tmp"))
(allow file-write* (subpath "/private/var/folders"))
(allow file-write* (literal "/dev/null"))
(allow file-write* (literal "/dev/ptmx"))
(allow file-write* (regex "^/dev/ttys[0-9]+"))
(allow file-write* (regex "^/dev/fd/[0-9]+"))

;; Network: loopback only
(allow network-bind (local ip "localhost:*"))
(allow network-inbound (local ip "localhost:*"))
(allow network-outbound (remote ip "localhost:*"))
(allow network* (remote unix-socket))
`

func skipIfNotMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox-exec tests only run on macOS")
	}
}

func skipIfNoSandboxExec(t *testing.T) {
	t.Helper()
	skipIfNotMacOS(t)
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not found in PATH")
	}
}

// writeSandboxProfile writes the test sandbox profile to a temp file
// and returns its path.
func writeSandboxProfile(t *testing.T, dir string) string {
	t.Helper()
	profilePath := filepath.Join(dir, "test-sandbox.sb")
	if err := os.WriteFile(profilePath, []byte(testSandboxProfile), 0644); err != nil {
		t.Fatalf("write sandbox profile: %v", err)
	}
	return profilePath
}

// runInSandbox executes a shell command inside the sandbox and returns
// stdout, stderr, and any error.
func runInSandbox(profilePath, projectDir, shellCmd string) (stdout, stderr string, err error) {
	cmd := exec.Command("sandbox-exec",
		"-D", "PROJECT_DIR="+projectDir,
		"-f", profilePath,
		"/bin/sh", "-c", shellCmd,
	)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// TestSandbox_WriteInsideProjectDir verifies that writing to files
// within the allowed project directory succeeds.
func TestSandbox_WriteInsideProjectDir(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	testFile := filepath.Join(projectDir, "test-write.txt")
	shellCmd := fmt.Sprintf("echo 'sandbox write test' > %q && cat %q", testFile, testFile)

	stdout, stderr, err := runInSandbox(profilePath, projectDir, shellCmd)
	if err != nil {
		t.Fatalf("write inside project dir should succeed: err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, "sandbox write test") {
		t.Errorf("expected written content in stdout, got: %q", stdout)
	}
}

// TestSandbox_ReadInsideProjectDir verifies that reading files
// within the allowed project directory succeeds.
func TestSandbox_ReadInsideProjectDir(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Pre-create a file to read
	testFile := filepath.Join(projectDir, "readable.txt")
	if err := os.WriteFile(testFile, []byte("hello from project"), 0644); err != nil {
		t.Fatalf("setup: write test file: %v", err)
	}

	stdout, stderr, err := runInSandbox(profilePath, projectDir, fmt.Sprintf("cat %q", testFile))
	if err != nil {
		t.Fatalf("read inside project dir should succeed: err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, "hello from project") {
		t.Errorf("expected file content, got: %q", stdout)
	}
}

// TestSandbox_DenyWriteOutsideProjectDir verifies that writing to
// paths outside the allowed project directory is denied by the sandbox.
func TestSandbox_DenyWriteOutsideProjectDir(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Try to write to home directory (outside project dir and /private/tmp)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}
	forbiddenFile := filepath.Join(homeDir, ".sandbox-test-should-not-exist")

	shellCmd := fmt.Sprintf("echo 'breach' > %q 2>&1; echo exit=$?", forbiddenFile)
	stdout, _, runErr := runInSandbox(profilePath, projectDir, shellCmd)
	_ = runErr

	// Primary assertion: verify the file was NOT created
	if _, statErr := os.Stat(forbiddenFile); statErr == nil {
		os.Remove(forbiddenFile)
		t.Fatal("sandbox allowed write outside project dir — file was created")
	}

	// Secondary assertion: shell should report non-zero exit
	if strings.Contains(stdout, "exit=0") {
		t.Error("expected non-zero exit from write attempt outside project dir")
	}
	t.Logf("write outside project dir correctly denied: %s", strings.TrimSpace(stdout))
}

// TestSandbox_DenyWriteToGTRoot verifies that a sandboxed polecat
// cannot write to the GT town root (~/gt/) — only its own worktree.
func TestSandbox_DenyWriteToGTRoot(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Try to write to home directory (simulates ~/gt/ — outside project dir AND /private/tmp)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}
	forbiddenFile := filepath.Join(homeDir, ".sandbox-gt-root-test")

	shellCmd := fmt.Sprintf("echo '{\"compromised\":true}' > %q 2>&1; echo exit=$?", forbiddenFile)
	stdout, _, _ := runInSandbox(profilePath, projectDir, shellCmd)

	if _, err := os.Stat(forbiddenFile); err == nil {
		os.Remove(forbiddenFile)
		t.Fatal("sandbox allowed write to home dir (simulating GT root)")
	}

	if strings.Contains(stdout, "exit=0") {
		t.Error("expected non-zero exit from write to GT root area")
	}
	t.Logf("write to GT root correctly denied: %s", strings.TrimSpace(stdout))
}

// TestSandbox_DenyExternalNetwork verifies that network connections
// to external hosts are denied (only loopback allowed).
func TestSandbox_DenyExternalNetwork(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Try to connect to an external IP — should be denied
	shellCmd := `curl -s --connect-timeout 3 http://1.1.1.1/ 2>&1; echo "exit=$?"`
	stdout, _, _ := runInSandbox(profilePath, projectDir, shellCmd)

	if strings.Contains(stdout, "exit=0") {
		t.Fatal("sandbox allowed external network connection — expected denial")
	}
	t.Logf("external network correctly denied: %s", strings.TrimSpace(stdout))
}

// TestSandbox_DenyDNSExfiltration verifies that DNS-based data
// exfiltration is blocked (no external network at all).
func TestSandbox_DenyDNSExfiltration(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Try to resolve an external hostname — should fail without network
	shellCmd := `curl -s --connect-timeout 3 http://example.com/ 2>&1; echo "exit=$?"`
	stdout, _, _ := runInSandbox(profilePath, projectDir, shellCmd)

	if strings.Contains(stdout, "exit=0") {
		t.Fatal("sandbox allowed external DNS resolution + connection")
	}
	t.Logf("DNS exfiltration correctly denied: %s", strings.TrimSpace(stdout))
}

// TestSandbox_AllowLoopbackNetwork verifies that loopback network
// connections are allowed (needed for Dolt SQL access on localhost).
func TestSandbox_AllowLoopbackNetwork(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)

	// Start a simple HTTP server on loopback
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start test server: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "sandbox-loopback-ok")
	})
	server := &http.Server{Handler: mux}
	go server.Serve(listener) //nolint:errcheck
	defer server.Close()

	// Connect to loopback from inside the sandbox
	shellCmd := fmt.Sprintf("curl -s http://127.0.0.1:%d/health 2>&1", port)
	stdout, stderr, err := runInSandbox(profilePath, projectDir, shellCmd)
	if err != nil {
		t.Fatalf("loopback connection should succeed: err=%v stderr=%q stdout=%q", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "sandbox-loopback-ok") {
		t.Errorf("expected loopback response, got: %q", stdout)
	}
}

// TestSandbox_ExecWrapperIntegration is the end-to-end test:
// it builds a startup command via BuildStartupCommand with sandbox-exec
// as the exec wrapper, runs it, and verifies the sandbox enforces
// write restrictions while allowing reads and writes inside the project dir.
func TestSandbox_ExecWrapperIntegration(t *testing.T) {
	skipIfNoSandboxExec(t)
	t.Parallel()

	// Set up directories
	projectDir := t.TempDir()
	profileDir := t.TempDir()
	profilePath := writeSandboxProfile(t, profileDir)
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Use home dir as the "outside" target — it's NOT under /private/tmp
	// so the sandbox will deny writes there.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir: %v", err)
	}
	breachFile := filepath.Join(homeDir, ".sandbox-integration-breach")
	// Ensure cleanup even if test fails
	defer os.Remove(breachFile)

	// Create a stub "agent" script that tests sandbox enforcement
	agentScript := filepath.Join(projectDir, "test-agent.sh")
	agentCode := fmt.Sprintf(`#!/bin/sh
# Test 1: Write inside project dir (should succeed)
echo "allowed-write" > %q/sandbox-test-ok.txt
if [ $? -eq 0 ]; then echo "PASS: write-inside-project"; else echo "FAIL: write-inside-project"; fi

# Test 2: Read back what we wrote (should succeed)
content=$(cat %q/sandbox-test-ok.txt 2>&1)
if [ "$content" = "allowed-write" ]; then echo "PASS: read-inside-project"; else echo "FAIL: read-inside-project got=$content"; fi

# Test 3: Write outside project dir (home dir — should be denied)
echo "breach" > %q 2>/dev/null
if [ $? -ne 0 ]; then echo "PASS: write-outside-denied"; else echo "FAIL: write-outside-denied"; fi

# Test 4: External network (should be denied)
curl -s --connect-timeout 2 http://1.1.1.1/ > /dev/null 2>&1
if [ $? -ne 0 ]; then echo "PASS: external-network-denied"; else echo "FAIL: external-network-denied"; fi

echo "DONE"
`, projectDir, projectDir, breachFile)

	if err := os.WriteFile(agentScript, []byte(agentCode), 0755); err != nil {
		t.Fatalf("write agent script: %v", err)
	}

	// Configure rig settings with sandbox-exec as the exec wrapper
	rigSettings := NewRigSettings()
	rigSettings.Runtime = &RuntimeConfig{
		Command: agentScript,
		ExecWrapper: []string{
			"sandbox-exec",
			"-D", "PROJECT_DIR=" + projectDir,
			"-f", profilePath,
		},
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// Build the startup command (this is what session_manager would do)
	startupCmd := BuildStartupCommand(
		map[string]string{"GT_ROLE": "testrig/polecats/test"},
		rigPath, "",
	)

	// Verify the command structure
	if !strings.Contains(startupCmd, "sandbox-exec") {
		t.Fatalf("expected sandbox-exec in startup command, got: %q", startupCmd)
	}
	if !strings.Contains(startupCmd, "-f "+profilePath) {
		t.Fatalf("expected profile path in command, got: %q", startupCmd)
	}

	// Strip "exec " prefix since we're not replacing the process
	runCmd := strings.TrimPrefix(startupCmd, "exec ")

	// Run the startup command via shell
	out, err := exec.Command("/bin/sh", "-c", runCmd).CombinedOutput()
	output := string(out)
	t.Logf("Startup command output:\n%s", output)
	if err != nil {
		t.Logf("Command error (may be expected for denied operations): %v", err)
	}

	// Verify all sandbox enforcement results
	if !strings.Contains(output, "PASS: write-inside-project") {
		t.Error("sandbox should allow writes inside project dir")
	}
	if !strings.Contains(output, "PASS: read-inside-project") {
		t.Error("sandbox should allow reads inside project dir")
	}
	if !strings.Contains(output, "PASS: write-outside-denied") {
		t.Error("sandbox should deny writes outside project dir")
	}
	if !strings.Contains(output, "PASS: external-network-denied") {
		t.Error("sandbox should deny external network access")
	}
	if !strings.Contains(output, "DONE") {
		t.Error("agent script did not complete — sandbox may have killed it")
	}

	// Double-check: the breach file should not exist
	if _, err := os.Stat(breachFile); err == nil {
		t.Fatal("sandbox breach: file was created outside project dir")
	}
}
