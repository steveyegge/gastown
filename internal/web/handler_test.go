package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/activity"
)

// MockConvoyFetcher is a mock implementation for testing.
type MockConvoyFetcher struct {
	Convoys []ConvoyRow
	Error   error
}

func (m *MockConvoyFetcher) FetchConvoys() ([]ConvoyRow, error) {
	return m.Convoys, m.Error
}

func TestConvoyHandler_RendersTemplate(t *testing.T) {
	mock := &MockConvoyFetcher{
		Convoys: []ConvoyRow{
			{
				ID:           "hq-cv-abc",
				Title:        "Test Convoy",
				Status:       "open",
				Progress:     "2/5",
				Completed:    2,
				Total:        5,
				LastActivity: activity.Calculate(time.Now().Add(-1 * time.Minute)),
			},
		},
	}

	handler, err := NewConvoyHandler(mock)
	if err != nil {
		t.Fatalf("NewConvoyHandler() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	// Check convoy data is rendered
	if !strings.Contains(body, "hq-cv-abc") {
		t.Error("Response should contain convoy ID")
	}
	if !strings.Contains(body, "Test Convoy") {
		t.Error("Response should contain convoy title")
	}
	if !strings.Contains(body, "2/5") {
		t.Error("Response should contain progress")
	}
}

func TestConvoyHandler_LastActivityColors(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wantClass string
	}{
		{"green for active", 30 * time.Second, "activity-green"},
		{"yellow for stale", 3 * time.Minute, "activity-yellow"},
		{"red for stuck", 10 * time.Minute, "activity-red"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockConvoyFetcher{
				Convoys: []ConvoyRow{
					{
						ID:           "hq-cv-test",
						Title:        "Test",
						Status:       "open",
						LastActivity: activity.Calculate(time.Now().Add(-tt.age)),
					},
				},
			}

			handler, err := NewConvoyHandler(mock)
			if err != nil {
				t.Fatalf("NewConvoyHandler() error = %v", err)
			}

			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			body := w.Body.String()
			if !strings.Contains(body, tt.wantClass) {
				t.Errorf("Response should contain %q", tt.wantClass)
			}
		})
	}
}

func TestConvoyHandler_EmptyConvoys(t *testing.T) {
	mock := &MockConvoyFetcher{
		Convoys: []ConvoyRow{},
	}

	handler, err := NewConvoyHandler(mock)
	if err != nil {
		t.Fatalf("NewConvoyHandler() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "No convoys") {
		t.Error("Response should show empty state message")
	}
}

func TestConvoyHandler_ContentType(t *testing.T) {
	mock := &MockConvoyFetcher{
		Convoys: []ConvoyRow{},
	}

	handler, err := NewConvoyHandler(mock)
	if err != nil {
		t.Fatalf("NewConvoyHandler() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}
}

func TestConvoyHandler_MultipleConvoys(t *testing.T) {
	mock := &MockConvoyFetcher{
		Convoys: []ConvoyRow{
			{ID: "hq-cv-1", Title: "First Convoy", Status: "open"},
			{ID: "hq-cv-2", Title: "Second Convoy", Status: "closed"},
			{ID: "hq-cv-3", Title: "Third Convoy", Status: "open"},
		},
	}

	handler, err := NewConvoyHandler(mock)
	if err != nil {
		t.Fatalf("NewConvoyHandler() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Check all convoys are rendered
	for _, id := range []string{"hq-cv-1", "hq-cv-2", "hq-cv-3"} {
		if !strings.Contains(body, id) {
			t.Errorf("Response should contain convoy %s", id)
		}
	}
}
