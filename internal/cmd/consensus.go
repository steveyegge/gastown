package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/consensus"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

var (
	consensusTimeout  time.Duration
	consensusJSON     bool
	consensusDryRun   bool
	consensusSessions []string
)

func init() {
	consensusCmd.Flags().DurationVar(&consensusTimeout, "timeout", 5*time.Minute, "Per-session wait timeout")
	consensusCmd.Flags().BoolVar(&consensusJSON, "json", false, "Output results as JSON")
	consensusCmd.Flags().BoolVar(&consensusDryRun, "dry-run", false, "Show target sessions without sending")
	consensusCmd.Flags().StringSliceVar(&consensusSessions, "session", nil, "Target specific sessions (repeatable)")
	rootCmd.AddCommand(consensusCmd)
}

var consensusCmd = &cobra.Command{
	Use:     "consensus <prompt>",
	Aliases: []string{"fanout"},
	GroupID: GroupWork,
	Short:   "Fan-out a prompt to multiple sessions and collect responses",
	Long: `Send the same prompt to multiple AI agent sessions in parallel
and collect their responses for comparison. Supports Claude, Gemini,
Codex, and other providers via GT_AGENT session detection.

By default, targets all idle crew and polecat sessions.
Use --session to target specific sessions.

Examples:
  gt consensus "What time is it?"
  gt consensus --timeout 10m "Summarize the current PR"
  gt consensus --session gt-crew-bear --session gt-crew-cat "test prompt"
  gt consensus --dry-run "show targets"
  gt consensus --json "prompt" | jq .`,
	Args: cobra.ExactArgs(1),
	RunE: runConsensus,
}

func runConsensus(cmd *cobra.Command, args []string) error {
	prompt := args[0]
	if prompt == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	// Resolve target sessions.
	sessions, err := resolveConsensusSessions()
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No idle sessions found to target.")
		return nil
	}

	// Dry run: show what would be targeted with provider info.
	if consensusDryRun {
		t := tmux.NewTmux()
		fmt.Printf("Would fan-out to %d session(s):\n\n", len(sessions))
		for _, s := range sessions {
			agent, err := t.GetEnvironment(s, "GT_AGENT")
			if err != nil || agent == "" {
				agent = "claude"
			}
			detection := "prompt"
			if preset := config.GetAgentPresetByName(agent); preset != nil {
				if preset.ReadyPromptPrefix == "" {
					detection = fmt.Sprintf("delay (%dms)", preset.ReadyDelayMs)
				}
			}
			fmt.Printf("  %-30s [%s, %s]\n", s, agent, detection)
		}
		fmt.Printf("\nPrompt: %s\n", prompt)
		fmt.Printf("Timeout: %s\n", consensusTimeout)
		return nil
	}

	// Run the consensus.
	t := tmux.NewTmux()
	runner := consensus.NewRunner(t)

	fmt.Printf("Sending prompt to %d session(s)...\n\n", len(sessions))

	result := runner.Run(consensus.Request{
		Prompt:   prompt,
		Sessions: sessions,
		Timeout:  consensusTimeout,
	})

	// Output results.
	if consensusJSON {
		return outputConsensusJSON(result)
	}
	outputConsensusText(result)
	return nil
}

// resolveConsensusSessions determines which sessions to target.
func resolveConsensusSessions() ([]string, error) {
	// If explicit sessions specified, use those (check idle status).
	if len(consensusSessions) > 0 {
		t := tmux.NewTmux()
		var idle []string
		for _, s := range consensusSessions {
			if t.IsIdle(s) {
				idle = append(idle, s)
			} else {
				fmt.Printf("%s %s (not idle, skipping)\n", style.WarningPrefix, s)
			}
		}
		return idle, nil
	}

	// Default: all idle crew + polecat sessions.
	agents, err := getAgentSessions(true) // include polecats
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	// Exclude self.
	self := os.Getenv("BD_ACTOR")

	t := tmux.NewTmux()
	var sessions []string
	for _, agent := range agents {
		if agent.Type != AgentCrew && agent.Type != AgentPolecat {
			continue
		}
		name := formatAgentName(agent)
		if self != "" && name == self {
			continue
		}
		if t.IsIdle(agent.Name) {
			sessions = append(sessions, agent.Name)
		}
	}
	return sessions, nil
}

func outputConsensusJSON(result *consensus.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputConsensusText(result *consensus.Result) {
	for i, sr := range result.Sessions {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 60))
		}

		statusIcon := style.SuccessPrefix
		switch sr.Status {
		case consensus.StatusTimeout:
			statusIcon = style.WarningPrefix
		case consensus.StatusError, consensus.StatusRateLimited:
			statusIcon = style.ErrorPrefix
		case consensus.StatusNotIdle:
			statusIcon = style.WarningPrefix
		}

		providerLabel := ""
		if sr.Provider != "" {
			providerLabel = fmt.Sprintf(" [%s]", sr.Provider)
		}
		fmt.Printf("%s %s%s  (%s, %s)\n", statusIcon, sr.Session, providerLabel, sr.Status, sr.Duration.Round(time.Millisecond))

		if sr.Error != "" {
			fmt.Printf("  Error: %s\n", sr.Error)
		}
		if sr.Response != "" {
			fmt.Println()
			// Indent response lines for readability.
			for _, line := range strings.Split(sr.Response, "\n") {
				fmt.Printf("  %s\n", line)
			}
			fmt.Println()
		}
	}

	fmt.Printf("\nTotal: %d session(s), %s\n", len(result.Sessions), result.Duration.Round(time.Millisecond))
}
