package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Gate command provides gt wrappers for gate operations.
// Most gate commands are in beads (bd gate ...), but gt provides
// integration with the Gas Town mail system for wake notifications.

var gateCmd = &cobra.Command{
	Use:     "gate",
	GroupID: GroupWork,
	Short:   "Gate coordination commands",
	Long: `Gate commands for async coordination.

Most gate commands are in beads:
  bd gate create   - Create a gate (timer, gh:run, human, mail)
  bd gate show     - Show gate details
  bd gate list     - List open gates
  bd gate close    - Close a gate
  bd gate approve  - Approve a human gate
  bd gate eval     - Evaluate and close elapsed gates

The gt gate command provides Gas Town integration:
  gt gate wake     - Send wake mail to gate waiters after close`,
}

var gateWakeCmd = &cobra.Command{
	Use:   "wake <gate-id>",
	Short: "Send wake mail to gate waiters",
	Long: `Send wake mail to all waiters on a gate.

This command should be called after a gate closes to notify waiting agents.
Typically called by Deacon after 'bd gate eval' or after manual gate close.

The wake mail includes:
  - Gate ID and close reason
  - Instructions to run 'gt resume'

Examples:
  # After manual gate close
  bd gate close gt-xxx --reason "Approved"
  gt gate wake gt-xxx

  # In Deacon patrol after gate eval
  for gate in $(bd gate eval --json | jq -r '.closed[]'); do
    gt gate wake $gate
  done`,
	Args: cobra.ExactArgs(1),
	RunE: runGateWake,
}

var (
	gateWakeJSON   bool
	gateWakeDryRun bool
)

func init() {
	gateWakeCmd.Flags().BoolVar(&gateWakeJSON, "json", false, "Output as JSON")
	gateWakeCmd.Flags().BoolVarP(&gateWakeDryRun, "dry-run", "n", false, "Show what would be done")

	gateCmd.AddCommand(gateWakeCmd)
	rootCmd.AddCommand(gateCmd)
}

// GateWakeResult represents the result of sending wake mail.
type GateWakeResult struct {
	GateID      string   `json:"gate_id"`
	CloseReason string   `json:"close_reason"`
	Waiters     []string `json:"waiters"`
	Notified    []string `json:"notified"`
	Failed      []string `json:"failed,omitempty"`
}

func runGateWake(cmd *cobra.Command, args []string) error {
	gateID := args[0]

	// Get gate info
	gateCheck := exec.Command("bd", "--no-daemon", "gate", "show", gateID, "--json")
	gateOutput, err := gateCheck.Output()
	if err != nil {
		return fmt.Errorf("gate '%s' not found or not accessible", gateID)
	}

	var gateInfo struct {
		ID          string   `json:"id"`
		Status      string   `json:"status"`
		CloseReason string   `json:"close_reason"`
		Waiters     []string `json:"waiters"`
	}
	if err := json.Unmarshal(gateOutput, &gateInfo); err != nil {
		return fmt.Errorf("parsing gate info: %w", err)
	}

	if gateInfo.Status != "closed" {
		return fmt.Errorf("gate '%s' is not closed (status: %s) - wake mail only sent for closed gates", gateID, gateInfo.Status)
	}

	if len(gateInfo.Waiters) == 0 {
		if gateWakeJSON {
			result := GateWakeResult{
				GateID:      gateID,
				CloseReason: gateInfo.CloseReason,
				Waiters:     []string{},
				Notified:    []string{},
			}
			return outputGateWakeResult(result)
		}
		fmt.Printf("%s Gate %s has no waiters to notify\n", style.Dim.Render("○"), gateID)
		return nil
	}

	if gateWakeDryRun {
		fmt.Printf("Would send wake mail for gate %s to:\n", gateID)
		for _, w := range gateInfo.Waiters {
			fmt.Printf("  - %s\n", w)
		}
		return nil
	}

	// Find town root for mail routing
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	router := mail.NewRouter(townRoot)

	result := GateWakeResult{
		GateID:      gateID,
		CloseReason: gateInfo.CloseReason,
		Waiters:     gateInfo.Waiters,
		Notified:    []string{},
		Failed:      []string{},
	}

	subject := fmt.Sprintf("🚦 GATE CLEARED: %s", gateID)
	body := fmt.Sprintf("Gate %s has closed.\n\nReason: %s\n\nRun 'gt resume' to continue your parked work.",
		gateID, gateInfo.CloseReason)

	for _, waiter := range gateInfo.Waiters {
		msg := &mail.Message{
			From:     "deacon/",
			To:       waiter,
			Subject:  subject,
			Body:     body,
			Type:     mail.TypeNotification,
			Priority: mail.PriorityHigh,
			Wisp:     true,
		}
		if err := router.Send(msg); err != nil {
			result.Failed = append(result.Failed, waiter)
		} else {
			result.Notified = append(result.Notified, waiter)
		}
	}

	if gateWakeJSON {
		return outputGateWakeResult(result)
	}

	fmt.Printf("%s Sent wake mail for gate %s\n", style.Bold.Render("🚦"), gateID)
	if len(result.Notified) > 0 {
		fmt.Printf("  Notified: %v\n", result.Notified)
	}
	if len(result.Failed) > 0 {
		fmt.Printf("  Failed: %v\n", result.Failed)
	}

	return nil
}

func outputGateWakeResult(result GateWakeResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
