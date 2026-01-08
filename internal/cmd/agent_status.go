package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	agentStatusJSON  bool
	agentStatusClear bool
)

var agentStatusCmd = &cobra.Command{
	Use:   "status <agent-bead> [message]",
	Short: "Get or set status message on agent beads",
	Long: `Get or set the human-readable status message on agent beads.

The status message describes what an agent is currently doing and
appears in 'gt status' output to provide visibility into agent activity.

USAGE:
  Get current status:
    gt agent status <agent-bead>

  Set status message:
    gt agent status <agent-bead> "Working on gt-dash4"
    gt agent status <agent-bead> "Running tests for feature X"

  Clear status message:
    gt agent status <agent-bead> --clear

EXAMPLES:
  # Check what a polecat is doing
  gt agent status gt-gastown-polecat-morsov

  # Set your status (as a working polecat)
  gt agent status gt-gastown-polecat-morsov "Implementing status messages"

  # Clear status when idle
  gt agent status gt-gastown-polecat-morsov --clear`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runAgentStatus,
}

func init() {
	agentStatusCmd.Flags().BoolVar(&agentStatusJSON, "json", false,
		"Output as JSON")
	agentStatusCmd.Flags().BoolVar(&agentStatusClear, "clear", false,
		"Clear the status message")

	// Add as subcommand of agents
	agentsCmd.AddCommand(agentStatusCmd)
}

// agentStatusResult holds the status query result.
type agentStatusResult struct {
	AgentBead     string `json:"agent_bead"`
	StatusMessage string `json:"status_message"`
}

func runAgentStatus(cmd *cobra.Command, args []string) error {
	agentBead := args[0]

	// Find beads directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	beadsDir := beads.ResolveBeadsDir(cwd)
	if beadsDir == "" {
		return fmt.Errorf("not in a beads workspace")
	}

	// Determine operation mode
	hasMessage := len(args) > 1
	if agentStatusClear {
		// Clear mode
		return setAgentStatusMessage(agentBead, beadsDir, "")
	}
	if hasMessage {
		// Set mode
		message := strings.Join(args[1:], " ")
		return setAgentStatusMessage(agentBead, beadsDir, message)
	}

	// Query mode
	return queryAgentStatus(agentBead, beadsDir)
}

// queryAgentStatus retrieves and displays the status message from an agent bead.
func queryAgentStatus(agentBead, beadsDir string) error {
	b := beads.NewWithBeadsDir("", beadsDir)

	issue, fields, err := b.GetAgentBead(agentBead)
	if err != nil {
		return fmt.Errorf("getting agent bead: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("agent bead not found: %s", agentBead)
	}

	statusMessage := ""
	if fields != nil {
		statusMessage = fields.StatusMessage
	}

	result := &agentStatusResult{
		AgentBead:     agentBead,
		StatusMessage: statusMessage,
	}

	if agentStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Human-readable output
	fmt.Printf("%s Agent: %s\n", style.Bold.Render("📋"), agentBead)

	if statusMessage == "" {
		fmt.Printf("  Status: %s\n", style.Dim.Render("(none)"))
	} else {
		fmt.Printf("  Status: %s\n", style.Info.Render(statusMessage))
	}

	return nil
}

// setAgentStatusMessage updates the status message on an agent bead.
func setAgentStatusMessage(agentBead, beadsDir, message string) error {
	b := beads.NewWithBeadsDir("", beadsDir)

	if err := b.UpdateAgentStatusMessage(agentBead, message); err != nil {
		return fmt.Errorf("updating status message: %w", err)
	}

	if message == "" {
		fmt.Printf("%s Cleared status for %s\n", style.Bold.Render("✓"), agentBead)
	} else {
		fmt.Printf("%s Updated status for %s: %s\n", style.Bold.Render("✓"), agentBead, style.Info.Render(message))
	}

	return nil
}
