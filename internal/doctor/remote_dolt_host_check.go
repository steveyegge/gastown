package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RemoteDoltHostCheck detects cases where daemon.json configures a remote Dolt
// host but GT_DOLT_HOST is not exported to the shell environment. While gt CLI
// tools auto-read daemon.json, direct dolt commands and third-party tools rely
// on the GT_DOLT_HOST environment variable.
type RemoteDoltHostCheck struct {
	BaseCheck
}

// NewRemoteDoltHostCheck creates a new remote Dolt host check.
func NewRemoteDoltHostCheck() *RemoteDoltHostCheck {
	return &RemoteDoltHostCheck{
		BaseCheck: BaseCheck{
			CheckName:        "remote-dolt-host",
			CheckDescription: "Verify GT_DOLT_HOST is set when daemon.json configures a remote Dolt server",
			CheckCategory:    CategoryConfig,
		},
	}
}

// Run checks for remote Dolt host misconfiguration.
func (c *RemoteDoltHostCheck) Run(ctx *CheckContext) *CheckResult {
	if ctx.TownRoot == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No town root — skipped",
		}
	}

	configuredHost := c.readDaemonJSONHost(ctx.TownRoot)
	if configuredHost == "" || !isRemoteHost(configuredHost) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Dolt server is local (no remote host configured)",
		}
	}

	shellHost := os.Getenv("GT_DOLT_HOST")
	if shellHost == configuredHost {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("GT_DOLT_HOST matches daemon.json (%s)", configuredHost),
		}
	}

	var details []string
	var message string

	if shellHost == "" {
		message = fmt.Sprintf("daemon.json has remote Dolt host (%s) but GT_DOLT_HOST not in shell", configuredHost)
		details = []string{
			fmt.Sprintf("daemon.json patrols.dolt_server.host = %q", configuredHost),
			"gt CLI tools auto-read daemon.json, but direct dolt commands need GT_DOLT_HOST.",
			fmt.Sprintf("  Add to your shell profile: export GT_DOLT_HOST=%s", configuredHost),
		}
	} else {
		message = fmt.Sprintf("GT_DOLT_HOST=%q conflicts with daemon.json host %q", shellHost, configuredHost)
		details = []string{
			fmt.Sprintf("daemon.json patrols.dolt_server.host = %q", configuredHost),
			fmt.Sprintf("GT_DOLT_HOST (shell env) = %q", shellHost),
			"Shell env takes precedence — gt CLI will connect to shell host, not daemon.json host.",
			"Update GT_DOLT_HOST or remove the daemon.json dolt_server.host to resolve the conflict.",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: message,
		Details: details,
		FixHint: fmt.Sprintf("Add 'export GT_DOLT_HOST=%s' to your shell profile", configuredHost),
	}
}

// readDaemonJSONHost reads the dolt_server.host field from daemon.json.
func (c *RemoteDoltHostCheck) readDaemonJSONHost(townRoot string) string {
	daemonJSONPath := filepath.Join(townRoot, "mayor", "daemon.json")
	data, err := os.ReadFile(daemonJSONPath)
	if err != nil {
		return ""
	}

	var dc struct {
		Patrols struct {
			DoltServer struct {
				Host string `json:"host"`
			} `json:"dolt_server"`
		} `json:"patrols"`
	}
	if err := json.Unmarshal(data, &dc); err != nil {
		return ""
	}
	return dc.Patrols.DoltServer.Host
}

// isRemoteHost returns true when the host is not local (mirrors doltserver.IsRemote).
func isRemoteHost(host string) bool {
	switch strings.ToLower(host) {
	case "", "127.0.0.1", "localhost", "::1", "[::1]":
		return false
	}
	return true
}
