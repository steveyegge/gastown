package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Default context budget thresholds (fraction of max context window).
const (
	defaultWarnThreshold = 0.75
	defaultSoftGate      = 0.85
	defaultHardGate      = 0.92

	// Claude Code's effective context window in tokens.
	// The actual model context is ~200k, but Claude Code uses some for system
	// prompt, tool definitions, etc. We use 200k as the reference ceiling
	// and measure cumulative input tokens against it.
	defaultMaxContextTokens = 200_000
)

// Roles that get hard-gated (tool calls blocked) vs warn-only by default.
var (
	defaultHardGateRoles = map[string]bool{
		"mayor":  true,
		"deacon": true,
	}
	defaultWarnOnlyRoles = map[string]bool{
		"polecat": true,
		"crew":    true,
	}
)

var tapGuardContextBudgetCmd = &cobra.Command{
	Use:   "context-budget",
	Short: "Monitor context window usage and enforce handoff thresholds",
	Long: `Monitor Claude Code context window consumption and enforce automatic
handoff when thresholds are reached.

This guard reads the active session's transcript file to estimate token
usage and enforces configurable thresholds:

  75% (warn)      - Injects reminder to consider handoff
  85% (soft gate)  - Stronger reminder with specific handoff instructions
  92% (hard gate)  - Blocks tool calls for configured roles, forcing handoff

Configuration via environment variables:
  GT_CONTEXT_BUDGET_WARN       - Warning threshold (default: 0.75)
  GT_CONTEXT_BUDGET_SOFT_GATE  - Soft gate threshold (default: 0.85)
  GT_CONTEXT_BUDGET_HARD_GATE  - Hard gate threshold (default: 0.92)
  GT_CONTEXT_BUDGET_MAX_TOKENS - Max context tokens (default: 200000)
  GT_CONTEXT_BUDGET_DISABLE    - Set to "1" to disable the guard

Hard gate roles (blocked at hard gate): mayor, deacon
Warn-only roles (never blocked): polecat, crew
Other roles (witness, refinery): get hard gate

Exit codes:
  0 - Operation allowed (below threshold or warn-only role)
  2 - Operation BLOCKED (hard gate exceeded for hard-gate role)`,
	RunE: runTapGuardContextBudget,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardContextBudgetCmd)
}

// contextBudgetConfig holds the resolved configuration for the guard.
type contextBudgetConfig struct {
	WarnThreshold float64
	SoftGate      float64
	HardGate      float64
	MaxTokens     int
	Role          string
	IsHardGated   bool
}

func loadContextBudgetConfig() *contextBudgetConfig {
	cfg := &contextBudgetConfig{
		WarnThreshold: defaultWarnThreshold,
		SoftGate:      defaultSoftGate,
		HardGate:      defaultHardGate,
		MaxTokens:     defaultMaxContextTokens,
	}

	// Override from environment
	if v := os.Getenv("GT_CONTEXT_BUDGET_WARN"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			cfg.WarnThreshold = f
		}
	}
	if v := os.Getenv("GT_CONTEXT_BUDGET_SOFT_GATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			cfg.SoftGate = f
		}
	}
	if v := os.Getenv("GT_CONTEXT_BUDGET_HARD_GATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			cfg.HardGate = f
		}
	}
	if v := os.Getenv("GT_CONTEXT_BUDGET_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxTokens = n
		}
	}

	// Detect role
	cfg.Role = detectCurrentRole()

	// Determine if this role gets hard-gated
	if defaultWarnOnlyRoles[cfg.Role] {
		cfg.IsHardGated = false
	} else {
		// mayor, deacon, and any other role (witness, refinery) get hard-gated
		cfg.IsHardGated = defaultHardGateRoles[cfg.Role] || cfg.Role == "witness" || cfg.Role == "refinery" || cfg.Role == ""
	}

	return cfg
}

