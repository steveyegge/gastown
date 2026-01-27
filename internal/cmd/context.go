package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	// defaultCooldownPeriod is the duration after escalation during which
	// further error detection is suppressed to avoid spam
	defaultCooldownPeriod = 10 * time.Minute
)

var (
	contextSession       string
	contextLines         int
	contextJSON          bool
	contextUsage         bool
	contextErrors        bool
	contextCircuitStatus bool
	contextCircuitReset  bool
	contextCheck         bool
	contextThreshold     int
)

var contextCmd = &cobra.Command{
	Use:     "context [session]",
	GroupID: GroupDiag,
	Short:   "Monitor Claude session context usage",
	Long: `Monitor Claude Code session context window usage and detect errors.

Commands:
  gt context --usage          # Check current context usage (stub: returns 0% for now)
  gt context --errors         # Detect recent context length errors in session output
  gt context --circuit-breaker-status  # Show circuit breaker state
  gt context --circuit-breaker-reset   # Reset circuit breaker state
  gt context --check          # Comprehensive check: usage + errors + circuit breaker

The command can optionally take a tmux session name as argument. If not provided,
it attempts to auto-detect the current session based on environment variables.

Circuit Breaker Pattern:
  After N consecutive context limit errors, the circuit breaker trips and
  triggers escalation actions (auto-mail witness, escalation bead creation).
  Default threshold: 3 consecutive failures.

Error Detection:
  Searches session output for patterns indicating context length exceeded,
  rate limits (429), or quota exceeded errors.

State Storage:
  Circuit breaker state is stored in XDG-compliant state directory:
  ~/.local/state/gastown/context-limit-state.json
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runContext,
}

func init() {
	rootCmd.AddCommand(contextCmd)

	contextCmd.Flags().StringVar(&contextSession, "session", "", "tmux session name (default: auto-detect)")
	contextCmd.Flags().IntVar(&contextLines, "lines", 100, "number of lines to capture from session output")
	contextCmd.Flags().BoolVar(&contextJSON, "json", false, "output as JSON")
	contextCmd.Flags().BoolVar(&contextUsage, "usage", false, "check current context usage (stub)")
	contextCmd.Flags().BoolVar(&contextErrors, "errors", false, "detect recent context length errors")
	contextCmd.Flags().BoolVar(&contextCircuitStatus, "circuit-breaker-status", false, "show circuit breaker state")
	contextCmd.Flags().BoolVar(&contextCircuitReset, "circuit-breaker-reset", false, "reset circuit breaker state")
	contextCmd.Flags().BoolVar(&contextCheck, "check", false, "comprehensive check: usage + errors + circuit breaker")
	contextCmd.Flags().IntVar(&contextThreshold, "threshold", 3, "consecutive failures before circuit breaker trips")

	// If no specific flag is provided, default to --check
	contextCmd.Flags().SetInterspersed(false)
}

func runContext(cmd *cobra.Command, args []string) error {
	// Determine session name
	session := contextSession
	if session == "" && len(args) > 0 {
		session = args[0]
	}
	// Create tmux instance
	tmuxClient := tmux.NewTmux()

	if session == "" {
		// Auto-detect session
		var err error
		session, err = autoDetectSession(tmuxClient)
		if err != nil {
			return fmt.Errorf("session auto-detection failed: %w", err)
		}
	}

	// If no specific flag is set, default to --check
	if !contextUsage && !contextErrors && !contextCircuitStatus && !contextCircuitReset && !contextCheck {
		contextCheck = true
	}

	// Execute requested operations
	if contextUsage {
		return runContextUsage(session, tmuxClient)
	}
	if contextErrors {
		return runContextErrors(session, tmuxClient)
	}
	if contextCircuitStatus {
		return runCircuitBreakerStatus()
	}
	if contextCircuitReset {
		return runCircuitBreakerReset()
	}
	if contextCheck {
		return runContextCheck(session, tmuxClient)
	}

	return nil
}

// ContextLimitState represents circuit breaker state for context limit errors
type ContextLimitState struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastFailureTime     time.Time `json:"last_failure_time"`
	LastSuccessTime     time.Time `json:"last_success_time"`
	ForceCooldownUntil  time.Time `json:"force_cooldown_until"`
	TrippedAt           time.Time `json:"tripped_at,omitempty"`
	EscalatedAt         time.Time `json:"escalated_at,omitempty"`
}

// stateFilePath returns the path to context limit state file
func stateFilePath() string {
	return filepath.Join(state.StateDir(), "context-limit-state.json")
}

// loadState loads circuit breaker state from disk
func loadState() (*ContextLimitState, error) {
	path := stateFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ContextLimitState{}, nil
		}
		return nil, err
	}

	var state ContextLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// saveState saves circuit breaker state to disk atomically
func saveState(state *ContextLimitState) error {
	path := stateFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return util.AtomicWriteJSON(path, state)
}

// autoDetectSession attempts to find the current Gas Town session
func autoDetectSession(tmuxClient *tmux.Tmux) (string, error) {
	// First check if we're inside a tmux session
	if tmux.IsInsideTmux() {
		// Get current session from TMUX environment variable
		// Format: /tmp/tmux-1000/default,1234
		tmuxEnv := os.Getenv("TMUX")
		if tmuxEnv != "" {
			parts := strings.Split(tmuxEnv, ",")
			if len(parts) >= 2 {
				// parts[0] is socket path, parts[1] is session ID
				// We need to map session ID to name
				sessionMap, err := tmuxClient.ListSessionIDs()
				if err == nil {
					for name, id := range sessionMap {
						if id == parts[1] {
							return name, nil
						}
					}
				}
			}
		}
	}

	// Fallback: list all sessions and pick first Gas Town session
	sessions, err := tmuxClient.ListSessions()
	if err != nil {
		return "", fmt.Errorf("listing tmux sessions: %w", err)
	}

	for _, session := range sessions {
		if strings.HasPrefix(session, "gt-") || strings.HasPrefix(session, "hq-") {
			return session, nil
		}
	}

	return "", fmt.Errorf("no Gas Town session found (gt-* or hq-*)")
}

// runContextUsage checks current context usage (stub implementation)
func runContextUsage(_ string, _ *tmux.Tmux) error {
	// TODO: Implement actual context usage detection via Claude Code API
	// For now, return stub value (session and tmuxClient will be used then)
	if contextJSON {
		fmt.Println(`{"usage_percent": 0, "message": "stub implementation"}`)
	} else {
		fmt.Println("Context usage: 0% (stub implementation)")
		fmt.Println("Note: Actual context usage detection not yet implemented")
	}
	return nil
}

// errorPatterns defines regex patterns for context-related errors
var errorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)context.*length.*exceeded`),
	regexp.MustCompile(`(?i)429.*Too.*Many.*Requests`),
	regexp.MustCompile(`(?i)quota.*exceeded`),
	regexp.MustCompile(`(?i)rate.*limit`),
	regexp.MustCompile(`(?i)token.*limit`),
	regexp.MustCompile(`(?i)memory.*limit`),
}

