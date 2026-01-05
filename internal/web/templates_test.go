package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/activity"
)

func TestConvoyTemplate_RendersConvoyList(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	data := ConvoyData{
		Convoys: []ConvoyRow{
			{
				ID:       "hq-cv-abc",
				Title:    "Feature X",
				Status:   "open",
				Progress: "2/5",
				Completed: 2,
				Total:    5,
				LastActivity: activity.Calculate(time.Now().Add(-1 * time.Minute)),
			},
			{
				ID:       "hq-cv-def",
				Title:    "Bugfix Y",
				Status:   "open",
				Progress: "1/3",
				Completed: 1,
				Total:    3,
				LastActivity: activity.Calculate(time.Now().Add(-3 * time.Minute)),
			},
		},
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check convoy IDs are rendered
	if !strings.Contains(output, "hq-cv-abc") {
		t.Error("Template should contain convoy ID hq-cv-abc")
	}
	if !strings.Contains(output, "hq-cv-def") {
		t.Error("Template should contain convoy ID hq-cv-def")
	}

	// Check titles are rendered
	if !strings.Contains(output, "Feature X") {
		t.Error("Template should contain title 'Feature X'")
	}
	if !strings.Contains(output, "Bugfix Y") {
		t.Error("Template should contain title 'Bugfix Y'")
	}
}

func TestConvoyTemplate_LastActivityColors(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	tests := []struct {
		name       string
		age        time.Duration
		wantClass  string
	}{
		{"green for 1 minute", 1 * time.Minute, "activity-green"},
		{"yellow for 3 minutes", 3 * time.Minute, "activity-yellow"},
		{"red for 10 minutes", 10 * time.Minute, "activity-red"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := ConvoyData{
				Convoys: []ConvoyRow{
					{
						ID:           "hq-cv-test",
						Title:        "Test",
						Status:       "open",
						LastActivity: activity.Calculate(time.Now().Add(-tt.age)),
					},
				},
			}

			var buf bytes.Buffer
			err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
			if err != nil {
				t.Fatalf("ExecuteTemplate() error = %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.wantClass) {
				t.Errorf("Template should contain class %q for %v age", tt.wantClass, tt.age)
			}
		})
	}
}

func TestConvoyTemplate_HtmxAutoRefresh(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	data := ConvoyData{
		Convoys: []ConvoyRow{
			{
				ID:     "hq-cv-test",
				Title:  "Test",
				Status: "open",
			},
		},
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check for htmx attributes
	if !strings.Contains(output, "hx-get") {
		t.Error("Template should contain hx-get for auto-refresh")
	}
	if !strings.Contains(output, "hx-trigger") {
		t.Error("Template should contain hx-trigger for auto-refresh")
	}
	if !strings.Contains(output, "every 10s") {
		t.Error("Template should refresh every 10 seconds")
	}
}

func TestConvoyTemplate_ProgressDisplay(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	data := ConvoyData{
		Convoys: []ConvoyRow{
			{
				ID:        "hq-cv-test",
				Title:     "Test",
				Status:    "open",
				Progress:  "3/7",
				Completed: 3,
				Total:     7,
			},
		},
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check progress is displayed
	if !strings.Contains(output, "3/7") {
		t.Error("Template should display progress '3/7'")
	}
}

func TestConvoyTemplate_StatusIndicators(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	data := ConvoyData{
		Convoys: []ConvoyRow{
			{
				ID:     "hq-cv-open",
				Title:  "Open Convoy",
				Status: "open",
			},
			{
				ID:     "hq-cv-closed",
				Title:  "Closed Convoy",
				Status: "closed",
			},
		},
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check status indicators
	if !strings.Contains(output, "status-open") {
		t.Error("Template should contain status-open class")
	}
	if !strings.Contains(output, "status-closed") {
		t.Error("Template should contain status-closed class")
	}
}

func TestConvoyTemplate_EmptyState(t *testing.T) {
	tmpl, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() error = %v", err)
	}

	data := ConvoyData{
		Convoys: []ConvoyRow{},
	}

	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "convoy.html", data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check for empty state message
	if !strings.Contains(output, "No convoys") {
		t.Error("Template should show empty state message when no convoys")
	}
}
