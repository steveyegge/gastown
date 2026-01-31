package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
)

// Resume command checks for cleared gates and resumes parked work.

var resumeCmd = &cobra.Command{
	Use:     "resume",
	GroupID: GroupWork,
	Short:   "Resume from parked work or check for handoff messages",
	Long: `Resume work that was parked on a gate, or check for handoff messages.

By default, this command checks for parked work (from 'gt park') and whether
its gate has cleared. If the gate is closed, it restores your work context.

With --handoff, it checks the inbox for handoff messages (messages with
"HANDOFF" in the subject) and displays them formatted for easy continuation.

The resume command:
  1. Checks for parked work state (default) or handoff messages (--handoff)
  2. For parked work: verifies gate has closed
  3. Restores the hook with your previous work
  4. Displays context notes to help you continue

Examples:
  gt resume              # Check for and resume parked work
  gt resume --status     # Just show parked work status without resuming
  gt resume --handoff    # Check inbox for handoff messages`,
	RunE: runResume,
}

var (
	resumeStatusOnly bool
	resumeJSON       bool
	resumeHandoff    bool
)

func init() {
	resumeCmd.Flags().BoolVar(&resumeStatusOnly, "status", false, "Just show parked work status")
	resumeCmd.Flags().BoolVar(&resumeJSON, "json", false, "Output as JSON")
	resumeCmd.Flags().BoolVar(&resumeHandoff, "handoff", false, "Check for handoff messages instead of parked work")
	rootCmd.AddCommand(resumeCmd)
}

// ResumeStatus represents the current resume state.
type ResumeStatus struct {
	HasParkedWork bool        `json:"has_parked_work"`
	ParkedWork    *ParkedWork `json:"parked_work,omitempty"`
	GateClosed    bool        `json:"gate_closed"`
	CloseReason   string      `json:"close_reason,omitempty"`
	CanResume     bool        `json:"can_resume"`
}

func runResume(cmd *cobra.Command, args []string) error {
	// If --handoff flag, check for handoff messages instead
	if resumeHandoff {
		return checkHandoffMessages()
	}

	// Detect agent identity
	agentID, _, cloneRoot, err := resolveSelfTarget()
	if err != nil {
		return fmt.Errorf("detecting agent identity: %w", err)
	}

	// Check for parked work
	parked, err := readParkedWork(cloneRoot, agentID)
	if err != nil {
		return fmt.Errorf("reading parked work: %w", err)
	}

	status := ResumeStatus{
		HasParkedWork: parked != nil,
		ParkedWork:    parked,
	}

	if parked == nil {
		if resumeJSON {
			return outputResumeStatus(status)
		}
		fmt.Printf("%s No parked work found\n", style.Dim.Render("○"))
		fmt.Printf("  Use 'gt park <gate-id>' to park work on a gate\n")
		return nil
	}

	// Check gate status
	gateCheck := exec.Command("bd", "--no-daemon", "gate", "show", parked.GateID, "--json")
	gateOutput, err := gateCheck.Output()
	gateNotFound := false
	if err != nil {
		// Gate might have been deleted (wisp cleanup) or is inaccessible
		// Treat as "gate gone" - allow clearing stale parked work
		gateNotFound = true
		status.GateClosed = true // Treat as closed so user can clear it
		status.CloseReason = "Gate no longer exists (may have been cleaned up)"
	} else {
		var gateInfo struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			CloseReason string `json:"close_reason"`
		}
		if err := json.Unmarshal(gateOutput, &gateInfo); err == nil {
			status.GateClosed = gateInfo.Status == "closed"
			status.CloseReason = gateInfo.CloseReason
		}
	}

	status.CanResume = status.GateClosed

	// Status-only mode
	if resumeStatusOnly {
		if resumeJSON {
			return outputResumeStatus(status)
		}
		return displayResumeStatus(status, parked)
	}

	// JSON output
	if resumeJSON {
		return outputResumeStatus(status)
	}

	// If gate not closed yet, show status and exit
	if !status.GateClosed {
		fmt.Printf("%s Work parked on gate %s (still open)\n",
			style.Bold.Render("🅿️"), parked.GateID)
		if parked.BeadID != "" {
			fmt.Printf("  Working on: %s\n", parked.BeadID)
		}
		fmt.Printf("  Parked at: %s\n", parked.ParkedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("\n%s Gate still open. Check back later or run 'bd gate show %s'\n",
			style.Dim.Render("⏳"), parked.GateID)
		return nil
	}

	// Gate closed - resume work!
	if gateNotFound {
		fmt.Printf("%s Gate %s no longer exists\n", style.Bold.Render("⚠️"), parked.GateID)
		fmt.Printf("  The gate may have been cleaned up. Restoring parked work anyway.\n")
	} else {
		fmt.Printf("%s Gate %s has cleared!\n", style.Bold.Render("🚦"), parked.GateID)
		if status.CloseReason != "" {
			fmt.Printf("  Reason: %s\n", status.CloseReason)
		}
	}

	// Pin the bead to restore work
	if parked.BeadID != "" {
		pinCmd := exec.Command("bd", "--no-daemon", "update", parked.BeadID, "--status=pinned", "--assignee="+agentID)
		pinCmd.Dir = cloneRoot
		pinCmd.Stderr = os.Stderr
		if err := pinCmd.Run(); err != nil {
			return fmt.Errorf("pinning bead: %w", err)
		}

		fmt.Printf("\n%s Restored work: %s\n", style.Bold.Render("📌"), parked.BeadID)
		if parked.Formula != "" {
			fmt.Printf("  Formula: %s\n", parked.Formula)
		}
	}

	// Show context
	if parked.Context != "" {
		fmt.Printf("\n%s Context:\n", style.Bold.Render("📝"))
		fmt.Println(parked.Context)
	}

	// Clear parked work state
	if err := clearParkedWork(cloneRoot, agentID); err != nil {
		// Non-fatal
		style.PrintWarning("could not clear parked state: %v", err)
	}

	fmt.Printf("\n%s Ready to continue!\n", style.Bold.Render("✓"))
	return nil
}

