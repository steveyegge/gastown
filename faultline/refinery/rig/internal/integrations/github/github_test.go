package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/outdoorsea/faultline/internal/integrations"
	"github.com/outdoorsea/faultline/internal/notify"
)

// roundTripFunc allows inline HTTP transport stubs.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func stubClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func jsonResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func testConfig() Config {
	return Config{
		Owner:   "test-owner",
		Repo:    "test-repo",
		Token:   "ghp_test_token",
		BaseURL: "https://faultline.example.com",
		Labels:  map[string]string{"error": "bug", "fatal": "critical"},
	}
}

func testEvent() notify.Event {
	return notify.Event{
		Type:       notify.EventNewIssue,
		ProjectID:  42,
		GroupID:    "abc-123-def",
		Title:      "NullPointerException in UserService",
		Culprit:    "com.example.UserService.getUser",
		Level:      "error",
		Platform:   "java",
		EventCount: 5,
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"valid", testConfig(), ""},
		{"missing owner", Config{Repo: "r", Token: "t"}, "owner is required"},
		{"missing repo", Config{Owner: "o", Token: "t"}, "repo is required"},
		{"missing token", Config{Owner: "o", Repo: "r"}, "token is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q should contain %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestAPIBaseURL(t *testing.T) {
	cfg := testConfig()
	if cfg.apiBase() != "https://api.github.com" {
		t.Errorf("default apiBase should be https://api.github.com, got %s", cfg.apiBase())
	}

	cfg.APIBaseURL = "https://github.corp.example.com/api/v3/"
	if cfg.apiBase() != "https://github.corp.example.com/api/v3" {
		t.Errorf("custom apiBase should strip trailing slash, got %s", cfg.apiBase())
	}
}

func TestType(t *testing.T) {
	g := New(testConfig(), stubClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, "{}"), nil
	}))
	if g.Type() != integrations.TypeGitHubIssues {
		t.Errorf("expected type %s, got %s", integrations.TypeGitHubIssues, g.Type())
	}
}

func TestOnNewIssue(t *testing.T) {
	var gotReq *http.Request
	var gotBody map[string]any

	client := stubClient(func(req *http.Request) (*http.Response, error) {
		gotReq = req.Clone(req.Context())
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResponse(201, `{"number": 1, "html_url": "https://github.com/test-owner/test-repo/issues/1"}`), nil
	})

	g := New(testConfig(), client)
	err := g.OnNewIssue(context.Background(), testEvent())
	if err != nil {
		t.Fatalf("OnNewIssue: %v", err)
	}

	if gotReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", gotReq.Method)
	}
	if !strings.Contains(gotReq.URL.Path, "/repos/test-owner/test-repo/issues") {
		t.Errorf("unexpected URL path: %s", gotReq.URL.Path)
	}
	if gotReq.Header.Get("Authorization") != "Bearer ghp_test_token" {
		t.Errorf("missing or wrong Authorization header")
	}

	title, _ := gotBody["title"].(string)
	if !strings.Contains(title, "NullPointerException") {
		t.Errorf("title should contain event title, got %q", title)
	}
	if !strings.HasPrefix(title, "[Faultline]") {
		t.Errorf("title should have [Faultline] prefix, got %q", title)
	}

	body, _ := gotBody["body"].(string)
	if !strings.Contains(body, "faultline-group-id:abc-123-def") {
		t.Errorf("body should contain faultline marker, got %q", body)
	}
	if !strings.Contains(body, "faultline.example.com") {
		t.Errorf("body should contain faultline backlink")
	}

	labels, _ := gotBody["labels"].([]any)
	if len(labels) < 2 {
		t.Fatalf("expected at least 2 labels, got %d", len(labels))
	}
	hasFL := false
	hasBug := false
	for _, l := range labels {
		switch l.(string) {
		case "faultline":
			hasFL = true
		case "bug":
			hasBug = true
		}
	}
	if !hasFL || !hasBug {
		t.Errorf("labels should include 'faultline' and 'bug' (mapped from error level), got %v", labels)
	}
}

