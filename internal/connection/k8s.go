package connection

import (
	"bytes"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// K8sConnection implements Connection for K8s pods via kubectl exec.
//
// File operations and command execution go through kubectl exec to the pod.
// Tmux operations go through the LOCAL tmux session that the terminal server
// maintains — that session's pane is piped to the pod's screen session via
// kubectl exec, so local tmux send-keys/capture-pane transparently bridges
// to the pod.
type K8sConnection struct {
	// PodName is the K8s pod name (e.g., "gt-gastown-polecat-alpha").
	PodName string

	// Namespace is the K8s namespace (e.g., "gastown-test").
	Namespace string

	// Container is the container name within the pod. Empty means default container.
	Container string

	// KubeConfig is the path to kubeconfig. Empty means default.
	KubeConfig string

	// tmux handles local tmux operations (terminal server manages the sessions).
	tmux *tmux.Tmux

	// execTimeout is the timeout for kubectl exec commands.
	execTimeout time.Duration
}

// K8sConnectionConfig holds configuration for creating a K8sConnection.
type K8sConnectionConfig struct {
	PodName    string
	Namespace  string
	Container  string
	KubeConfig string
}

// DefaultExecTimeout is the default timeout for kubectl exec commands.
const DefaultExecTimeout = 30 * time.Second

// NewK8sConnection creates a Connection to a K8s pod.
func NewK8sConnection(cfg K8sConnectionConfig) *K8sConnection {
	return &K8sConnection{
		PodName:     cfg.PodName,
		Namespace:   cfg.Namespace,
		Container:   cfg.Container,
		KubeConfig:  cfg.KubeConfig,
		tmux:        tmux.NewTmux(),
		execTimeout: DefaultExecTimeout,
	}
}

// Name returns the pod name as the connection identifier.
func (c *K8sConnection) Name() string {
	return c.PodName
}

// IsLocal returns false — K8s connections are remote.
func (c *K8sConnection) IsLocal() bool {
	return false
}

// kubectlExec runs a command inside the pod via kubectl exec.
func (c *K8sConnection) kubectlExec(cmd string, args ...string) ([]byte, error) {
	kubectlArgs := c.kubectlBaseArgs()
	kubectlArgs = append(kubectlArgs, "--")
	kubectlArgs = append(kubectlArgs, cmd)
	kubectlArgs = append(kubectlArgs, args...)

	command := exec.Command("kubectl", kubectlArgs...) //nolint:gosec
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- command.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "No such file") {
				return nil, &NotFoundError{Path: strings.Join(args, " ")}
			}
			if strings.Contains(errMsg, "Permission denied") {
				return nil, &PermissionError{Path: strings.Join(args, " "), Op: cmd}
			}
			return nil, &ConnectionError{
				Op:      "kubectl exec",
				Machine: c.PodName,
				Err:     fmt.Errorf("%s: %w", errMsg, err),
			}
		}
		return stdout.Bytes(), nil
	case <-time.After(c.execTimeout):
		_ = command.Process.Kill()
		return nil, &ConnectionError{
			Op:      "kubectl exec",
			Machine: c.PodName,
			Err:     fmt.Errorf("timed out after %v", c.execTimeout),
		}
	}
}

// kubectlExecStdin runs a command with stdin data piped to the pod.
func (c *K8sConnection) kubectlExecStdin(stdin []byte, cmd string, args ...string) ([]byte, error) {
	kubectlArgs := c.kubectlBaseArgs()
	kubectlArgs = append(kubectlArgs, "-i") // stdin
	kubectlArgs = append(kubectlArgs, "--")
	kubectlArgs = append(kubectlArgs, cmd)
	kubectlArgs = append(kubectlArgs, args...)

	command := exec.Command("kubectl", kubectlArgs...) //nolint:gosec
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- command.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			return nil, &ConnectionError{
				Op:      "kubectl exec",
				Machine: c.PodName,
				Err:     fmt.Errorf("%s: %w", errMsg, err),
			}
		}
		return stdout.Bytes(), nil
	case <-time.After(c.execTimeout):
		_ = command.Process.Kill()
		return nil, &ConnectionError{
			Op:      "kubectl exec",
			Machine: c.PodName,
			Err:     fmt.Errorf("timed out after %v", c.execTimeout),
		}
	}
}

// kubectlBaseArgs builds the kubectl exec base arguments.
func (c *K8sConnection) kubectlBaseArgs() []string {
	args := []string{"exec", c.PodName}
	if c.Namespace != "" {
		args = append(args, "-n", c.Namespace)
	}
	if c.Container != "" {
		args = append(args, "-c", c.Container)
	}
	if c.KubeConfig != "" {
		args = []string{"--kubeconfig", c.KubeConfig, "exec", c.PodName}
		if c.Namespace != "" {
			args = append(args, "-n", c.Namespace)
		}
		if c.Container != "" {
			args = append(args, "-c", c.Container)
		}
	}
	return args
}

// ReadFile reads a file from the pod via kubectl exec.
func (c *K8sConnection) ReadFile(path string) ([]byte, error) {
	return c.kubectlExec("cat", path)
}

// WriteFile writes data to a file in the pod via kubectl exec.
func (c *K8sConnection) WriteFile(path string, data []byte, perm fs.FileMode) error {
	// Use tee to write stdin to the file
	_, err := c.kubectlExecStdin(data, "tee", path)
	if err != nil {
		return err
	}
	// Set permissions
	_, err = c.kubectlExec("chmod", fmt.Sprintf("%o", perm), path)
	return err
}

