package artisan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Common errors
var (
	ErrArtisanExists      = errors.New("artisan already exists")
	ErrArtisanNotFound    = errors.New("artisan not found")
	ErrInvalidArtisanName = errors.New("invalid artisan name")
)

// Manager manages artisan workspaces within a rig.
type Manager struct {
	rigName  string
	rigPath  string
	townRoot string
}

// NewManager creates a new artisan manager for the given rig.
func NewManager(rigName, rigPath, townRoot string) *Manager {
	return &Manager{
		rigName:  rigName,
		rigPath:  rigPath,
		townRoot: townRoot,
	}
}

// artisanDir returns the path to an artisan's directory.
func (m *Manager) artisanDir(name string) string {
	return filepath.Join(m.rigPath, "artisans", name)
}

// stateFile returns the path to an artisan's state file.
func (m *Manager) stateFile(name string) string {
	return filepath.Join(m.artisanDir(name), "state.json")
}

// lockPath returns the path to the lock file for an artisan.
func (m *Manager) lockPath(name string) string {
	lockDir := filepath.Join(m.townRoot, ".runtime", "locks")
	return filepath.Join(lockDir, fmt.Sprintf("artisan-%s.lock", name))
}

// lock acquires an exclusive lock for the named artisan.
func (m *Manager) lock(name string) (*flock.Flock, error) {
	lockDir := filepath.Dir(m.lockPath(name))
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	fl := flock.New(m.lockPath(name))
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquiring lock for artisan %s: %w", name, err)
	}
	return fl, nil
}

// ValidateName checks if a name is valid for an artisan.
// Artisan names must be lowercase with optional hyphens and numbers (e.g., "frontend-1").
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidArtisanName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: name cannot be . or ..", ErrInvalidArtisanName)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%w: name cannot contain / or \\", ErrInvalidArtisanName)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: name cannot contain ..", ErrInvalidArtisanName)
	}
	if strings.Contains(name, " ") {
		return fmt.Errorf("%w: name cannot contain spaces", ErrInvalidArtisanName)
	}
	return nil
}

// Add creates a new artisan workspace directory with state metadata.
// The actual git clone is handled by the caller (cmd layer) since it
// requires rig-level infrastructure (git, remotes, beads).
func (m *Manager) Add(name, specialty string) (*Worker, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	fl, err := m.lock(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fl.Unlock() }()

	dir := m.artisanDir(name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrArtisanExists, name)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating artisan directory: %w", err)
	}

	// Create mail directory
	if err := os.MkdirAll(filepath.Join(dir, "mail"), 0o755); err != nil {
		return nil, fmt.Errorf("creating mail directory: %w", err)
	}

	now := time.Now()
	worker := &Worker{
		Name:      name,
		Rig:       m.rigName,
		Specialty: specialty,
		ClonePath: dir,
		Branch:    "main",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := m.saveState(worker); err != nil {
		// Clean up on failure
		_ = os.RemoveAll(dir)
		return nil, err
	}

	return worker, nil
}

// Remove removes an artisan workspace.
func (m *Manager) Remove(name string, force bool) error {
	fl, err := m.lock(name)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Unlock() }()

	dir := m.artisanDir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrArtisanNotFound, name)
	}

	return os.RemoveAll(dir)
}

// List returns all artisan workers in this rig.
func (m *Manager) List() ([]*Worker, error) {
	artisansDir := filepath.Join(m.rigPath, "artisans")
	entries, err := os.ReadDir(artisansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading artisans directory: %w", err)
	}

	var workers []*Worker
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		worker, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid entries
		}
		workers = append(workers, worker)
	}
	return workers, nil
}

// Get returns the artisan worker with the given name.
func (m *Manager) Get(name string) (*Worker, error) {
	dir := m.artisanDir(name)
	stateFile := m.stateFile(name)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory exists but no state file — return minimal info
			if _, statErr := os.Stat(dir); statErr == nil {
				return &Worker{
					Name:      name,
					Rig:       m.rigName,
					ClonePath: dir,
				}, nil
			}
			return nil, fmt.Errorf("%w: %s", ErrArtisanNotFound, name)
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var worker Worker
	if err := json.Unmarshal(data, &worker); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	// Directory name is source of truth
	worker.Name = name
	worker.ClonePath = dir
	if worker.Rig == "" {
		worker.Rig = m.rigName
	}

	return &worker, nil
}

// saveState writes the worker state to disk.
func (m *Manager) saveState(worker *Worker) error {
	data, err := json.MarshalIndent(worker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(m.stateFile(worker.Name), data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	return nil
}