func TestOnResolved(t *testing.T) {
	var requests []struct {
		method string
		url    string
		body   map[string]any
	}

	client := stubClient(func(req *http.Request) (*http.Response, error) {
		var parsed map[string]any
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &parsed)
		}
		requests = append(requests, struct {
			method string
			url    string
			body   map[string]any
		}{req.Method, req.URL.String(), parsed})

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/search/issues") {
			return jsonResponse(200, `{"items": [{"number": 7}]}`), nil
		}
		return jsonResponse(200, `{}`), nil
	})

	g := New(testConfig(), client)
	event := testEvent()
	event.Type = notify.EventResolved

	err := g.OnResolved(context.Background(), event)
	if err != nil {
		t.Fatalf("OnResolved: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 API calls (search + comment + close), got %d", len(requests))
	}

	// First: search
	if requests[0].method != http.MethodGet {
		t.Errorf("first call should be GET search, got %s", requests[0].method)
	}

	// Second: comment
	if requests[1].method != http.MethodPost {
		t.Errorf("second call should be POST comment, got %s", requests[1].method)
	}
	if !strings.Contains(requests[1].url, "/issues/7/comments") {
		t.Errorf("comment should target issue 7, got %s", requests[1].url)
	}

	// Third: close
	if requests[2].method != http.MethodPatch {
		t.Errorf("third call should be PATCH close, got %s", requests[2].method)
	}
	if state, _ := requests[2].body["state"].(string); state != "closed" {
		t.Errorf("expected state=closed, got %s", state)
	}
}

func TestOnResolvedNoMatchingIssue(t *testing.T) {
	client := stubClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"items": []}`), nil
	})

	g := New(testConfig(), client)
	err := g.OnResolved(context.Background(), testEvent())
	if err != nil {
		t.Fatalf("OnResolved with no match should succeed, got: %v", err)
	}
}

func TestOnRegression(t *testing.T) {
	var requests []struct {
		method string
		url    string
	}

	client := stubClient(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, struct {
			method string
			url    string
		}{req.Method, req.URL.String()})

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/search/issues") {
			return jsonResponse(200, `{"items": [{"number": 12}]}`), nil
		}
		return jsonResponse(200, `{}`), nil
	})

	g := New(testConfig(), client)
	event := testEvent()
	event.Type = notify.EventRegression

	err := g.OnRegression(context.Background(), event)
	if err != nil {
		t.Fatalf("OnRegression: %v", err)
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 API calls (search + comment + reopen), got %d", len(requests))
	}

	// Verify reopen
	if !strings.Contains(requests[2].url, "/issues/12") {
		t.Errorf("should reopen issue 12, got %s", requests[2].url)
	}
}

func TestOnRegressionNoExistingIssue(t *testing.T) {
	var created bool
	client := stubClient(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			return jsonResponse(200, `{"items": []}`), nil
		}
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/repos/") {
			created = true
			return jsonResponse(201, `{"number": 1}`), nil
		}
		return jsonResponse(200, `{}`), nil
	})

	g := New(testConfig(), client)
	event := testEvent()
	event.Type = notify.EventRegression

	err := g.OnRegression(context.Background(), event)
	if err != nil {
		t.Fatalf("OnRegression: %v", err)
	}
	if !created {
		t.Error("expected a new issue to be created when no existing issue found")
	}
}

func TestAPIError(t *testing.T) {
	client := stubClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(422, `{"message": "Validation Failed"}`), nil
	})

	g := New(testConfig(), client)
	err := g.OnNewIssue(context.Background(), testEvent())
	if err == nil {
		t.Fatal("expected error on 422 response")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestFactoryRegistration(t *testing.T) {
	if !integrations.Registered(integrations.TypeGitHubIssues) {
		t.Fatal("github_issues should be registered after init()")
	}

	cfg := `{"owner": "o", "repo": "r", "token": "t"}`
	intg, err := integrations.New(integrations.TypeGitHubIssues, json.RawMessage(cfg))
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if intg.Type() != integrations.TypeGitHubIssues {
		t.Errorf("expected type %s, got %s", integrations.TypeGitHubIssues, intg.Type())
	}
}

func TestFactoryInvalidConfig(t *testing.T) {
	_, err := integrations.New(integrations.TypeGitHubIssues, json.RawMessage(`{"owner": ""}`))
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestLabelsFor(t *testing.T) {
	g := New(testConfig(), nil)

	labels := g.labelsFor(notify.Event{Level: "error"})
	if len(labels) != 2 || labels[0] != "faultline" || labels[1] != "bug" {
		t.Errorf("expected [faultline bug], got %v", labels)
	}

	labels = g.labelsFor(notify.Event{Level: "info"})
	if len(labels) != 1 || labels[0] != "faultline" {
		t.Errorf("expected [faultline] for unmapped level, got %v", labels)
	}
}

func TestFaultlineMarker(t *testing.T) {
	marker := faultlineMarker("abc-123")
	if marker != "faultline-group-id:abc-123" {
		t.Errorf("unexpected marker: %s", marker)
	}
}
