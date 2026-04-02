package sourcemap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Store manages source map files on the filesystem.
// Files are stored under {root}/{project_id}/{release}/{filename}.
type Store struct {
	Root string // e.g. ~/.faultline/sourcemaps
}

// NewStore creates a Store with the given root directory.
func NewStore(root string) *Store {
	return &Store{Root: root}
}

// DefaultStore creates a Store using ~/.faultline/sourcemaps.
func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("sourcemap store: %w", err)
	}
	root := filepath.Join(home, ".faultline", "sourcemaps")
	return NewStore(root), nil
}

func (s *Store) dir(projectID int64, release string) string {
	return filepath.Join(s.Root, strconv.FormatInt(projectID, 10), release)
}

// Save writes a source map file to disk.
func (s *Store) Save(projectID int64, release, filename string, data []byte) error {
	dir := s.dir(projectID, release)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("sourcemap save mkdir: %w", err)
	}
	path := filepath.Join(dir, filepath.Base(filename))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("sourcemap save write: %w", err)
	}
	return nil
}

// Load reads a source map file from disk.
func (s *Store) Load(projectID int64, release, filename string) ([]byte, error) {
	path := filepath.Join(s.dir(projectID, release), filepath.Base(filename))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sourcemap load: %w", err)
	}
	return data, nil
}

// Delete removes all source map files for a release.
func (s *Store) Delete(projectID int64, release string) error {
	dir := s.dir(projectID, release)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("sourcemap delete: %w", err)
	}
	return nil
}

// List returns the filenames of all source maps uploaded for a release.
func (s *Store) List(projectID int64, release string) ([]string, error) {
	dir := s.dir(projectID, release)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("sourcemap list: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
