package doctor

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// HookSingletonCheck verifies that each agent has at most one handoff bead.
// Multiple handoff beads for the same role create ambiguity about which work
// is actually assigned to the agent.
type HookSingletonCheck struct {
	FixableCheck
	duplicates []duplicateHandoff
}

type duplicateHandoff struct {
	role      string
	beadsDir  string
	beadIDs   []string // All bead IDs with this role's handoff title
}

// NewHookSingletonCheck creates a new hook singleton check.
func NewHookSingletonCheck() *HookSingletonCheck {
	return &HookSingletonCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hook-singleton",
				CheckDescription: "Ensure each agent has at most one handoff bead",
			},
		},
	}
}

// Run checks all pinned beads for duplicate handoff beads per role.
func (c *HookSingletonCheck) Run(ctx *CheckContext) *CheckResult {
	c.duplicates = nil

	var details []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	townDupes := c.checkBeadsDir(townBeadsDir)
	for _, dupe := range townDupes {
		details = append(details, c.formatDuplicate(dupe))
	}
	c.duplicates = append(c.duplicates, townDupes...)

	// Check rig-level beads
	rigDirs := c.findRigBeadsDirs(ctx.TownRoot)
	for _, rigDir := range rigDirs {
		rigDupes := c.checkBeadsDir(rigDir)
		for _, dupe := range rigDupes {
			details = append(details, c.formatDuplicate(dupe))
		}
		c.duplicates = append(c.duplicates, rigDupes...)
	}

	if len(c.duplicates) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All agents have at most one handoff bead",
		}
	}

	totalDupes := 0
	for _, d := range c.duplicates {
		totalDupes += len(d.beadIDs) - 1 // Count extra beads beyond the first
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d role(s) with duplicate handoff beads", len(c.duplicates)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to remove duplicates, or 'bd close <id>' manually",
	}
}

// checkBeadsDir checks all pinned beads in a directory for duplicate handoffs.
func (c *HookSingletonCheck) checkBeadsDir(beadsDir string) []duplicateHandoff {
	var duplicates []duplicateHandoff

	b := beads.New(filepath.Dir(beadsDir))

	// List all pinned beads
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Priority: -1,
	})
	if err != nil {
		return nil
	}

	// Group by handoff title pattern
	// Handoff beads have title like "{role} Handoff"
	handoffsByRole := make(map[string][]string) // role -> []beadID

	for _, bead := range pinnedBeads {
		if strings.HasSuffix(bead.Title, " Handoff") {
			role := strings.TrimSuffix(bead.Title, " Handoff")
			handoffsByRole[role] = append(handoffsByRole[role], bead.ID)
		}
	}

	// Find roles with multiple handoff beads
	for role, beadIDs := range handoffsByRole {
		if len(beadIDs) > 1 {
			// Sort for consistent output
			sort.Strings(beadIDs)
			duplicates = append(duplicates, duplicateHandoff{
				role:     role,
				beadsDir: beadsDir,
				beadIDs:  beadIDs,
			})
		}
	}

	// Sort duplicates by role for consistent output
	sort.Slice(duplicates, func(i, j int) bool {
		return duplicates[i].role < duplicates[j].role
	})

	return duplicates
}

// findRigBeadsDirs finds all rig-level .beads directories.
func (c *HookSingletonCheck) findRigBeadsDirs(townRoot string) []string {
	var dirs []string

	cmd := exec.Command("find", townRoot, "-maxdepth", "2", "-type", "d", "-name", ".beads")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		if line == filepath.Join(townRoot, ".beads") {
			continue
		}
		if strings.Contains(line, "/mayor/") {
			continue
		}
		dirs = append(dirs, line)
	}

	return dirs
}

// formatDuplicate formats a duplicate handoff for display.
func (c *HookSingletonCheck) formatDuplicate(dupe duplicateHandoff) string {
	return fmt.Sprintf("%s: %d handoff beads (%s)",
		dupe.role, len(dupe.beadIDs), strings.Join(dupe.beadIDs, ", "))
}

