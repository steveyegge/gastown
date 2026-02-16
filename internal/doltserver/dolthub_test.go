package doltserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

func TestDoltHubToken(t *testing.T) {
	// Save and restore original value
	orig := os.Getenv("DOLTHUB_TOKEN")
	defer os.Setenv("DOLTHUB_TOKEN", orig)

	os.Setenv("DOLTHUB_TOKEN", "dhat.v1.test123")
	if got := DoltHubToken(); got != "dhat.v1.test123" {
		t.Errorf("DoltHubToken() = %q, want %q", got, "dhat.v1.test123")
	}

	os.Unsetenv("DOLTHUB_TOKEN")
	if got := DoltHubToken(); got != "" {
		t.Errorf("DoltHubToken() = %q, want empty", got)
	}
}

func TestDoltHubOrg(t *testing.T) {
	orig := os.Getenv("DOLTHUB_ORG")
	defer os.Setenv("DOLTHUB_ORG", orig)

	os.Setenv("DOLTHUB_ORG", "bvts")
	if got := DoltHubOrg(); got != "bvts" {
		t.Errorf("DoltHubOrg() = %q, want %q", got, "bvts")
	}

	os.Unsetenv("DOLTHUB_ORG")
	if got := DoltHubOrg(); got != "" {
		t.Errorf("DoltHubOrg() = %q, want empty", got)
	}
}

func TestDoltHubRepoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"beads_gt", "beads-gt"},
		{"beads_bd", "beads-bd"},
		{"hq", "gt-hq"},
		{"laser", "laser"},
		{"payment_portal", "payment-portal"},
		{"a_b_c", "a-b-c"},
	}
	for _, tt := range tests {
		got := DoltHubRepoName(tt.input)
		if got != tt.want {
			t.Errorf("DoltHubRepoName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDoltHubRemoteURL(t *testing.T) {
	got := DoltHubRemoteURL("bvts", "beads-gt")
	want := "https://doltremoteapi.dolthub.com/bvts/beads-gt"
	if got != want {
		t.Errorf("DoltHubRemoteURL() = %q, want %q", got, want)
	}
}

func TestCreateDoltHubRepo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/database" {
			t.Errorf("expected path /database, got %s", r.URL.Path)
		}
		if r.Header.Get("authorization") != "token test-token" {
			t.Errorf("expected auth header, got %q", r.Header.Get("authorization"))
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["ownerName"] != "bvts" {
			t.Errorf("ownerName = %q, want bvts", body["ownerName"])
		}
		if body["repoName"] != "beads-gt" {
			t.Errorf("repoName = %q, want beads-gt", body["repoName"])
		}
		if body["visibility"] != "private" {
			t.Errorf("visibility = %q, want private", body["visibility"])
		}

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{
			"status":           "Success",
			"repository_owner": "bvts",
			"repository_name":  "beads-gt",
		})
	}))
	defer server.Close()

	// Override the API base so CreateDoltHubRepo hits our mock server.
	orig := dolthubAPIBase
	dolthubAPIBase = server.URL
	defer func() { dolthubAPIBase = orig }()

	if err := CreateDoltHubRepo("bvts", "beads-gt", "test-token"); err != nil {
		t.Fatalf("CreateDoltHubRepo returned error: %v", err)
	}
}

func TestCreateDoltHubRepo_AlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/database" {
			t.Errorf("expected path /database, got %s", r.URL.Path)
		}
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "Error",
			"message": "repository already exists",
		})
	}))
	defer server.Close()

	// Override the API base so CreateDoltHubRepo hits our mock server.
	orig := dolthubAPIBase
	dolthubAPIBase = server.URL
	defer func() { dolthubAPIBase = orig }()

	// "already exists" should be treated as success (nil error).
	if err := CreateDoltHubRepo("bvts", "beads-gt", "test-token"); err != nil {
		t.Fatalf("CreateDoltHubRepo returned error for 409 already-exists: %v", err)
	}
}

func TestAddRemote_NoExistingRemote(t *testing.T) {
	// Create a temporary Dolt database to test remote addition.
	// This requires dolt to be installed.
	tmpDir := t.TempDir()

	// Initialize a minimal dolt repo
	initCmd := exec.Command("dolt", "init")
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Skipf("dolt not available: %v (%s)", err, output)
	}

	// Add remote
	if err := AddRemote(tmpDir, "testorg", "testrepo"); err != nil {
		t.Fatalf("AddRemote failed: %v", err)
	}

	// Verify remote was added
	remote, err := HasRemote(tmpDir)
	if err != nil {
		t.Fatalf("HasRemote failed: %v", err)
	}
	want := "https://doltremoteapi.dolthub.com/testorg/testrepo"
	if remote != want {
		t.Errorf("remote = %q, want %q", remote, want)
	}
}

func TestAddRemote_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	initCmd := exec.Command("dolt", "init")
	initCmd.Dir = tmpDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Skipf("dolt not available: %v (%s)", err, output)
	}

	// Add remote twice — second should be a no-op
	if err := AddRemote(tmpDir, "testorg", "testrepo"); err != nil {
		t.Fatalf("first AddRemote failed: %v", err)
	}
	if err := AddRemote(tmpDir, "testorg", "testrepo"); err != nil {
		t.Fatalf("second AddRemote should be idempotent: %v", err)
	}
}
