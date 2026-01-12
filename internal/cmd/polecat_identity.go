package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Polecat identity command flags
var (
	polecatIdentityListJSON   bool
	polecatIdentityShowJSON   bool
	polecatIdentityRemoveForce bool
)

var polecatIdentityCmd = &cobra.Command{
	Use:     "identity",
	Aliases: []string{"id"},
	Short:   "Manage polecat identities",
	Long: `Manage polecat identity beads in rigs.

Identity beads track polecat metadata, CV history, and lifecycle state.
Use subcommands to create, list, show, rename, or remove identities.`,
	RunE: requireSubcommand,
}

var polecatIdentityAddCmd = &cobra.Command{
	Use:   "add <rig> [name]",
	Short: "Create an identity bead for a polecat",
	Long: `Create an identity bead for a polecat in a rig.

If name is not provided, a name will be generated from the rig's name pool.

The identity bead tracks:
  - Role type (polecat)
  - Rig assignment
  - Agent state
  - Hook bead (current work)
  - Cleanup status

Example:
  gt polecat identity add gastown Toast
  gt polecat identity add gastown  # auto-generate name`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPolecatIdentityAdd,
}

var polecatIdentityListCmd = &cobra.Command{
	Use:   "list <rig>",
	Short: "List polecat identity beads in a rig",
	Long: `List all polecat identity beads in a rig.

Shows:
  - Polecat name
  - Agent state
  - Current hook (if any)
  - Whether worktree exists

Example:
  gt polecat identity list gastown
  gt polecat identity list gastown --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatIdentityList,
}

var polecatIdentityShowCmd = &cobra.Command{
	Use:   "show <rig> <name>",
	Short: "Show identity bead details and CV summary",
	Long: `Show detailed identity bead information for a polecat.

Displays:
  - Identity bead fields
  - CV history (past work)
  - Current hook bead details

Example:
  gt polecat identity show gastown Toast
  gt polecat identity show gastown Toast --json`,
	Args: cobra.ExactArgs(2),
	RunE: runPolecatIdentityShow,
}

var polecatIdentityRenameCmd = &cobra.Command{
	Use:   "rename <rig> <old-name> <new-name>",
	Short: "Rename a polecat identity (preserves CV)",
	Long: `Rename a polecat identity bead, preserving CV history.

The rename:
  1. Creates a new identity bead with the new name
  2. Copies CV history links to the new bead
  3. Closes the old bead with a reference to the new one

Safety checks:
  - Old identity must exist
  - New name must not already exist
  - Polecat session must not be running

Example:
  gt polecat identity rename gastown Toast Imperator`,
	Args: cobra.ExactArgs(3),
	RunE: runPolecatIdentityRename,
}

var polecatIdentityRemoveCmd = &cobra.Command{
	Use:   "remove <rig> <name>",
	Short: "Remove a polecat identity",
	Long: `Remove a polecat identity bead.

Safety checks:
  - No active tmux session
  - No work on hook (unless using --force)
  - Warns if CV exists

Use --force to bypass safety checks.

Example:
  gt polecat identity remove gastown Toast
  gt polecat identity remove gastown Toast --force`,
	Args: cobra.ExactArgs(2),
	RunE: runPolecatIdentityRemove,
}

func init() {
	// List flags
	polecatIdentityListCmd.Flags().BoolVar(&polecatIdentityListJSON, "json", false, "Output as JSON")

	// Show flags
	polecatIdentityShowCmd.Flags().BoolVar(&polecatIdentityShowJSON, "json", false, "Output as JSON")

	// Remove flags
	polecatIdentityRemoveCmd.Flags().BoolVarP(&polecatIdentityRemoveForce, "force", "f", false, "Force removal, bypassing safety checks")

	// Add subcommands to identity
	polecatIdentityCmd.AddCommand(polecatIdentityAddCmd)
	polecatIdentityCmd.AddCommand(polecatIdentityListCmd)
	polecatIdentityCmd.AddCommand(polecatIdentityShowCmd)
	polecatIdentityCmd.AddCommand(polecatIdentityRenameCmd)
	polecatIdentityCmd.AddCommand(polecatIdentityRemoveCmd)

	// Add identity to polecat command
	polecatCmd.AddCommand(polecatIdentityCmd)
}

// IdentityInfo holds identity bead information for display.
type IdentityInfo struct {
	Rig            string `json:"rig"`
	Name           string `json:"name"`
	BeadID         string `json:"bead_id"`
	AgentState     string `json:"agent_state,omitempty"`
	HookBead       string `json:"hook_bead,omitempty"`
	CleanupStatus  string `json:"cleanup_status,omitempty"`
	WorktreeExists bool   `json:"worktree_exists"`
	SessionRunning bool   `json:"session_running"`
}

func runPolecatIdentityAdd(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	var polecatName string

	if len(args) > 1 {
		polecatName = args[1]
	}

	// Get rig
	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Generate name if not provided
	if polecatName == "" {
		polecatGit := git.NewGit(r.Path)
		mgr := polecat.NewManager(r, polecatGit)
		polecatName, err = mgr.AllocateName()
		if err != nil {
			return fmt.Errorf("generating polecat name: %w", err)
		}
		fmt.Printf("Generated name: %s\n", polecatName)
	}

	// Check if identity already exists
	bd := beads.New(r.Path)
	beadID := beads.PolecatBeadID(rigName, polecatName)
	existingIssue, _, _ := bd.GetAgentBead(beadID)
	if existingIssue != nil && existingIssue.Status != "closed" {
		return fmt.Errorf("identity bead %s already exists", beadID)
	}

	// Create identity bead
	fields := &beads.AgentFields{
		RoleType:   "polecat",
		Rig:        rigName,
		AgentState: "idle",
	}

	title := fmt.Sprintf("Polecat %s in %s", polecatName, rigName)
	issue, err := bd.CreateOrReopenAgentBead(beadID, title, fields)
	if err != nil {
		return fmt.Errorf("creating identity bead: %w", err)
	}

	fmt.Printf("%s Created identity bead: %s\n", style.SuccessPrefix, issue.ID)
	fmt.Printf("  Polecat: %s\n", polecatName)
	fmt.Printf("  Rig:     %s\n", rigName)

	return nil
}

func runPolecatIdentityList(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	// Get rig
	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Get all agent beads
	bd := beads.New(r.Path)
	agentBeads, err := bd.ListAgentBeads()
	if err != nil {
		return fmt.Errorf("listing agent beads: %w", err)
	}

	// Filter for polecat beads in this rig
	identities := []IdentityInfo{} // Initialize to empty slice (not nil) for JSON
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)

	for id, issue := range agentBeads {
		// Parse the bead ID to check if it's a polecat for this rig
		beadRig, role, name, ok := beads.ParseAgentBeadID(id)
		if !ok || role != "polecat" || beadRig != rigName {
			continue
		}

		// Skip closed beads
		if issue.Status == "closed" {
			continue
		}

		fields := beads.ParseAgentFields(issue.Description)

		// Check if worktree exists
		worktreeExists := false
		mgr := polecat.NewManager(r, nil)
		if p, err := mgr.Get(name); err == nil && p != nil {
			worktreeExists = true
		}

		// Check if session is running
		sessionRunning, _ := polecatMgr.IsRunning(name)

		info := IdentityInfo{
			Rig:            rigName,
			Name:           name,
			BeadID:         id,
			AgentState:     fields.AgentState,
			HookBead:       issue.HookBead,
			CleanupStatus:  fields.CleanupStatus,
			WorktreeExists: worktreeExists,
			SessionRunning: sessionRunning,
		}
		if info.HookBead == "" {
			info.HookBead = fields.HookBead
		}
		identities = append(identities, info)
	}

	// JSON output
	if polecatIdentityListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(identities)
	}

	// Human-readable output
	if len(identities) == 0 {
		fmt.Printf("No polecat identities found in %s.\n", rigName)
		return nil
	}

	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("Polecat Identities in %s", rigName)))

	for _, info := range identities {
		// Status indicators
		sessionIcon := style.Dim.Render("○")
		if info.SessionRunning {
			sessionIcon = style.Success.Render("●")
		}

		worktreeIcon := ""
		if info.WorktreeExists {
			worktreeIcon = " " + style.Dim.Render("[worktree]")
		}

		// Agent state with color
		stateStr := info.AgentState
		if stateStr == "" {
			stateStr = "unknown"
		}
		switch stateStr {
		case "working":
			stateStr = style.Info.Render(stateStr)
		case "done":
			stateStr = style.Success.Render(stateStr)
		case "stuck":
			stateStr = style.Warning.Render(stateStr)
		default:
			stateStr = style.Dim.Render(stateStr)
		}

		fmt.Printf("  %s %s  %s%s\n", sessionIcon, style.Bold.Render(info.Name), stateStr, worktreeIcon)

		if info.HookBead != "" {
			fmt.Printf("    Hook: %s\n", style.Dim.Render(info.HookBead))
		}
	}

	fmt.Printf("\n%d identity bead(s)\n", len(identities))
	return nil
}

// IdentityDetails holds detailed identity information for show command.
type IdentityDetails struct {
	IdentityInfo
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	CVBeads     []string `json:"cv_beads,omitempty"`
}

func runPolecatIdentityShow(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	polecatName := args[1]

	// Get rig
	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	// Get identity bead
	bd := beads.New(r.Path)
	beadID := beads.PolecatBeadID(rigName, polecatName)
	issue, fields, err := bd.GetAgentBead(beadID)
	if err != nil {
		return fmt.Errorf("getting identity bead: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("identity bead %s not found", beadID)
	}

	// Check worktree and session
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)
	mgr := polecat.NewManager(r, nil)

	worktreeExists := false
	if p, err := mgr.Get(polecatName); err == nil && p != nil {
		worktreeExists = true
	}
	sessionRunning, _ := polecatMgr.IsRunning(polecatName)

	// Build details
	details := IdentityDetails{
		IdentityInfo: IdentityInfo{
			Rig:            rigName,
			Name:           polecatName,
			BeadID:         beadID,
			AgentState:     fields.AgentState,
			HookBead:       issue.HookBead,
			CleanupStatus:  fields.CleanupStatus,
			WorktreeExists: worktreeExists,
			SessionRunning: sessionRunning,
		},
		Title:     issue.Title,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
	}
	if details.HookBead == "" {
		details.HookBead = fields.HookBead
	}

	// Get CV beads (work history) - beads that were assigned to this polecat
	// Assignee format is "rig/name" (e.g., "gastown/Toast")
	assignee := fmt.Sprintf("%s/%s", rigName, polecatName)
	cvBeads, _ := bd.ListByAssignee(assignee)
	for _, cv := range cvBeads {
		if cv.ID != beadID && cv.Status == "closed" {
			details.CVBeads = append(details.CVBeads, cv.ID)
		}
	}

	// JSON output
	if polecatIdentityShowJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(details)
	}

	// Human-readable output
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("Identity: %s/%s", rigName, polecatName)))

	fmt.Printf("  Bead ID:       %s\n", details.BeadID)
	fmt.Printf("  Title:         %s\n", details.Title)

	// Status
	sessionStr := style.Dim.Render("stopped")
	if details.SessionRunning {
		sessionStr = style.Success.Render("running")
	}
	fmt.Printf("  Session:       %s\n", sessionStr)

	worktreeStr := style.Dim.Render("no")
	if details.WorktreeExists {
		worktreeStr = style.Success.Render("yes")
	}
	fmt.Printf("  Worktree:      %s\n", worktreeStr)

	// Agent state
	stateStr := details.AgentState
	if stateStr == "" {
		stateStr = "unknown"
	}
	switch stateStr {
	case "working":
		stateStr = style.Info.Render(stateStr)
	case "done":
		stateStr = style.Success.Render(stateStr)
	case "stuck":
		stateStr = style.Warning.Render(stateStr)
	default:
		stateStr = style.Dim.Render(stateStr)
	}
	fmt.Printf("  Agent State:   %s\n", stateStr)

	// Hook
	if details.HookBead != "" {
		fmt.Printf("  Hook:          %s\n", details.HookBead)
	} else {
		fmt.Printf("  Hook:          %s\n", style.Dim.Render("(empty)"))
	}

	// Cleanup status
	if details.CleanupStatus != "" {
		fmt.Printf("  Cleanup:       %s\n", details.CleanupStatus)
	}

	// Timestamps
	if details.CreatedAt != "" {
		fmt.Printf("  Created:       %s\n", style.Dim.Render(details.CreatedAt))
	}
	if details.UpdatedAt != "" {
		fmt.Printf("  Updated:       %s\n", style.Dim.Render(details.UpdatedAt))
	}

	// CV summary
	fmt.Println()
	fmt.Printf("%s\n", style.Bold.Render("CV (Work History)"))
	if len(details.CVBeads) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no completed work)"))
	} else {
		for _, cv := range details.CVBeads {
			fmt.Printf("  - %s\n", cv)
		}
	}

	return nil
}

func runPolecatIdentityRename(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	oldName := args[1]
	newName := args[2]

	// Validate names
	if oldName == newName {
		return fmt.Errorf("old and new names are the same")
	}

	// Get rig
	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	bd := beads.New(r.Path)
	oldBeadID := beads.PolecatBeadID(rigName, oldName)
	newBeadID := beads.PolecatBeadID(rigName, newName)

	// Check old identity exists
	oldIssue, oldFields, err := bd.GetAgentBead(oldBeadID)
	if err != nil {
		return fmt.Errorf("getting old identity bead: %w", err)
	}
	if oldIssue == nil || oldIssue.Status == "closed" {
		return fmt.Errorf("identity bead %s not found or already closed", oldBeadID)
	}

	// Check new identity doesn't exist
	newIssue, _, _ := bd.GetAgentBead(newBeadID)
	if newIssue != nil && newIssue.Status != "closed" {
		return fmt.Errorf("identity bead %s already exists", newBeadID)
	}

	// Safety check: no active session
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)
	running, _ := polecatMgr.IsRunning(oldName)
	if running {
		return fmt.Errorf("cannot rename: polecat session %s is running", oldName)
	}

	// Create new identity bead with inherited fields
	newFields := &beads.AgentFields{
		RoleType:      "polecat",
		Rig:           rigName,
		AgentState:    oldFields.AgentState,
		CleanupStatus: oldFields.CleanupStatus,
	}

	newTitle := fmt.Sprintf("Polecat %s in %s", newName, rigName)
	_, err = bd.CreateOrReopenAgentBead(newBeadID, newTitle, newFields)
	if err != nil {
		return fmt.Errorf("creating new identity bead: %w", err)
	}

	// Close old bead with reference to new one
	closeReason := fmt.Sprintf("renamed to %s", newBeadID)
	if err := bd.CloseWithReason(closeReason, oldBeadID); err != nil {
		// Try to clean up new bead
		_ = bd.CloseWithReason("rename failed", newBeadID)
		return fmt.Errorf("closing old identity bead: %w", err)
	}

	fmt.Printf("%s Renamed identity:\n", style.SuccessPrefix)
	fmt.Printf("  Old: %s\n", oldBeadID)
	fmt.Printf("  New: %s\n", newBeadID)
	fmt.Printf("\n%s Note: If a worktree exists for %s, you'll need to recreate it with the new name.\n",
		style.Warning.Render("⚠"), oldName)

	return nil
}

func runPolecatIdentityRemove(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	polecatName := args[1]

	// Get rig
	_, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	bd := beads.New(r.Path)
	beadID := beads.PolecatBeadID(rigName, polecatName)

	// Check identity exists
	issue, fields, err := bd.GetAgentBead(beadID)
	if err != nil {
		return fmt.Errorf("getting identity bead: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("identity bead %s not found", beadID)
	}
	if issue.Status == "closed" {
		return fmt.Errorf("identity bead %s is already closed", beadID)
	}

	// Safety checks (unless --force)
	if !polecatIdentityRemoveForce {
		var reasons []string

		// Check for active session
		t := tmux.NewTmux()
		polecatMgr := polecat.NewSessionManager(t, r)
		running, _ := polecatMgr.IsRunning(polecatName)
		if running {
			reasons = append(reasons, "session is running")
		}

		// Check for work on hook
		hookBead := issue.HookBead
		if hookBead == "" && fields != nil {
			hookBead = fields.HookBead
		}
		if hookBead != "" {
			// Check if hooked bead is still open
			hookedIssue, _ := bd.Show(hookBead)
			if hookedIssue != nil && hookedIssue.Status != "closed" {
				reasons = append(reasons, fmt.Sprintf("has work on hook (%s)", hookBead))
			}
		}

		if len(reasons) > 0 {
			fmt.Printf("%s Cannot remove identity %s:\n", style.Error.Render("Error:"), beadID)
			for _, r := range reasons {
				fmt.Printf("  - %s\n", r)
			}
			fmt.Println("\nUse --force to bypass safety checks.")
			return fmt.Errorf("safety checks failed")
		}

		// Warn if CV exists
		assignee := fmt.Sprintf("%s/%s", rigName, polecatName)
		cvBeads, _ := bd.ListByAssignee(assignee)
		cvCount := 0
		for _, cv := range cvBeads {
			if cv.ID != beadID && cv.Status == "closed" {
				cvCount++
			}
		}
		if cvCount > 0 {
			fmt.Printf("%s Warning: This polecat has %d completed work item(s) in CV.\n",
				style.Warning.Render("⚠"), cvCount)
		}
	}

	// Close the identity bead
	if err := bd.CloseWithReason("removed via gt polecat identity remove", beadID); err != nil {
		return fmt.Errorf("closing identity bead: %w", err)
	}

	fmt.Printf("%s Removed identity bead: %s\n", style.SuccessPrefix, beadID)
	return nil
}
