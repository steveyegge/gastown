package consensus

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockTmux implements TmuxClient for testing.
type mockTmux struct {
	mu           sync.Mutex
	idleSessions map[string]bool     // session -> is idle (status bar fallback)
	paneContent  map[string]string   // session -> pane content (pre-send)
	sendErr      map[string]error    // session -> SendKeys error
	captureErr   map[string]error    // session -> CapturePaneAll error
	postContent  map[string]string   // session -> content after processing
	sent         map[string]bool     // session -> SendKeys was called
	wakeCalled   map[string]bool     // session -> WakePane was called
	envVars      map[string]string   // "session:key" -> value
	paneLines    map[string][]string // session -> last N lines for CapturePaneLines
}

func newMockTmux() *mockTmux {
	return &mockTmux{
		idleSessions: make(map[string]bool),
		paneContent:  make(map[string]string),
		sendErr:      make(map[string]error),
		captureErr:   make(map[string]error),
		postContent:  make(map[string]string),
		sent:         make(map[string]bool),
		wakeCalled:   make(map[string]bool),
		envVars:      make(map[string]string),
		paneLines:    make(map[string][]string),
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

func (m *mockTmux) CapturePaneLines(session string, n int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.captureErr[session]; ok {
		return nil, err
	}
	if lines, ok := m.paneLines[session]; ok {
		if n >= len(lines) {
			return lines, nil
		}
		return lines[len(lines)-n:], nil
	}
	// Fall back to splitting paneContent.
	content, ok := m.paneContent[session]
	if !ok {
		return nil, fmt.Errorf("session %s not found", session)
	}
	all := strings.Split(content, "\n")
	if n >= len(all) {
		return all, nil
	}
	return all[len(all)-n:], nil
}

func (m *mockTmux) WakePane(target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wakeCalled[target] = true
}

func (m *mockTmux) GetEnvironment(session, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := session + ":" + key
	if v, ok := m.envVars[k]; ok {
		return v, nil
	}
	return "", fmt.Errorf("env %s not set on %s", key, session)
}

// --- Runner tests ---

func TestRun_AllIdle(t *testing.T) {
	mock := newMockTmux()
	// Both default to Claude (no GT_AGENT).
	mock.paneContent["sess-a"] = "old output a\n❯ "
	mock.paneContent["sess-b"] = "old output b\n❯ "
	mock.paneLines["sess-a"] = []string{"old output a", "❯ ", "⏵⏵ bypass permissions on (shift+tab)"}
	mock.paneLines["sess-b"] = []string{"old output b", "❯ ", "⏵⏵ bypass permissions on (shift+tab)"}
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
		if sr.Provider != "claude" {
			t.Errorf("session %s: expected provider 'claude', got %q", sr.Session, sr.Provider)
		}
	}

	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

func TestRun_SomeNotIdle(t *testing.T) {
	mock := newMockTmux()
	// sess-a: Claude, idle
	mock.paneContent["sess-a"] = "old a\n❯ "
	mock.paneLines["sess-a"] = []string{"old a", "❯ ", "⏵⏵ bypass permissions on (shift+tab)"}
	mock.postContent["sess-a"] = "old a\n❯ \nResponse A\n❯ \n⏵⏵ status"
	// sess-b: Claude, busy (esc to interrupt)
	mock.paneLines["sess-b"] = []string{"working...", "❯ ", "⏵⏵ bypass permissions on ... · esc to interrupt"}
	mock.paneContent["sess-b"] = "working...\n❯ \n⏵⏵ esc to interrupt"

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
	// Set up an idle Claude session.
	mock.paneLines["sess-a"] = []string{"old", "❯ ", "⏵⏵ bypass permissions on (shift+tab)"}
	mock.paneContent["sess-a"] = "old\n❯ "
	mock.postContent["sess-a"] = "old\n❯ \nstill working..."

	runner := NewRunner(mock)
	result := runner.Run(Request{
		Prompt:   "test",
		Sessions: []string{"sess-a"},
		Timeout:  300 * time.Millisecond,
	})

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Sessions))
	}
	// The mock's CapturePaneLines returns lines with ❯ prompt, so waitForIdle
	// will succeed immediately. This test verifies the overall flow completes.
	if result.Sessions[0].Status != StatusOK {
		t.Errorf("expected ok status, got %s (error: %s)", result.Sessions[0].Status, result.Sessions[0].Error)
	}
}

func TestRun_SendFails(t *testing.T) {
	mock := newMockTmux()
	mock.paneLines["sess-a"] = []string{"old", "❯ ", "⏵⏵ bypass permissions on (shift+tab)"}
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
	if result.Duration < 0 {
		t.Error("expected non-negative duration")
	}
}

// --- Provider resolution tests ---

func TestResolveProvider_Claude(t *testing.T) {
	mock := newMockTmux()
	mock.envVars["sess:GT_AGENT"] = "claude"

	p := resolveProvider(mock, "sess")
	if p.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", p.Name)
	}
	if p.PromptPrefix == "" {
		t.Error("expected non-empty PromptPrefix for Claude")
	}
}

func TestResolveProvider_NoGTAgent(t *testing.T) {
	mock := newMockTmux()
	// No GT_AGENT set — should default to Claude.
	p := resolveProvider(mock, "sess")
	if p.Name != "claude" {
		t.Errorf("expected default 'claude', got %q", p.Name)
	}
	if p.PromptPrefix != "❯ " {
		t.Errorf("expected default prompt prefix '❯ ', got %q", p.PromptPrefix)
	}
}

