package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// bdMolTimeout is the timeout for bd molecule operations.
	bdMolTimeout = 15 * time.Second
)

// dogMol tracks a molecule (wisp) lifecycle for a daemon dog patrol.
// Graceful degradation: if bd fails, the dog still does its work — molecule
// tracking is observability, not control flow.
type dogMol struct {
	rootID   string // Root wisp ID (e.g., "gt-wisp-abc123"), empty if pour failed.
	stepIDs  map[string]string // step slug -> wisp issue ID
	bdPath   string
	townRoot string
	logger   interface{ Printf(string, ...interface{}) }
}

// pourDogMolecule creates an ephemeral wisp molecule from a formula.
// Returns a dogMol handle for closing steps. If bd fails, returns a no-op
// handle so the caller can proceed without error checking.
func (d *Daemon) pourDogMolecule(formulaName string, vars map[string]string) *dogMol {
	dm := &dogMol{
		stepIDs:  make(map[string]string),
		bdPath:   d.bdPath,
		townRoot: d.config.TownRoot,
		logger:   d.logger,
	}

	// Build args: bd mol wisp <formula> --var k=v ...
	args := []string{"mol", "wisp", formulaName}
	for k, v := range vars {
		args = append(args, "--var", fmt.Sprintf("%s=%s", k, v))
	}

	out, err := dm.runBd(args...)
	if err != nil {
		d.logger.Printf("dog_molecule: pour %s failed (non-fatal): %v", formulaName, err)
		return dm
	}

	// Parse root ID from output. bd mol wisp prints the root ID on the first line.
	// Example output: "✓ Spawned wisp: gt-wisp-abc123 — Reap stale wisps..."
	dm.rootID = parseWispID(out)
	if dm.rootID == "" {
		d.logger.Printf("dog_molecule: pour %s: could not parse root ID from output: %s", formulaName, out)
		return dm
	}

	// Discover step IDs by listing children of the root wisp.
	dm.discoverSteps()

	d.logger.Printf("dog_molecule: poured %s → %s (%d steps)", formulaName, dm.rootID, len(dm.stepIDs))
	return dm
}

// closeStep marks a molecule step as closed.
func (dm *dogMol) closeStep(stepSlug string) {
	if dm.rootID == "" {
		return // No molecule — graceful degradation.
	}

	stepID, ok := dm.stepIDs[stepSlug]
	if !ok {
		dm.logger.Printf("dog_molecule: closeStep %q: unknown step (known: %v)", stepSlug, dm.knownSteps())
		return
	}

	_, err := dm.runBd("close", stepID)
	if err != nil {
		dm.logger.Printf("dog_molecule: close step %s (%s) failed (non-fatal): %v", stepSlug, stepID, err)
		return
	}
}

// failStep marks a molecule step as failed with a reason.
func (dm *dogMol) failStep(stepSlug, reason string) {
	if dm.rootID == "" {
		return
	}

	stepID, ok := dm.stepIDs[stepSlug]
	if !ok {
		dm.logger.Printf("dog_molecule: failStep %q: unknown step", stepSlug)
		return
	}

	_, err := dm.runBd("close", stepID, "--reason", reason)
	if err != nil {
		dm.logger.Printf("dog_molecule: fail step %s (%s) failed (non-fatal): %v", stepSlug, stepID, err)
	}
}

// close closes the root molecule wisp.
func (dm *dogMol) close() {
	if dm.rootID == "" {
		return
	}

	_, err := dm.runBd("close", dm.rootID)
	if err != nil {
		dm.logger.Printf("dog_molecule: close root %s failed (non-fatal): %v", dm.rootID, err)
	}
}