// detectCurrentRole returns the current Gas Town role from environment variables.
func detectCurrentRole() string {
	if v := os.Getenv("GT_ROLE"); v != "" {
		return strings.ToLower(v)
	}

	// Fall back to checking individual role env vars
	roleEnvMap := map[string]string{
		"GT_MAYOR":    "mayor",
		"GT_DEACON":   "deacon",
		"GT_POLECAT":  "polecat",
		"GT_CREW":     "crew",
		"GT_WITNESS":  "witness",
		"GT_REFINERY": "refinery",
	}
	for env, role := range roleEnvMap {
		if os.Getenv(env) != "" {
			return role
		}
	}

	return ""
}

func runTapGuardContextBudget(cmd *cobra.Command, args []string) error {
	// Check if disabled
	if os.Getenv("GT_CONTEXT_BUDGET_DISABLE") == "1" {
		return nil
	}

	cfg := loadContextBudgetConfig()

	// Find the current session transcript
	usage, err := getCurrentSessionUsage()
	if err != nil {
		// Can't determine usage — allow the operation rather than blocking
		return nil
	}

	// Calculate context usage ratio.
	// The key metric is input_tokens from the most recent assistant message,
	// which reflects the current context window size. But since we're summing
	// all messages, we use cumulative input tokens as a proxy — each turn's
	// input_tokens roughly equals the context window consumed at that point.
	// The last input_tokens value is the best estimate of current context size.
	if usage.LastInputTokens == 0 {
		return nil // No usage data yet
	}

	ratio := float64(usage.LastInputTokens) / float64(cfg.MaxTokens)

	if ratio >= cfg.HardGate {
		return handleHardGate(cfg, usage, ratio)
	}

	if ratio >= cfg.SoftGate {
		return handleSoftGate(cfg, usage, ratio)
	}

	if ratio >= cfg.WarnThreshold {
		return handleWarn(cfg, usage, ratio)
	}

	// Below all thresholds — allow silently
	return nil
}

func handleWarn(cfg *contextBudgetConfig, usage *contextBudgetUsage, ratio float64) error {
	pct := int(ratio * 100)
	remaining := cfg.MaxTokens - usage.LastInputTokens

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "⚠️  Context budget at %d%% (%dk / %dk tokens)\n",
		pct, usage.LastInputTokens/1000, cfg.MaxTokens/1000)
	fmt.Fprintf(os.Stderr, "   Remaining: ~%dk tokens\n", remaining/1000)
	fmt.Fprintln(os.Stderr, "   Consider using gt handoff to pass remaining work to a fresh session.")
	fmt.Fprintln(os.Stderr, "")

	return nil // Allow the operation
}

func handleSoftGate(cfg *contextBudgetConfig, usage *contextBudgetUsage, ratio float64) error {
	pct := int(ratio * 100)
	remaining := cfg.MaxTokens - usage.LastInputTokens

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintf(os.Stderr,  "║  ⚠️  CONTEXT BUDGET AT %d%% — HANDOFF RECOMMENDED                 ║\n", pct)
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintf(os.Stderr,  "║  Used: %dk / %dk tokens (remaining: ~%dk)                  ║\n",
		usage.LastInputTokens/1000, cfg.MaxTokens/1000, remaining/1000)
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  You should hand off NOW to preserve context quality.            ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Run: gt handoff -s \"Context budget\" -m \"<what remains>\"          ║")
	fmt.Fprintln(os.Stderr, "║  Or:  gt done  (if work is complete)                             ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")

	return nil // Allow but strongly encourage handoff
}

