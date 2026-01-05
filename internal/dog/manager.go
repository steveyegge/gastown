package dog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
)

// Common errors
var (
	ErrDogExists   = errors.New("dog already exists")
	ErrDogNotFound = errors.New("dog not found")
	ErrNoRigs      = errors.New("no rigs configured")
)

// Manager handles dog lifecycle in the kennel.
type Manager struct {
	townRoot   string
	kennelPath string // ~/gt/deacon/dogs/
	rigsConfig *config.RigsConfig
}

// NewManager creates a new dog manager.
func NewManager(townRoot string, rigsConfig *config.RigsConfig) *Manager {
	return &Manager{
		townRoot:   townRoot,
		kennelPath: filepath.Join(townRoot, "deacon", "dogs"),
		rigsConfig: rigsConfig,
	}
}

// dogDir returns the directory for a dog.
func (m *Manager) dogDir(name string) string {
	return filepath.Join(m.kennelPath, name)
}

// exists checks if a dog exists.
func (m *Manager) exists(name string) bool {
	_, err := os.Stat(m.dogDir(name))
	return err == nil
}

// stateFilePath returns the path to a dog's state file.
func (m *Manager) stateFilePath(name string) string {
	return filepath.Join(m.dogDir(name), ".dog.json")
}

// Add creates a new dog in the kennel with worktrees into each rig.
// Each dog gets a worktree per rig (e.g., dogs/alpha/gastown/, dogs/alpha/beads/).
// Worktrees are created from each rig's bare repo (.repo.git) or mayor/rig.
func (m *Manager) Add(name string) (*Dog, error) {
	if m.exists(name) {
		return nil, ErrDogExists
	}

	// Verify we have rigs to create worktrees into
	if len(m.rigsConfig.Rigs) == 0 {
		return nil, ErrNoRigs
	}

	dogPath := m.dogDir(name)

	// Create kennel dir if needed
	if err := os.MkdirAll(m.kennelPath, 0755); err != nil {
		return nil, fmt.Errorf("creating kennel dir: %w", err)
	}

	// Create dog directory
	if err := os.MkdirAll(dogPath, 0755); err != nil {
		return nil, fmt.Errorf("creating dog dir: %w", err)
	}

	// Track cleanup on failure
	cleanup := func() { _ = os.RemoveAll(dogPath) }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Create worktrees into each rig
	worktrees := make(map[string]string)
	for rigName := range m.rigsConfig.Rigs {
		worktreePath, err := m.createRigWorktree(dogPath, name, rigName)
		if err != nil {
			return nil, fmt.Errorf("creating worktree for rig %s: %w", rigName, err)
		}
		worktrees[rigName] = worktreePath
	}

	// Create initial state file
	now := time.Now()
	state := &DogState{
		Name:       name,
		State:      StateIdle,
		LastActive: now,
		Worktrees:  worktrees,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := m.saveState(name, state); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	success = true
	return &Dog{
		Name:       name,
		State:      StateIdle,
		Path:       dogPath,
		Worktrees:  worktrees,
		LastActive: now,
		CreatedAt:  now,
	}, nil
}

// createRigWorktree creates a worktree for a dog into a specific rig.
// Uses the rig's bare repo (.repo.git) if available, otherwise mayor/rig.
// Branch naming: dog/<dog-name>-<rig>-<timestamp> for uniqueness.
func (m *Manager) createRigWorktree(dogPath, dogName, rigName string) (string, error) {
	rigPath := filepath.Join(m.townRoot, rigName)
	worktreePath := filepath.Join(dogPath, rigName)

	// Find the repo base (bare repo or mayor/rig)
	repoGit, err := m.findRepoBase(rigPath)
	if err != nil {
		return "", fmt.Errorf("finding repo base for %s: %w", rigName, err)
	}

	// Unique branch per dog-rig combination
	branchName := fmt.Sprintf("dog/%s-%s-%d", dogName, rigName, time.Now().UnixMilli())

	// Create worktree with new branch
	if err := repoGit.WorktreeAdd(worktreePath, branchName); err != nil {
		return "", fmt.Errorf("creating worktree: %w", err)
	}

	return worktreePath, nil
}

// findRepoBase locates the git repo base for a rig.
// Prefers .repo.git (bare repo), falls back to mayor/rig.
func (m *Manager) findRepoBase(rigPath string) (*git.Git, error) {
	// Check for shared bare repo
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if info, err := os.Stat(bareRepoPath); err == nil && info.IsDir() {
		return git.NewGitWithDir(bareRepoPath, ""), nil
	}

	// Fall back to mayor/rig
	mayorPath := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(mayorPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no repo base found (neither .repo.git nor mayor/rig exists)")
	}
	return git.NewGit(mayorPath), nil
}

// Remove deletes a dog from the kennel.
// Removes all worktrees and the dog directory.
func (m *Manager) Remove(name string) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	dogPath := m.dogDir(name)

	// Load state to get worktree paths
	state, err := m.loadState(name)
	if err != nil {
		// State file may be missing, proceed with cleanup
		state = &DogState{Worktrees: make(map[string]string)}
	}

	// Remove worktrees from each rig
	for rigName, worktreePath := range state.Worktrees {
		rigPath := filepath.Join(m.townRoot, rigName)
		repoGit, err := m.findRepoBase(rigPath)
		if err != nil {
			// Log but continue with other rigs
			fmt.Printf("Warning: could not find repo base for %s: %v\n", rigName, err)
			continue
		}

		// Try to remove worktree properly
		if err := repoGit.WorktreeRemove(worktreePath, true); err != nil {
			// Log but continue - will remove directory below
			fmt.Printf("Warning: could not remove worktree %s: %v\n", worktreePath, err)
		}

		// Prune stale entries
		_ = repoGit.WorktreePrune()
	}

	// Remove dog directory
	if err := os.RemoveAll(dogPath); err != nil {
		return fmt.Errorf("removing dog dir: %w", err)
	}

	return nil
}

