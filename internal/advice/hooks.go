// Package advice provides advice hook execution for Gas Town agents.
//
// Advice hooks are commands defined in advice beads that run at specific
// lifecycle points (session-end, before-commit, before-push, before-handoff).
// This package implements the hook execution engine (gt-08ast5).
package advice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Trigger constants define when advice hooks should execute.
const (
	TriggerSessionEnd    = "session-end"
	TriggerBeforeCommit  = "before-commit"
	TriggerBeforePush    = "before-push"
	TriggerBeforeHandoff = "before-handoff"
)

// OnFailure constants define behavior when a hook command fails.
const (
	OnFailureBlock  = "block"  // Abort the lifecycle action
	OnFailureWarn   = "warn"   // Show warning but continue
	OnFailureIgnore = "ignore" // Silent continue
)

// Default values
const (
	DefaultTimeout   = 30   // seconds
	MaxTimeout       = 300  // seconds (5 minutes)
	MaxCommandLength = 1000 // characters
)

// ValidTriggers lists all valid hook trigger values.
var ValidTriggers = []string{
	TriggerSessionEnd,
	TriggerBeforeCommit,
	TriggerBeforePush,
	TriggerBeforeHandoff,
}

// ValidOnFailure lists all valid failure behavior values.
var ValidOnFailure = []string{
	OnFailureBlock,
	OnFailureWarn,
	OnFailureIgnore,
}

// Hook represents an advice hook to be executed.
type Hook struct {
	// ID is the bead ID this hook came from
	ID string

	// Command is the shell command to execute
	Command string

	// Trigger is when this hook should run
	Trigger string

	// Timeout is max execution time in seconds (default: 30, max: 300)
	Timeout int

	// OnFailure is what to do if command fails (block, warn, ignore)
	OnFailure string

	// Priority determines execution order (lower = first)
	Priority int

	// Title is the advice bead title for logging
	Title string
}

// HookResult represents the outcome of executing a hook.
type HookResult struct {
	// Hook is the hook that was executed
	Hook *Hook

	// Success is true if the command exited with code 0
	Success bool

	// ExitCode is the command's exit code
	ExitCode int

	// Output is the combined stdout and stderr
	Output string

	// Duration is how long the hook took to execute
	Duration time.Duration

	// Error is any execution error (not command failure)
	Error error

	// TimedOut is true if the hook was killed due to timeout
	TimedOut bool
}

// Runner executes advice hooks with timeout and failure handling.
type Runner struct {
	// WorkDir is the working directory for hook execution
	WorkDir string

	// EnvVars are additional environment variables to set
	EnvVars map[string]string

	// AgentID is the executing agent's identifier
	AgentID string

	// Shell is the shell to use (default: sh)
	Shell string
}

// NewRunner creates a new advice hook runner.
func NewRunner(workDir string, agentID string) *Runner {
	return &Runner{
		WorkDir: workDir,
		AgentID: agentID,
		EnvVars: make(map[string]string),
		Shell:   "sh",
	}
}

// SetEnv adds an environment variable for hook execution.
func (r *Runner) SetEnv(key, value string) {
	r.EnvVars[key] = value
}

