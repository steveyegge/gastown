package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// runMoleculeBurn burns (destroys) the current molecule attachment.
func runMoleculeBurn(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Determine target agent
	var target string
	if len(args) > 0 {
		target = args[0]
	} else {
		// Auto-detect using env-aware role detection
		roleInfo, err := GetRoleWithContext(cwd, townRoot)
		if err != nil {
			return fmt.Errorf("detecting role: %w", err)
		}
		roleCtx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		target = buildAgentIdentity(roleCtx)
		if target == "" {
			return fmt.Errorf("cannot determine agent identity (role: %s)", roleCtx.Role)
		}
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Find agent's pinned bead (handoff bead)
	parts := strings.Split(target, "/")
	role := parts[len(parts)-1]

	handoff, err := b.FindHandoffBead(role)
	if err != nil {
		return fmt.Errorf("finding handoff bead: %w", err)
	}
	if handoff == nil {
		return fmt.Errorf("no handoff bead found for %s", target)
	}

	// Check for attached molecule
	attachment := beads.ParseAttachmentFields(handoff)
	if attachment == nil || attachment.AttachedMolecule == "" {
		fmt.Printf("%s No molecule attached to %s - nothing to burn\n",
			style.Dim.Render("ℹ"), target)
		return nil
	}

	moleculeID := attachment.AttachedMolecule

	// Recursively close all descendant step issues before detaching
	// This prevents orphaned step issues from accumulating (gt-psj76.1)
	childrenClosed := closeDescendants(b, moleculeID)

	// Detach the molecule with audit logging (this "burns" it by removing the attachment)
	_, err = b.DetachMoleculeWithAudit(handoff.ID, beads.DetachOptions{
		Operation: "burn",
		Agent:     target,
		Reason:    "molecule burned by agent",
	})
	if err != nil {
		return fmt.Errorf("detaching molecule: %w", err)
	}

	if moleculeJSON {
		result := map[string]interface{}{
			"burned":          moleculeID,
			"from":            target,
			"handoff_id":      handoff.ID,
			"children_closed": childrenClosed,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("%s Burned molecule %s from %s\n",
		style.Bold.Render("🔥"), moleculeID, target)
	if childrenClosed > 0 {
		fmt.Printf("  Closed %d step issues\n", childrenClosed)
	}

	return nil
}

// runMoleculeSquash squashes the current molecule into a digest.
func runMoleculeSquash(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Determine target agent
	var target string
	if len(args) > 0 {
		target = args[0]
	} else {
		// Auto-detect using env-aware role detection
		roleInfo, err := GetRoleWithContext(cwd, townRoot)
		if err != nil {
			return fmt.Errorf("detecting role: %w", err)
		}
		roleCtx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		target = buildAgentIdentity(roleCtx)
		if target == "" {
			return fmt.Errorf("cannot determine agent identity (role: %s)", roleCtx.Role)
		}
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Find agent's pinned bead (handoff bead)
	parts := strings.Split(target, "/")
	role := parts[len(parts)-1]

	handoff, err := b.FindHandoffBead(role)
	if err != nil {
		return fmt.Errorf("finding handoff bead: %w", err)
	}
	if handoff == nil {
		return fmt.Errorf("no handoff bead found for %s", target)
	}

	// Check for attached molecule
	attachment := beads.ParseAttachmentFields(handoff)
	if attachment == nil || attachment.AttachedMolecule == "" {
		fmt.Printf("%s No molecule attached to %s - nothing to squash\n",
			style.Dim.Render("ℹ"), target)
		return nil
	}

	moleculeID := attachment.AttachedMolecule

	// Recursively close all descendant step issues before squashing
	// This prevents orphaned step issues from accumulating (gt-psj76.1)
	childrenClosed := closeDescendants(b, moleculeID)

	// Get progress info for the digest
	progress, _ := getMoleculeProgressInfo(b, moleculeID)

	// Create a digest issue
	digestTitle := fmt.Sprintf("Digest: %s", moleculeID)
	digestDesc := fmt.Sprintf(`Squashed molecule execution.

molecule: %s
agent: %s
squashed_at: %s
`, moleculeID, target, time.Now().UTC().Format(time.RFC3339))

	if progress != nil {
		digestDesc += fmt.Sprintf(`
## Execution Summary
- Steps: %d/%d completed
- Status: %s
`, progress.DoneSteps, progress.TotalSteps, func() string {
			if progress.Complete {
				return "complete"
			}
			return "partial"
		}())
	}

	// Create the digest bead (ephemeral to avoid JSONL pollution)
	// Per-cycle digests are aggregated daily by 'gt patrol digest'
	digestIssue, err := b.Create(beads.CreateOptions{
		Title:       digestTitle,
		Description: digestDesc,
		Type:        "task",
		Priority:    4,       // P4 - backlog priority for digests
		Actor:       target,
		Ephemeral:   true,    // Don't export to JSONL - daily aggregation handles permanent record
	})
	if err != nil {
		return fmt.Errorf("creating digest: %w", err)
	}

	// Add the digest label (non-fatal: digest works without label)
	_ = b.Update(digestIssue.ID, beads.UpdateOptions{
		AddLabels: []string{"digest"},
	})

	// Close the digest immediately
	closedStatus := "closed"
	err = b.Update(digestIssue.ID, beads.UpdateOptions{
		Status: &closedStatus,
	})
	if err != nil {
		style.PrintWarning("Created digest but couldn't close it: %v", err)
	}

	// Detach the molecule from the handoff bead with audit logging
	_, err = b.DetachMoleculeWithAudit(handoff.ID, beads.DetachOptions{
		Operation: "squash",
		Agent:     target,
		Reason:    fmt.Sprintf("molecule squashed to digest %s", digestIssue.ID),
	})
	if err != nil {
		return fmt.Errorf("detaching molecule: %w", err)
	}

	if moleculeJSON {
		result := map[string]interface{}{
			"squashed":        moleculeID,
			"digest_id":       digestIssue.ID,
			"from":            target,
			"handoff_id":      handoff.ID,
			"children_closed": childrenClosed,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("%s Squashed molecule %s → digest %s\n",
		style.Bold.Render("📦"), moleculeID, digestIssue.ID)
	if childrenClosed > 0 {
		fmt.Printf("  Closed %d step issues\n", childrenClosed)
	}

	return nil
}

// closeDescendants recursively closes all descendant issues of a parent.
// Returns the count of issues closed. Logs warnings on errors but doesn't fail.
func closeDescendants(b *beads.Beads, parentID string) int {
	children, err := b.List(beads.ListOptions{
		Parent: parentID,
		Status: "all",
	})
	if err != nil {
		style.PrintWarning("could not list children of %s: %v", parentID, err)
		return 0
	}

	if len(children) == 0 {
		return 0
	}

	// First, recursively close grandchildren
	totalClosed := 0
	for _, child := range children {
		totalClosed += closeDescendants(b, child.ID)
	}

	// Then close direct children
	var idsToClose []string
	for _, child := range children {
		if child.Status != "closed" {
			idsToClose = append(idsToClose, child.ID)
		}
	}

	if len(idsToClose) > 0 {
		if closeErr := b.Close(idsToClose...); closeErr != nil {
			style.PrintWarning("could not close children of %s: %v", parentID, closeErr)
		} else {
			totalClosed += len(idsToClose)
		}
	}

	return totalClosed
}
