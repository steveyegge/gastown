package rally

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TavernProfile holds the subset of tavern-profile.yaml fields used for
// knowledge matching. Full schema at rally_tavern/mayor/rig/templates/tavern-profile.yaml.
type TavernProfile struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	Facets struct {
		Platform     string   `yaml:"platform"`
		Languages    []string `yaml:"languages"`
		Frameworks   []string `yaml:"frameworks"`
		Runtime      string   `yaml:"runtime"`
		BuildSystem  string   `yaml:"build_system"`
	} `yaml:"facets"`

	Architecture struct {
		Style string `yaml:"style"`
	} `yaml:"architecture"`

	Security struct {
		AuthMethod    string   `yaml:"auth_method"`
		MultiTenant   bool     `yaml:"multi_tenant"`
		SensitiveData []string `yaml:"sensitive_data"`
	} `yaml:"security"`

	Constraints struct {
		MustUse  []string `yaml:"must_use"`
		MustAvoid []string `yaml:"must_avoid"`
	} `yaml:"constraints"`

	Needs []string `yaml:"needs"`
	Tags  []string `yaml:"tags"`
}

// LoadProfile reads tavern-profile.yaml from repoRoot.
//
// Returns (nil, nil) if the file is absent — callers must handle gracefully.
func LoadProfile(repoRoot string) (*TavernProfile, error) {
	path := filepath.Join(repoRoot, "tavern-profile.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var p TavernProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ToSearchQuery converts the profile into a SearchQuery for the knowledge index.
//
// Strategy:
//   - Tags from profile → tag matches
//   - Languages[0] combined with frameworks[0] → codebase_type candidate
//   - Needs + name → freetext
func (p *TavernProfile) ToSearchQuery() SearchQuery {
	q := SearchQuery{}

	// Tags directly from profile.
	q.Tags = append(q.Tags, p.Tags...)

	// Add languages and frameworks as tags too (common in knowledge entries).
	q.Tags = append(q.Tags, p.Facets.Languages...)
	q.Tags = append(q.Tags, p.Facets.Frameworks...)

	// Derive codebase_type from primary language + framework if available.
	if len(p.Facets.Languages) > 0 {
		lang := p.Facets.Languages[0]
		if len(p.Facets.Frameworks) > 0 {
			q.CodebaseType = lang + "-" + p.Facets.Frameworks[0]
		} else {
			q.CodebaseType = lang
		}
	}

	// Use first "need" as freetext if present.
	if len(p.Needs) > 0 {
		q.Text = p.Needs[0]
	}

	return q
}
