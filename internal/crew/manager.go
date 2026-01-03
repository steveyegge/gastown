package crew

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/util"
)

// Common errors
var (
	ErrCrewExists   = errors.New("crew worker already exists")
	ErrCrewNotFound = errors.New("crew worker not found")
	ErrHasChanges   = errors.New("crew worker has uncommitted changes")
)

// Manager handles crew worker lifecycle.
type Manager struct {
	rig *rig.Rig
	git *git.Git
}

// NewManager creates a new crew manager.
func NewManager(r *rig.Rig, g *git.Git) *Manager {
	return &Manager{
		rig: r,
		git: g,
	}
}

// crewDir returns the directory for a crew worker.
func (m *Manager) crewDir(name string) string {
	return filepath.Join(m.rig.Path, "crew", name)
}

// stateFile returns the state file path for a crew worker.
func (m *Manager) stateFile(name string) string {
	return filepath.Join(m.crewDir(name), "state.json")
}

// mailDir returns the mail directory path for a crew worker.
func (m *Manager) mailDir(name string) string {
	return filepath.Join(m.crewDir(name), "mail")
}

// exists checks if a crew worker exists.
func (m *Manager) exists(name string) bool {
	_, err := os.Stat(m.crewDir(name))
	return err == nil
}

// Add creates a new crew worker with a clone of the rig.
func (m *Manager) Add(name string, createBranch bool) (*CrewWorker, error) {
	if m.exists(name) {
		return nil, ErrCrewExists
	}

	crewPath := m.crewDir(name)

	// Create crew directory if needed
	crewBaseDir := filepath.Join(m.rig.Path, "crew")
	if err := os.MkdirAll(crewBaseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating crew dir: %w", err)
	}

	// Clone the rig repo
	if m.rig.LocalRepo != "" {
		if err := m.git.CloneWithReference(m.rig.GitURL, crewPath, m.rig.LocalRepo); err != nil {
			fmt.Printf("Warning: could not clone with local repo reference: %v\n", err)
			if err := m.git.Clone(m.rig.GitURL, crewPath); err != nil {
				return nil, fmt.Errorf("cloning rig: %w", err)
			}
		}
	} else {
		if err := m.git.Clone(m.rig.GitURL, crewPath); err != nil {
			return nil, fmt.Errorf("cloning rig: %w", err)
		}
	}

	crewGit := git.NewGit(crewPath)
	branchName := "main"

	// Optionally create a working branch
	if createBranch {
		branchName = fmt.Sprintf("crew/%s", name)
		if err := crewGit.CreateBranch(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("creating branch: %w", err)
		}
		if err := crewGit.Checkout(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("checking out branch: %w", err)
		}
	}

	// Create mail directory for mail delivery
	mailPath := m.mailDir(name)
	if err := os.MkdirAll(mailPath, 0755); err != nil {
		_ = os.RemoveAll(crewPath) // best-effort cleanup
		return nil, fmt.Errorf("creating mail dir: %w", err)
	}

	// Set up shared beads: crew uses rig's shared beads via redirect file
	if err := m.setupSharedBeads(crewPath); err != nil {
		// Non-fatal - crew can still work, warn but don't fail
		fmt.Printf("Warning: could not set up shared beads: %v\n", err)
	}

	// Provision .claude/commands/ with standard slash commands (e.g., /handoff)
	// This ensures crew workers have Gas Town utilities even if source repo lacks them.
	if err := templates.ProvisionCommands(crewPath); err != nil {
		// Non-fatal - crew can still work, warn but don't fail
		fmt.Printf("Warning: could not provision slash commands: %v\n", err)
	}

	// NOTE: We intentionally do NOT write to CLAUDE.md here.
	// Gas Town context is injected ephemerally via SessionStart hook (gt prime).
	// Writing to CLAUDE.md would overwrite project instructions and leak
	// Gas Town internals into the project repo when workers commit/push.

	// Create crew worker state
	now := time.Now()
	crew := &CrewWorker{
		Name:      name,
		Rig:       m.rig.Name,
		ClonePath: crewPath,
		Branch:    branchName,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save state
	if err := m.saveState(crew); err != nil {
		_ = os.RemoveAll(crewPath) // best-effort cleanup
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return crew, nil
}

// Remove deletes a crew worker.
func (m *Manager) Remove(name string, force bool) error {
	if !m.exists(name) {
		return ErrCrewNotFound
	}

	crewPath := m.crewDir(name)

	if !force {
		crewGit := git.NewGit(crewPath)
		hasChanges, err := crewGit.HasUncommittedChanges()
		if err == nil && hasChanges {
			return ErrHasChanges
		}
	}

	// Remove directory
	if err := os.RemoveAll(crewPath); err != nil {
		return fmt.Errorf("removing crew dir: %w", err)
	}

	return nil
}

// List returns all crew workers in the rig.
func (m *Manager) List() ([]*CrewWorker, error) {
	crewBaseDir := filepath.Join(m.rig.Path, "crew")

	entries, err := os.ReadDir(crewBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading crew dir: %w", err)
	}

	var workers []*CrewWorker
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		worker, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid workers
		}
		workers = append(workers, worker)
	}

	return workers, nil
}

// Get returns a specific crew worker by name.
func (m *Manager) Get(name string) (*CrewWorker, error) {
	if !m.exists(name) {
		return nil, ErrCrewNotFound
	}

	return m.loadState(name)
}

