package rally

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// KnowledgeEntry is a single entry from the rally_tavern knowledge base.
// It covers all three subdirectory types: practices, solutions, learned.
type KnowledgeEntry struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Summary       string   `yaml:"summary"`
	Details       string   `yaml:"details"`
	Tags          []string `yaml:"tags"`
	CodebaseType  string   `yaml:"codebase_type"`
	ContributedBy string   `yaml:"contributed_by"`
	CreatedAt     string   `yaml:"created_at"`
	VerifiedBy    []string `yaml:"verified_by"`

	// Type-specific fields (practices)
	Gotchas  []string `yaml:"gotchas"`
	Examples []string `yaml:"examples"`

	// Type-specific fields (solutions)
	Problem  string `yaml:"problem"`
	Solution string `yaml:"solution"`

	// Type-specific fields (learned)
	Context string `yaml:"context"`
	Lesson  string `yaml:"lesson"`

	// Lifecycle metadata
	LastVerified string `yaml:"last_verified,omitempty"` // RFC3339 date last confirmed still valid
	Deprecated   bool   `yaml:"deprecated,omitempty"`    // true if entry is outdated
	SupersededBy string `yaml:"superseded_by,omitempty"` // ID of replacement entry

	// Internal metadata
	Kind string `yaml:"-"` // "practice", "solution", "learned"
}

// SearchQuery holds the criteria for a knowledge search.
type SearchQuery struct {
	Text         string   // freetext substring match against title/summary/details
	Tags         []string // exact tag matches (OR)
	CodebaseType string   // exact codebase_type match
}

// KnowledgeIndex holds all loaded knowledge entries.
type KnowledgeIndex struct {
	entries []KnowledgeEntry
}

// LoadKnowledgeIndex loads the rally_tavern knowledge base from
// $gtRoot/rally_tavern/mayor/rig/knowledge/.
//
// Returns (nil, nil) if rally_tavern is absent — callers must handle gracefully.
func LoadKnowledgeIndex(gtRoot string) (*KnowledgeIndex, error) {
	knowledgeRoot := filepath.Join(gtRoot, "rally_tavern", "mayor", "rig", "knowledge")

	if _, err := os.Stat(knowledgeRoot); os.IsNotExist(err) {
		return nil, nil
	}

	var entries []KnowledgeEntry

	kinds := []struct {
		dir  string
		kind string
	}{
		{"practices", "practice"},
		{"solutions", "solution"},
		{"learned", "learned"},
	}

	for _, k := range kinds {
		dir := filepath.Join(knowledgeRoot, k.dir)
		loaded, err := loadDir(dir, k.kind)
		if err != nil {
			return nil, err
		}
		entries = append(entries, loaded...)
	}

	return &KnowledgeIndex{entries: entries}, nil
}

// loadDir reads all .yaml files in dir and parses them as KnowledgeEntry.
func loadDir(dir, kind string) ([]KnowledgeEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var results []KnowledgeEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}

		var entry KnowledgeEntry
		if err := yaml.Unmarshal(data, &entry); err != nil {
			// Skip unparseable files rather than failing the whole index.
			continue
		}
		entry.Kind = kind
		results = append(results, entry)
	}
	return results, nil
}

// Search returns entries matching the query, sorted by relevance.
//
// Scoring:
//   - Exact tag match: +3 per tag hit
//   - CodebaseType match: +2
//   - Text match in title: +2
//   - Text match in summary/details/solution/lesson: +1
func (idx *KnowledgeIndex) Search(q SearchQuery) []KnowledgeEntry {
	type scored struct {
		entry KnowledgeEntry
		score int
	}

	var results []scored
	for _, e := range idx.entries {
		if e.Deprecated {
			continue // never surface deprecated entries in search
		}
		s := score(e, q)
		if s > 0 {
			results = append(results, scored{e, s})
		}
	}

	// Sort descending by score (simple insertion sort — small N).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	out := make([]KnowledgeEntry, len(results))
	for i, r := range results {
		out[i] = r.entry
	}
	return out
}

// Len returns the total number of indexed entries.
func (idx *KnowledgeIndex) Len() int {
	return len(idx.entries)
}

func score(e KnowledgeEntry, q SearchQuery) int {
	s := 0

	// Tag matches (OR — any matching tag counts).
	for _, qt := range q.Tags {
		for _, et := range e.Tags {
			if strings.EqualFold(qt, et) {
				s += 3
				break
			}
		}
	}

	// CodebaseType match.
	if q.CodebaseType != "" && strings.EqualFold(e.CodebaseType, q.CodebaseType) {
		s += 2
	}

	// Text matches.
	if q.Text != "" {
		lower := strings.ToLower(q.Text)
		if strings.Contains(strings.ToLower(e.Title), lower) {
			s += 2
		}
		for _, field := range []string{e.Summary, e.Details, e.Solution, e.Lesson, e.Problem} {
			if strings.Contains(strings.ToLower(field), lower) {
				s++
				break
			}
		}
	}

	return s
}