// List returns all dogs in the kennel.
func (m *Manager) List() ([]*Dog, error) {
	entries, err := os.ReadDir(m.kennelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading kennel: %w", err)
	}

	var dogs []*Dog
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dog, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid dogs
		}
		dogs = append(dogs, dog)
	}

	return dogs, nil
}

// Get returns a specific dog by name.
func (m *Manager) Get(name string) (*Dog, error) {
	if !m.exists(name) {
		return nil, ErrDogNotFound
	}

	state, err := m.loadState(name)
	if err != nil {
		// Return minimal dog if state file is missing
		return &Dog{
			Name:  name,
			State: StateIdle,
			Path:  m.dogDir(name),
		}, nil
	}

	return &Dog{
		Name:       name,
		State:      state.State,
		Path:       m.dogDir(name),
		Worktrees:  state.Worktrees,
		LastActive: state.LastActive,
		Work:       state.Work,
		CreatedAt:  state.CreatedAt,
	}, nil
}

// SetState updates a dog's state and last-active timestamp.
func (m *Manager) SetState(name string, state State) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	dogState, err := m.loadState(name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	dogState.State = state
	dogState.LastActive = time.Now()
	dogState.UpdatedAt = time.Now()

	return m.saveState(name, dogState)
}

// AssignWork assigns work to a dog and sets it to working state.
func (m *Manager) AssignWork(name, work string) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	state, err := m.loadState(name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	state.State = StateWorking
	state.Work = work
	state.LastActive = time.Now()
	state.UpdatedAt = time.Now()

	return m.saveState(name, state)
}

// ClearWork clears a dog's work assignment and sets it to idle.
func (m *Manager) ClearWork(name string) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	state, err := m.loadState(name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	state.State = StateIdle
	state.Work = ""
	state.LastActive = time.Now()
	state.UpdatedAt = time.Now()

	return m.saveState(name, state)
}

// Refresh recreates all worktrees for a dog with fresh branches.
// This is useful when worktrees have drifted or become stale.
func (m *Manager) Refresh(name string) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	state, err := m.loadState(name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	dogPath := m.dogDir(name)
	newWorktrees := make(map[string]string)

	// Recreate each worktree
	for rigName := range m.rigsConfig.Rigs {
		rigPath := filepath.Join(m.townRoot, rigName)
		oldWorktreePath := state.Worktrees[rigName]

		// Find repo base
		repoGit, err := m.findRepoBase(rigPath)
		if err != nil {
			return fmt.Errorf("finding repo base for %s: %w", rigName, err)
		}

		// Remove old worktree if it exists
		if oldWorktreePath != "" {
			_ = repoGit.WorktreeRemove(oldWorktreePath, true)
			_ = os.RemoveAll(oldWorktreePath)
			_ = repoGit.WorktreePrune()
		}

		// Fetch latest from origin
		_ = repoGit.Fetch("origin")

		// Create fresh worktree
		worktreePath, err := m.createRigWorktree(dogPath, name, rigName)
		if err != nil {
			return fmt.Errorf("creating worktree for %s: %w", rigName, err)
		}
		newWorktrees[rigName] = worktreePath
	}

	// Update state
	state.Worktrees = newWorktrees
	state.LastActive = time.Now()
	state.UpdatedAt = time.Now()

	return m.saveState(name, state)
}