// discoverSteps lists children of the root wisp and maps step slugs to IDs.
// Step titles in the formula are like "Scan databases for stale wisps" —
// we match on the step ID embedded in the wisp title or metadata.
func (dm *dogMol) discoverSteps() {
	if dm.rootID == "" {
		return
	}

	// Use bd show to get children. The mol wisp command creates child wisps
	// whose titles include the step ID from the formula.
	out, err := dm.runBd("show", dm.rootID, "--children", "--format=jsonl")
	if err != nil {
		dm.logger.Printf("dog_molecule: discover steps for %s failed: %v", dm.rootID, err)
		return
	}

	// Parse children output. Each line has an issue with title containing the step slug.
	// Format from bd show --children --format=jsonl: {"id":"...","title":"step: Scan databases...","status":"open"}
	// We extract step slugs by matching known formula step patterns.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		// Simple parsing: look for id and title fields.
		id := extractJSONField(line, "id")
		title := extractJSONField(line, "title")
		if id == "" || title == "" {
			continue
		}

		// Map known step slugs from the title. The wisp title typically starts
		// with the step title from the formula.
		titleLower := strings.ToLower(title)
		switch {
		case strings.Contains(titleLower, "scan"):
			dm.stepIDs["scan"] = id
		case strings.Contains(titleLower, "reap"):
			dm.stepIDs["reap"] = id
		case strings.Contains(titleLower, "purge"):
			dm.stepIDs["purge"] = id
		case strings.Contains(titleLower, "report"):
			dm.stepIDs["report"] = id
		case strings.Contains(titleLower, "export"):
			dm.stepIDs["export"] = id
		case strings.Contains(titleLower, "push"):
			dm.stepIDs["push"] = id
		case strings.Contains(titleLower, "diagnos"):
			dm.stepIDs["diagnose"] = id
		case strings.Contains(titleLower, "backup"):
			dm.stepIDs["backup"] = id
		case strings.Contains(titleLower, "probe"):
			dm.stepIDs["probe"] = id
		case strings.Contains(titleLower, "inspect"):
			dm.stepIDs["inspect"] = id
		case strings.Contains(titleLower, "clean"):
			dm.stepIDs["clean"] = id
		case strings.Contains(titleLower, "verif"):
			dm.stepIDs["verify"] = id
		case strings.Contains(titleLower, "compact"):
			dm.stepIDs["compact"] = id
		}
	}
}

// knownSteps returns the list of known step slugs for debugging.
func (dm *dogMol) knownSteps() []string {
	var steps []string
	for k := range dm.stepIDs {
		steps = append(steps, k)
	}
	return steps
}

// runBd executes a bd command and returns stdout.
func (dm *dogMol) runBd(args ...string) (string, error) {
	bdPath := dm.bdPath
	if bdPath == "" {
		bdPath = "bd"
	}

	ctx, cancel := context.WithTimeout(context.Background(), bdMolTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bdPath, args...)
	cmd.Dir = dm.townRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s: %s", err, errMsg)
		}
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// parseWispID extracts a wisp ID from bd mol wisp output.
// Looks for patterns like "gt-wisp-abc123" or any ID containing "-wisp-".
func parseWispID(output string) string {
	for _, word := range strings.Fields(output) {
		// Strip ANSI codes and punctuation.
		cleaned := stripANSI(word)
		cleaned = strings.TrimRight(cleaned, ".,;:!?")
		if strings.Contains(cleaned, "-wisp-") {
			return cleaned
		}
	}
	// Fallback: look for any bead-like ID (prefix-xxxx pattern).
	for _, word := range strings.Fields(output) {
		cleaned := stripANSI(word)
		cleaned = strings.TrimRight(cleaned, ".,;:!?")
		if len(cleaned) > 3 && strings.Contains(cleaned, "-") && !strings.HasPrefix(cleaned, "--") {
			// Could be a bead ID like "gt-abc123".
			return cleaned
		}
	}
	return ""
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip escape sequence.
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++ // Skip the terminating letter.
				}
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// extractJSONField does a simple extraction of a string field from a JSON line.
// This avoids importing encoding/json for a simple parse operation.
func extractJSONField(line, field string) string {
	key := fmt.Sprintf(`"%s":"`, field)
	idx := strings.Index(line, key)
	if idx < 0 {
		// Try with space after colon.
		key = fmt.Sprintf(`"%s": "`, field)
		idx = strings.Index(line, key)
		if idx < 0 {
			return ""
		}
	}
	start := idx + len(key)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}