// extractErrorLines finds lines matching error patterns
func extractErrorLines(output string) []string {
	var errorLines []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		for _, pattern := range errorPatterns {
			if pattern.MatchString(line) {
				errorLines = append(errorLines, line)
				break // only count each line once
			}
		}
	}
	return errorLines
}

// triggerEscalation sends auto-mail witness and creates escalation bead
func triggerEscalation(state *ContextLimitState) error {
	// Check if already escalated
	if !state.EscalatedAt.IsZero() {
		return nil
	}

	// Get current agent identity
	sender := detectSender()
	// Determine polecat name for subject
	polecat := os.Getenv("GT_POLECAT")
	if polecat == "" {
		// Extract from sender if possible
		if strings.Contains(sender, "/") {
			parts := strings.Split(sender, "/")
			if len(parts) >= 2 {
				polecat = parts[len(parts)-1]
			}
		}
	}
	subject := fmt.Sprintf("CONTEXT_LIMIT: %s", polecat)

	// Send auto-mail witness
	// We'll use exec to call gt mail send witness -s subject
	// This avoids complex dependencies
	cmd := exec.Command("gt", "mail", "send", "witness", "-s", subject)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sending auto-mail witness: %w\noutput: %s", err, output)
	}

	// Create escalation bead
	cmd = exec.Command("gt", "escalate", "--severity", "high", "--reason", "Context limit circuit breaker tripped", "--source", sender)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Log error but don't fail - mail sent is primary action
		fmt.Printf("Warning: Failed to create escalation bead: %v\n%s", err, output)
	}

	state.EscalatedAt = time.Now()
	state.ForceCooldownUntil = time.Now().Add(defaultCooldownPeriod)
	return nil
}

