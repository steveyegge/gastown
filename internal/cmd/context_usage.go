package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// contextWarnThresholdPct is the percentage at which context is "WARN".
	// Below this: exit 0 (LOW — safe to continue).
	// At or above: exit 1 (WARN — prepare for handoff soon).
	contextWarnThresholdPct = 65.0

	// contextCriticalThresholdPct is the percentage at which context is "CRITICAL".
	// At or above: exit 2 (CRITICAL — immediate handoff required).
	contextCriticalThresholdPct = 80.0

	// defaultContextWindowTokens is the context window size used when the model
	// cannot be determined. All modern Claude models (claude-3.5+, claude-4.x)
	// have 200k-token context windows.
	defaultContextWindowTokens = 200_000
)

var contextUsageFlag bool

var contextCmd = &cobra.Command{
	Use:     "context",
	GroupID: GroupDiag,
	Short:   "Check Claude session context window usage",
	Long: `Check the current Claude session's context window usage.

Reads the most recent Claude Code transcript (JSONL) to determine how much
of the context window is currently in use, based on the last assistant turn's
reported token counts (input + cache_read + cache_creation).

With --usage, also signals the usage level via exit code:

  Exit 0: LOW     — context < 65%, safe to continue
  Exit 1: WARN    — context 65-80%, prepare for handoff
  Exit 2: CRITICAL — context >= 80%, immediate handoff required

Example (in deacon patrol formula):

  gt context --usage
  # exit 0 → keep working
  # exit 1 or 2 → initiate handoff`,
	RunE: runContextUsage,
}

func init() {
	contextCmd.Flags().BoolVar(&contextUsageFlag, "usage", false,
		"Output usage percentage and exit 1 (WARN ≥65%) or 2 (CRITICAL ≥80%)")
	rootCmd.AddCommand(contextCmd)
}

func runContextUsage(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	projectDir, err := getClaudeProjectDir(workDir)
	if err != nil {
		return fmt.Errorf("finding Claude project dir: %w", err)
	}

	transcriptPath, err := findLatestTranscript(projectDir)
	if err != nil {
		// No transcript found — report 0% and exit cleanly.
		fmt.Println("0.0% (no transcript found)")
		return nil
	}

	model, totalTokens, err := readLastTurnContext(transcriptPath)
	if err != nil {
		return fmt.Errorf("reading transcript: %w", err)
	}

	windowSize := contextWindowForModel(model)
	percentage := float64(totalTokens) / float64(windowSize) * 100.0

	usedK := totalTokens / 1000
	windowK := windowSize / 1000

	level := "LOW"
	switch {
	case percentage >= contextCriticalThresholdPct:
		level = "CRITICAL"
	case percentage >= contextWarnThresholdPct:
		level = "WARN"
	}

	fmt.Printf("%.1f%% (%dk/%dk) [%s] — %s\n", percentage, usedK, windowK, model, level)

	if contextUsageFlag {
		switch level {
		case "CRITICAL":
			os.Exit(2)
		case "WARN":
			os.Exit(1)
		}
	}

	return nil
}

// readLastTurnContext reads the last assistant message in a transcript and returns
// the model name and total context tokens consumed in that turn
// (input_tokens + cache_read_input_tokens + cache_creation_input_tokens).
//
// The last turn's input token count reflects the full current conversation size
// sent to the API, making it a reliable proxy for context window usage.
func readLastTurnContext(transcriptPath string) (model string, totalTokens int, err error) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)

	// contextEntry is a minimal struct for parsing JSONL lines.
	type contextEntry struct {
		Type    string `json:"type"`
		Message *struct {
			Model string `json:"model"`
			Usage *struct {
				InputTokens              int `json:"input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}

	var lastModel string
	var lastTotal int

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry contextEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" || entry.Message == nil || entry.Message.Usage == nil {
			continue
		}

		u := entry.Message.Usage
		total := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		if total == 0 {
			continue
		}

		if entry.Message.Model != "" {
			lastModel = entry.Message.Model
		}
		lastTotal = total
	}

	if err := scanner.Err(); err != nil {
		return "", 0, err
	}

	return lastModel, lastTotal, nil
}

// contextWindowForModel returns the context window token limit for a given model.
// All current Claude models (claude-3.5-haiku, claude-3.5-sonnet, claude-haiku-4-5,
// claude-sonnet-4-6, claude-opus-4-6, etc.) have a 200k-token context window.
func contextWindowForModel(model string) int {
	if strings.HasPrefix(model, "claude-") {
		return defaultContextWindowTokens
	}
	return defaultContextWindowTokens
}
