// Package witness provides the polecat monitoring agent.
package witness

import (
	"fmt"
	"regexp"
	"strings"
)

// Artisan protocol message prefixes.
const (
	ArtisanDonePrefix = "ARTISAN_DONE "
	ArtisanHelpPrefix = "ARTISAN_HELP: "
)

// PatternArtisanDone matches ARTISAN_DONE <name> subjects.
var PatternArtisanDone = regexp.MustCompile(`^ARTISAN_DONE\s+(\S+)`)

// ArtisanDonePayload contains parsed ARTISAN_DONE message fields.
type ArtisanDonePayload struct {
	Name     string
	ExitType string // COMPLETED, ESCALATED, DEFERRED, PHASE_COMPLETE
	BeadID   string
	MRID     string
	Branch   string
}

// ParseArtisanDone extracts payload from an ARTISAN_DONE message.
// Subject format: ARTISAN_DONE <artisan-name>
// Body format:
//
//	Exit: COMPLETED|ESCALATED|DEFERRED|PHASE_COMPLETE
//	Issue: <bead-id>
//	MR: <mr-id>
//	Branch: <branch>
func ParseArtisanDone(subject, body string) (*ArtisanDonePayload, error) {
	matches := PatternArtisanDone.FindStringSubmatch(subject)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid ARTISAN_DONE subject: %s", subject)
	}

	payload := &ArtisanDonePayload{
		Name: matches[1],
	}

	// Parse body for structured fields
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Exit:") {
			payload.ExitType = strings.TrimSpace(strings.TrimPrefix(line, "Exit:"))
		} else if strings.HasPrefix(line, "Issue:") {
			payload.BeadID = strings.TrimSpace(strings.TrimPrefix(line, "Issue:"))
		} else if strings.HasPrefix(line, "MR:") {
			payload.MRID = strings.TrimSpace(strings.TrimPrefix(line, "MR:"))
		} else if strings.HasPrefix(line, "Branch:") {
			payload.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		}
	}

	return payload, nil
}

// IsArtisanMessage returns true if the subject matches an artisan protocol message.
func IsArtisanMessage(subject string) bool {
	return strings.HasPrefix(subject, ArtisanDonePrefix) ||
		strings.HasPrefix(subject, ArtisanHelpPrefix)
}
