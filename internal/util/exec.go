package util

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ExecWithOutput runs a command in the specified directory and returns stdout.
// If the command fails, stderr content is included in the error message.
func ExecWithOutput(workDir, cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...) //nolint:gosec // G204: callers validate args
	c.Dir = workDir

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ExecRun runs a command in the specified directory.
// If the command fails, stderr content is included in the error message.
func ExecRun(workDir, cmd string, args ...string) error {
	c := exec.Command(cmd, args...) //nolint:gosec // G204: callers validate args
	c.Dir = workDir

	var stderr bytes.Buffer
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}
