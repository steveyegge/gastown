package quota

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPUsageClient_FetchUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/organizations/test-org/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		cookie := r.Header.Get("Cookie")
		if cookie != "sessionKey=test-cookie" {
			t.Errorf("unexpected cookie: %s", cookie)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"five_hour": map[string]interface{}{
				"utilization": 73.2,
				"resets_at":   "2026-02-28T18:00:00Z",
			},
			"seven_day": map[string]interface{}{
				"utilization": 45.1,
				"resets_at":   "2026-03-07T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	// Patch the client to use the test server
	client := &HTTPUsageClient{
		client: server.Client(),
	}

	// Override URL by using a custom transport
	transport := &rewriteTransport{
		base:    server.Client().Transport,
		baseURL: server.URL,
	}
	client.client.Transport = transport

	usage, err := client.FetchUsage("test-org", "test-cookie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if usage.FiveHour == nil {
		t.Fatal("expected five_hour data")
	}
	if usage.FiveHour.Utilization != 73.2 {
		t.Errorf("expected 73.2, got %f", usage.FiveHour.Utilization)
	}
	if usage.SevenDay == nil {
		t.Fatal("expected seven_day data")
	}
	if usage.SevenDay.Utilization != 45.1 {
		t.Errorf("expected 45.1, got %f", usage.SevenDay.Utilization)
	}
}

func TestHTTPUsageClient_FetchUsage_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &HTTPUsageClient{client: server.Client()}
	client.client.Transport = &rewriteTransport{
		base:    server.Client().Transport,
		baseURL: server.URL,
	}

	_, err := client.FetchUsage("bad-org", "bad-cookie")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestUsageInfo_MaxUtilization(t *testing.T) {
	tests := []struct {
		name     string
		info     UsageInfo
		expected float64
	}{
		{
			name:     "both windows",
			info:     UsageInfo{FiveHour: &UsageWindow{Utilization: 80}, SevenDay: &UsageWindow{Utilization: 50}},
			expected: 80,
		},
		{
			name:     "seven_day higher",
			info:     UsageInfo{FiveHour: &UsageWindow{Utilization: 30}, SevenDay: &UsageWindow{Utilization: 90}},
			expected: 90,
		},
		{
			name:     "five_hour only",
			info:     UsageInfo{FiveHour: &UsageWindow{Utilization: 60}},
			expected: 60,
		},
		{
			name:     "empty",
			info:     UsageInfo{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.MaxUtilization()
			if got != tt.expected {
				t.Errorf("MaxUtilization() = %f, want %f", got, tt.expected)
			}
		})
	}
}

func TestReadOrgID(t *testing.T) {
	dir := t.TempDir()

	// Write a .claude.json with oauthAccount containing organizationUuid
	data := map[string]interface{}{
		"oauthAccount": map[string]interface{}{
			"organizationUuid": "test-org-uuid-123",
			"accountUuid":      "test-account-uuid",
		},
	}
	raw, _ := json.Marshal(data)
	os.WriteFile(filepath.Join(dir, ".claude.json"), raw, 0644)

	orgID := ReadOrgID(dir)
	if orgID != "test-org-uuid-123" {
		t.Errorf("expected 'test-org-uuid-123', got %q", orgID)
	}
}

func TestReadOrgID_NoFile(t *testing.T) {
	orgID := ReadOrgID("/nonexistent/path")
	if orgID != "" {
		t.Errorf("expected empty string, got %q", orgID)
	}
}

func TestReadOrgID_NoOrgField(t *testing.T) {
	dir := t.TempDir()
	data := map[string]interface{}{
		"oauthAccount": map[string]interface{}{
			"accountUuid": "test-account-uuid",
		},
	}
	raw, _ := json.Marshal(data)
	os.WriteFile(filepath.Join(dir, ".claude.json"), raw, 0644)

	orgID := ReadOrgID(dir)
	if orgID != "" {
		t.Errorf("expected empty string, got %q", orgID)
	}
}

// rewriteTransport rewrites requests to go to the test server.
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server, preserving path and query
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
