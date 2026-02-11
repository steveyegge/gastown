package terminal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCoopBackendImplementsInterface verifies CoopBackend satisfies Backend.
func TestCoopBackendImplementsInterface(t *testing.T) {
	var _ Backend = (*CoopBackend)(nil)
}

func newTestCoop(t *testing.T, handler http.Handler) (*CoopBackend, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	b := NewCoopBackend(CoopConfig{})
	b.AddSession("test", srv.URL)
	return b, srv
}

func TestCoopBackend_HasSession_Running(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		pid := int32(1234)
		json.NewEncoder(w).Encode(coopHealthResponse{
			Status: "running",
			PID:    &pid,
			Ready:  true,
		})
	}))
	defer srv.Close()

	ok, err := b.HasSession("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected session to be running")
	}
}

func TestCoopBackend_HasSession_NotRegistered(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})

	ok, err := b.HasSession("missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for unregistered session")
	}
}

func TestCoopBackend_HasSession_Unreachable(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})
	b.AddSession("dead", "http://127.0.0.1:1") // port 1 — won't connect

	ok, err := b.HasSession("dead")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for unreachable host")
	}
}

func TestCoopBackend_HasSession_NoPID(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(coopHealthResponse{
			Status: "running",
			PID:    nil,
			Ready:  false,
		})
	}))
	defer srv.Close()

	ok, err := b.HasSession("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false when PID is nil")
	}
}

func TestCoopBackend_CapturePane(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/screen/text" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("line1\nline2\nline3\nline4\nline5"))
	}))
	defer srv.Close()

	text, err := b.CapturePane("test", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "line3\nline4\nline5" {
		t.Errorf("got %q, want last 3 lines", text)
	}
}

func TestCoopBackend_CapturePane_AllLines(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("a\nb\nc"))
	}))
	defer srv.Close()

	text, err := b.CapturePane("test", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "a\nb\nc" {
		t.Errorf("got %q, want full text", text)
	}
}

func TestCoopBackend_CapturePane_NotRegistered(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})

	_, err := b.CapturePane("missing", 10)
	if err == nil {
		t.Fatal("expected error for unregistered session")
	}
}

func TestCoopBackend_NudgeSession(t *testing.T) {
	var gotMessage string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/nudge" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req coopNudgeRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotMessage = req.Message
		json.NewEncoder(w).Encode(coopNudgeResponse{Delivered: true})
	}))
	defer srv.Close()

	err := b.NudgeSession("test", "hello agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMessage != "hello agent" {
		t.Errorf("got message %q, want %q", gotMessage, "hello agent")
	}
}

func TestCoopBackend_NudgeSession_NotDelivered(t *testing.T) {
	reason := "agent_busy"
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(coopNudgeResponse{
			Delivered: false,
			Reason:    &reason,
		})
	}))
	defer srv.Close()

	err := b.NudgeSession("test", "hello")
	if err == nil {
		t.Fatal("expected error when nudge not delivered")
	}
	if !strings.Contains(err.Error(), "agent_busy") {
		t.Errorf("error should contain reason, got: %v", err)
	}
}

func TestCoopBackend_SendKeys(t *testing.T) {
	var gotKeys []string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/input/keys" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req coopKeysRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotKeys = req.Keys
		json.NewEncoder(w).Encode(map[string]int{"bytes_written": 2})
	}))
	defer srv.Close()

	err := b.SendKeys("test", "Enter Escape")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotKeys) != 2 || gotKeys[0] != "Enter" || gotKeys[1] != "Escape" {
		t.Errorf("got keys %v, want [Enter Escape]", gotKeys)
	}
}

func TestCoopBackend_SendKeys_Empty(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})
	b.AddSession("test", "http://unused")

	err := b.SendKeys("test", "")
	if err != nil {
		t.Fatalf("unexpected error for empty keys: %v", err)
	}
}

func TestCoopBackend_AgentState(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/state" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CoopAgentState{
			Agent:         "claude-code",
			State:         "working",
			DetectionTier: "tier2_nats",
		})
	}))
	defer srv.Close()

	state, err := b.AgentState("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.State != "working" {
		t.Errorf("got state %q, want %q", state.State, "working")
	}
	if state.Agent != "claude-code" {
		t.Errorf("got agent %q, want %q", state.Agent, "claude-code")
	}
}

