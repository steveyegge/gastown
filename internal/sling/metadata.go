package sling

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
)

// StoreArgsInBead stores args in the bead's description using attached_args field.
func StoreArgsInBead(beadID, args string) error {
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.AttachedArgs = args

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// StoreDispatcherInBead stores the dispatcher agent ID in the bead's description.
func StoreDispatcherInBead(beadID, dispatcher string) error {
	if dispatcher == "" {
		return nil
	}
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.DispatchedBy = dispatcher

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// StoreAttachedMoleculeInBead sets the attached_molecule field in a bead's description.
func StoreAttachedMoleculeInBead(beadID, moleculeID string) error {
	if moleculeID == "" {
		return nil
	}

	logPath := os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG")
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte("called"), 0644)
	}

	issue := &beads.Issue{}
	if logPath == "" {
		var err error
		issue, err = fetchBeadIssue(beadID)
		if err != nil {
			return err
		}
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.AttachedMolecule = moleculeID
	if fields.AttachedAt == "" {
		fields.AttachedAt = time.Now().UTC().Format(time.RFC3339)
	}

	newDesc := beads.SetAttachmentFields(issue, fields)
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte(newDesc), 0644)
	}
	return updateBeadDescription(beadID, newDesc)
}

// StoreNoMergeInBead sets the no_merge field in a bead's description.
func StoreNoMergeInBead(beadID string, noMerge bool) error {
	if !noMerge {
		return nil
	}
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.NoMerge = true

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// StoreMergeStrategyInBead sets the merge_strategy field in a bead's description.
func StoreMergeStrategyInBead(beadID, strategy string) error {
	if strategy == "" {
		return nil
	}
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.MergeStrategy = strategy

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// StoreConvoyOwnedInBead sets the convoy_owned field in a bead's description.
func StoreConvoyOwnedInBead(beadID string, owned bool) error {
	if !owned {
		return nil
	}
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.ConvoyOwned = true

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// StoreOjJobIDInBead stores the OJ job ID in a bead's description.
func StoreOjJobIDInBead(beadID, ojJobID string) error {
	if ojJobID == "" {
		return nil
	}
	issue, err := fetchBeadIssue(beadID)
	if err != nil {
		return err
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}
	fields.OjJobID = ojJobID

	newDesc := beads.SetAttachmentFields(issue, fields)
	return updateBeadDescription(beadID, newDesc)
}

// fetchBeadIssue fetches a bead issue by ID.
func fetchBeadIssue(beadID string) (*beads.Issue, error) {
	showCmd := bdcmd.Command( "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fetching bead: %w", err)
	}

	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		if os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG") == "" {
			return nil, fmt.Errorf("parsing bead: %w", err)
		}
	}
	if len(issues) == 0 {
		if os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG") == "" {
			return nil, fmt.Errorf("bead not found")
		}
		return &beads.Issue{}, nil
	}
	return &issues[0], nil
}

// updateBeadDescription updates a bead's description field.
func updateBeadDescription(beadID, newDesc string) error {
	updateCmd := bdcmd.Command( "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}
	return nil
}