// Execute runs a single hook with timeout and returns the result.
func (r *Runner) Execute(hook *Hook) *HookResult {
	result := &HookResult{
		Hook: hook,
	}

	start := time.Now()

	// Validate hook
	if err := ValidateHook(hook); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	// Determine timeout
	timeout := hook.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Create command - don't use CommandContext because we need to handle
	// process group killing ourselves for proper child process cleanup
	cmd := exec.Command(r.Shell, "-c", hook.Command)
	cmd.Dir = r.WorkDir

	// Set up process group so we can kill child processes on timeout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Set environment
	cmd.Env = os.Environ()
	for key, value := range r.EnvVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}
	// Add standard advice hook env vars
	cmd.Env = append(cmd.Env, fmt.Sprintf("GT_ADVICE_HOOK_ID=%s", hook.ID))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GT_ADVICE_HOOK_TRIGGER=%s", hook.Trigger))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GT_AGENT_ID=%s", r.AgentID))
	cmd.Env = append(cmd.Env, fmt.Sprintf("GT_WORK_DIR=%s", r.WorkDir))

	// Capture output using pipes
	var outputBuf strings.Builder
	cmd.Stdout = &writerWrapper{&outputBuf}
	cmd.Stderr = &writerWrapper{&outputBuf}

	// Start the command
	if err := cmd.Start(); err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	// Wait for command in a goroutine
	type waitResult struct {
		err error
	}
	done := make(chan waitResult, 1)
	go func() {
		err := cmd.Wait()
		done <- waitResult{err: err}
	}()

	// Wait for completion or timeout
	select {
	case <-ctx.Done():
		// Timeout - kill the entire process group
		if cmd.Process != nil {
			// Kill the process group (negative PID kills the group)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		// Wait for process to actually exit
		<-done
		result.Duration = time.Since(start)
		result.Output = outputBuf.String()
		result.TimedOut = true
		result.Error = fmt.Errorf("hook timed out after %d seconds", timeout)
		result.ExitCode = -1
		return result

	case wr := <-done:
		result.Duration = time.Since(start)
		result.Output = outputBuf.String()

		// Check for errors
		if wr.err != nil {
			if exitErr, ok := wr.err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
				result.Success = false
			} else {
				result.Error = wr.err
				result.ExitCode = -1
			}
			return result
		}

		result.Success = true
		result.ExitCode = 0
		return result
	}
}

// writerWrapper wraps a strings.Builder to implement io.Writer
type writerWrapper struct {
	sb *strings.Builder
}

func (w *writerWrapper) Write(p []byte) (n int, err error) {
	return w.sb.Write(p)
}

// RunAll executes multiple hooks in priority order and returns all results.
// Returns an error if any hook with on_failure=block fails.
func (r *Runner) RunAll(hooks []*Hook) ([]*HookResult, error) {
	var results []*HookResult
	var blockingError error

	for _, hook := range hooks {
		result := r.Execute(hook)
		results = append(results, result)

		// Handle failure based on on_failure setting
		if !result.Success && result.Error == nil {
			// Command failed (non-zero exit)
			onFailure := hook.OnFailure
			if onFailure == "" {
				onFailure = OnFailureWarn
			}

			if onFailure == OnFailureBlock && blockingError == nil {
				blockingError = fmt.Errorf("hook %q failed with exit code %d: %s",
					hook.ID, result.ExitCode, TruncateOutput(result.Output, 200))
			}
		}

		// Also block on execution errors (malformed command, etc.)
		if result.Error != nil && hook.OnFailure == OnFailureBlock && blockingError == nil {
			blockingError = fmt.Errorf("hook %q execution error: %w", hook.ID, result.Error)
		}
	}

	return results, blockingError
}

// ValidateHook checks if a hook is valid for execution.
func ValidateHook(hook *Hook) error {
	if hook == nil {
		return fmt.Errorf("hook is nil")
	}

	if hook.Command == "" {
		return fmt.Errorf("hook command is empty")
	}

	if len(hook.Command) > MaxCommandLength {
		return fmt.Errorf("hook command exceeds maximum length of %d characters", MaxCommandLength)
	}

	if hook.Trigger != "" && !isValidTrigger(hook.Trigger) {
		return fmt.Errorf("invalid hook trigger: %q", hook.Trigger)
	}

	if hook.OnFailure != "" && !isValidOnFailure(hook.OnFailure) {
		return fmt.Errorf("invalid on_failure value: %q", hook.OnFailure)
	}

	if hook.Timeout < 0 {
		return fmt.Errorf("hook timeout cannot be negative")
	}

	return nil
}

// IsValidTrigger checks if a trigger value is valid.
func IsValidTrigger(trigger string) bool {
	return isValidTrigger(trigger)
}

// IsValidOnFailure checks if an on_failure value is valid.
func IsValidOnFailure(onFailure string) bool {
	return isValidOnFailure(onFailure)
}

func isValidTrigger(trigger string) bool {
	for _, t := range ValidTriggers {
		if t == trigger {
			return true
		}
	}
	return false
}

func isValidOnFailure(onFailure string) bool {
	for _, f := range ValidOnFailure {
		if f == onFailure {
			return true
		}
	}
	return false
}

// TruncateOutput truncates a string to maxLen characters, adding "..." if truncated.
func TruncateOutput(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