func outputResumeStatus(status ResumeStatus) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func displayResumeStatus(status ResumeStatus, parked *ParkedWork) error {
	if !status.HasParkedWork {
		fmt.Printf("%s No parked work\n", style.Dim.Render("○"))
		return nil
	}

	gateStatus := "open"
	gateIcon := "⏳"
	if status.GateClosed {
		gateStatus = "closed"
		gateIcon = "✓"
	}

	fmt.Printf("%s Parked work status:\n", style.Bold.Render("🅿️"))
	fmt.Printf("  Gate: %s %s (%s)\n", gateIcon, parked.GateID, gateStatus)
	if parked.BeadID != "" {
		fmt.Printf("  Bead: %s\n", parked.BeadID)
	}
	if parked.Formula != "" {
		fmt.Printf("  Formula: %s\n", parked.Formula)
	}
	fmt.Printf("  Parked: %s\n", parked.ParkedAt.Format("2006-01-02 15:04:05"))

	if status.GateClosed {
		fmt.Printf("\n%s Gate cleared! Run 'gt resume' (without --status) to restore work.\n",
			style.Bold.Render("→"))
	}

	return nil
}

// checkHandoffMessages checks the inbox for handoff messages and displays them.
func checkHandoffMessages() error {
	// Get inbox in JSON format
	inboxCmd := exec.Command("gt", "mail", "inbox", "--json")
	output, err := inboxCmd.Output()
	if err != nil {
		// Fallback to non-JSON if --json not supported
		inboxCmd = exec.Command("gt", "mail", "inbox")
		output, err = inboxCmd.Output()
		if err != nil {
			return fmt.Errorf("checking inbox: %w", err)
		}
		// Check for HANDOFF in output
		outputStr := string(output)
		if !containsHandoff(outputStr) {
			fmt.Printf("%s No handoff messages in inbox\n", style.Dim.Render("○"))
			fmt.Printf("  Handoff messages have 'HANDOFF' in the subject.\n")
			return nil
		}
		fmt.Printf("%s Found handoff message(s):\n\n", style.Bold.Render("🤝"))
		fmt.Println(outputStr)
		fmt.Printf("\n%s Read with: gt mail read <id>\n", style.Bold.Render("→"))
		return nil
	}

	// Parse JSON output to find handoff messages
	var messages []struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		From    string `json:"from"`
		Date    string `json:"date"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(output, &messages); err != nil {
		// JSON parse failed, use plain text output
		inboxCmd = exec.Command("gt", "mail", "inbox")
		output, err = inboxCmd.Output()
		if err != nil {
			return fmt.Errorf("fallback inbox check failed: %w", err)
		}
		outputStr := string(output)
		if containsHandoff(outputStr) {
			fmt.Printf("%s Found handoff message(s):\n\n", style.Bold.Render("🤝"))
			fmt.Println(outputStr)
		} else {
			fmt.Printf("%s No handoff messages in inbox\n", style.Dim.Render("○"))
		}
		return nil
	}

	// Find messages with HANDOFF in subject
	type handoffMsg struct {
		ID      string
		Subject string
		From    string
		Date    string
		Body    string
	}
	var handoffs []handoffMsg
	for _, msg := range messages {
		if containsHandoff(msg.Subject) {
			handoffs = append(handoffs, handoffMsg{
				ID:      msg.ID,
				Subject: msg.Subject,
				From:    msg.From,
				Date:    msg.Date,
				Body:    msg.Body,
			})
		}
	}

	if len(handoffs) == 0 {
		fmt.Printf("%s No handoff messages in inbox\n", style.Dim.Render("○"))
		fmt.Printf("  Handoff messages have 'HANDOFF' in the subject.\n")
		fmt.Printf("  Use 'gt handoff -s \"...\"' to create one when handing off.\n")
		return nil
	}

	fmt.Printf("%s Found %d handoff message(s):\n\n", style.Bold.Render("🤝"), len(handoffs))

	for i, msg := range handoffs {
		fmt.Printf("--- Handoff %d: %s ---\n", i+1, msg.ID)
		fmt.Printf("Subject: %s\n", msg.Subject)
		fmt.Printf("From: %s\n", msg.From)
		if msg.Date != "" {
			fmt.Printf("Date: %s\n", msg.Date)
		}
		if msg.Body != "" {
			fmt.Printf("\n%s\n", msg.Body)
		}
		fmt.Println()
	}

	if len(handoffs) == 1 {
		fmt.Printf("%s Read full message: gt mail read %s\n", style.Bold.Render("→"), handoffs[0].ID)
	} else {
		fmt.Printf("%s Read messages: gt mail read <id>\n", style.Bold.Render("→"))
	}
	fmt.Printf("%s Clear after reading: gt mail close <id>\n", style.Dim.Render("💡"))

	return nil
}

// containsHandoff checks if a string contains "HANDOFF" (case-insensitive).
func containsHandoff(s string) bool {
	upper := strings.ToUpper(s)
	return strings.Contains(upper, "HANDOFF")
}
