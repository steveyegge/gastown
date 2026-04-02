package notify

import (
	"strings"
	"testing"

	"github.com/outdoorsea/faultline/internal/db"
)

func TestRenderTemplate(t *testing.T) {
	vars := map[string]string{
		"event_type":  "new_issue",
		"title":       "TypeError: undefined",
		"level":       "error",
		"culprit":     "app.main",
		"event_count": "5",
		"project_id":  "1",
		"group_id":    "abc123",
		"platform":    "javascript",
		"url":         "http://localhost:8080/api/1/issues/abc123",
		"timestamp":   "2026-01-01T00:00:00Z",
	}

	body := `{"event": "{{event_type}}", "title": "{{title}}", "level": "{{level}}", "events": {{event_count}}}`
	result := RenderTemplate(body, vars)

	if !strings.Contains(result, `"event": "new_issue"`) {
		t.Errorf("expected event_type substitution, got: %s", result)
	}
	if !strings.Contains(result, `"title": "TypeError: undefined"`) {
		t.Errorf("expected title substitution, got: %s", result)
	}
	if !strings.Contains(result, `"events": 5`) {
		t.Errorf("expected event_count substitution, got: %s", result)
	}
}

func TestRenderTemplateUnknownVars(t *testing.T) {
	vars := map[string]string{"title": "test"}
	body := `{"title": "{{title}}", "unknown": "{{unknown_var}}"}`
	result := RenderTemplate(body, vars)

	if !strings.Contains(result, `"unknown": "{{unknown_var}}"`) {
		t.Errorf("unknown variables should be left as-is, got: %s", result)
	}
}

func TestFindTemplate(t *testing.T) {
	templates := []db.WebhookTemplate{
		{ID: "1", Name: "All", EventType: "*", Body: "all-body"},
		{ID: "2", Name: "New Issue", EventType: "new_issue", Body: "new-body"},
		{ID: "3", Name: "Resolved", EventType: "resolved", Body: "resolved-body"},
	}

	// Exact match preferred over wildcard.
	tmpl := FindTemplate(templates, "new_issue")
	if tmpl == nil || tmpl.ID != "2" {
		t.Error("expected exact match for new_issue")
	}

	tmpl = FindTemplate(templates, "resolved")
	if tmpl == nil || tmpl.ID != "3" {
		t.Error("expected exact match for resolved")
	}

	// Falls back to wildcard.
	tmpl = FindTemplate(templates, "regression")
	if tmpl == nil || tmpl.ID != "1" {
		t.Error("expected wildcard match for regression")
	}

	// No match when no templates.
	tmpl = FindTemplate(nil, "new_issue")
	if tmpl != nil {
		t.Error("expected nil for empty template list")
	}

	// No match when only specific types exist.
	specific := []db.WebhookTemplate{
		{ID: "2", Name: "New Issue", EventType: "new_issue", Body: "new-body"},
	}
	tmpl = FindTemplate(specific, "regression")
	if tmpl != nil {
		t.Error("expected nil when no wildcard and no exact match")
	}
}

func TestTemplateVars(t *testing.T) {
	event := Event{
		Type:       EventNewIssue,
		ProjectID:  42,
		GroupID:    "grp-123",
		Title:      "Error: test",
		Culprit:    "main.go",
		Level:      "error",
		Platform:   "go",
		EventCount: 10,
		BeadID:     "fl-abc",
	}

	vars := TemplateVars(event, "http://faultline.local")

	if vars["project_id"] != "42" {
		t.Errorf("expected project_id=42, got %s", vars["project_id"])
	}
	if vars["event_count"] != "10" {
		t.Errorf("expected event_count=10, got %s", vars["event_count"])
	}
	if vars["url"] != "http://faultline.local/api/42/issues/grp-123" {
		t.Errorf("expected url with base, got %s", vars["url"])
	}
	if vars["bead_id"] != "fl-abc" {
		t.Errorf("expected bead_id=fl-abc, got %s", vars["bead_id"])
	}
}

func TestTemplateVarsNoBaseURL(t *testing.T) {
	event := Event{Type: EventNewIssue, ProjectID: 1, GroupID: "g1"}
	vars := TemplateVars(event, "")
	if vars["url"] != "" {
		t.Errorf("expected empty url without base URL, got %s", vars["url"])
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{`hello`, `hello`},
		{`say "hi"`, `say \"hi\"`},
		{"line1\nline2", `line1\nline2`},
		{`back\slash`, `back\\slash`},
	}

	for _, tt := range tests {
		got := escapeJSON(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDefaultTemplates(t *testing.T) {
	defaults := DefaultTemplates()
	if len(defaults) != 3 {
		t.Fatalf("expected 3 default templates, got %d", len(defaults))
	}

	ids := map[string]bool{}
	for _, d := range defaults {
		if d.ID == "" || d.Name == "" || d.Body == "" {
			t.Errorf("default template missing required fields: %+v", d)
		}
		if !d.IsDefault {
			t.Errorf("default template should have IsDefault=true: %s", d.ID)
		}
		ids[d.ID] = true
	}

	for _, expected := range []string{"default-generic", "default-jira", "default-linear"} {
		if !ids[expected] {
			t.Errorf("missing default template: %s", expected)
		}
	}
}
