package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	// defaultArchivistDogInterval is the patrol interval for scanning rig notes.
	defaultArchivistDogInterval = 5 * time.Minute

	// archivistDogCooldownPerRig is the minimum time between dispatches for a single rig.
	archivistDogCooldownPerRig = 10 * time.Minute

	// archivistDogDiagEveryN logs a diagnostic summary every N scans.
	archivistDogDiagEveryN = 10

	// archivistDogBeadQueryTimeout is the max time to wait for bd subprocess calls.
	archivistDogBeadQueryTimeout = 30 * time.Second

	// archivistDogBeadListLimit caps the number of closed beads returned per scan.
	archivistDogBeadListLimit = 500
)

// archivistDogScanCount tracks how many scans have run.
var archivistDogScanCount atomic.Int64

// ArchivistDogConfig holds configuration for the archivist_dog patrol.
// This patrol scans rigs for unprocessed domain notes and dispatches
// bridge-local archivists to extract knowledge from them.
type ArchivistDogConfig struct {
	Enabled      bool   `json:"enabled"`
	IntervalStr  string `json:"interval,omitempty"`
	AgentCommand string `json:"agent_command,omitempty"` // default: "claude"
}

const defaultArchivistAgentCommand = "claude"

// archivistAgentCommand returns the configured agent command, or the default.
func archivistAgentCommand(config *DaemonPatrolConfig) string {
	if config != nil && config.Patrols != nil && config.Patrols.ArchivistDog != nil {
		if config.Patrols.ArchivistDog.AgentCommand != "" {
			return config.Patrols.ArchivistDog.AgentCommand
		}
	}
	return defaultArchivistAgentCommand
}

// archivistPromptTemplate is the prompt given to the bridge-local archivist agent.
// Arguments: rig name, notes dir (relative), domain dir (relative), file list,
// bead-notes section (may be empty string when no bead sources are attached).
const archivistPromptTemplate = `You are a bridge-local archivist for the "%s" rig.

Your job: process unprocessed domain notes and integrate them into the domain taxonomy.

You have TWO input sources this run:

  (A) File-based raw notes in %s — listed below, one markdown file per finding.
  (B) Bead/wisp turn-notes — attached inline below (when present). These are the
      turn-by-turn playback that polecats wrote during the work, captured as
      "bd notes" on a bead or wisp. They are DURABLE HISTORY in Dolt and must
      NOT be deleted.

Domain directory: %s
Pending note files (source A): %s
%s

Instructions:
1. Read each .md file in %s (source A).
2. For each attached bead/wisp notes block below (source B), treat the note text
   as a turn-by-turn playback of one polecat's reasoning.
3. Extract key findings from BOTH sources: API behaviors, build gotchas, data
   model insights, integration patterns, and the reasoning trail (alternatives
   considered, rejected paths, corrections, backtracks).
4. Integrate factual findings into the appropriate curated domain document.
   Integrate reasoning / turn-level detail into the "## Background" or
   "## Reasoning" section of that curated doc (see the section structure in
   formulas/laws-prospector.formula.toml § 8). If the curated doc has no such
   section, add one.
5. If no appropriate document exists, create one with a clear filename.
6. Update the domain README.md index to reference any new or updated documents.
7. DELETE each processed note FILE (source A) after its content is integrated.
8. Do NOT delete or modify any bead/wisp (source B) — they are durable history.
   The daemon's archivist-state tracker records that their notes were collated.
9. If a note contains unclear or incomplete information, integrate what you can
   and note gaps.

Keep domain docs concise, source-attributed, and organized by topic.
Do NOT create git commits — file changes are committed by the bridge operator.`

// archivistDogCooldowns tracks per-rig dispatch cooldowns.
// Only accessed from the daemon's heartbeat goroutine, but wrapped
// in a mutex for safety in case that invariant changes.
type archivistDogCooldowns struct {
	mu             sync.Mutex
	lastDispatched map[string]time.Time // rig name -> last dispatch time
}

func newArchivistDogCooldowns() *archivistDogCooldowns {
	return &archivistDogCooldowns{
		lastDispatched: make(map[string]time.Time),
	}
}

// canDispatch returns true if enough time has passed since the last dispatch for this rig.
func (c *archivistDogCooldowns) canDispatch(rig string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	last, ok := c.lastDispatched[rig]
	if !ok {
		return true
	}
	return time.Since(last) >= archivistDogCooldownPerRig
}

