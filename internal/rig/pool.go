package rig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/git"
)

// PoolRegistry tracks all refs in the shared pool.
type PoolRegistry struct {
	Version int                     `json:"version"`
	Refs    map[string]PoolRefEntry `json:"refs"`
}

// PoolRefEntry describes a single ref in the shared pool.
type PoolRefEntry struct {
	GitURL     string    `json:"git_url"`
	Branch     string    `json:"branch,omitempty"`
	Shallow    bool      `json:"shallow,omitempty"`
	AddedByTown string   `json:"added_by_town,omitempty"`
	AddedAt    time.Time `json:"added_at"`
}

// ResolvePoolPath returns the shared ref pool path from GT_REF_POOL env var.
// Returns "" if unset.
func ResolvePoolPath() string {
	return os.Getenv("GT_REF_POOL")
}

// poolRegistryPath returns the path to registry.json in the pool.
func poolRegistryPath(poolPath string) string {
	return filepath.Join(poolPath, "registry.json")
}

// LoadPoolRegistry reads the pool registry from disk.
// Returns an empty registry if the file doesn't exist.
func LoadPoolRegistry(poolPath string) (*PoolRegistry, error) {
	regPath := poolRegistryPath(poolPath)
	data, err := os.ReadFile(regPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PoolRegistry{Version: 1, Refs: make(map[string]PoolRefEntry)}, nil
		}
		return nil, fmt.Errorf("reading pool registry: %w", err)
	}

	var reg PoolRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing pool registry: %w", err)
	}
	if reg.Refs == nil {
		reg.Refs = make(map[string]PoolRefEntry)
	}
	return &reg, nil
}

// SavePoolRegistry writes the pool registry to disk.
func SavePoolRegistry(poolPath string, reg *PoolRegistry) error {
	if err := os.MkdirAll(poolPath, 0755); err != nil {
		return fmt.Errorf("creating pool dir: %w", err)
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pool registry: %w", err)
	}
	return os.WriteFile(poolRegistryPath(poolPath), data, 0644)
}

// PoolRefPath returns the on-disk path for a pool ref clone.
func PoolRefPath(poolPath, alias string) string {
	return filepath.Join(poolPath, alias)
}

// RegisterPoolRef ensures a ref is cloned into the pool and registered.
// If the clone already exists, it's a no-op for the clone but updates the registry entry.
func RegisterPoolRef(poolPath, alias, gitURL, branch, townName string) error {
	if err := os.MkdirAll(poolPath, 0755); err != nil {
		return fmt.Errorf("creating pool dir: %w", err)
	}

	reg, err := LoadPoolRegistry(poolPath)
	if err != nil {
		return err
	}

	dest := PoolRefPath(poolPath, alias)

	// Clone if not already present on disk
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		g := git.NewGit("")
		if branch != "" {
			if err := g.CloneBranch(gitURL, dest, branch); err != nil {
				return fmt.Errorf("cloning to pool: %w", err)
			}
		} else {
			if err := g.Clone(gitURL, dest); err != nil {
				return fmt.Errorf("cloning to pool: %w", err)
			}
		}
	}

	reg.Refs[alias] = PoolRefEntry{
		GitURL:      gitURL,
		Branch:      branch,
		Shallow:     true,
		AddedByTown: townName,
		AddedAt:     time.Now(),
	}

	return SavePoolRegistry(poolPath, reg)
}

// RemovePoolRef removes a ref from the pool registry and deletes its clone.
func RemovePoolRef(poolPath, alias string) error {
	reg, err := LoadPoolRegistry(poolPath)
	if err != nil {
		return err
	}

	if _, exists := reg.Refs[alias]; !exists {
		return fmt.Errorf("pool ref %q not found", alias)
	}

	// Remove clone
	dest := PoolRefPath(poolPath, alias)
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("removing pool clone: %w", err)
	}

	delete(reg.Refs, alias)
	return SavePoolRegistry(poolPath, reg)
}

// SyncPoolRef pulls the latest for a single pool ref.
func SyncPoolRef(poolPath, alias string) error {
	reg, err := LoadPoolRegistry(poolPath)
	if err != nil {
		return err
	}

	entry, exists := reg.Refs[alias]
	if !exists {
		return fmt.Errorf("pool ref %q not found", alias)
	}

	dest := PoolRefPath(poolPath, alias)
	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("pool ref %q clone missing at %s", alias, dest)
	}

	g := git.NewGit(dest)
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching pool ref %q: %w", alias, err)
	}

	branch := entry.Branch
	if branch == "" {
		var bErr error
		branch, bErr = g.CurrentBranch()
		if bErr != nil {
			return fmt.Errorf("detecting branch for pool ref %q: %w", alias, bErr)
		}
	}

	return g.Pull("origin", branch)
}

// SyncAllPoolRefs pulls the latest for all pool refs.
func SyncAllPoolRefs(poolPath string) error {
	reg, err := LoadPoolRegistry(poolPath)
	if err != nil {
		return err
	}

	for alias := range reg.Refs {
		if err := SyncPoolRef(poolPath, alias); err != nil {
			return fmt.Errorf("syncing pool ref %q: %w", alias, err)
		}
	}
	return nil
}

// NormalizeGitURL canonicalizes a git URL to a comparable form.
// Both "git@github.com:org/repo.git" and "https://github.com/org/repo"
// become "github.com/org/repo".
func NormalizeGitURL(url string) string {
	s := url

	// Strip trailing .git
	s = strings.TrimSuffix(s, ".git")

	// Handle SSH format: git@github.com:org/repo
	if strings.HasPrefix(s, "git@") {
		s = strings.TrimPrefix(s, "git@")
		s = strings.Replace(s, ":", "/", 1)
		return s
	}

	// Handle HTTPS format: https://github.com/org/repo
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	return s
}

// GitURLsMatch returns true if two git URLs point to the same repository.
func GitURLsMatch(a, b string) bool {
	return NormalizeGitURL(a) == NormalizeGitURL(b)
}
