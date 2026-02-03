package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/session"
)

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
				CheckCategory:    CategoryHooks,
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
func (c *HookAttachmentValidCheck) checkBeadsDir(beadsDir, _ string) []invalidAttachment { // location unused but kept for future diagnostic output
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
				CheckCategory:    CategoryHooks,
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

// OrphanedAttachmentsCheck detects handoff beads for agents that no longer exist.
// This happens when a polecat worktree is deleted but its handoff bead remains,
// leaving molecules attached to non-existent agents.
type OrphanedAttachmentsCheck struct {
	BaseCheck
	orphans []orphanedHandoff
}

type orphanedHandoff struct {
	beadID    string
	beadTitle string
	beadsDir  string
	agent     string // Parsed agent identity
}

// NewOrphanedAttachmentsCheck creates a new orphaned attachments check.
func NewOrphanedAttachmentsCheck() *OrphanedAttachmentsCheck {
	return &OrphanedAttachmentsCheck{
		BaseCheck: BaseCheck{
			CheckName:        "orphaned-attachments",
			CheckDescription: "Detect handoff beads for non-existent agents",
			CheckCategory:    CategoryHooks,
		},
	}
}

// Run checks all handoff beads for orphaned agents.
func (c *OrphanedAttachmentsCheck) Run(ctx *CheckContext) *CheckResult {
	c.orphans = nil

	var details []string

	// Check town-level beads
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	townOrphans := c.checkBeadsDir(townBeadsDir, ctx.TownRoot)
	for _, orph := range townOrphans {
		details = append(details, c.formatOrphan(orph))
	}
	c.orphans = append(c.orphans, townOrphans...)

	// Check rig-level beads using the shared helper
	attachCheck := &HookAttachmentValidCheck{}
	rigDirs := attachCheck.findRigBeadsDirs(ctx.TownRoot)
	for _, rigDir := range rigDirs {
		rigOrphans := c.checkBeadsDir(rigDir, ctx.TownRoot)
		for _, orph := range rigOrphans {
			details = append(details, c.formatOrphan(orph))
		}
		c.orphans = append(c.orphans, rigOrphans...)
	}

	if len(c.orphans) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No orphaned handoff beads found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d orphaned handoff bead(s)", len(c.orphans)),
		Details: details,
		FixHint: "Reassign with 'gt sling <id> <agent>', or close with 'bd close <id>'",
	}
}

// checkBeadsDir checks for orphaned handoff beads in a directory.
func (c *OrphanedAttachmentsCheck) checkBeadsDir(beadsDir, townRoot string) []orphanedHandoff {
	var orphans []orphanedHandoff

	b := beads.New(filepath.Dir(beadsDir))

	// List all pinned beads
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Priority: -1,
	})
	if err != nil {
		return nil
	}

	for _, bead := range pinnedBeads {
		// Check if title matches handoff pattern (ends with " Handoff")
		if !strings.HasSuffix(bead.Title, " Handoff") {
			continue
		}

		// Extract agent identity from title
		agent := strings.TrimSuffix(bead.Title, " Handoff")
		if agent == "" {
			continue
		}

		// Check if agent worktree exists
		if !c.agentExists(agent, townRoot) {
			orphans = append(orphans, orphanedHandoff{
				beadID:    bead.ID,
				beadTitle: bead.Title,
				beadsDir:  beadsDir,
				agent:     agent,
			})
		}
	}

	return orphans
}

