package rig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/git"
)

// RefsDir returns the path to the refs directory for a rig.
func RefsDir(rigPath string) string {
	return filepath.Join(rigPath, "refs")
}

// RefPath returns the path to a specific ref within a rig.
func RefPath(rigPath, alias string) string {
	return filepath.Join(rigPath, "refs", alias)
}

// RefStatus describes the current state of a linked ref.
type RefStatus struct {
	Alias    string `json:"alias"`
	Type     string `json:"type"`   // "clone" or "symlink"
	Path     string `json:"path"`   // resolved path on disk
	Status   string `json:"status"` // "ok", "missing", "broken"
	GitURL   string `json:"git_url,omitempty"`
	FromRig  string `json:"from_rig,omitempty"`
	Branch   string `json:"branch,omitempty"`
}

// LinkExternalRef clones an external git repo as a read-only reference.
// Uses shallow, single-branch clone for efficiency.
func LinkExternalRef(rigPath, alias, gitURL, branch string) error {
	if err := ValidateRefAlias(alias); err != nil {
		return err
	}

	dest := RefPath(rigPath, alias)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("ref %q already exists at %s", alias, dest)
	}

	// Ensure refs directory exists
	if err := os.MkdirAll(RefsDir(rigPath), 0755); err != nil {
		return fmt.Errorf("creating refs dir: %w", err)
	}

	g := git.NewGit("")
	if branch != "" {
		return g.CloneBranch(gitURL, dest, branch)
	}
	return g.Clone(gitURL, dest)
}

// LinkSameTownRef creates a symlink to a sibling rig's refinery clone.
// Zero disk cost, always current.
func LinkSameTownRef(townRoot, rigPath, alias, fromRig string) error {
	if err := ValidateRefAlias(alias); err != nil {
		return err
	}

	dest := RefPath(rigPath, alias)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("ref %q already exists at %s", alias, dest)
	}

	// Validate source rig exists and has refinery/rig/
	sourceRigPath := filepath.Join(townRoot, fromRig)
	sourceClone := filepath.Join(sourceRigPath, "refinery", "rig")
	if _, err := os.Stat(sourceClone); err != nil {
		return fmt.Errorf("source rig %q has no refinery/rig/ directory: %w", fromRig, err)
	}

	// Ensure refs directory exists
	if err := os.MkdirAll(RefsDir(rigPath), 0755); err != nil {
		return fmt.Errorf("creating refs dir: %w", err)
	}

	// Create relative symlink for portability
	rel, err := filepath.Rel(RefsDir(rigPath), sourceClone)
	if err != nil {
		// Fall back to absolute symlink
		rel = sourceClone
	}

	return os.Symlink(rel, dest)
}

// UnlinkRef removes a linked reference (symlink or clone).
func UnlinkRef(rigPath, alias string) error {
	dest := RefPath(rigPath, alias)

	info, err := os.Lstat(dest)
	if err != nil {
		return fmt.Errorf("ref %q not found: %w", alias, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(dest)
	}
	return os.RemoveAll(dest)
}

// SyncRef pulls the latest for a cloned ref. No-op for symlinks.
func SyncRef(rigPath, alias string) error {
	dest := RefPath(rigPath, alias)

	info, err := os.Lstat(dest)
	if err != nil {
		return fmt.Errorf("ref %q not found: %w", alias, err)
	}

	// Symlinks are always current — nothing to sync
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	g := git.NewGit(dest)
	if err := g.Fetch("origin"); err != nil {
		return fmt.Errorf("fetching ref %q: %w", alias, err)
	}

	branch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("detecting branch for ref %q: %w", alias, err)
	}

	return g.Pull("origin", branch)
}

// SyncAllRefs syncs all cloned refs. Returns the first error encountered.
func SyncAllRefs(rigPath string, refs map[string]RefEntry) error {
	for alias, entry := range refs {
		if entry.FromRig != "" {
			continue // symlinks don't need sync
		}
		if err := SyncRef(rigPath, alias); err != nil {
			return fmt.Errorf("syncing ref %q: %w", alias, err)
		}
	}
	return nil
}

// ListRefs returns status information for all configured refs.
func ListRefs(rigPath string, refs map[string]RefEntry) []RefStatus {
	var result []RefStatus
	for alias, entry := range refs {
		rs := RefStatus{
			Alias:   alias,
			GitURL:  entry.GitURL,
			FromRig: entry.FromRig,
			Branch:  entry.Branch,
		}

		dest := RefPath(rigPath, alias)
		info, err := os.Lstat(dest)
		if err != nil {
			rs.Status = "missing"
			rs.Path = dest
			result = append(result, rs)
			continue
		}

		rs.Path = dest

		if info.Mode()&os.ModeSymlink != 0 {
			rs.Type = "symlink"
			target, err := os.Readlink(dest)
			if err != nil {
				rs.Status = "broken"
			} else {
				// Resolve relative to refs dir
				resolved := target
				if !filepath.IsAbs(target) {
					resolved = filepath.Join(RefsDir(rigPath), target)
				}
				if _, err := os.Stat(resolved); err != nil {
					rs.Status = "broken"
				} else {
					rs.Status = "ok"
				}
			}
		} else {
			rs.Type = "clone"
			rs.Status = "ok"
		}

		result = append(result, rs)
	}
	return result
}
