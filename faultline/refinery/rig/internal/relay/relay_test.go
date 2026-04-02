package relay_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/outdoorsea/faultline/internal/relay"
)

func newTestStore(t *testing.T) *relay.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := relay.NewStore(dbPath, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStoreInsertAndPoll(t *testing.T) {
	s := newTestStore(t)

	// Insert two envelopes.
	id1, err := s.Insert(1, "key1", []byte("payload-one"))
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id2, err := s.Insert(2, "key2", []byte("payload-two"))
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id1 >= id2 {
		t.Fatalf("IDs should be monotonically increasing: %d >= %d", id1, id2)
	}

	// Poll all.
	envs, err := s.Poll(0, 100)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envelopes, got %d", len(envs))
	}
	if string(envs[0].Payload) != "payload-one" {
		t.Errorf("expected payload-one, got %s", envs[0].Payload)
	}

	// Poll with since.
	envs, err = s.Poll(id1, 100)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 envelope after since=%d, got %d", id1, len(envs))
	}
}

func TestStoreAck(t *testing.T) {
	s := newTestStore(t)

	id1, _ := s.Insert(1, "key1", []byte("p1"))
	_, _ = s.Insert(1, "key1", []byte("p2"))

	// Ack the first.
	n, err := s.Ack([]int64{id1})
	if err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 acked, got %d", n)
	}

	// Poll should return only the unacked one.
	envs, err := s.Poll(0, 100)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 unacked envelope, got %d", len(envs))
	}
}

func TestStoreStats(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.Insert(1, "k", []byte("a"))
	_, _ = s.Insert(1, "k", []byte("b"))
	id3, _ := s.Insert(1, "k", []byte("c"))

	_, _ = s.Ack([]int64{id3})

	total, unpulled, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
	if unpulled != 2 {
		t.Errorf("expected unpulled=2, got %d", unpulled)
	}
}

func newTestHandler(t *testing.T) (*relay.Handler, *relay.Store) {
	t.Helper()
	s := newTestStore(t)
	auth, err := relay.NewAuth([]string{"1:testkey", "2:otherkey"})
	if err != nil {
		t.Fatalf("NewAuth: %v", err)
	}
	h := &relay.Handler{
		Store: s,
		Auth:  auth,
		Log:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
	return h, s
}

func TestHandlerIngest(t *testing.T) {
	h, s := newTestHandler(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// POST with sentry key in query param.
	body := []byte(`{"event_id":"abc123"}`)
	req := httptest.NewRequest("POST", "/api/1/envelope/?sentry_key=testkey", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify stored.
	envs, _ := s.Poll(0, 100)
	if len(envs) != 1 {
		t.Fatalf("expected 1 stored envelope, got %d", len(envs))
	}
	if string(envs[0].Payload) != string(body) {
		t.Errorf("payload mismatch")
	}
}

func TestHandlerIngestNoAuth(t *testing.T) {
	h, _ := newTestHandler(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/1/envelope/", bytes.NewReader([]byte("data")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlerPollAndAck(t *testing.T) {
	h, s := newTestHandler(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Insert directly.
	_, _ = s.Insert(1, "testkey", []byte("envelope-data"))

	// Poll.
	req := httptest.NewRequest("GET", "/relay/poll?since=0&limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("poll: expected 200, got %d", w.Code)
	}

	var pollResp struct {
		Envelopes []struct {
			ID int64 `json:"id"`
		} `json:"envelopes"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatalf("decode poll response: %v", err)
	}
	if pollResp.Count != 1 {
		t.Fatalf("poll: expected count=1, got %d", pollResp.Count)
	}

	// Ack.
	ackBody, _ := json.Marshal(map[string]any{"ids": []int64{pollResp.Envelopes[0].ID}})
	req = httptest.NewRequest("POST", "/relay/ack", bytes.NewReader(ackBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ack: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ackResp struct {
		Acked int64 `json:"acked"`
	}
	if err := json.NewDecoder(w.Body).Decode(&ackResp); err != nil {
		t.Fatalf("decode ack response: %v", err)
	}
	if ackResp.Acked != 1 {
		t.Fatalf("ack: expected 1, got %d", ackResp.Acked)
	}

	// Poll again — should be empty.
	req = httptest.NewRequest("GET", "/relay/poll?since=0&limit=10", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	respBody, _ := io.ReadAll(w.Body)
	var pollResp2 struct{ Count int }
	_ = json.Unmarshal(respBody, &pollResp2)
	if pollResp2.Count != 0 {
		t.Fatalf("after ack, expected 0 envelopes, got %d", pollResp2.Count)
	}
}

func TestHandlerHealth(t *testing.T) {
	h, _ := newTestHandler(t)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct{ Status string }
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected ok, got %s", resp.Status)
	}
}