// agentExists checks if an agent's worktree exists.
// Agent identities follow patterns like:
//   - "gastown/nux" or "gastown/polecats/nux" → polecat at <townRoot>/gastown/polecats/nux
//   - "gastown/crew/joe" → crew at <townRoot>/gastown/crew/joe
//   - "mayor" → mayor at <townRoot>/mayor
//   - "gastown/witness" or "gastown-witness" → witness at <townRoot>/gastown/witness
//   - "gastown/refinery" or "gastown-refinery" → refinery at <townRoot>/gastown/refinery
//
// Uses session.ParseAddress() for standard slash-format parsing, with fallback
// for legacy hyphenated formats (witness/refinery).
func (c *OrphanedAttachmentsCheck) agentExists(agent, townRoot string) bool {
	// First try parsing as standard address format (slash-separated)
	identity, err := session.ParseAddress(agent)
	if err == nil {
		return c.identityWorktreeExists(identity, townRoot)
	}

	// Fallback: Handle legacy hyphenated formats for witness/refinery
	// These are used in older handoff beads: "gastown-witness", "gastown-refinery"
	if strings.HasSuffix(agent, "-witness") {
		rig := strings.TrimSuffix(agent, "-witness")
		if rig != "" {
			path := filepath.Join(townRoot, rig, "witness")
			return dirExists(path)
		}
	}
	if strings.HasSuffix(agent, "-refinery") {
		rig := strings.TrimSuffix(agent, "-refinery")
		if rig != "" {
			path := filepath.Join(townRoot, rig, "refinery")
			return dirExists(path)
		}
	}

	// Unknown pattern - return false to surface as potential orphan
	// This is intentional: unrecognized patterns should be flagged for review
	// rather than silently assumed to exist (which caused false negatives)
	return false
}

// identityWorktreeExists checks if the worktree for a parsed AgentIdentity exists.
func (c *OrphanedAttachmentsCheck) identityWorktreeExists(identity *session.AgentIdentity, townRoot string) bool {
	var path string
	switch identity.Role {
	case session.RoleMayor:
		path = filepath.Join(townRoot, "mayor")
	case session.RoleDeacon:
		path = filepath.Join(townRoot, "deacon")
	case session.RoleWitness:
		path = filepath.Join(townRoot, identity.Rig, "witness")
	case session.RoleRefinery:
		path = filepath.Join(townRoot, identity.Rig, "refinery")
	case session.RoleCrew:
		path = filepath.Join(townRoot, identity.Rig, "crew", identity.Name)
	case session.RolePolecat:
		path = filepath.Join(townRoot, identity.Rig, "polecats", identity.Name)
	default:
		// Unknown role - return false to surface as potential orphan
		return false
	}
	return dirExists(path)
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// formatOrphan formats an orphaned handoff for display.
func (c *OrphanedAttachmentsCheck) formatOrphan(orph orphanedHandoff) string {
	return fmt.Sprintf("%s: agent %q no longer exists", orph.beadID, orph.agent)
}

// HookErrorsCheck detects recent hook execution errors.
// This surfaces errors that were logged via "gt hooks report-error" so they
// can be investigated rather than silently ignored.
type HookErrorsCheck struct {
	FixableCheck
	errorCount int
}

// NewHookErrorsCheck creates a new hook errors check.
func NewHookErrorsCheck() *HookErrorsCheck {
	return &HookErrorsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "hook-errors",
				CheckDescription: "Check for recent hook execution errors",
				CheckCategory:    CategoryHooks,
			},
		},
	}
}

// Run checks for recent hook errors.
func (c *HookErrorsCheck) Run(ctx *CheckContext) *CheckResult {
	log := hooks.NewErrorLog(ctx.TownRoot)

	// Get errors from the last hour (recent enough to be actionable)
	errors, err := log.GetRecentErrors(10)
	if err != nil {
		// Can't read errors - this is OK, might not exist yet
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No hook error log found",
		}
	}

	if len(errors) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No recent hook errors",
		}
	}

	c.errorCount = len(errors)

	// Build details list
	var details []string
	for _, e := range errors {
		countStr := ""
		if e.Count > 1 {
			countStr = fmt.Sprintf(" (x%d)", e.Count)
		}
		details = append(details, fmt.Sprintf("%s [%s] exit %d%s - %s",
			e.HookType, truncateStr(e.Command, 30), e.ExitCode, countStr, e.Role))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d recent hook error(s)", len(errors)),
		Details: details,
		FixHint: "View with 'gt hooks errors' or clear with 'gt hooks errors --clear'",
	}
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Fix clears the hook error log.
func (c *HookErrorsCheck) Fix(ctx *CheckContext) error {
	log := hooks.NewErrorLog(ctx.TownRoot)
	return log.ClearErrors()
}