// triggerHandoff prints advice to restart agent via gt handoff.
// This is a stub that just prints guidance - actual handoff requires user action.
func triggerHandoff() {
	fmt.Println("⚠️  Context limit error detected. Consider running 'gt handoff' to restart agent with fresh context.")
}

// runContextErrors detects context limit errors in session output
func runContextErrors(session string, tmuxClient *tmux.Tmux) error {
	// Capture session output
	output, err := tmuxClient.CapturePane(session, contextLines)
	if err != nil {
		return fmt.Errorf("capturing session output: %w", err)
	}

	// Extract error lines
	errorLines := extractErrorLines(output)

	// Load circuit breaker state
	state, err := loadState()
	if err != nil {
		return fmt.Errorf("loading circuit breaker state: %w", err)
	}

	// Check if we're in cooldown period
	now := time.Now()
	if now.Before(state.ForceCooldownUntil) {
		if contextJSON {
			fmt.Printf(`{"cooldown_active": true, "cooldown_until": %q, "message": "error detection suppressed due to cooldown period"}`, state.ForceCooldownUntil.Format(time.RFC3339))
		} else {
			fmt.Printf("⏸️  Error detection suppressed (cooldown until %s)\n", formatTime(state.ForceCooldownUntil))
		}
		return nil
	}

	// Update state based on detection
	if len(errorLines) > 0 {
		state.ConsecutiveFailures++
		state.LastFailureTime = now

		// Check if circuit breaker should trip
		if state.ConsecutiveFailures >= contextThreshold {
			state.TrippedAt = now
			// Trigger escalation if not already escalated
			if state.EscalatedAt.IsZero() {
				if err := triggerEscalation(state); err != nil {
					fmt.Printf("Warning: Failed to trigger escalation: %v\n", err)
				}
				// Also suggest handoff to restart agent with fresh context
				triggerHandoff()
			}
		}
	} else {
		state.ConsecutiveFailures = 0
		state.LastSuccessTime = now
	}

	// Save updated state
	if err := saveState(state); err != nil {
		return fmt.Errorf("saving circuit breaker state: %w", err)
	}

	// Output results
	if contextJSON {
		fmt.Printf(`{"errors_found": %d, "error_lines": %q, "consecutive_failures": %d, "circuit_breaker_tripped": %v, "escalated": %v}`,
			len(errorLines), errorLines, state.ConsecutiveFailures, state.ConsecutiveFailures >= contextThreshold, !state.EscalatedAt.IsZero())
	} else {
		if len(errorLines) > 0 {
			fmt.Printf("Found %d context-related error line(s) in session '%s'\n", len(errorLines), session)
			for i, line := range errorLines {
				fmt.Printf("  %d. %s\n", i+1, strings.TrimSpace(line))
			}
			fmt.Printf("Consecutive failures: %d/%d\n", state.ConsecutiveFailures, contextThreshold)
			if state.ConsecutiveFailures >= contextThreshold {
				fmt.Println("⚠️  CIRCUIT BREAKER TRIPPED")
				if !state.EscalatedAt.IsZero() {
					fmt.Printf("  Escalation triggered at: %s\n", formatTime(state.EscalatedAt))
				}
			}
		} else {
			fmt.Printf("No context-related errors found in session '%s'\n", session)
			fmt.Printf("Consecutive failures: %d/%d\n", state.ConsecutiveFailures, contextThreshold)
		}
	}

	return nil
}

