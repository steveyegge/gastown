package integrations

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/outdoorsea/faultline/internal/notify"
)

func TestNewPagerDutyMissingRoutingKey(t *testing.T) {
	_, err := newPagerDuty(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing routing_key")
	}
}

func TestNewPagerDutyInvalidJSON(t *testing.T) {
	_, err := newPagerDuty(json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewPagerDutyDefaults(t *testing.T) {
	intg, err := newPagerDuty(json.RawMessage(`{"routing_key":"test-key"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pd := intg.(*pagerDuty)
	if pd.config.RoutingKey != "test-key" {
		t.Errorf("expected routing_key 'test-key', got %q", pd.config.RoutingKey)
	}
	if pd.config.SeverityMapping["fatal"] != "critical" {
		t.Errorf("expected fatal→critical default, got %q", pd.config.SeverityMapping["fatal"])
	}
	if pd.config.SeverityMapping["error"] != "error" {
		t.Errorf("expected error→error default, got %q", pd.config.SeverityMapping["error"])
	}
}

func TestNewPagerDutyCustomSeverity(t *testing.T) {
	cfg := `{"routing_key":"k","severity_mapping":{"fatal":"warning"}}`
	intg, err := newPagerDuty(json.RawMessage(cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pd := intg.(*pagerDuty)
	if pd.config.SeverityMapping["fatal"] != "warning" {
		t.Errorf("expected custom mapping fatal→warning, got %q", pd.config.SeverityMapping["fatal"])
	}
}

func TestPagerDutyType(t *testing.T) {
	intg, _ := newPagerDuty(json.RawMessage(`{"routing_key":"k"}`))
	if intg.Type() != TypePagerDuty {
		t.Errorf("expected type %s, got %s", TypePagerDuty, intg.Type())
	}
}

func TestPagerDutyRegistered(t *testing.T) {
	if !Registered(TypePagerDuty) {
		t.Fatal("pagerduty should be registered via init()")
	}
}

// testPDServer returns an httptest.Server that captures requests.
type pdCapture struct {
	requests []pdEvent
}

func newPDTestServer(t *testing.T, capture *pdCapture, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev pdEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		capture.requests = append(capture.requests, ev)
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(`{"status":"success","dedup_key":"test"}`))
	}))
}

func newTestPD(t *testing.T, serverURL string) *pagerDuty {
	t.Helper()
	intg, err := newPagerDuty(json.RawMessage(`{"routing_key":"test-routing-key","service_id":"SVC123"}`))
	if err != nil {
		t.Fatalf("newPagerDuty: %v", err)
	}
	pd := intg.(*pagerDuty)
	return pd
}

func TestPagerDutyOnNewIssue(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 202)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	// Override the send to use our test server by wrapping with a custom client transport.
	pd.client = srv.Client()

	// We need to override the URL. Use a roundtripper.
	pd.client.Transport = rewriteTransport{url: srv.URL}

	event := notify.Event{
		Type:       notify.EventNewIssue,
		ProjectID:  42,
		GroupID:    "grp-abc",
		Title:      "NullPointerException",
		Culprit:    "com.example.App.main",
		Level:      "fatal",
		Platform:   "java",
		EventCount: 5,
	}

	err := pd.OnNewIssue(context.Background(), event)
	if err != nil {
		t.Fatalf("OnNewIssue: %v", err)
	}

	if len(capture.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(capture.requests))
	}
	req := capture.requests[0]
	if req.EventAction != "trigger" {
		t.Errorf("expected action 'trigger', got %q", req.EventAction)
	}
	if req.RoutingKey != "test-routing-key" {
		t.Errorf("expected routing key, got %q", req.RoutingKey)
	}
	if req.DedupKey != "faultline-42-grp-abc" {
		t.Errorf("expected dedup key 'faultline-42-grp-abc', got %q", req.DedupKey)
	}
	if req.Payload == nil {
		t.Fatal("expected payload")
	}
	if req.Payload.Severity != "critical" {
		t.Errorf("expected severity 'critical' for fatal, got %q", req.Payload.Severity)
	}
	if req.Payload.Component != "SVC123" {
		t.Errorf("expected component 'SVC123', got %q", req.Payload.Component)
	}
	if req.Payload.Source != "faultline" {
		t.Errorf("expected source 'faultline', got %q", req.Payload.Source)
	}
}

func TestPagerDutyOnResolved(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 202)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	pd.client.Transport = rewriteTransport{url: srv.URL}

	event := notify.Event{
		Type:      notify.EventResolved,
		ProjectID: 42,
		GroupID:   "grp-abc",
		Title:     "NullPointerException",
		Level:     "error",
	}

	err := pd.OnResolved(context.Background(), event)
	if err != nil {
		t.Fatalf("OnResolved: %v", err)
	}

	if len(capture.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(capture.requests))
	}
	req := capture.requests[0]
	if req.EventAction != "resolve" {
		t.Errorf("expected action 'resolve', got %q", req.EventAction)
	}
	if req.Payload != nil {
		t.Error("resolve events should not have a payload")
	}
}

func TestPagerDutyOnRegression(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 202)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	pd.client.Transport = rewriteTransport{url: srv.URL}

	event := notify.Event{
		Type:      notify.EventRegression,
		ProjectID: 42,
		GroupID:   "grp-abc",
		Title:     "NullPointerException",
		Level:     "error",
	}

	err := pd.OnRegression(context.Background(), event)
	if err != nil {
		t.Fatalf("OnRegression: %v", err)
	}

	if len(capture.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(capture.requests))
	}
	if capture.requests[0].EventAction != "trigger" {
		t.Errorf("regression should trigger, got %q", capture.requests[0].EventAction)
	}
}

func TestPagerDutyHTTPError(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 429)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	pd.client.Transport = rewriteTransport{url: srv.URL}

	err := pd.OnNewIssue(context.Background(), notify.Event{
		Type:      notify.EventNewIssue,
		ProjectID: 1,
		GroupID:   "g",
		Level:     "error",
		Title:     "test",
	})
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
}

func TestPagerDutyMapSeverityUnknownLevel(t *testing.T) {
	intg, _ := newPagerDuty(json.RawMessage(`{"routing_key":"k"}`))
	pd := intg.(*pagerDuty)
	if s := pd.mapSeverity("unknown"); s != "error" {
		t.Errorf("unknown level should default to 'error', got %q", s)
	}
}

func TestPagerDutySummaryWithCulprit(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 202)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	pd.client.Transport = rewriteTransport{url: srv.URL}

	_ = pd.OnNewIssue(context.Background(), notify.Event{
		Type:      notify.EventNewIssue,
		ProjectID: 1,
		GroupID:   "g",
		Title:     "Crash",
		Culprit:   "main.go:42",
		Level:     "error",
	})

	if len(capture.requests) != 1 {
		t.Fatal("expected 1 request")
	}
	summary := capture.requests[0].Payload.Summary
	expected := "[error] Crash in main.go:42"
	if summary != expected {
		t.Errorf("expected summary %q, got %q", expected, summary)
	}
}

func TestPagerDutySummaryWithoutCulprit(t *testing.T) {
	capture := &pdCapture{}
	srv := newPDTestServer(t, capture, 202)
	defer srv.Close()

	pd := newTestPD(t, srv.URL)
	pd.client.Transport = rewriteTransport{url: srv.URL}

	_ = pd.OnNewIssue(context.Background(), notify.Event{
		Type:      notify.EventNewIssue,
		ProjectID: 1,
		GroupID:   "g",
		Title:     "Crash",
		Level:     "error",
	})

	if len(capture.requests) != 1 {
		t.Fatal("expected 1 request")
	}
	summary := capture.requests[0].Payload.Summary
	expected := "[error] Crash"
	if summary != expected {
		t.Errorf("expected summary %q, got %q", expected, summary)
	}
}

// rewriteTransport rewrites all requests to the test server URL.
type rewriteTransport struct {
	url string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.url[len("http://"):]
	return http.DefaultTransport.RoundTrip(req)
}