// Fix removes duplicate handoff beads, keeping only the most recent one.
func (c *HookSingletonCheck) Fix(ctx *CheckContext) error {
	var errors []string

	for _, dupe := range c.duplicates {
		b := beads.New(filepath.Dir(dupe.beadsDir))

		// Keep the last one (most recently created based on ID sort), close others
		// IDs are sorted, so we close all but the last
		toClose := dupe.beadIDs[:len(dupe.beadIDs)-1]

		for _, id := range toClose {
			if err := b.Close(id); err != nil {
				errors = append(errors, fmt.Sprintf("failed to close %s: %v", id, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// HookAttachmentValidCheck verifies that attached molecules exist and are not closed.
// This detects when a hook's attached_molecule field points to a non-existent or
// closed issue, which can leave agents with stale work assignments.
type HookAttachmentValidCheck struct {
	FixableCheck
	invalidAttachments []invalidAttachment
}

type invalidAttachment struct {
	pinnedBeadID   string
	pinnedBeadDir  string // Directory where the pinned bead was found
	moleculeID     string
	reason         string // "not_found" or "closed"
}

// NewHookAttachmentValidCheck creates a new hook attachment validation check.
func NewHookAttachmentValidCheck() *HookAttachmentValidCheck {
	return &HookAttachmentValidCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hook-attachment-valid",
				CheckDescription: "Verify attached molecules exist and are not closed",
			},
		},
	}
}

// Run checks all pinned beads for invalid molecule attachments.
func (c *HookAttachmentValidCheck) Run(ctx *CheckContext) *CheckResult {
	c.invalidAttachments = nil

	var details []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	townInvalid := c.checkBeadsDir(townBeadsDir, "town")
	for _, inv := range townInvalid {
		details = append(details, c.formatInvalid(inv))
	}
	c.invalidAttachments = append(c.invalidAttachments, townInvalid...)

	// Check rig-level beads
	rigDirs := c.findRigBeadsDirs(ctx.TownRoot)
	for _, rigDir := range rigDirs {
		rigName := filepath.Base(filepath.Dir(rigDir))
		rigInvalid := c.checkBeadsDir(rigDir, rigName)
		for _, inv := range rigInvalid {
			details = append(details, c.formatInvalid(inv))
		}
		c.invalidAttachments = append(c.invalidAttachments, rigInvalid...)
	}

	if len(c.invalidAttachments) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All hook attachments are valid",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d invalid hook attachment(s)", len(c.invalidAttachments)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to detach invalid molecules, or 'gt mol detach <pinned-bead-id>' manually",
	}
}

// checkBeadsDir checks all pinned beads in a directory for invalid attachments.
func (c *HookAttachmentValidCheck) checkBeadsDir(beadsDir, location string) []invalidAttachment {
	var invalid []invalidAttachment

	b := beads.New(filepath.Dir(beadsDir))

	// List all pinned beads
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Priority: -1,
	})
	if err != nil {
		// Can't list pinned beads - silently skip this directory
		return nil
	}

	for _, pinnedBead := range pinnedBeads {
		// Parse attachment fields from the pinned bead
		attachment := beads.ParseAttachmentFields(pinnedBead)
		if attachment == nil || attachment.AttachedMolecule == "" {
			continue // No attachment, skip
		}

		// Verify the attached molecule exists and is not closed
		molecule, err := b.Show(attachment.AttachedMolecule)
		if err != nil {
			// Molecule not found
			invalid = append(invalid, invalidAttachment{
				pinnedBeadID:  pinnedBead.ID,
				pinnedBeadDir: beadsDir,
				moleculeID:    attachment.AttachedMolecule,
				reason:        "not_found",
			})
			continue
		}

		if molecule.Status == "closed" {
			invalid = append(invalid, invalidAttachment{
				pinnedBeadID:  pinnedBead.ID,
				pinnedBeadDir: beadsDir,
				moleculeID:    attachment.AttachedMolecule,
				reason:        "closed",
			})
		}
	}

	return invalid
}

// findRigBeadsDirs finds all rig-level .beads directories.
func (c *HookAttachmentValidCheck) findRigBeadsDirs(townRoot string) []string {
	var dirs []string

	// Look for .beads directories in rig subdirectories
	// Pattern: <townRoot>/<rig>/.beads (but NOT <townRoot>/.beads which is town-level)
	cmd := exec.Command("find", townRoot, "-maxdepth", "2", "-type", "d", "-name", ".beads")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		// Skip town-level .beads
		if line == filepath.Join(townRoot, ".beads") {
			continue
		}
		// Skip mayor directory
		if strings.Contains(line, "/mayor/") {
			continue
		}
		dirs = append(dirs, line)
	}

	return dirs
}

