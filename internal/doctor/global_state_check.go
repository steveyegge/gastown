// ABOUTME: Doctor check for Gas Town global state configuration.
// ABOUTME: Validates that state directories and shell integration are properly configured.

package doctor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/shell"
	"github.com/steveyegge/gastown/internal/state"
)

type GlobalStateCheck struct {
	BaseCheck
}

func NewGlobalStateCheck() *GlobalStateCheck {
	return &GlobalStateCheck{
		BaseCheck: BaseCheck{
			CheckName:        "global-state",
			CheckDescription: "Validates Gas Town global state and shell integration",
			CheckCategory:    CategoryCore,
		},
	}
}

func (c *GlobalStateCheck) Run(ctx *CheckContext) *CheckResult {
	result := &CheckResult{
		Name:   c.Name(),
		Status: StatusOK,
	}

	var details []string
	var warnings []string
	var errors []string

	s, err := state.Load()
	if err != nil {
		if os.IsNotExist(err) {
			result.Message = "Global state not initialized"
			result.FixHint = "Run: gt enable"
			result.Status = StatusWarning
			return result
		}
		result.Message = "Cannot read global state"
		result.Details = []string{err.Error()}
		result.Status = StatusError
		return result
	}

	if s.Enabled {
		details = append(details, "Gas Town: enabled")
	} else {
		details = append(details, "Gas Town: disabled")
		warnings = append(warnings, "Gas Town is disabled globally")
	}

	if s.Version != "" {
		details = append(details, "Version: "+s.Version)
	}

	if s.MachineID != "" {
		details = append(details, "Machine ID: "+s.MachineID)
	}

	rcPath := shell.RCFilePath(shell.DetectShell())
	if hasShellIntegration(rcPath) {
		details = append(details, "Shell integration: installed ("+rcPath+")")
	} else {
		warnings = append(warnings, "Shell integration not installed")
	}

	hookPath := filepath.Join(state.ConfigDir(), "shell-hook.sh")
	if _, err := os.Stat(hookPath); err == nil {
		details = append(details, "Hook script: present")
	} else {
		if hasShellIntegration(rcPath) {
			errors = append(errors, "Hook script missing but shell integration installed")
		}
	}

	result.Details = details

	if len(errors) > 0 {
		result.Status = StatusError
		result.Message = errors[0]
		result.FixHint = "Run: gt install --shell"
	} else if len(warnings) > 0 {
		result.Status = StatusWarning
		result.Message = warnings[0]
		if !s.Enabled {
			result.FixHint = "Run: gt enable"
		} else {
			result.FixHint = "Run: gt install --shell"
		}
	} else {
		result.Message = "Global state healthy"
	}

	return result
}

func hasShellIntegration(rcPath string) bool {
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "Gas Town Integration")
}