// MkdirAll creates directories in the pod via kubectl exec.
func (c *K8sConnection) MkdirAll(path string, perm fs.FileMode) error {
	_, err := c.kubectlExec("mkdir", "-p", path)
	if err != nil {
		return err
	}
	_, err = c.kubectlExec("chmod", fmt.Sprintf("%o", perm), path)
	return err
}

// Remove removes a file or empty directory in the pod via kubectl exec.
func (c *K8sConnection) Remove(path string) error {
	_, err := c.kubectlExec("rm", path)
	return err
}

// RemoveAll removes a file or directory tree in the pod via kubectl exec.
func (c *K8sConnection) RemoveAll(path string) error {
	_, err := c.kubectlExec("rm", "-rf", path)
	return err
}

// Stat returns file info for a path in the pod via kubectl exec.
func (c *K8sConnection) Stat(path string) (FileInfo, error) {
	// Use stat with a parseable format
	out, err := c.kubectlExec("stat", "-c", "%n|%s|%a|%Y|%F", path)
	if err != nil {
		return nil, err
	}
	return parseStatOutput(string(out))
}

// parseStatOutput parses the output of stat -c "%n|%s|%a|%Y|%F".
func parseStatOutput(output string) (FileInfo, error) {
	output = strings.TrimSpace(output)
	parts := strings.SplitN(output, "|", 5)
	if len(parts) < 5 {
		return nil, fmt.Errorf("unexpected stat output: %s", output)
	}

	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing size: %w", err)
	}

	modeVal, err := strconv.ParseUint(parts[2], 8, 32)
	if err != nil {
		return nil, fmt.Errorf("parsing mode: %w", err)
	}

	epoch, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing mtime: %w", err)
	}

	isDir := strings.Contains(parts[4], "directory")

	return BasicFileInfo{
		FileName:    parts[0],
		FileSize:    size,
		FileMode:    fs.FileMode(modeVal),
		FileModTime: time.Unix(epoch, 0),
		FileIsDir:   isDir,
	}, nil
}

// Glob returns files matching the pattern in the pod via kubectl exec.
func (c *K8sConnection) Glob(pattern string) ([]string, error) {
	// Use sh -c with ls to expand the glob. Pattern must be passed unquoted
	// for glob expansion to work; this is safe because ls -d only reads metadata.
	out, err := c.kubectlExec("sh", "-c", fmt.Sprintf("ls -d %s 2>/dev/null || true", pattern))
	if err != nil {
		return nil, err
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

// shellQuote wraps a string in single quotes for safe shell expansion.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Exists checks if a path exists in the pod via kubectl exec.
func (c *K8sConnection) Exists(path string) (bool, error) {
	_, err := c.kubectlExec("test", "-e", path)
	if err != nil {
		// test -e exits non-zero if path doesn't exist
		var connErr *ConnectionError
		if ok := isConnectionError(err, &connErr); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// isConnectionError checks if an error is a ConnectionError (non-network failure from test/stat).
func isConnectionError(err error, target **ConnectionError) bool {
	if ce, ok := err.(*ConnectionError); ok {
		*target = ce
		return true
	}
	return false
}

// Exec runs a command in the pod via kubectl exec.
func (c *K8sConnection) Exec(cmd string, args ...string) ([]byte, error) {
	return c.kubectlExec(cmd, args...)
}

// ExecDir runs a command in a specific directory in the pod.
func (c *K8sConnection) ExecDir(dir, cmd string, args ...string) ([]byte, error) {
	// Wrap in sh -c with cd, quoting arguments to prevent injection
	shellCmd := fmt.Sprintf("cd %s && %s", shellQuote(dir), shellQuote(cmd))
	for _, a := range args {
		shellCmd += " " + shellQuote(a)
	}
	return c.kubectlExec("sh", "-c", shellCmd)
}

// ExecEnv runs a command with environment variables in the pod.
func (c *K8sConnection) ExecEnv(env map[string]string, cmd string, args ...string) ([]byte, error) {
	// Use env command to set variables
	envArgs := []string{}
	for k, v := range env {
		envArgs = append(envArgs, k+"="+v)
	}
	envArgs = append(envArgs, cmd)
	envArgs = append(envArgs, args...)
	return c.kubectlExec("env", envArgs...)
}

// Tmux operations route through LOCAL tmux sessions managed by the terminal server.
// The terminal server creates tmux sessions that pipe to the pod's screen session
// via kubectl exec. So TmuxSendKeys goes to a local tmux session, which flows
// through kubectl exec into the pod's screen.

// TmuxNewSession creates a new local tmux session.
func (c *K8sConnection) TmuxNewSession(name, dir string) error {
	return c.tmux.NewSession(name, dir)
}

// TmuxKillSession kills a local tmux session.
func (c *K8sConnection) TmuxKillSession(name string) error {
	return c.tmux.KillSessionWithProcesses(name)
}

// TmuxSendKeys sends keys to the local tmux session (which pipes to pod screen).
func (c *K8sConnection) TmuxSendKeys(session, keys string) error {
	return c.tmux.SendKeys(session, keys)
}

// TmuxCapturePane captures output from the local tmux session.
func (c *K8sConnection) TmuxCapturePane(session string, lines int) (string, error) {
	return c.tmux.CapturePane(session, lines)
}

// TmuxHasSession checks if a local tmux session exists.
func (c *K8sConnection) TmuxHasSession(name string) (bool, error) {
	return c.tmux.HasSession(name)
}

// TmuxListSessions lists all local tmux sessions.
func (c *K8sConnection) TmuxListSessions() ([]string, error) {
	return c.tmux.ListSessions()
}

// Verify K8sConnection implements Connection.
var _ Connection = (*K8sConnection)(nil)