// formatInvalid formats an invalid attachment for display.
func (c *HookAttachmentValidCheck) formatInvalid(inv invalidAttachment) string {
	reasonText := "not found"
	if inv.reason == "closed" {
		reasonText = "is closed"
	}
	return fmt.Sprintf("%s: attached molecule %s %s", inv.pinnedBeadID, inv.moleculeID, reasonText)
}

// Fix detaches all invalid molecule attachments.
func (c *HookAttachmentValidCheck) Fix(ctx *CheckContext) error {
	var errors []string

	for _, inv := range c.invalidAttachments {
		b := beads.New(filepath.Dir(inv.pinnedBeadDir))

		_, err := b.DetachMolecule(inv.pinnedBeadID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to detach from %s: %v", inv.pinnedBeadID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// HookSingletonCheck ensures each agent has at most one handoff bead.
// Detects when multiple pinned beads exist with the same "{role} Handoff" title,
// which can cause confusion about which handoff is authoritative.
type HookSingletonCheck struct {
	FixableCheck
	duplicates []duplicateHandoff
}

type duplicateHandoff struct {
	title     string
	beadsDir  string
	beadIDs   []string // All IDs with this title (first one is kept, rest are duplicates)
}

// NewHookSingletonCheck creates a new hook singleton check.
func NewHookSingletonCheck() *HookSingletonCheck {
	return &HookSingletonCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hook-singleton",
				CheckDescription: "Ensure each agent has at most one handoff bead",
			},
		},
	}
}

// Run checks all pinned beads for duplicate handoff titles.
func (c *HookSingletonCheck) Run(ctx *CheckContext) *CheckResult {
	c.duplicates = nil

	var details []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	townDups := c.checkBeadsDir(townBeadsDir)
	for _, dup := range townDups {
		details = append(details, c.formatDuplicate(dup))
	}
	c.duplicates = append(c.duplicates, townDups...)

	// Check rig-level beads using the shared helper
	attachCheck := &HookAttachmentValidCheck{}
	rigDirs := attachCheck.findRigBeadsDirs(ctx.TownRoot)
	for _, rigDir := range rigDirs {
		rigDups := c.checkBeadsDir(rigDir)
		for _, dup := range rigDups {
			details = append(details, c.formatDuplicate(dup))
		}
		c.duplicates = append(c.duplicates, rigDups...)
	}

	if len(c.duplicates) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "All handoff beads are unique",
		}
	}

	totalDups := 0
	for _, dup := range c.duplicates {
		totalDups += len(dup.beadIDs) - 1 // Count extras beyond the first
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d duplicate handoff bead(s)", totalDups),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to close duplicates, or 'bd close <id>' manually",
	}
}

// checkBeadsDir checks for duplicate handoff beads in a directory.
func (c *HookSingletonCheck) checkBeadsDir(beadsDir string) []duplicateHandoff {
	var duplicates []duplicateHandoff

	b := beads.New(filepath.Dir(beadsDir))

	// List all pinned beads
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Priority: -1,
	})
	if err != nil {
		return nil
	}

	// Group pinned beads by title (only those matching "{role} Handoff" pattern)
	titleToIDs := make(map[string][]string)
	for _, bead := range pinnedBeads {
		// Check if title matches handoff pattern (ends with " Handoff")
		if strings.HasSuffix(bead.Title, " Handoff") {
			titleToIDs[bead.Title] = append(titleToIDs[bead.Title], bead.ID)
		}
	}

	// Find duplicates (titles with more than one bead)
	for title, ids := range titleToIDs {
		if len(ids) > 1 {
			duplicates = append(duplicates, duplicateHandoff{
				title:    title,
				beadsDir: beadsDir,
				beadIDs:  ids,
			})
		}
	}

	return duplicates
}

// formatDuplicate formats a duplicate handoff for display.
func (c *HookSingletonCheck) formatDuplicate(dup duplicateHandoff) string {
	return fmt.Sprintf("%q has %d beads: %s", dup.title, len(dup.beadIDs), strings.Join(dup.beadIDs, ", "))
}

// Fix closes duplicate handoff beads, keeping the first one.
func (c *HookSingletonCheck) Fix(ctx *CheckContext) error {
	var errors []string

	for _, dup := range c.duplicates {
		b := beads.New(filepath.Dir(dup.beadsDir))

		// Close all but the first bead (keep the oldest/first one)
		toClose := dup.beadIDs[1:]
		if len(toClose) > 0 {
			err := b.CloseWithReason("duplicate handoff bead", toClose...)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to close duplicates for %q: %v", dup.title, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}
