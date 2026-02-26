package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Default context budget thresholds (fraction of max context window).
const (
	defaultWarnThreshold    = 0.75
	defaultSoftGate         = 0.85
	defaultHardGate         = 0.92
	defaultMaxContextTokens = 200_000
)

var tapGuardContextBudgetCmd = &cobra.Command{
	Use:   "context-budget",
	Short: "Monitor context window usage and enforce handoff thresholds",
	Long: `Monitor Claude Code context window consumption and enforce automatic
handoff when thresholds are reached.

This guard reads the active session's transcript file to track token
usage and enforces configurable thresholds:

  75% (warn)      - Injects reminder to consider handoff
  85% (soft gate) - Stronger reminder with specific handoff instructions
  92% (hard gate) - Blocks tool calls for mayor/deacon/witness/refinery roles

Configuration via environment variables:
  GT_CONTEXT_BUDGET_WARN       - Warning threshold (default: 0.75)
  GT_CONTEXT_BUDGET_SOFT_GATE  - Soft gate threshold (default: 0.85)
  GT_CONTEXT_BUDGET_HARD_GATE  - Hard gate threshold (default: 0.92)
  GT_CONTEXT_BUDGET_MAX_TOKENS - Max context tokens (default: 200000)
  GT_CONTEXT_BUDGET_DISABLE    - Set to "1" to disable the guard entirely

Hard-gated roles (tool calls blocked at hard gate): mayor, deacon, witness, refinery
Warn-only roles (never blocked): polecat, crew

Exit codes:
  0 - Operation allowed
  2 - Operation BLOCKED (hard gate exceeded for hard-gated role)`,
	RunE: runTapGuardContextBudget,
}

func init() {
	tapGuardCmd.AddCommand(tapGuardContextBudgetCmd)
}

// contextBudgetConfig holds resolved configuration for the guard.
type contextBudgetConfig struct {
	WarnThreshold float64
	SoftGate      float64
	HardGate      float64
	MaxTokens     int
	HardGated     bool // true for mayor/deacon/witness/refinery and unknown roles
}

func loadContextBudgetConfig() *contextBudgetConfig {
	cfg := &contextBudgetConfig{
		WarnThreshold: defaultWarnThreshold,
		SoftGate:      defaultSoftGate,
		HardGate:      defaultHardGate,
		MaxTokens:     defaultMaxContextTokens,
	}

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

	// Polecat and crew are warn-only; all other roles (including unknown) get hard-gated.
	role := detectCurrentRole()
	cfg.HardGated = role != "polecat" && role != "crew"

	return cfg
}

// detectCurrentRole returns the current Gas Town role from environment variables.
func detectCurrentRole() string {
	if v := os.Getenv("GT_ROLE"); v != "" {
		return strings.ToLower(v)
	}
	switch {
	case os.Getenv("GT_POLECAT") != "":
		return "polecat"
	case os.Getenv("GT_CREW") != "":
		return "crew"
	case os.Getenv("GT_MAYOR") != "":
		return "mayor"
	case os.Getenv("GT_DEACON") != "":
		return "deacon"
	case os.Getenv("GT_WITNESS") != "":
		return "witness"
	case os.Getenv("GT_REFINERY") != "":
		return "refinery"
	}
	return ""
}

func runTapGuardContextBudget(cmd *cobra.Command, args []string) error {
	if os.Getenv("GT_CONTEXT_BUDGET_DISABLE") == "1" {
		return nil
	}

	cfg := loadContextBudgetConfig()

	cwd, err := os.Getwd()
	if err != nil {
		return nil // fail open
	}
	projectDir, err := getClaudeProjectDir(cwd)
	if err != nil {
		return nil
	}
	transcriptPath, err := findLatestTranscript(projectDir)
	if err != nil {
		return nil
	}

	lastInputTokens, err := parseLastInputTokens(transcriptPath)
	if err != nil || lastInputTokens == 0 {
		return nil
	}

	ratio := float64(lastInputTokens) / float64(cfg.MaxTokens)
	pct := int(ratio * 100)
	used := lastInputTokens / 1000
	max := cfg.MaxTokens / 1000

	switch {
	case ratio >= cfg.HardGate:
		fmt.Fprintf(os.Stderr, "\n🛑 CONTEXT BUDGET EXCEEDED (%d%%) — %dk/%dk tokens\n", pct, used, max)
		fmt.Fprintln(os.Stderr, "   You MUST hand off remaining work NOW.")
		fmt.Fprintln(os.Stderr, "   Run: gt handoff -s \"Context exhausted\" -m \"<remaining work>\"")
		fmt.Fprintln(os.Stderr, "   Or:  gt done  (if work is complete)")
		fmt.Fprintln(os.Stderr, "")
		if cfg.HardGated {
			return NewSilentExit(2)
		}

	case ratio >= cfg.SoftGate:
		fmt.Fprintf(os.Stderr, "\n⚠️  CONTEXT BUDGET AT %d%% — HANDOFF RECOMMENDED — %dk/%dk tokens\n", pct, used, max)
		fmt.Fprintln(os.Stderr, "   Run: gt handoff -s \"Context budget\" -m \"<what remains>\"")
		fmt.Fprintln(os.Stderr, "   Or:  gt done  (if work is complete)")
		fmt.Fprintln(os.Stderr, "")

	case ratio >= cfg.WarnThreshold:
		remaining := (cfg.MaxTokens - lastInputTokens) / 1000
		fmt.Fprintf(os.Stderr, "\n⚠️  Context budget at %d%% (%dk/%dk tokens, ~%dk remaining)\n", pct, used, max, remaining)
		fmt.Fprintln(os.Stderr, "   Consider using gt handoff to pass remaining work to a fresh session.")
		fmt.Fprintln(os.Stderr, "")
	}

	return nil
}

// parseLastInputTokens reads a transcript and returns the input_tokens from
// the last assistant message, which is the best proxy for current context size.
func parseLastInputTokens(transcriptPath string) (int, error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)

	var last int
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg TranscriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type == "assistant" && msg.Message != nil && msg.Message.Usage != nil {
			last = msg.Message.Usage.InputTokens
		}
	}
	return last, scanner.Err()
}