// markDispatched records that a dispatch was made for this rig.
func (c *archivistDogCooldowns) markDispatched(rig string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastDispatched[rig] = time.Now()
}

// archivistDogInterval returns the configured interval, or the default.
func archivistDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.ArchivistDog != nil {
		if config.Patrols.ArchivistDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.ArchivistDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultArchivistDogInterval
}

// beadNote is a single pickup source backed by a bead or wisp with non-empty notes.
type beadNote struct {
	ID    string // bead ID, e.g. "sbx-gastown-326f" or "sbx-gastown-wisp-abc"
	Kind  string // "bead" or "wisp"
	Title string
	Notes string
}

// rigNotes holds scan results for a single rig.
type rigNotes struct {
	Rig       string
	Files     []string   // relative paths within domain/notes/
	BeadNotes []beadNote // bead/wisp notes attached to this rig (source B)
}

// scanRigNotes scans townRoot for rigs with unprocessed domain notes.
// Returns a list of rigs that have .md files in rigs/<rig>/domain/notes/.
func scanRigNotes(townRoot string) []rigNotes {
	var results []rigNotes

	entries, err := os.ReadDir(filepath.Join(townRoot, "rigs"))
	if err != nil {
		// Try top-level rig directories (symlink layout)
		entries, err = os.ReadDir(townRoot)
		if err != nil {
			return nil
		}
	}

	for _, entry := range entries {
		rigName := entry.Name()
		if strings.HasPrefix(rigName, ".") {
			continue
		}

		// Check both rigs/<rig>/domain/notes/ and <rig>/domain/notes/ layouts
		notesDir := filepath.Join(townRoot, "rigs", rigName, "domain", "notes")
		info, err := os.Stat(notesDir)
		if err != nil || !info.IsDir() {
			// Try top-level layout
			notesDir = filepath.Join(townRoot, rigName, "domain", "notes")
			info, err = os.Stat(notesDir)
			if err != nil || !info.IsDir() {
				continue
			}
		}

		noteEntries, err := os.ReadDir(notesDir)
		if err != nil {
			continue
		}

		var files []string
		for _, ne := range noteEntries {
			if ne.IsDir() || strings.HasPrefix(ne.Name(), ".") {
				continue
			}
			if strings.HasSuffix(ne.Name(), ".md") {
				files = append(files, ne.Name())
			}
		}

		if len(files) > 0 {
			results = append(results, rigNotes{Rig: rigName, Files: files})
		}
	}

	return results
}

