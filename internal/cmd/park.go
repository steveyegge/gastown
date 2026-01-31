package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

// Park command parks work on a gate, allowing agent to exit safely.
// When the gate closes, waiters are notified and can resume.

var parkCmd = &cobra.Command{
	Use:     "park <gate-id>",
	GroupID: GroupWork,
	Short:   "Park work on a gate for async resumption",
	Long: `Park current work on a gate, allowing the agent to exit safely.

When you need to wait for an external condition (timer, CI, human approval),
park your work on a gate. When the gate closes, you'll receive wake mail.

The park command:
  1. Saves your current hook state (molecule/bead you're working on)
  2. Adds you as a waiter on the gate
  3. Stores context notes in the parked state

After parking, you can exit the session safely. Use 'gt resume' to check
for cleared gates and continue work.

Examples:
  # Create a timer gate and park work on it
  bd gate create --await timer:30m --title "Coffee break"
  gt park <gate-id> -m "Taking a break, will resume auth work"

  # Park on a human approval gate
  bd gate create --await human:deploy-approval --notify ops/
  gt park <gate-id> -m "Deploy staged, awaiting approval"

  # Park on a GitHub Actions gate
  bd gate create --await gh:run:123456789
  gt park <gate-id> -m "Waiting for CI to complete"`,
	Args: cobra.ExactArgs(1),
	RunE: runPark,
}

var (
	parkMessage string
	parkDryRun  bool
)

func init() {
	parkCmd.Flags().StringVarP(&parkMessage, "message", "m", "", "Context notes for resumption")
	parkCmd.Flags().BoolVarP(&parkDryRun, "dry-run", "n", false, "Show what would be done without executing")
	rootCmd.AddCommand(parkCmd)
}

// ParkedWork represents work state saved when parking on a gate.
type ParkedWork struct {
	// AgentID is the agent that parked (e.g., "gastown/crew/max")
	AgentID string `json:"agent_id"`

	// GateID is the gate we're parked on
	GateID string `json:"gate_id"`

	// BeadID is the bead/molecule we were working on
	BeadID string `json:"bead_id,omitempty"`

	// Formula is the formula attached to the work (if any)
	Formula string `json:"formula,omitempty"`

	// Context is additional context notes from the agent
	Context string `json:"context,omitempty"`

	// ParkedAt is when the work was parked
	ParkedAt time.Time `json:"parked_at"`
}

func runPark(cmd *cobra.Command, args []string) error {
	gateID := args[0]

	// Verify gate exists and is open
	gateCheck := exec.Command("bd", "--no-daemon", "gate", "show", gateID, "--json")
	gateOutput, err := gateCheck.Output()
	if err != nil {
		return fmt.Errorf("gate '%s' not found or not accessible", gateID)
	}

	// Parse gate info to verify it's open
	var gateInfo struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(gateOutput, &gateInfo); err != nil {
		return fmt.Errorf("parsing gate info: %w", err)
	}
	if gateInfo.Status == "closed" {
		return fmt.Errorf("gate '%s' is already closed - nothing to park on", gateID)
	}

	// Detect agent identity
	agentID, _, cloneRoot, err := resolveSelfTarget()
	if err != nil {
		return fmt.Errorf("detecting agent identity: %w", err)
	}

	// Read current pinned bead (if any)
	var beadID, formula, hookContext string
	workDir, err := findLocalBeadsDir()
	if err == nil {
		b := beads.New(workDir)
		pinnedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusPinned,
			Assignee: agentID,
			Priority: -1,
		})
		if err == nil && len(pinnedBeads) > 0 {
			beadID = pinnedBeads[0].ID
			// Extract molecule from attachment fields
			if attachment := beads.ParseAttachmentFields(pinnedBeads[0]); attachment != nil {
				formula = attachment.AttachedMolecule
			}
			// Context is part of the bead description, not stored separately
			hookContext = pinnedBeads[0].Description
		}
	}

	// Build context combining hook context and new message
	context := ""
	if hookContext != "" && parkMessage != "" {
		context = hookContext + "\n---\n" + parkMessage
	} else if hookContext != "" {
		context = hookContext
	} else if parkMessage != "" {
		context = parkMessage
	}

	// Create parked work record
	parked := &ParkedWork{
		AgentID:  agentID,
		GateID:   gateID,
		BeadID:   beadID,
		Formula:  formula,
		Context:  context,
		ParkedAt: time.Now(),
	}

	if parkDryRun {
		fmt.Printf("Would park on gate %s\n", gateID)
		fmt.Printf("  Agent: %s\n", agentID)
		if beadID != "" {
			fmt.Printf("  Bead: %s\n", beadID)
		}
		if formula != "" {
			fmt.Printf("  Formula: %s\n", formula)
		}
		if context != "" {
			fmt.Printf("  Context: %s\n", context)
		}
		fmt.Printf("Would add %s as waiter on gate\n", agentID)
		return nil
	}

	// Add agent as waiter on the gate
	waitCmd := exec.Command("bd", "--no-daemon", "gate", "wait", gateID, "--notify", agentID)
	if err := waitCmd.Run(); err != nil {
		// Not fatal - might already be a waiter
		fmt.Printf("%s Note: could not add as waiter (may already be registered)\n", style.Dim.Render("⚠"))
	}

	// Store parked work in a file (alongside hook files)
	parkedPath := parkedWorkPath(cloneRoot, agentID)
	parkedJSON, err := json.MarshalIndent(parked, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling parked work: %w", err)
	}
	if err := os.WriteFile(parkedPath, parkedJSON, 0644); err != nil {
		return fmt.Errorf("writing parked state: %w", err)
	}

	fmt.Printf("%s Parked work on gate %s\n", style.Bold.Render("🅿️"), gateID)
	if beadID != "" {
		fmt.Printf("  Working on: %s\n", beadID)
	}
	if context != "" {
		// Truncate for display
		displayContext := context
		if len(displayContext) > 80 {
			displayContext = displayContext[:77] + "..."
		}
		fmt.Printf("  Context: %s\n", displayContext)
	}
	fmt.Printf("\n%s You can now safely exit. Run 'gt resume' to check for cleared gates.\n",
		style.Dim.Render("→"))

	return nil
}

// parkedWorkPath returns the file path for an agent's parked work state.
func parkedWorkPath(cloneRoot, agentID string) string {
	return filepath.Join(cloneRoot, ".beads", fmt.Sprintf("parked-%s.json", strings.ReplaceAll(agentID, "/", "_")))
}

// readParkedWork reads the parked work state for an agent.
func readParkedWork(cloneRoot, agentID string) (*ParkedWork, error) {
	parkedPath := parkedWorkPath(cloneRoot, agentID)
	data, err := os.ReadFile(parkedPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var parked ParkedWork
	if err := json.Unmarshal(data, &parked); err != nil {
		return nil, err
	}
	return &parked, nil
}

// clearParkedWork removes the parked work state for an agent.
func clearParkedWork(cloneRoot, agentID string) error {
	parkedPath := parkedWorkPath(cloneRoot, agentID)
	err := os.Remove(parkedPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
