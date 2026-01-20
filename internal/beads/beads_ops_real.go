package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// RealBeadsOps implements BeadsOps using the actual beads CLI.
type RealBeadsOps struct {
	townRoot string
}

// NewRealBeadsOps creates a new RealBeadsOps for the given town root.
func NewRealBeadsOps(townRoot string) *RealBeadsOps {
	return &RealBeadsOps{townRoot: townRoot}
}

// IsTownLevelBead returns true if the bead is a town-level bead (hq-* prefix).
func (r *RealBeadsOps) IsTownLevelBead(beadID string) bool {
	return strings.HasPrefix(beadID, "hq-")
}

// GetRigForBead returns the rig name for a given bead ID based on its prefix.
func (r *RealBeadsOps) GetRigForBead(beadID string) string {
	return GetRigNameForBead(r.townRoot, beadID)
}

// LabelAdd adds a label to a bead.
func (r *RealBeadsOps) LabelAdd(beadID, label string) error {
	// Resolve the bead's directory from its prefix
	prefix := ExtractPrefix(beadID)
	rigPath := GetRigPathForPrefix(r.townRoot, prefix)
	if rigPath == "" {
		rigPath = r.townRoot // fallback
	}

	return r.runBd(rigPath, "update", beadID, "--add-label", label)
}

// LabelRemove removes a label from a bead.
func (r *RealBeadsOps) LabelRemove(beadID, label string) error {
	// Resolve the bead's directory from its prefix
	prefix := ExtractPrefix(beadID)
	rigPath := GetRigPathForPrefix(r.townRoot, prefix)
	if rigPath == "" {
		rigPath = r.townRoot // fallback
	}

	return r.runBd(rigPath, "update", beadID, "--remove-label", label)
}

// ListByLabelAllRigs returns all beads with the given label across all rigs.
func (r *RealBeadsOps) ListByLabelAllRigs(label string) (map[string][]BeadInfo, error) {
	result := make(map[string][]BeadInfo)

	// Get all rigs from rigs.json
	rigs, err := r.getAllRigs()
	if err != nil {
		return nil, err
	}

	// Query each rig for beads with the label
	for rigName, rigPath := range rigs {
		issues, err := r.listBeads(rigPath, "open", label)
		if err != nil {
			// Skip rigs that fail - they might not have the label or beads DB
			continue
		}

		var beads []BeadInfo
		for _, issue := range issues {
			beads = append(beads, BeadInfo{
				ID:     issue.ID,
				Status: issue.Status,
				Labels: issue.Labels,
			})
		}

		if len(beads) > 0 {
			result[rigName] = beads
		}
	}

	return result, nil
}

// getAllRigs returns a map of rig name to rig path from rigs.json.
func (r *RealBeadsOps) getAllRigs() (map[string]string, error) {
	rigsConfigPath := filepath.Join(r.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil, err
	}

	rigs := make(map[string]string)
	for rigName := range rigsConfig.Rigs {
		// Rig path is townRoot/rigName
		rigs[rigName] = filepath.Join(r.townRoot, rigName)
	}

	return rigs, nil
}

// runBd runs a bd command in the specified directory.
func (r *RealBeadsOps) runBd(dir string, args ...string) error {
	fullArgs := append([]string{"--no-daemon"}, args...)
	cmd := exec.Command("bd", fullArgs...)
	cmd.Dir = dir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd %s: %s", strings.Join(args, " "), stderr.String())
	}

	return nil
}

// listBeads runs bd list in the specified directory and returns parsed issues.
func (r *RealBeadsOps) listBeads(dir, status, label string) ([]*Issue, error) {
	args := []string{"--no-daemon", "list", "--json"}
	if status != "" {
		args = append(args, "--status="+status)
	}
	if label != "" {
		args = append(args, "--label="+label)
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bd list: %s", stderr.String())
	}

	var issues []*Issue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}
