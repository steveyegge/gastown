package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

const (
	// defaultArchivistDogInterval is the patrol interval for scanning rig notes.
	defaultArchivistDogInterval = 5 * time.Minute

	// archivistDogCooldownPerRig is the minimum time between dispatches for a single rig.
	archivistDogCooldownPerRig = 10 * time.Minute

	// archivistDogDiagEveryN logs a diagnostic summary every N scans.
	archivistDogDiagEveryN = 10
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
// Arguments: rig name, notes dir (relative), domain dir (relative), file list.
const archivistPromptTemplate = `You are a bridge-local archivist for the "%s" rig.

Your job: process unprocessed domain notes and integrate them into the domain taxonomy.

Notes directory: %s
Domain directory: %s
Pending notes: %s

Instructions:
1. Read each .md file in the notes directory
2. For each note, extract key findings (API behaviors, build gotchas, data model insights, integration patterns)
3. Integrate findings into the appropriate domain document in the domain directory
4. If no appropriate document exists, create one with a clear filename
5. Update the domain README.md index to reference any new or updated documents
6. Delete each processed note file after its content is integrated
7. If a note contains unclear or incomplete information, integrate what you can and note gaps

Keep domain docs concise, source-attributed, and organized by topic.
Do NOT create git commits — file changes are committed by the bridge operator.`

// archivistDogCooldowns tracks per-rig dispatch cooldowns.
// Only accessed from the daemon's heartbeat goroutine, but wrapped
// in a mutex for safety in case that invariant changes.
type archivistDogCooldowns struct {
	mu          sync.Mutex
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

// rigNotes holds scan results for a single rig.
type rigNotes struct {
	Rig   string
	Files []string // relative paths within domain/notes/
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

	rigs := scanRigNotes(d.config.TownRoot)

	if len(rigs) == 0 {
		if scanNum%archivistDogDiagEveryN == 1 {
			d.logger.Printf("archivist_dog: alive (scan #%d, no unprocessed notes)", scanNum)
		}
		return
	}

	// Dispatch bridge-local archivists for rigs with pending notes.
	dispatched := 0
	skipped := 0
	for _, rn := range rigs {
		if !d.archivistCooldowns.canDispatch(rn.Rig) {
			d.logger.Printf("archivist_dog: %s has %d notes but in cooldown, skipping",
				rn.Rig, len(rn.Files))
			skipped++
			continue
		}

		d.logger.Printf("archivist_dog: %s has %d unprocessed notes: %s",
			rn.Rig, len(rn.Files), strings.Join(rn.Files, ", "))

		if err := d.dispatchBridgeArchivist(rn); err != nil {
			d.logger.Printf("archivist_dog: dispatch failed for %s: %v", rn.Rig, err)
			continue
		}

		d.archivistCooldowns.markDispatched(rn.Rig)
		dispatched++
	}

	d.logger.Printf("archivist_dog: scan #%d — %d rig(s) with notes, dispatched=%d skipped=%d (cooldown)",
		scanNum, len(rigs), dispatched, skipped)
}

// dispatchBridgeArchivist spawns a bridge-local archivist agent to process
// domain notes for a rig. The agent runs in the bridge root (townRoot) with
// no worktree — it reads/writes rigs/<rig>/domain/ directly.
func (d *Daemon) dispatchBridgeArchivist(rn rigNotes) error {
	agentCmd := archivistAgentCommand(d.patrolConfig)

	notesDir := filepath.Join("rigs", rn.Rig, "domain", "notes")
	domainDir := filepath.Join("rigs", rn.Rig, "domain")
	prompt := fmt.Sprintf(archivistPromptTemplate,
		rn.Rig, notesDir, domainDir, strings.Join(rn.Files, ", "))

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

	d.logger.Printf("archivist_dog: dispatched bridge-local archivist for %s (pid=%d, notes=%d)",
		rn.Rig, cmd.Process.Pid, len(rn.Files))

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
