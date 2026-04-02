package notify

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/outdoorsea/faultline/internal/db"
)

// mockProvider implements ProjectConfigProvider for testing.
type mockProvider struct {
	configs map[int64][2]string // projectID -> [url, type]
}

func (m *mockProvider) WebhookConfig(_ context.Context, projectID int64) (string, string) {
	if c, ok := m.configs[projectID]; ok {
		return c[0], c[1]
	}
	return "", ""
}

func (m *mockProvider) WebhookTemplates(_ context.Context, _ int64) []db.WebhookTemplate {
	return nil
}

// recorder captures notifications for testing.
type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) Notify(_ context.Context, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func TestDispatcherProjectWebhook(t *testing.T) {
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fallback := &recorder{}
	provider := &mockProvider{
		configs: map[int64][2]string{
			1: {srv.URL, "generic"},
		},
	}

	d := NewDispatcher(fallback, provider, "", slog.Default())
	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 1,
		Title:     "Test",
	})

	if !received {
		t.Error("project webhook should have received the notification")
	}
	if fallback.count() != 0 {
		t.Error("fallback should NOT have received the notification when project webhook is configured")
	}
}

func TestDispatcherFallback(t *testing.T) {
	fallback := &recorder{}
	provider := &mockProvider{configs: map[int64][2]string{}}

	d := NewDispatcher(fallback, provider, "", slog.Default())
	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 99,
		Title:     "Test",
	})

	if fallback.count() != 1 {
		t.Errorf("fallback should have received 1 notification, got %d", fallback.count())
	}
}

func TestDispatcherNoWebhook(t *testing.T) {
	provider := &mockProvider{configs: map[int64][2]string{}}
	d := NewDispatcher(nil, provider, "", slog.Default())

	// Should not panic when neither project nor fallback webhook is configured.
	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 1,
		Title:     "Test",
	})
}

func TestDispatcherSlackType(t *testing.T) {
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	provider := &mockProvider{
		configs: map[int64][2]string{
			1: {srv.URL, "slack"},
		},
	}

	d := NewDispatcher(nil, provider, "http://faultline.local", slog.Default())
	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 1,
		Title:     "Test",
		Level:     "error",
	})

	if !received {
		t.Error("slack webhook should have received the notification")
	}
}

func TestDispatcherDiscordType(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	provider := &mockProvider{
		configs: map[int64][2]string{
			1: {srv.URL + "/webhook/123", "discord"},
		},
	}

	d := NewDispatcher(nil, provider, "", slog.Default())
	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 1,
		Title:     "Test",
		Level:     "error",
	})

	if receivedPath != "/webhook/123/slack" {
		t.Errorf("discord webhook should append /slack, got path %s", receivedPath)
	}
}

func TestDispatcherNilProvider(t *testing.T) {
	fallback := &recorder{}
	d := NewDispatcher(fallback, nil, "", slog.Default())

	d.Notify(context.Background(), Event{
		Type:      EventNewIssue,
		ProjectID: 1,
		Title:     "Test",
	})

	if fallback.count() != 1 {
		t.Error("should fall back when provider is nil")
	}
}
