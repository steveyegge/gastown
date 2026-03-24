package rally

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Nomination is a candidate knowledge entry submitted by an agent.
// It is serialized as YAML for transit in mail bodies.
type Nomination struct {
	// Required fields
	Category string `yaml:"category"` // "practice", "solution", "learned"
	Title    string `yaml:"title"`
	Summary  string `yaml:"summary"`

	// Optional common fields
	Details      string   `yaml:"details,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
	CodebaseType string   `yaml:"codebase_type,omitempty"`

	// Category-specific fields
	Gotchas  []string `yaml:"gotchas,omitempty"`  // practice
	Examples []string `yaml:"examples,omitempty"` // practice
	Problem  string   `yaml:"problem,omitempty"`  // solution
	Solution string   `yaml:"solution,omitempty"` // solution
	Context  string   `yaml:"context,omitempty"`  // learned
	Lesson   string   `yaml:"lesson,omitempty"`   // learned

	// Provenance (auto-populated by gt rally nominate, not agent-supplied)
	NominatedBy  string `yaml:"nominated_by"`
	NominatedAt  string `yaml:"nominated_at"`
	SourceIssue  string `yaml:"source_issue,omitempty"`
	NominationID string `yaml:"nomination_id"`
}

// Valid knowledge categories.
var validCategories = map[string]bool{
	"practice": true,
	"solution": true,
	"learned":  true,
}

// mailBodyPrefix is the protocol sentinel for nomination wire format.
const mailBodyPrefix = "RALLY_NOMINATION_V1\n---\n"

// Validate checks that the nomination has required fields and valid values.
func (n *Nomination) Validate() error {
	if !validCategories[n.Category] {
		return fmt.Errorf("category must be one of: practice, solution, learned (got %q)", n.Category)
	}
	if strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if len(n.Title) > 120 {
		return fmt.Errorf("title must be 120 characters or fewer (got %d)", len(n.Title))
	}
	if strings.TrimSpace(n.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	return nil
}

// ToMailBody serializes the nomination to the wire format used in mail bodies.
func (n *Nomination) ToMailBody() (string, error) {
	data, err := yaml.Marshal(n)
	if err != nil {
		return "", fmt.Errorf("serializing nomination: %w", err)
	}
	return mailBodyPrefix + string(data), nil
}

// ParseNominationFromMailBody parses a nomination from a mail body.
// Returns an error if the body does not start with the expected sentinel.
func ParseNominationFromMailBody(body string) (*Nomination, error) {
	if !strings.HasPrefix(body, mailBodyPrefix) {
		return nil, fmt.Errorf("not a nomination mail body (missing RALLY_NOMINATION_V1 sentinel)")
	}
	yamlPart := strings.TrimPrefix(body, mailBodyPrefix)
	var n Nomination
	if err := yaml.Unmarshal([]byte(yamlPart), &n); err != nil {
		return nil, fmt.Errorf("parsing nomination YAML: %w", err)
	}
	return &n, nil
}

// ToKnowledgeEntry converts an accepted nomination into a KnowledgeEntry
// ready to be written to the knowledge directory.
func (n *Nomination) ToKnowledgeEntry() KnowledgeEntry {
	slug := titleToSlug(n.Title)
	suffix := n.NominationID
	if suffix != "" {
		// use just the hex part after "nom-"
		suffix = strings.TrimPrefix(suffix, "nom-")
	}

	id := slug
	if suffix != "" {
		id = slug + "-" + suffix
	}

	e := KnowledgeEntry{
		ID:           id,
		Title:        n.Title,
		Summary:      n.Summary,
		Details:      n.Details,
		Tags:         n.Tags,
		CodebaseType: n.CodebaseType,
		Kind:         n.Category,
		// Provenance
		Gotchas:  n.Gotchas,
		Solution: n.Solution,
		Lesson:   n.Lesson,
	}
	return e
}

// GenerateNominationID returns a unique nomination ID in the form "nom-<6hex>".
func GenerateNominationID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based suffix
		return fmt.Sprintf("nom-%06x", time.Now().UnixNano()&0xFFFFFF)
	}
	return "nom-" + hex.EncodeToString(b)
}

// titleToSlug converts a title to a kebab-case slug suitable for a filename.
func titleToSlug(title string) string {
	slug := strings.ToLower(title)
	var b strings.Builder
	prevDash := false
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