// RefreshRig recreates the worktree for a specific rig.
func (m *Manager) RefreshRig(name, rigName string) error {
	if !m.exists(name) {
		return ErrDogNotFound
	}

	if _, ok := m.rigsConfig.Rigs[rigName]; !ok {
		return fmt.Errorf("rig %s not found in config", rigName)
	}

	state, err := m.loadState(name)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	dogPath := m.dogDir(name)
	rigPath := filepath.Join(m.townRoot, rigName)
	oldWorktreePath := state.Worktrees[rigName]

	// Find repo base
	repoGit, err := m.findRepoBase(rigPath)
	if err != nil {
		return fmt.Errorf("finding repo base: %w", err)
	}

	// Remove old worktree if it exists
	if oldWorktreePath != "" {
		_ = repoGit.WorktreeRemove(oldWorktreePath, true)
		_ = os.RemoveAll(oldWorktreePath)
		_ = repoGit.WorktreePrune()
	}

	// Fetch latest
	_ = repoGit.Fetch("origin")

	// Create fresh worktree
	worktreePath, err := m.createRigWorktree(dogPath, name, rigName)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Update state
	state.Worktrees[rigName] = worktreePath
	state.LastActive = time.Now()
	state.UpdatedAt = time.Now()

	return m.saveState(name, state)
}

// CleanupStaleBranches removes orphaned dog branches from all rigs.
// Returns total branches deleted across all rigs.
func (m *Manager) CleanupStaleBranches() (int, error) {
	totalDeleted := 0

	for rigName := range m.rigsConfig.Rigs {
		rigPath := filepath.Join(m.townRoot, rigName)
		repoGit, err := m.findRepoBase(rigPath)
		if err != nil {
			continue
		}

		deleted, err := m.cleanupStaleBranchesForRig(repoGit, rigName)
		if err != nil {
			fmt.Printf("Warning: cleanup failed for rig %s: %v\n", rigName, err)
			continue
		}
		totalDeleted += deleted
	}

	return totalDeleted, nil
}

// cleanupStaleBranchesForRig removes orphaned dog branches in a specific rig.
func (m *Manager) cleanupStaleBranchesForRig(repoGit *git.Git, rigName string) (int, error) {
	// List all dog branches
	branches, err := repoGit.ListBranches("dog/*")
	if err != nil {
		return 0, err
	}

	if len(branches) == 0 {
		return 0, nil
	}

	// Get list of current dogs
	dogs, err := m.List()
	if err != nil {
		return 0, err
	}

	// Build set of current dog branches for this rig
	currentBranches := make(map[string]bool)
	for _, dog := range dogs {
		if dog.Worktrees != nil {
			if worktreePath, ok := dog.Worktrees[rigName]; ok {
				// Get branch name for this worktree
				worktreeGit := git.NewGit(worktreePath)
				if branch, err := worktreeGit.CurrentBranch(); err == nil {
					currentBranches[branch] = true
				}
			}
		}
	}

	// Delete orphaned branches
	deleted := 0
	for _, branch := range branches {
		if currentBranches[branch] {
			continue
		}
		if err := repoGit.DeleteBranch(branch, true); err != nil {
			fmt.Printf("Warning: could not delete branch %s: %v\n", branch, err)
			continue
		}
		deleted++
	}

	return deleted, nil
}

// loadState loads a dog's state from .dog.json.
func (m *Manager) loadState(name string) (*DogState, error) {
	data, err := os.ReadFile(m.stateFilePath(name))
	if err != nil {
		return nil, err
	}

	var state DogState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// saveState saves a dog's state to .dog.json.
func (m *Manager) saveState(name string, state *DogState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.stateFilePath(name), data, 0644) //nolint:gosec // G306: dog state is non-sensitive operational data
}

// GetIdleDog returns an idle dog suitable for work assignment.
// Returns nil if no idle dogs are available.
func (m *Manager) GetIdleDog() (*Dog, error) {
	dogs, err := m.List()
	if err != nil {
		return nil, err
	}

	for _, dog := range dogs {
		if dog.State == StateIdle {
			return dog, nil
		}
	}

	return nil, nil // No idle dogs
}

// IdleCount returns the number of idle dogs.
func (m *Manager) IdleCount() (int, error) {
	dogs, err := m.List()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, dog := range dogs {
		if dog.State == StateIdle {
			count++
		}
	}
	return count, nil
}

// WorkingCount returns the number of working dogs.
func (m *Manager) WorkingCount() (int, error) {
	dogs, err := m.List()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, dog := range dogs {
		if dog.State == StateWorking {
			count++
		}
	}
	return count, nil
}
