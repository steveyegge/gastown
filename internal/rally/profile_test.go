package rally

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile_Absent(t *testing.T) {
	p, err := LoadProfile(t.TempDir())
	if err != nil {
		t.Fatalf("expected nil error for absent profile, got: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil profile for absent file")
	}
}

func TestLoadProfile_Parses(t *testing.T) {
	root := t.TempDir()
	yaml := `id: project-test
name: "Test Project"
facets:
  languages: [go]
  frameworks: [cobra]
architecture:
  style: cli
constraints:
  must_use: [go, sqlite]
needs:
  - "Fast CLI startup"
  - "Cross-platform builds"
tags: [cli, go, testing]
`
	if err := os.WriteFile(filepath.Join(root, "tavern-profile.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadProfile(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil profile")
	}
	if p.ID != "project-test" {
		t.Errorf("expected id=project-test, got %q", p.ID)
	}
	if len(p.Facets.Languages) == 0 || p.Facets.Languages[0] != "go" {
		t.Errorf("expected languages=[go], got %v", p.Facets.Languages)
	}
	if len(p.Needs) != 2 {
		t.Errorf("expected 2 needs, got %d", len(p.Needs))
	}
}

func TestTavernProfile_ToSearchQuery(t *testing.T) {
	p := &TavernProfile{
		Tags: []string{"security", "auth"},
		Needs: []string{"Rate limiting on webhook endpoints"},
	}
	p.Facets.Languages = []string{"python"}
	p.Facets.Frameworks = []string{"flask"}

	q := p.ToSearchQuery()

	if q.CodebaseType != "python-flask" {
		t.Errorf("expected codebase_type=python-flask, got %q", q.CodebaseType)
	}

	wantTags := map[string]bool{"security": true, "auth": true, "python": true, "flask": true}
	for _, tag := range q.Tags {
		if !wantTags[tag] {
			t.Errorf("unexpected tag %q", tag)
		}
		delete(wantTags, tag)
	}
	if len(wantTags) > 0 {
		t.Errorf("missing tags: %v", wantTags)
	}

	if q.Text != "Rate limiting on webhook endpoints" {
		t.Errorf("expected text from first need, got %q", q.Text)
	}
}

func TestTavernProfile_ToSearchQuery_NoLanguage(t *testing.T) {
	p := &TavernProfile{
		Tags:  []string{"devops"},
		Needs: []string{},
	}

	q := p.ToSearchQuery()

	if q.CodebaseType != "" {
		t.Errorf("expected empty codebase_type with no languages, got %q", q.CodebaseType)
	}
	if q.Text != "" {
		t.Errorf("expected empty text with no needs, got %q", q.Text)
	}
}