// saveState persists crew worker state to disk using atomic write.
func (m *Manager) saveState(crew *CrewWorker) error {
	stateFile := m.stateFile(crew.Name)
	if err := util.AtomicWriteJSON(stateFile, crew); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// loadState reads crew worker state from disk.
func (m *Manager) loadState(name string) (*CrewWorker, error) {
	stateFile := m.stateFile(name)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return minimal crew worker if state file missing
			return &CrewWorker{
				Name:      name,
				Rig:       m.rig.Name,
				ClonePath: m.crewDir(name),
			}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var crew CrewWorker
	if err := json.Unmarshal(data, &crew); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	// Backfill essential fields if missing (handles empty or incomplete state.json)
	if crew.Name == "" {
		crew.Name = name
	}
	if crew.Rig == "" {
		crew.Rig = m.rig.Name
	}
	if crew.ClonePath == "" {
		crew.ClonePath = m.crewDir(name)
	}

	return &crew, nil
}

// Rename renames a crew worker from oldName to newName.
func (m *Manager) Rename(oldName, newName string) error {
	if !m.exists(oldName) {
		return ErrCrewNotFound
	}
	if m.exists(newName) {
		return ErrCrewExists
	}

	oldPath := m.crewDir(oldName)
	newPath := m.crewDir(newName)

	// Rename directory
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("renaming crew dir: %w", err)
	}

	// Update state file with new name and path
	crew, err := m.loadState(newName)
	if err != nil {
		// Rollback on error (best-effort)
		_ = os.Rename(newPath, oldPath)
		return fmt.Errorf("loading state: %w", err)
	}

	crew.Name = newName
	crew.ClonePath = newPath
	crew.UpdatedAt = time.Now()

	if err := m.saveState(crew); err != nil {
		// Rollback on error (best-effort)
		_ = os.Rename(newPath, oldPath)
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// Pristine ensures a crew worker is up-to-date with remote.
// It runs git pull --rebase and bd sync.
func (m *Manager) Pristine(name string) (*PristineResult, error) {
	if !m.exists(name) {
		return nil, ErrCrewNotFound
	}

	crewPath := m.crewDir(name)
	crewGit := git.NewGit(crewPath)

	result := &PristineResult{
		Name: name,
	}

	// Check for uncommitted changes
	hasChanges, err := crewGit.HasUncommittedChanges()
	if err != nil {
		return nil, fmt.Errorf("checking changes: %w", err)
	}
	result.HadChanges = hasChanges

	// Pull latest (use origin and current branch)
	if err := crewGit.Pull("origin", ""); err != nil {
		result.PullError = err.Error()
	} else {
		result.Pulled = true
	}

	// Run bd sync
	if err := m.runBdSync(crewPath); err != nil {
		result.SyncError = err.Error()
	} else {
		result.Synced = true
	}

	return result, nil
}

// runBdSync runs bd sync in the given directory.
func (m *Manager) runBdSync(dir string) error {
	cmd := exec.Command("bd", "sync")
	cmd.Dir = dir
	return cmd.Run()
}

// PristineResult captures the results of a pristine operation.
type PristineResult struct {
	Name       string `json:"name"`
	HadChanges bool   `json:"had_changes"`
	Pulled     bool   `json:"pulled"`
	PullError  string `json:"pull_error,omitempty"`
	Synced     bool   `json:"synced"`
	SyncError  string `json:"sync_error,omitempty"`
}

// setupSharedBeads creates a redirect file so the crew worker uses the rig's shared .beads database.
// This eliminates the need for git sync between crew clones - all crew members share one database.
//
// Structure:
//
//	rig/
//	  mayor/rig/.beads/     <- Shared database (the canonical location)
//	  crew/
//	    <name>/
//	      .beads/
//	        redirect        <- Contains "../../mayor/rig/.beads"
func (m *Manager) setupSharedBeads(crewPath string) error {
	// The shared beads database is at rig/mayor/rig/.beads/
	// Crew clones are at rig/crew/<name>/
	// So the relative path is ../../mayor/rig/.beads
	sharedBeadsPath := filepath.Join(m.rig.Path, "mayor", "rig", ".beads")

	// Verify the shared beads exists
	if _, err := os.Stat(sharedBeadsPath); os.IsNotExist(err) {
		// Fall back to rig root .beads if mayor/rig doesn't exist
		sharedBeadsPath = filepath.Join(m.rig.Path, ".beads")
		if _, err := os.Stat(sharedBeadsPath); os.IsNotExist(err) {
			return fmt.Errorf("no shared beads database found")
		}
	}

	// Create crew's .beads directory
	crewBeadsDir := filepath.Join(crewPath, ".beads")
	if err := os.MkdirAll(crewBeadsDir, 0755); err != nil {
		return fmt.Errorf("creating crew .beads dir: %w", err)
	}

	// Calculate relative path from crew/.beads/ to shared beads
	// crew/<name>/.beads/ -> ../../mayor/rig/.beads or ../../.beads
	var redirectContent string
	if _, err := os.Stat(filepath.Join(m.rig.Path, "mayor", "rig", ".beads")); err == nil {
		redirectContent = "../../mayor/rig/.beads\n"
	} else {
		redirectContent = "../../.beads\n"
	}

	// Create redirect file
	redirectPath := filepath.Join(crewBeadsDir, "redirect")
	if err := os.WriteFile(redirectPath, []byte(redirectContent), 0644); err != nil {
		return fmt.Errorf("creating redirect file: %w", err)
	}

	return nil
}
