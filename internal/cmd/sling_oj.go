// Package cmd provides OJ dispatch for gt sling.
// When GT_SLING_OJ=1, polecat lifecycle is managed by OJ instead of tmux.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

// ojSlingEnabled returns true when OJ dispatch is active.
func ojSlingEnabled() bool {
	return os.Getenv("GT_SLING_OJ") == "1"
}

// OjDispatchInfo contains info about a dispatched OJ sling job.
type OjDispatchInfo struct {
	JobID       string // OJ job ID returned by oj run
	PolecatName string // Allocated polecat name
	AgentID     string // Full agent ID (rig/polecats/name)
}

// dispatchToOj replaces SpawnPolecatForSling + SessionManager.Start with oj run.
// GT still owns: name allocation, formula instantiation, bead lifecycle.
// OJ owns: workspace creation, agent spawn, step execution, monitoring, crash recovery, cleanup.
func dispatchToOj(rigName string, opts SlingSpawnOptions, beadID, instructions, base, townRoot string) (*OjDispatchInfo, error) {
	// Load rig config for name allocation
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	// Allocate a polecat name (GT owns name allocation)
	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	polecatMgr := polecat.NewManager(r, polecatGit, t)

	polecatName, err := polecatMgr.AllocateName()
	if err != nil {
		return nil, fmt.Errorf("allocating polecat name: %w", err)
	}
	fmt.Printf("Allocated polecat: %s\n", polecatName)

	agentID := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)

	// Dispatch to OJ daemon via oj run
	ojArgs := []string{
		"run", "gt-sling",
		"--var", fmt.Sprintf("issue=%s", beadID),
		"--var", fmt.Sprintf("instructions=%s", instructions),
		"--var", fmt.Sprintf("base=%s", base),
		"--var", fmt.Sprintf("rig=%s", rigName),
		"--var", fmt.Sprintf("polecat_name=%s", polecatName),
		"--var", fmt.Sprintf("town_root=%s", townRoot),
	}

	cmd := exec.Command("oj", ojArgs...)
	cmd.Dir = townRoot
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	if err != nil {
		// OJ dispatch failed — release the allocated name to prevent leak
		fmt.Printf("%s OJ dispatch failed, releasing polecat name %s\n",
			style.Warning.Render("!"), polecatName)
		releasePolecatName(polecatMgr, polecatName)
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("oj run failed: %s\n%s", exitErr, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("oj run failed: %w", err)
	}

	// Parse OJ job ID from output
	jobID := parseOjJobID(string(output))
	if jobID == "" {
		fmt.Printf("%s Could not parse OJ job ID from output\n", style.Warning.Render("!"))
	}

	return &OjDispatchInfo{
		JobID:       jobID,
		PolecatName: polecatName,
		AgentID:     agentID,
	}, nil
}

// parseOjJobID extracts the job ID from oj run output.
// Expected format: "Job started: <id>" or JSON with "id" field.
func parseOjJobID(output string) string {
	output = strings.TrimSpace(output)

	// Try JSON first
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &result); err == nil && result.ID != "" {
		return result.ID
	}

	// Try "Job started: <id>" or "job_id: <id>" format
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Job started: ") {
			return strings.TrimPrefix(line, "Job started: ")
		}
		if strings.HasPrefix(line, "job_id: ") {
			return strings.TrimPrefix(line, "job_id: ")
		}
	}

	// Fallback: if single-line, treat as job ID
	if !strings.Contains(output, "\n") && len(output) < 64 && output != "" {
		return output
	}

	return ""
}

// storeOjJobIDInBead stores the OJ job ID in a bead's description.
func storeOjJobIDInBead(beadID, ojJobID string) error {
	if ojJobID == "" {
		return nil
	}

	showCmd := exec.Command("bd", "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("parsing bead: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("bead not found")
	}
	issue := &issues[0]

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	fields.OjJobID = ojJobID
	newDesc := beads.SetAttachmentFields(issue, fields)

	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// releasePolecatName attempts to release an allocated polecat name after OJ failure.
// This prevents name allocation leak (Risk #1 in the plan).
func releasePolecatName(mgr *polecat.Manager, name string) {
	if err := mgr.Remove(name, true); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not release polecat name %s: %v\n", name, err)
	}
}

// getBeadInstructions reads the instructions from a bead for passing to OJ.
func getBeadInstructions(beadID string) string {
	showCmd := exec.Command("bd", "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return ""
	}

	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		return ""
	}

	return issues[0].Title
}

// ensureOjRunbook ensures the gt-sling.hcl runbook exists in .oj/runbooks/.
// For V1, the runbook is stored in library/gastown/sling.hcl and copied to .oj/runbooks/.
func ensureOjRunbook(townRoot string) error {
	ojRunbookDir := filepath.Join(townRoot, ".oj", "runbooks")
	targetPath := filepath.Join(ojRunbookDir, "gt-sling.hcl")

	// Check if runbook already exists
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	// Try to find source from library path
	sourcePath := filepath.Join(townRoot, "library", "gastown", "sling.hcl")
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("runbook source not found at %s", sourcePath)
	}

	// Ensure directory exists
	if err := os.MkdirAll(ojRunbookDir, 0755); err != nil {
		return fmt.Errorf("creating runbook directory: %w", err)
	}

	// Copy the runbook
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("reading runbook: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("writing runbook: %w", err)
	}

	return nil
}

// GetBeadBase reads the base branch from a bead, defaulting to "main".
func GetBeadBase(beadID string) string {
	showCmd := exec.Command("bd", "show", beadID, "--json", "--allow-stale")
	out, err := showCmd.Output()
	if err != nil {
		return "main"
	}

	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		return "main"
	}

	// Check labels for base: prefix
	for _, label := range issues[0].Labels {
		if strings.HasPrefix(label, "base:") {
			return strings.TrimPrefix(label, "base:")
		}
	}

	return "main"
}

