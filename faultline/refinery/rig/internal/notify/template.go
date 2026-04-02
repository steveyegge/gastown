package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
)

// DefaultTemplates returns the built-in webhook payload templates.
func DefaultTemplates() []db.WebhookTemplate {
	return []db.WebhookTemplate{
		{
			ID:        "default-generic",
			Name:      "Generic JSON",
			EventType: "*",
			IsDefault: true,
			Body: `{
  "event_type": "{{event_type}}",
  "project_id": {{project_id}},
  "group_id": "{{group_id}}",
  "title": "{{title}}",
  "culprit": "{{culprit}}",
  "level": "{{level}}",
  "platform": "{{platform}}",
  "event_count": {{event_count}},
  "url": "{{url}}",
  "timestamp": "{{timestamp}}"
}`,
		},
		{
			ID:        "default-jira",
			Name:      "Jira Issue",
			EventType: "new_issue",
			IsDefault: true,
			Body: `{
  "fields": {
    "project": {"key": "{{jira_project_key}}"},
    "issuetype": {"name": "Bug"},
    "summary": "[{{level}}] {{title}}",
    "description": "Culprit: {{culprit}}\nPlatform: {{platform}}\nEvents: {{event_count}}\nGroup: {{group_id}}\n\nView in Faultline: {{url}}",
    "labels": ["faultline", "{{level}}", "{{platform}}"]
  }
}`,
		},
		{
			ID:        "default-linear",
			Name:      "Linear Issue",
			EventType: "new_issue",
			IsDefault: true,
			Body: `{
  "title": "[{{level}}] {{title}}",
  "description": "**Culprit:** {{culprit}}\n**Platform:** {{platform}}\n**Events:** {{event_count}}\n**Group:** {{group_id}}\n\n[View in Faultline]({{url}})",
  "priority": {{linear_priority}},
  "teamId": "{{linear_team_id}}",
  "labelIds": []
}`,
		},
	}
}

// TemplateVars builds the variable map from an Event for template substitution.
func TemplateVars(event Event, baseURL string) map[string]string {
	issueURL := ""
	if baseURL != "" {
		issueURL = fmt.Sprintf("%s/api/%d/issues/%s", baseURL, event.ProjectID, event.GroupID)
	}

	return map[string]string{
		"event_type":       string(event.Type),
		"project_id":       strconv.FormatInt(event.ProjectID, 10),
		"group_id":         event.GroupID,
		"title":            escapeJSON(event.Title),
		"culprit":          escapeJSON(event.Culprit),
		"level":            event.Level,
		"platform":         event.Platform,
		"event_count":      strconv.Itoa(event.EventCount),
		"bead_id":          event.BeadID,
		"prev_bead_id":     event.PrevBeadID,
		"url":              issueURL,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"jira_project_key": "PROJ",
		"linear_priority":  "2",
		"linear_team_id":   "",
	}
}

// RenderTemplate substitutes {{variable}} placeholders in a template body.
func RenderTemplate(body string, vars map[string]string) string {
	result := body
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// FindTemplate returns the best matching template for an event type from the
// given list. It prefers exact event_type matches over wildcard ("*") templates.
// Returns nil if no template matches.
func FindTemplate(templates []db.WebhookTemplate, eventType string) *db.WebhookTemplate {
	var wildcard *db.WebhookTemplate
	for i := range templates {
		if templates[i].EventType == eventType {
			return &templates[i]
		}
		if templates[i].EventType == "*" {
			wildcard = &templates[i]
		}
	}
	return wildcard
}

// escapeJSON escapes special characters for safe embedding in JSON string values.
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