// runCircuitBreakerStatus displays circuit breaker state
func runCircuitBreakerStatus() error {
	state, err := loadState()
	if err != nil {
		return fmt.Errorf("loading circuit breaker state: %w", err)
	}

	if contextJSON {
		fmt.Printf(`{"consecutive_failures": %d, "last_failure_time": %q, "last_success_time": %q, "force_cooldown_until": %q, "tripped_at": %q, "escalated_at": %q}`,
			state.ConsecutiveFailures,
			state.LastFailureTime.Format(time.RFC3339),
			state.LastSuccessTime.Format(time.RFC3339),
			state.ForceCooldownUntil.Format(time.RFC3339),
			state.TrippedAt.Format(time.RFC3339),
			state.EscalatedAt.Format(time.RFC3339))
	} else {
		fmt.Println("Circuit Breaker Status:")
		fmt.Printf("  Consecutive failures: %d/%d\n", state.ConsecutiveFailures, contextThreshold)
		fmt.Printf("  Last failure time: %s\n", formatTime(state.LastFailureTime))
		fmt.Printf("  Last success time: %s\n", formatTime(state.LastSuccessTime))
		fmt.Printf("  Force cooldown until: %s\n", formatTime(state.ForceCooldownUntil))
		fmt.Printf("  Tripped at: %s\n", formatTime(state.TrippedAt))
		fmt.Printf("  Escalated at: %s\n", formatTime(state.EscalatedAt))

		if state.ConsecutiveFailures >= contextThreshold {
			fmt.Println("  Status: ⚠️ TRIPPED (escalation required)")
		} else if state.ConsecutiveFailures > 0 {
			fmt.Printf("  Status: ⚠️ WARNING (%d consecutive failures)\n", state.ConsecutiveFailures)
		} else {
			fmt.Println("  Status: ✅ OK")
		}
	}

	return nil
}

// runCircuitBreakerReset resets circuit breaker state
func runCircuitBreakerReset() error {
	state := &ContextLimitState{}
	if err := saveState(state); err != nil {
		return fmt.Errorf("resetting circuit breaker state: %w", err)
	}

	if contextJSON {
		fmt.Println(`{"status": "reset"}`)
	} else {
		fmt.Println("Circuit breaker state reset")
	}
	return nil
}

// runContextCheck performs comprehensive context check
func runContextCheck(session string, tmuxClient *tmux.Tmux) error {
	// Check for errors first
	if err := runContextErrors(session, tmuxClient); err != nil {
		return err
	}

	// Check circuit breaker status
	state, err := loadState()
	if err != nil {
		return fmt.Errorf("loading circuit breaker state: %w", err)
	}

	// If circuit breaker tripped, recommend actions
	if state.ConsecutiveFailures >= contextThreshold {
		fmt.Println("\n🚨 ACTION REQUIRED: Context limit circuit breaker tripped")
		fmt.Println("Recommended actions:")
		fmt.Println("  1. Auto-mail witness: gt mail send witness -s 'CONTEXT_LIMIT: <polecat>'")
		fmt.Println("  2. Create escalation: gt escalate --severity high --reason \"Context limit circuit breaker tripped\"")
		fmt.Println("  3. Consider handoff: gt handoff (if available for this agent type)")
	}

	checkAutoCompactConfig()

	return nil
}

// checkAutoCompactConfig checks if auto-compact environment variable is set
func checkAutoCompactConfig() {
	// Check for CLAUDE_AUTOCOMPACT_PCT_OVERRIDE environment variable
	value := os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE")
	if value == "" {
		fmt.Println("⚠️  WARNING: CLAUDE_AUTOCOMPACT_PCT_OVERRIDE not set")
		fmt.Println("   For deepseek endpoint (131K limit), set to 60 to auto-compact at 78K tokens")
		fmt.Println("   Add to agent startup: export CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=60")
	} else {
		// Validate it's a reasonable percentage
		if percent, err := strconv.Atoi(value); err == nil {
			if percent < 30 || percent > 90 {
				fmt.Printf("⚠️  WARNING: CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=%s may be too %s\n",
					value,
					ternary(percent < 30, "low", "high"))
				fmt.Println("   Recommended: 60 for deepseek (131K limit)")
			} else {
				fmt.Printf("✅ CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=%s (auto-compact at %s%% of context limit)\n", value, value)
			}
		}
	}
}

// ternary helper for inline condition
func ternary(condition bool, trueVal, falseVal string) string {
	if condition {
		return trueVal
	}
	return falseVal
}

// formatTime formats time for display
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02 15:04:05")
}
