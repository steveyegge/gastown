// Package fleet provides SSH-based remote polecat dispatch across machines.
package fleet

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SSHResult holds the output from an SSH command execution.
type SSHResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunSSH executes a command on a remote machine via SSH.
// The target is an SSH alias or user@host string.
func RunSSH(target string, command string, timeout time.Duration) (*SSHResult, error) {
	// StrictHostKeyChecking=accept-new accepts unrecognized keys on first connect
	// and remembers them, but rejects changed keys (MITM detection). This is an
	// acceptable tradeoff for fleet machines on a Tailscale network where the
	// transport is already authenticated and encrypted via WireGuard.
	ctx := exec.Command("ssh",
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		target,
		command,
	)

	var stdout, stderr bytes.Buffer
	ctx.Stdout = &stdout
	ctx.Stderr = &stderr

	// Use a channel-based timeout since exec.CommandContext kills with SIGKILL
	done := make(chan error, 1)
	if err := ctx.Start(); err != nil {
		return nil, fmt.Errorf("starting SSH to %s: %w", target, err)
	}
	go func() { done <- ctx.Wait() }()

	select {
	case err := <-done:
		result := &SSHResult{
			Stdout: stdout.String(),
			Stderr: stderr.String(),
		}
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
				return result, fmt.Errorf("SSH command failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Stderr))
			}
			return result, fmt.Errorf("SSH to %s: %w", target, err)
		}
		return result, nil
	case <-time.After(timeout):
		_ = ctx.Process.Kill()
		return nil, fmt.Errorf("SSH to %s timed out after %s", target, timeout)
	}
}

// RunSSHInteractive starts an interactive SSH session (for tmux attach).
func RunSSHInteractive(target string, command string) *exec.Cmd {
	return exec.Command("ssh", "-t", target, command)
}

// Ping checks SSH connectivity to a machine.
func Ping(target string) (time.Duration, error) {
	start := time.Now()
	result, err := RunSSH(target, "echo ok", 15*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, err
	}
	if strings.TrimSpace(result.Stdout) != "ok" {
		return elapsed, fmt.Errorf("unexpected ping response: %q", result.Stdout)
	}
	return elapsed, nil
}