// rawBeadListEntry is the subset of `bd list --json` output we care about.
type rawBeadListEntry struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Notes  string   `json:"notes"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

// scanBeadNotes queries beads for closed bead/wisp entries with non-empty notes
// attached to a rig label. Entries already marked collated in state are
// filtered out. The result is grouped by rig.
func scanBeadNotes(ctx context.Context, townRoot string, state *archivistDogState) (map[string][]beadNote, error) {
	cmd := exec.CommandContext(ctx, "bd", "list",
		"--status=closed",
		"--json",
		"--flat",
		"--limit", fmt.Sprintf("%d", archivistDogBeadListLimit),
	)
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(townRoot))
	util.SetDetachedProcessGroup(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var entries []rawBeadListEntry
	if err := json.Unmarshal(bytes.TrimSpace(output), &entries); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	return filterBeadNotesForPickup(entries, state), nil
}

// filterBeadNotesForPickup is the pure part of scanBeadNotes — it takes raw
// bead entries plus the current state and returns a rig -> []beadNote map
// containing only entries that have non-empty notes, a rig label, and have
// not already been marked collated.
func filterBeadNotesForPickup(entries []rawBeadListEntry, state *archivistDogState) map[string][]beadNote {
	out := make(map[string][]beadNote)
	for _, e := range entries {
		notes := strings.TrimSpace(e.Notes)
		if notes == "" {
			continue
		}
		if state != nil && state.IsCollated(e.ID) {
			continue
		}
		rig := pickRigLabel(e.Labels)
		if rig == "" {
			continue
		}
		kind := "bead"
		if strings.Contains(e.ID, "-wisp-") {
			kind = "wisp"
		}
		out[rig] = append(out[rig], beadNote{
			ID:    e.ID,
			Kind:  kind,
			Title: e.Title,
			Notes: notes,
		})
	}
	return out
}

// infraLabels are labels that do not identify a rig.
var infraLabels = map[string]bool{
	"prime":      true,
	"wisp":       true,
	"molecule":   true,
	"convoy":     true,
	"template":   true,
	"agent":      true,
	"role":       true,
	"message":    true,
	"escalation": true,
}

// pickRigLabel returns the first label that is plausibly a rig name.
// Bead labels may include infra tags like "prime" that are not rigs; we skip
// those. The first remaining label is treated as the rig.
func pickRigLabel(labels []string) string {
	for _, l := range labels {
		if l == "" {
			continue
		}
		if infraLabels[l] {
			continue
		}
		return l
	}
	return ""
}

// mergeRigNotes merges file-based and bead-note scans into a single []rigNotes.
// Rigs that appear in either source are included.
func mergeRigNotes(fileResults []rigNotes, beadResults map[string][]beadNote) []rigNotes {
	byRig := make(map[string]*rigNotes)
	for i := range fileResults {
		rn := fileResults[i]
		byRig[rn.Rig] = &rigNotes{Rig: rn.Rig, Files: rn.Files}
	}
	for rig, notes := range beadResults {
		if rn, ok := byRig[rig]; ok {
			rn.BeadNotes = notes
		} else {
			byRig[rig] = &rigNotes{Rig: rig, BeadNotes: notes}
		}
	}
	out := make([]rigNotes, 0, len(byRig))
	for _, rn := range byRig {
		out = append(out, *rn)
	}
	return out
}

// archivistDogState is the local tracker persisted to daemon/archivist-state.json.
// It records which bead/wisp IDs have already had their notes collated, so the
// archivist_dog does not re-dispatch on every scan.
type archivistDogState struct {
	Collated map[string]collatedEntry `json:"collated"`
}

type collatedEntry struct {
	LastCollatedAt string `json:"last_collated_at"`
}

// IsCollated reports whether the given bead/wisp ID has already been collated.
func (s *archivistDogState) IsCollated(id string) bool {
	if s == nil || s.Collated == nil {
		return false
	}
	_, ok := s.Collated[id]
	return ok
}

// MarkCollated records that the given bead/wisp ID has been handed to an archivist.
func (s *archivistDogState) MarkCollated(id string, at time.Time) {
	if s.Collated == nil {
		s.Collated = make(map[string]collatedEntry)
	}
	s.Collated[id] = collatedEntry{LastCollatedAt: at.UTC().Format(time.RFC3339)}
}

func archivistStatePath(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "archivist-state.json")
}

func loadArchivistDogState(townRoot string) *archivistDogState {
	state := &archivistDogState{Collated: make(map[string]collatedEntry)}
	data, err := os.ReadFile(archivistStatePath(townRoot))
	if err != nil {
		return state
	}
	_ = json.Unmarshal(data, state)
	if state.Collated == nil {
		state.Collated = make(map[string]collatedEntry)
	}
	return state
}

func saveArchivistDogState(townRoot string, state *archivistDogState) error {
	if state == nil {
		return nil
	}
	dir := filepath.Join(townRoot, "daemon")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(archivistStatePath(townRoot), data, 0644)
}

// runArchivistDog is the daemon patrol method that scans for unprocessed rig notes
// and dispatches archivists. Called on each ticker fire.
func (d *Daemon) runArchivistDog() {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Printf("archivist_dog: recovered from panic: %v", r)
		}
	}()

	if !d.isPatrolActive("archivist_dog") {
		return
	}

	scanNum := archivistDogScanCount.Add(1)

	state := loadArchivistDogState(d.config.TownRoot)

	fileResults := scanRigNotes(d.config.TownRoot)

	ctx, cancel := context.WithTimeout(d.ctx, archivistDogBeadQueryTimeout)
	defer cancel()

	beadResults, err := scanBeadNotes(ctx, d.config.TownRoot, state)
	if err != nil {
		// Bead scan failures are non-fatal — we still want the file-based path
		// to work even if bd is unreachable. Log and continue.
		d.logger.Printf("archivist_dog: bead scan failed: %v", err)
		beadResults = nil
	}

	rigs := mergeRigNotes(fileResults, beadResults)

	if len(rigs) == 0 {
		if scanNum%archivistDogDiagEveryN == 1 {
			d.logger.Printf("archivist_dog: alive (scan #%d, no unprocessed notes)", scanNum)
		}
		return
	}

	// Dispatch bridge-local archivists for rigs with pending notes.
	dispatched := 0
	skipped := 0
	stateDirty := false
	for _, rn := range rigs {
		if !d.archivistCooldowns.canDispatch(rn.Rig) {
			d.logger.Printf("archivist_dog: %s has %d file-notes + %d bead-notes but in cooldown, skipping",
				rn.Rig, len(rn.Files), len(rn.BeadNotes))
			skipped++
			continue
		}

		d.logger.Printf("archivist_dog: %s has %d file-notes (%s) + %d bead-notes",
			rn.Rig, len(rn.Files), strings.Join(rn.Files, ", "), len(rn.BeadNotes))

		if err := d.dispatchBridgeArchivist(rn); err != nil {
			d.logger.Printf("archivist_dog: dispatch failed for %s: %v", rn.Rig, err)
			continue
		}

		d.archivistCooldowns.markDispatched(rn.Rig)
		dispatched++

		// Mark bead/wisp notes collated on successful dispatch. If the archivist
		// itself subsequently fails mid-collation, notes are still durable in
		// bd — an operator can delete entries from archivist-state.json to
		// force re-pickup.
		now := time.Now()
		for _, bn := range rn.BeadNotes {
			state.MarkCollated(bn.ID, now)
			stateDirty = true
		}
	}

	if stateDirty {
		if err := saveArchivistDogState(d.config.TownRoot, state); err != nil {
			d.logger.Printf("archivist_dog: failed to save state: %v", err)
		}
	}

	d.logger.Printf("archivist_dog: scan #%d — %d rig(s) with notes, dispatched=%d skipped=%d (cooldown)",
		scanNum, len(rigs), dispatched, skipped)
}

// renderBeadNotesSection formats the bead/wisp notes block for inline inclusion
// in the archivist prompt. Returns an empty string when no bead notes attach.
func renderBeadNotesSection(notes []beadNote) string {
	if len(notes) == 0 {
		return "Attached bead/wisp notes (source B): none this run."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Attached bead/wisp notes (source B): %d entries.\n", len(notes))
	for _, n := range notes {
		fmt.Fprintf(&b, "\n--- %s (%s) — %s ---\n", n.ID, n.Kind, n.Title)
		b.WriteString(n.Notes)
		if !strings.HasSuffix(n.Notes, "\n") {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// dispatchBridgeArchivist spawns a bridge-local archivist agent to process
// domain notes for a rig. The agent runs in the bridge root (townRoot) with
// no worktree — it reads/writes rigs/<rig>/domain/ directly.
func (d *Daemon) dispatchBridgeArchivist(rn rigNotes) error {
	agentCmd := archivistAgentCommand(d.patrolConfig)

	notesDir := filepath.Join("rigs", rn.Rig, "domain", "notes")
	domainDir := filepath.Join("rigs", rn.Rig, "domain")
	beadSection := renderBeadNotesSection(rn.BeadNotes)
	fileList := strings.Join(rn.Files, ", ")
	if fileList == "" {
		fileList = "(none)"
	}
	prompt := fmt.Sprintf(archivistPromptTemplate,
		rn.Rig, notesDir, domainDir, fileList, beadSection, notesDir)

	args := []string{
		"-p", prompt,
		"--allowed-tools", "Read,Write,Edit,Glob,Grep,Bash",
		"--dangerously-skip-permissions",
		"--bare",
	}
	cmd := exec.Command(agentCmd, args...)
	cmd.Dir = d.config.TownRoot
	util.SetDetachedProcessGroup(cmd)

	// Direct output to a per-rig log file for debugging.
	logDir := filepath.Join(d.config.TownRoot, "daemon")
	_ = os.MkdirAll(logDir, 0755)
	logPath := filepath.Join(logDir, fmt.Sprintf("archivist-%s.log", rn.Rig))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		d.logger.Printf("archivist_dog: warning: can't open log %s: %v", logPath, err)
		// Continue without log capture — dispatch is more important.
	} else {
		fmt.Fprintf(logFile, "\n--- archivist dispatch %s ---\n", time.Now().UTC().Format(time.RFC3339))
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return fmt.Errorf("start %s: %w", agentCmd, err)
	}

	d.logger.Printf("archivist_dog: dispatched bridge-local archivist for %s (pid=%d, file-notes=%d, bead-notes=%d)",
		rn.Rig, cmd.Process.Pid, len(rn.Files), len(rn.BeadNotes))

	// Don't wait — let the archivist run in the background.
	// Goroutine reaps the process and closes the log file.
	go func() {
		_ = cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
	}()

	return nil
}