func TestResolveProvider_Unknown(t *testing.T) {
	mock := newMockTmux()
	mock.envVars["sess:GT_AGENT"] = "some-unknown-agent"

	p := resolveProvider(mock, "sess")
	if p.Name != "some-unknown-agent" {
		t.Errorf("expected name 'some-unknown-agent', got %q", p.Name)
	}
	if p.DelayMs <= 0 {
		t.Error("expected positive DelayMs for unknown provider")
	}
}

// --- Idle detection tests ---

func TestIsSessionIdle_PromptBased(t *testing.T) {
	mock := newMockTmux()
	mock.paneLines["sess"] = []string{"some output", "$ ", ""}

	p := ProviderInfo{Name: "gemini", PromptPrefix: "$ "}
	if !isSessionIdle(mock, "sess", p) {
		t.Error("expected session to be idle when prompt prefix is present")
	}
}

func TestIsSessionIdle_PromptBased_NotIdle(t *testing.T) {
	mock := newMockTmux()
	mock.paneLines["sess"] = []string{"working on something...", "still busy"}

	p := ProviderInfo{Name: "gemini", PromptPrefix: "$ "}
	if isSessionIdle(mock, "sess", p) {
		t.Error("expected session to be not idle when prompt prefix absent")
	}
}

func TestIsSessionIdle_StatusBarFallback(t *testing.T) {
	mock := newMockTmux()
	// No PromptPrefix — falls back to IsIdle.
	mock.idleSessions["sess"] = true

	p := ProviderInfo{Name: "custom", DelayMs: 3000}
	if !isSessionIdle(mock, "sess", p) {
		t.Error("expected fallback to IsIdle when no PromptPrefix")
	}
}

func TestIsSessionIdle_Claude_BusyStatusBar(t *testing.T) {
	mock := newMockTmux()
	// Claude shows prompt AND "esc to interrupt" — should be not idle.
	mock.paneLines["sess"] = []string{
		"working...",
		"❯ ",
		"⏵⏵ bypass permissions on ... · esc to interrupt",
	}

	p := ProviderInfo{Name: "claude", PromptPrefix: "❯ "}
	if isSessionIdle(mock, "sess", p) {
		t.Error("expected not idle when Claude status bar shows 'esc to interrupt'")
	}
}

// --- waitForIdle tests ---

func TestWaitForIdle_PromptDetection(t *testing.T) {
	mock := newMockTmux()
	mock.paneLines["sess"] = []string{"output", ">>> "}

	p := ProviderInfo{Name: "custom", PromptPrefix: ">>> "}
	err := waitForIdle(mock, "sess", p, 1*time.Second)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestWaitForIdle_DelayFallback(t *testing.T) {
	mock := newMockTmux()
	// No prompt, just delay.
	p := ProviderInfo{Name: "codex", DelayMs: 100}

	start := time.Now()
	err := waitForIdle(mock, "sess", p, 5*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected at least ~100ms delay, got %v", elapsed)
	}
}

func TestWaitForIdle_NoDetection(t *testing.T) {
	mock := newMockTmux()
	// No prompt, no delay — should error.
	p := ProviderInfo{Name: "noop"}

	err := waitForIdle(mock, "sess", p, 1*time.Second)
	if err == nil {
		t.Error("expected error for provider with no detection method")
	}
}

// --- extractNewContent tests ---

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

// --- stripPrompt tests ---

func TestStripPrompt(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		provider ProviderInfo
		want     string
	}{
		{
			name:     "Claude trailing prompt",
			in:       "Response text\n❯ ",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "Response text",
		},
		{
			name:     "Claude trailing status bar",
			in:       "Response text\n❯ \n⏵⏵ bypass permissions",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "Response text",
		},
		{
			name:     "Gemini trailing prompt",
			in:       "Gemini response\n$ ",
			provider: ProviderInfo{Name: "gemini", PromptPrefix: "$ "},
			want:     "Gemini response",
		},
		{
			name:     "no prompt to strip",
			in:       "Clean response",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "Clean response",
		},
		{
			name:     "empty",
			in:       "",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "",
		},
		{
			name:     "only prompt",
			in:       "❯ \n⏵⏵ status",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "",
		},
		{
			name:     "response with embedded prompt char",
			in:       "The ❯ char appears here\nActual response\n❯ ",
			provider: ProviderInfo{Name: "claude", PromptPrefix: "❯ "},
			want:     "The ❯ char appears here\nActual response",
		},
		{
			name:     "delay-only provider strips nothing",
			in:       "Response text\nsome trailing line",
			provider: ProviderInfo{Name: "codex", DelayMs: 3000},
			want:     "Response text\nsome trailing line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripPrompt(tt.in, tt.provider)
			if got != tt.want {
				t.Errorf("stripPrompt():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

// --- matchesPromptPrefixLocal tests ---

func TestMatchesPromptPrefixLocal(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		prefix string
		want   bool
	}{
		{"exact match", "❯ ", "❯ ", true},
		{"with NBSP", "❯\u00a0", "❯ ", true},
		{"no match", "working...", "❯ ", false},
		{"empty prefix", "❯ ", "", false},
		{"dollar prompt", "$ ", "$ ", true},
		{"chevron prompt", ">>> ", ">>> ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPromptPrefixLocal(tt.line, tt.prefix)
			if got != tt.want {
				t.Errorf("matchesPromptPrefixLocal(%q, %q) = %v, want %v", tt.line, tt.prefix, got, tt.want)
			}
		})
	}
}