func TestCoopBackend_RespondToPrompt(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/respond" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req CoopRespondRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Accept == nil || !*req.Accept {
			t.Error("expected accept=true")
		}
		json.NewEncoder(w).Encode(CoopRespondResponse{Delivered: true})
	}))
	defer srv.Close()

	accept := true
	err := b.RespondToPrompt("test", CoopRespondRequest{Accept: &accept})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCoopBackend_AuthToken(t *testing.T) {
	var gotAuth string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(coopHealthResponse{Status: "running"})
	}))
	b.token = "secret-token"
	defer srv.Close()

	b.HasSession("test")
	if gotAuth != "Bearer secret-token" {
		t.Errorf("got auth %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestCoopBackend_SessionManagement(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})

	b.AddSession("a", "http://host-a:8080")
	b.AddSession("b", "http://host-b:8080")

	url, err := b.baseURL("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://host-a:8080" {
		t.Errorf("got %q, want %q", url, "http://host-a:8080")
	}

	b.RemoveSession("a")
	_, err = b.baseURL("a")
	if err == nil {
		t.Error("expected error after removing session")
	}

	url, err = b.baseURL("b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://host-b:8080" {
		t.Errorf("got %q, want %q", url, "http://host-b:8080")
	}
}

func TestCoopBackend_TrailingSlash(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})
	b.AddSession("s", "http://host:8080/")

	url, err := b.baseURL("s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "http://host:8080" {
		t.Errorf("got %q, want %q (trailing slash should be stripped)", url, "http://host:8080")
	}
}

// --- Phase 2: Coop-first method tests ---

func TestCoopBackend_KillSession(t *testing.T) {
	var gotSignal string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/signal" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req struct{ Signal string }
		json.NewDecoder(r.Body).Decode(&req)
		gotSignal = req.Signal
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := b.KillSession("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSignal != "SIGTERM" {
		t.Errorf("got signal %q, want SIGTERM", gotSignal)
	}
}

func TestCoopBackend_KillSession_NotRegistered(t *testing.T) {
	b := NewCoopBackend(CoopConfig{})
	err := b.KillSession("missing")
	if err == nil {
		t.Fatal("expected error for unregistered session")
	}
}

func TestCoopBackend_IsAgentRunning_True(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(coopStatusResponse{State: "running", PID: new(int32)})
	}))
	defer srv.Close()

	running, err := b.IsAgentRunning("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running {
		t.Error("expected running=true")
	}
}

func TestCoopBackend_IsAgentRunning_Exited(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopStatusResponse{State: "exited"})
	}))
	defer srv.Close()

	running, err := b.IsAgentRunning("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Error("expected running=false for exited process")
	}
}

func TestCoopBackend_GetAgentState(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/state" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CoopAgentState{
			State: "permission_prompt",
			Agent: "claude-code",
		})
	}))
	defer srv.Close()

	state, err := b.GetAgentState("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != "permission_prompt" {
		t.Errorf("got state %q, want %q", state, "permission_prompt")
	}
}

func TestCoopBackend_SetEnvironment(t *testing.T) {
	var gotKey, gotValue string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		// Path should be /api/v1/env/MY_VAR
		gotKey = strings.TrimPrefix(r.URL.Path, "/api/v1/env/")
		var req struct{ Value string }
		json.NewDecoder(r.Body).Decode(&req)
		gotValue = req.Value
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := b.SetEnvironment("test", "MY_VAR", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "MY_VAR" {
		t.Errorf("got key %q, want MY_VAR", gotKey)
	}
	if gotValue != "hello" {
		t.Errorf("got value %q, want hello", gotValue)
	}
}

func TestCoopBackend_GetEnvironment(t *testing.T) {
	val := "world"
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		key := strings.TrimPrefix(r.URL.Path, "/api/v1/env/")
		json.NewEncoder(w).Encode(coopEnvResponse{Key: key, Value: &val})
	}))
	defer srv.Close()

	result, err := b.GetEnvironment("test", "MY_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "world" {
		t.Errorf("got %q, want %q", result, "world")
	}
}

func TestCoopBackend_GetEnvironment_NotFound(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := b.GetEnvironment("test", "MISSING")
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestCoopBackend_GetEnvironment_NullValue(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopEnvResponse{Key: "X", Value: nil})
	}))
	defer srv.Close()

	_, err := b.GetEnvironment("test", "X")
	if err == nil {
		t.Fatal("expected error for null value")
	}
}

func TestCoopBackend_GetPaneWorkDir(t *testing.T) {
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/session/cwd" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(coopCwdResponse{Cwd: "/home/agent/workspace"})
	}))
	defer srv.Close()

	cwd, err := b.GetPaneWorkDir("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cwd != "/home/agent/workspace" {
		t.Errorf("got %q, want %q", cwd, "/home/agent/workspace")
	}
}

func TestCoopBackend_SendInput(t *testing.T) {
	var gotText string
	var gotEnter bool
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/input" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req coopInputRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotText = req.Text
		gotEnter = req.Enter
		json.NewEncoder(w).Encode(map[string]int{"bytes_written": len(req.Text)})
	}))
	defer srv.Close()

	err := b.SendInput("test", "hello world", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotText != "hello world" {
		t.Errorf("got text %q, want %q", gotText, "hello world")
	}
	if !gotEnter {
		t.Error("expected enter=true")
	}
}

func TestCoopBackend_SwitchSession(t *testing.T) {
	var gotEnv map[string]string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/session/switch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		var req coopSwitchRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotEnv = req.ExtraEnv
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := b.SwitchSession("test", SwitchConfig{
		ExtraEnv: map[string]string{"KEY": "val"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotEnv["KEY"] != "val" {
		t.Errorf("got env %v, want KEY=val", gotEnv)
	}
}

func TestCoopBackend_RespawnPane(t *testing.T) {
	var gotPath, gotMethod string
	b, srv := newTestCoop(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := b.RespawnPane("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/session/switch" {
		t.Errorf("expected /api/v1/session/switch, got %s", gotPath)
	}
	if gotMethod != "PUT" {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
}
