package rig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestPath is the relative path for rig manifests inside a repo.
const ManifestPath = ".gt/rig.toml"

// ManifestVersion is the current supported manifest schema version.
const ManifestVersion = 1

// Manifest defines defaults for rig setup and crew creation.
type Manifest struct {
	Version int `toml:"version"`

	Rig struct {
		Name          string `toml:"name"`
		Prefix        string `toml:"prefix"`
		DefaultBranch string `toml:"default_branch"`
	} `toml:"rig"`

	Git struct {
		Upstream   string `toml:"upstream"`
		Origin     string `toml:"origin"`
		ForkPolicy string `toml:"fork_policy"`
	} `toml:"git"`

	Setup struct {
		Command string `toml:"command"`
		Workdir string `toml:"workdir"`
	} `toml:"setup"`

	Settings struct {
		Path string `toml:"path"`
	} `toml:"settings"`

	Crew []ManifestCrew `toml:"crew"`
}

// ManifestCrew defines a crew workspace entry in the manifest.
type ManifestCrew struct {
	Name    string            `toml:"name"`
	Agent   string            `toml:"agent"`
	Model   string            `toml:"model"`
	Account string            `toml:"account"`
	Branch  interface{}       `toml:"branch"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
}

// CrewSpec is the normalized crew configuration used by rig setup.
type CrewSpec struct {
	Name         string
	Agent        string
	Model        string
	Account      string
	CreateBranch bool
	BranchName   string
	Args         []string
	Env          map[string]string
}

// LoadManifest reads and parses a rig manifest from the repo root.
// Returns (nil, nil) if the manifest is not present.
func LoadManifest(repoRoot string) (*Manifest, error) {
	path := filepath.Join(repoRoot, ManifestPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest Manifest
	if _, err := toml.Decode(string(data), &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// Validate ensures the manifest uses a supported version.
func (m *Manifest) Validate() error {
	if m.Version == 0 {
		return fmt.Errorf("manifest version missing (expected %d)", ManifestVersion)
	}
	if m.Version != ManifestVersion {
		return fmt.Errorf("unsupported manifest version %d (expected %d)", m.Version, ManifestVersion)
	}
	return nil
}

// CrewSpecs returns normalized crew entries from the manifest.
func (m *Manifest) CrewSpecs() ([]CrewSpec, error) {
	specs := make([]CrewSpec, 0, len(m.Crew))
	for _, entry := range m.Crew {
		spec, err := normalizeCrew(entry)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// CrewByName returns crew specs keyed by name.
func (m *Manifest) CrewByName() (map[string]CrewSpec, error) {
	specs, err := m.CrewSpecs()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]CrewSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	return byName, nil
}

func normalizeCrew(entry ManifestCrew) (CrewSpec, error) {
	if strings.TrimSpace(entry.Name) == "" {
		return CrewSpec{}, fmt.Errorf("crew entry missing name")
	}

	spec := CrewSpec{
		Name:    entry.Name,
		Agent:   entry.Agent,
		Model:   entry.Model,
		Account: entry.Account,
		Args:    entry.Args,
		Env:     entry.Env,
	}

	switch v := entry.Branch.(type) {
	case nil:
		// no branch override
	case bool:
		spec.CreateBranch = v
	case string:
		if strings.TrimSpace(v) != "" {
			spec.CreateBranch = true
			spec.BranchName = v
		}
	default:
		return CrewSpec{}, fmt.Errorf("crew %s has invalid branch value", entry.Name)
	}

	return spec, nil
}
