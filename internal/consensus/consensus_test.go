package consensus

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockTmux implements TmuxClient for testing.
type mockTmux struct {
	mu           sync.Mutex
	idleSessions map[string]bool           // session -> is idle
	paneContent  map[string]string         // session -> pane content (pre-send)
	sendErr      map[string]error          // session -> SendKeys error
	waitErr      map[string]error          // session -> WaitForIdle error
	captureErr   map[string]error          // session -> CapturePaneAll error
	postContent  map[string]string         // session -> content after processing
	sent         map[string]bool           // session -> SendKeys was called
	wakeCalled   map[string]bool           // session -> WakePane was called
}

func newMockTmux() *mockTmux {
	return &mockTmux{
		idleSessions: make(map[string]bool),
		paneContent:  make(map[string]string),
		sendErr:      make(map[string]error),
		waitErr:      make(map[string]error),
		captureErr:   make(map[string]error),
		postContent:  make(map[string]string),
		sent:         make(map[string]bool),
		wakeCalled:   make(map[string]bool),
	}
}

func (m *mockTmux) IsIdle(session string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idleSessions[session]
}

func (m *mockTmux) SendKeys(session, keys string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.sendErr[session]; ok {
		return err
	}
	m.sent[session] = true
	return nil
}

func (m *mockTmux) WaitForIdle(session string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.waitErr[session]; ok {
		return err
	}
	return nil
}

func (m *mockTmux) CapturePaneAll(session string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.captureErr[session]; ok {
		return "", err
	}
	// After send + wake, return post content to simulate completed response.
	if m.sent[session] && m.wakeCalled[session] {
		if post, ok := m.postContent[session]; ok {
			return post, nil
		}
	}
	content, ok := m.paneContent[session]
	if !ok {
		return "", fmt.Errorf("session %s not found", session)
	}
	return content, nil
}

func (m *mockTmux) WakePane(target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wakeCalled[target] = true
}

func TestRun_AllIdle(t *testing.T) {
	mock := newMockTmux()
	mock.idleSessions["sess-a"] = true
	mock.idleSessions["sess-b"] = true
	mock.paneContent["sess-a"] = "old output a\n❯ "
	mock.paneContent["sess-b"] = "old output b\n❯ "
	mock.postContent["sess-a"] = "old output a\n❯ \nResponse from A\n❯ \n⏵⏵ status"
	mock.postContent["sess-b"] = "old output b\n❯ \nResponse from B\n❯ \n⏵⏵ status"

	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "What time is it?",
		Sessions: []string{"sess-a", "sess-b"},
		Timeout:  5 * time.Minute,
	})

	if result.Prompt != "What time is it?" {
		t.Errorf("expected prompt preserved, got %q", result.Prompt)
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("expected 2 session results, got %d", len(result.Sessions))
	}

	for _, sr := range result.Sessions {
		if sr.Status != StatusOK {
			t.Errorf("session %s: expected status ok, got %s (error: %s)", sr.Session, sr.Status, sr.Error)
		}
		if sr.Response == "" {
			t.Errorf("session %s: expected non-empty response", sr.Session)
		}
	}

	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestRun_SomeNotIdle(t *testing.T) {
	mock := newMockTmux()
	mock.idleSessions["sess-a"] = true
	mock.idleSessions["sess-b"] = false // busy
	mock.paneContent["sess-a"] = "old a\n❯ "
	mock.postContent["sess-a"] = "old a\n❯ \nResponse A\n❯ "

	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "test",
		Sessions: []string{"sess-a", "sess-b"},
		Timeout:  1 * time.Minute,
	})

	if len(result.Sessions) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Sessions))
	}

	statusMap := make(map[string]ResultStatus)
	for _, sr := range result.Sessions {
		statusMap[sr.Session] = sr.Status
	}

	if statusMap["sess-a"] != StatusOK {
		t.Errorf("sess-a: expected ok, got %s", statusMap["sess-a"])
	}
	if statusMap["sess-b"] != StatusNotIdle {
		t.Errorf("sess-b: expected not_idle, got %s", statusMap["sess-b"])
	}
}

func TestRun_Timeout(t *testing.T) {
	mock := newMockTmux()
	mock.idleSessions["sess-a"] = true
	mock.paneContent["sess-a"] = "old\n❯ "
	mock.waitErr["sess-a"] = fmt.Errorf("idle timeout exceeded")

	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "test",
		Sessions: []string{"sess-a"},
		Timeout:  1 * time.Second,
	})

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Sessions))
	}
	if result.Sessions[0].Status != StatusTimeout {
		t.Errorf("expected timeout status, got %s", result.Sessions[0].Status)
	}
}

func TestRun_SendFails(t *testing.T) {
	mock := newMockTmux()
	mock.idleSessions["sess-a"] = true
	mock.paneContent["sess-a"] = "old\n❯ "
	mock.sendErr["sess-a"] = fmt.Errorf("send failed: session dead")

	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "test",
		Sessions: []string{"sess-a"},
		Timeout:  1 * time.Minute,
	})

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Sessions))
	}
	if result.Sessions[0].Status != StatusError {
		t.Errorf("expected error status, got %s", result.Sessions[0].Status)
	}
	if result.Sessions[0].Error == "" {
		t.Error("expected error message")
	}
}

func TestRun_NoSessions(t *testing.T) {
	mock := newMockTmux()
	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "test",
		Sessions: nil,
		Timeout:  1 * time.Minute,
	})

	if len(result.Sessions) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Sessions))
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestExtractNewContent(t *testing.T) {
	tests := []struct {
		name string
		pre  string
		post string
		want string
	}{
		{
			name: "simple diff",
			pre:  "old output\n❯ ",
			post: "old output\n❯ \nNew response line 1\nNew response line 2\n❯ ",
			want: "New response line 1\nNew response line 2\n❯",
		},
		{
			name: "empty pre",
			pre:  "",
			post: "Some output here",
			want: "Some output here",
		},
		{
			name: "no new content",
			pre:  "same content\n❯ ",
			post: "same content\n❯ ",
			want: "",
		},
		{
			name: "multi-line response",
			pre:  "previous output\n❯ ",
			post: "previous output\n❯ \nLine 1\nLine 2\nLine 3\n❯ \n⏵⏵ status bar",
			want: "Line 1\nLine 2\nLine 3\n❯ \n⏵⏵ status bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNewContent(tt.pre, tt.post)
			if got != tt.want {
				t.Errorf("extractNewContent():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestStripTrailingPrompt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trailing prompt",
			in:   "Response text\n❯ ",
			want: "Response text",
		},
		{
			name: "trailing status bar",
			in:   "Response text\n❯ \n⏵⏵ bypass permissions",
			want: "Response text",
		},
		{
			name: "no prompt",
			in:   "Clean response",
			want: "Clean response",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "only prompt",
			in:   "❯ \n⏵⏵ status",
			want: "",
		},
		{
			name: "response with embedded prompt char",
			in:   "The ❯ char appears here\nActual response\n❯ ",
			want: "The ❯ char appears here\nActual response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTrailingPrompt(tt.in)
			if got != tt.want {
				t.Errorf("stripTrailingPrompt():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}
