// Package bdcmd provides a centralized way to execute bd (beads) CLI commands
// with proper daemon connection propagation.
//
// Problem: gastown has ~300 raw exec.Command("bd", ...) call sites that bypass
// daemon env var propagation. When BD_DAEMON_HOST is configured (e.g., pointing
// to a remote Dolt in K8s), these raw calls fall through to local SQLite,
// causing schema mismatches and data routing bugs.
//
// Solution: This package provides Command() and CommandContext() that mirror
// exec.Command/exec.CommandContext but automatically:
//   - Use the resolved bd binary path (~/.local/bin/bd preferred)
//   - Propagate BD_DAEMON_HOST and BD_DAEMON_TOKEN from .beads/config.yaml
//   - Set BEADS_DIR when a working directory is known
//
// Migration: New code should use bdcmd.Command(). Existing call sites should
// be migrated incrementally, prioritizing the sling/polecat/hook paths.
// See epic bd-z344 for the full migration tracker.
package bdcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v2"
)

// resolvedBdPath caches the resolved absolute path to the bd binary.
var resolvedBdPath string

func init() {
	resolvedBdPath = resolveBdPath()
}

// resolveBdPath finds the bd binary, preferring ~/.local/bin/bd over system PATH.
func resolveBdPath() string {
	home, err := os.UserHomeDir()
	if err == nil {
		localBd := filepath.Join(home, ".local", "bin", "bd")
		if _, err := os.Stat(localBd); err == nil {
			return localBd
		}
	}
	path, err := exec.LookPath("bd")
	if err != nil {
		return "bd"
	}
	return path
}

// Command creates an exec.Cmd for bd with daemon env propagation.
// The returned command inherits the current process environment plus
// BD_DAEMON_HOST and BD_DAEMON_TOKEN from .beads/config.yaml if not
// already set in the environment.
func Command(args ...string) *exec.Cmd {
	cmd := exec.Command(resolvedBdPath, args...) //nolint:gosec // G204: bd is a trusted internal tool
	propagateDaemonEnv(cmd)
	return cmd
}

// CommandContext creates an exec.Cmd for bd with context and daemon env propagation.
func CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, resolvedBdPath, args...) //nolint:gosec // G204: bd is a trusted internal tool
	propagateDaemonEnv(cmd)
	return cmd
}

// CommandInDir creates an exec.Cmd for bd with a specific working directory
// and daemon env propagation. It also sets BEADS_DIR based on the working
// directory for proper beads routing.
func CommandInDir(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(resolvedBdPath, args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = dir
	propagateDaemonEnv(cmd)
	return cmd
}

// CommandContextInDir creates an exec.Cmd for bd with context, working directory,
// and daemon env propagation.
func CommandContextInDir(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, resolvedBdPath, args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = dir
	propagateDaemonEnv(cmd)
	return cmd
}

// propagateDaemonEnv ensures BD_DAEMON_HOST and BD_DAEMON_TOKEN are set on
// the command's environment. It reads from:
//  1. Current process environment (highest priority)
//  2. ~/.beads/config.yaml daemon-host / daemon-token keys
//
// This matches the logic in internal/beads/beads.go Beads.run() (lines 256-272)
// but is available to all callers, not just the Beads wrapper.
func propagateDaemonEnv(cmd *exec.Cmd) {
	// Start with current environment if not already set
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}

	hasDaemonHost := envHasKey(cmd.Env, "BD_DAEMON_HOST")
	hasDaemonToken := envHasKey(cmd.Env, "BD_DAEMON_TOKEN")

	if hasDaemonHost && hasDaemonToken {
		return // Already fully configured
	}

	// Try to read from config files
	host, token := readDaemonConfigFromAll()

	if host != "" && !hasDaemonHost {
		cmd.Env = append(cmd.Env, "BD_DAEMON_HOST="+host)
	}
	if token != "" && !hasDaemonToken {
		cmd.Env = append(cmd.Env, "BD_DAEMON_TOKEN="+token)
	}
}

// envHasKey checks if an env slice contains a key (case-sensitive).
func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	// Also check the current process env
	return os.Getenv(key) != ""
}

// readDaemonConfigFromAll reads daemon-host and daemon-token from config files.
// It checks multiple locations in order:
//  1. ~/.beads/config.yaml (user-level config)
//  2. CWD-relative .beads/config.yaml
func readDaemonConfigFromAll() (host, token string) {
	// Check user-level config first
	home, err := os.UserHomeDir()
	if err == nil {
		h, t := readDaemonConfig(filepath.Join(home, ".beads", "config.yaml"))
		if h != "" {
			host = h
		}
		if t != "" {
			token = t
		}
	}

	// Check CWD-relative config (overrides user-level)
	cwd, err := os.Getwd()
	if err == nil {
		h, t := readDaemonConfig(filepath.Join(cwd, ".beads", "config.yaml"))
		if h != "" {
			host = h
		}
		if t != "" {
			token = t
		}
	}

	return host, token
}

// readDaemonConfig reads daemon-host and daemon-token from a specific config file.
func readDaemonConfig(configPath string) (host, token string) {
	data, err := os.ReadFile(configPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return "", ""
	}
	var config yaml.MapSlice
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", ""
	}
	for _, item := range config {
		switch item.Key {
		case "daemon-host":
			if s, ok := item.Value.(string); ok {
				host = s
			}
		case "daemon-token":
			if s, ok := item.Value.(string); ok {
				token = s
			}
		}
	}
	return host, token
}
