package sling

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
)

// IsTrackedByConvoy checks if an issue is already being tracked by a convoy.
// Returns the convoy ID if tracked, empty string otherwise.
func IsTrackedByConvoy(beadID, townRoot string) string {
	depCmd := bdcmd.Command( "dep", "list", beadID, "--direction=up", "--type=tracks", "--json")
	depCmd.Dir = townRoot
	out, err := depCmd.Output()
	if err != nil {
		return ""
	}

	var trackers []struct {
		ID        string `json:"id"`
		IssueType string `json:"issue_type"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(out, &trackers); err != nil {
		return ""
	}

	for _, tracker := range trackers {
		if tracker.IssueType == "convoy" && tracker.Status == "open" {
			return tracker.ID
		}
	}
	return ""
}

// CreateAutoConvoy creates an auto-convoy for a single issue and tracks it.
// Returns the created convoy ID.
func CreateAutoConvoy(beadID, beadTitle, assignee, townRoot string, opts ConvoyOptions) (string, error) {
	convoyID := fmt.Sprintf("hq-cv-%s", generateShortID())
	convoyTitle := fmt.Sprintf("Work: %s", beadTitle)
	description := fmt.Sprintf("Auto-created convoy tracking %s", beadID)

	if opts.Owned {
		description += "\nOwned: true"
	}
	if opts.MergeStrategy != "" {
		description += fmt.Sprintf("\nMerge: %s", opts.MergeStrategy)
	}

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}
	if assignee != "" {
		createArgs = append(createArgs, "--assignee="+assignee)
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	createCmd := bdcmd.Command( createArgs...)
	createCmd.Dir = townRoot
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating convoy: %w", err)
	}

	trackBeadID := FormatTrackBeadID(beadID, townRoot)
	depArgs := []string{"dep", "add", convoyID, trackBeadID, "--type=tracks"}
	depCmd := bdcmd.Command( depArgs...)
	depCmd.Dir = townRoot
	depCmd.Stderr = os.Stderr
	if err := depCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not add tracking relation: %v\n", err)
	}

	return convoyID, nil
}

// AddToConvoy adds a bead to an existing convoy.
func AddToConvoy(convoyID, beadID, townRoot string) error {
	showArgs := []string{"show", convoyID, "--json"}
	showCmd := bdcmd.Command( showArgs...)
	showCmd.Dir = townRoot
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	var convoys []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Type   string `json:"issue_type"`
	}
	if err := json.Unmarshal(out, &convoys); err != nil || len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]
	if convoy.Type != "convoy" {
		return fmt.Errorf("'%s' is not a convoy (type: %s)", convoyID, convoy.Type)
	}

	if convoy.Status == "closed" {
		reopenArgs := []string{"update", convoyID, "--status=open"}
		reopenCmd := bdcmd.Command( reopenArgs...)
		reopenCmd.Dir = townRoot
		if err := reopenCmd.Run(); err != nil {
			return fmt.Errorf("couldn't reopen convoy: %w", err)
		}
	}

	trackBeadID := FormatTrackBeadID(beadID, townRoot)
	depArgs := []string{"dep", "add", convoyID, trackBeadID, "--type=tracks"}
	depCmd := bdcmd.Command( depArgs...)
	depCmd.Dir = townRoot
	depCmd.Stderr = os.Stderr
	if err := depCmd.Run(); err != nil {
		return fmt.Errorf("adding tracking relation: %w", err)
	}

	return nil
}

// FormatTrackBeadID formats a bead ID for use in convoy tracking dependencies.
func FormatTrackBeadID(beadID, townRoot string) string {
	if strings.HasPrefix(beadID, "hq-") {
		return beadID
	}
	if extRef := beads.ResolveToExternalRef(townRoot, beadID); extRef != "" {
		return extRef
	}
	return beadID
}

func generateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}