func handleHardGate(cfg *contextBudgetConfig, usage *contextBudgetUsage, ratio float64) error {
	pct := int(ratio * 100)

	// Build a summary of recent work from the transcript
	workSummary := ""
	if usage.RecentToolUse != "" {
		workSummary = fmt.Sprintf("  Recent activity: %s", usage.RecentToolUse)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintf(os.Stderr,  "║  🛑 CONTEXT BUDGET EXCEEDED (%d%%)                                ║\n", pct)
	fmt.Fprintln(os.Stderr, "╠══════════════════════════════════════════════════════════════════╣")
	fmt.Fprintf(os.Stderr,  "║  Used: %dk / %dk tokens — budget exhausted                 ║\n",
		usage.LastInputTokens/1000, cfg.MaxTokens/1000)
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	if workSummary != "" {
		fmt.Fprintf(os.Stderr, "║%s\n", workSummary)
		fmt.Fprintln(os.Stderr, "║                                                                  ║")
	}
	fmt.Fprintln(os.Stderr, "║  You MUST hand off remaining work NOW.                          ║")
	fmt.Fprintln(os.Stderr, "║                                                                  ║")
	fmt.Fprintln(os.Stderr, "║  Run: gt handoff -s \"Context exhausted\" -m \"<remaining work>\"     ║")
	fmt.Fprintln(os.Stderr, "║  Or:  gt done  (if work is complete)                             ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr, "")

	if cfg.IsHardGated {
		return NewSilentExit(2) // BLOCK the tool call
	}

	// Warn-only roles: strong message but don't block
	return nil
}

// contextBudgetUsage holds the parsed usage data for context budget evaluation.
type contextBudgetUsage struct {
	// LastInputTokens is the input_tokens from the most recent assistant message.
	// This is the best proxy for current context window size, since each turn's
	// input includes the full conversation history.
	LastInputTokens int

	// TotalOutputTokens is cumulative output tokens across all assistant messages.
	TotalOutputTokens int

	// RecentToolUse summarizes recent tool activity for handoff context.
	RecentToolUse string
}

// getCurrentSessionUsage finds and parses the current session's transcript.
func getCurrentSessionUsage() (*contextBudgetUsage, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting cwd: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home: %w", err)
	}

	// Convert working directory to Claude's project directory naming
	projectName := strings.ReplaceAll(cwd, "/", "-")
	projectDir := filepath.Join(home, ".claude", "projects", projectName)

	// Find the most recently modified transcript
	transcriptPath, err := findActiveTranscript(projectDir)
	if err != nil {
		return nil, fmt.Errorf("finding transcript: %w", err)
	}

	return parseContextBudgetUsage(transcriptPath)
}

// findActiveTranscript finds the most recently modified .jsonl transcript file.
// Similar to findLatestTranscript in costs.go but scoped to the context budget guard.
func findActiveTranscript(projectDir string) (string, error) {
	var latestPath string
	var latestTime time.Time

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != projectDir {
			return fs.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestPath = path
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if latestPath == "" {
		return "", fmt.Errorf("no transcript files found in %s", projectDir)
	}

	return latestPath, nil
}

// parseContextBudgetUsage reads a transcript and extracts the data needed
// for context budget evaluation.
func parseContextBudgetUsage(transcriptPath string) (*contextBudgetUsage, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	usage := &contextBudgetUsage{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)

	var recentTools []string

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg TranscriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Track assistant messages for token usage
		if msg.Type == "assistant" && msg.Message != nil && msg.Message.Usage != nil {
			// Each assistant message's input_tokens reflects the context window size
			// at that point in the conversation. The last one is the current size.
			usage.LastInputTokens = msg.Message.Usage.InputTokens
			usage.TotalOutputTokens += msg.Message.Usage.OutputTokens
		}

		// Track tool use for recent activity summary
		if msg.Type == "tool_use" {
			// Extract tool name from the message for summary
			var toolMsg struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(line, &toolMsg) == nil && toolMsg.Name != "" {
				recentTools = append(recentTools, toolMsg.Name)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Summarize recent tool activity (last few tools)
	if len(recentTools) > 0 {
		start := len(recentTools) - 5
		if start < 0 {
			start = 0
		}
		usage.RecentToolUse = strings.Join(recentTools[start:], ", ")
	}

	return usage, nil
}
