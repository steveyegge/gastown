package terminal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCoopStateWatcher_ReceivesStateChange(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		if r.URL.Query().Get("subscribe") != "state" {
			t.Errorf("expected subscribe=state, got %s", r.URL.Query().Get("subscribe"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		evt := StateChangeEvent{
			Event: "transition",
			Prev:  "working",
			Next:  "idle",
			Seq:   42,
		}
		data, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, data)

		// Keep connection open briefly so client can read.
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:    srv.URL,
		BufferSize: 8,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	select {
	case evt := <-w.StateCh():
		if evt.Prev != "working" {
			t.Errorf("prev = %q, want %q", evt.Prev, "working")
		}
		if evt.Next != "idle" {
			t.Errorf("next = %q, want %q", evt.Next, "idle")
		}
		if evt.Seq != 42 {
			t.Errorf("seq = %d, want 42", evt.Seq)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for state change event")
	}
}

func TestCoopStateWatcher_ReceivesExitEvent(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		code := 0
		evt := ExitEvent{Event: "exit", Code: &code}
		data, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:    srv.URL,
		BufferSize: 8,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	select {
	case evt := <-w.ExitCh():
		if evt.Code == nil || *evt.Code != 0 {
			t.Errorf("exit code = %v, want 0", evt.Code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for exit event")
	}
}

func TestCoopStateWatcher_StateChangeWithPrompt(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		evt := StateChangeEvent{
			Event: "transition",
			Prev:  "working",
			Next:  "prompt",
			Seq:   100,
			Prompt: &PromptContext{
				Type:    "permission",
				Message: "Bash: npm install",
			},
		}
		data, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	select {
	case evt := <-w.StateCh():
		if evt.Next != "prompt" {
			t.Errorf("next = %q, want %q", evt.Next, "prompt")
		}
		if evt.Prompt == nil {
			t.Fatal("expected prompt context")
		}
		if evt.Prompt.Type != "permission" {
			t.Errorf("prompt type = %q, want %q", evt.Prompt.Type, "permission")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}

func TestCoopStateWatcher_Close(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Hold connection open.
		time.Sleep(10 * time.Second)
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:        srv.URL,
		ReconnectDelay: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}

	// Give it time to connect.
	time.Sleep(100 * time.Millisecond)

	// Close should not hang.
	done := make(chan struct{})
	go func() {
		w.Close()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not return in time")
	}
}

func TestCoopStateWatcher_Reconnects(t *testing.T) {
	var connectCount int
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectCount++
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Close immediately to trigger reconnect.
		conn.Close()
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:        srv.URL,
		ReconnectDelay: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}

	// Wait for a few reconnection attempts.
	time.Sleep(300 * time.Millisecond)
	w.Close()

	if connectCount < 2 {
		t.Errorf("expected at least 2 connections (reconnect), got %d", connectCount)
	}
}

func TestCoopStateWatcher_IgnoresUnknownTypes(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send unknown type, then transition.
		conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"pong"}`))
		conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"screen","lines":["test"]}`))

		evt := StateChangeEvent{Event: "transition", Prev: "starting", Next: "working", Seq: 1}
		data, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	select {
	case evt := <-w.StateCh():
		if evt.Next != "working" {
			t.Errorf("next = %q, want %q", evt.Next, "working")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}

func TestCoopStateWatcher_AuthToken(t *testing.T) {
	var gotToken string
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:        srv.URL,
		Token:          "my-secret",
		ReconnectDelay: 5 * time.Second, // Don't reconnect quickly
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	w.Close()

	if gotToken != "my-secret" {
		t.Errorf("token = %q, want %q", gotToken, "my-secret")
	}
}

func TestCoopStateWatcher_WsURL(t *testing.T) {
	tests := []struct {
		baseURL string
		token   string
		want    string
	}{
		{"http://localhost:8080", "", "ws://localhost:8080/ws?subscribe=state"},
		{"https://coop.example.com", "", "wss://coop.example.com/ws?subscribe=state"},
		{"http://localhost:8080", "tok", "ws://localhost:8080/ws?subscribe=state&token=tok"},
		{"http://localhost:8080/", "", "ws://localhost:8080/ws?subscribe=state"},
	}

	for _, tt := range tests {
		w := &CoopStateWatcher{baseURL: strings.TrimRight(tt.baseURL, "/"), token: tt.token}
		got, err := w.wsURL()
		if err != nil {
			t.Errorf("wsURL(%q): %v", tt.baseURL, err)
			continue
		}
		if got != tt.want {
			t.Errorf("wsURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestCoopBackend_WatchState(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		evt := StateChangeEvent{Event: "transition", Prev: "working", Next: "idle", Seq: 5}
		data, _ := json.Marshal(evt)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	b := NewCoopBackend(CoopConfig{Token: "test-token"})
	b.AddSession("agent1", srv.URL)

	w, err := b.WatchState("agent1", CoopStateWatcherConfig{})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}
	defer w.Close()

	select {
	case evt := <-w.StateCh():
		if evt.Next != "idle" {
			t.Errorf("next = %q, want %q", evt.Next, "idle")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}
