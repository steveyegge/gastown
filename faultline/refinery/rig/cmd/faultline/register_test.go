package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	// Run in a temp directory so project files don't interfere.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	tests := []struct {
		file string
		want string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"Podfile", "swift"},
		{"Package.swift", "swift"},
	}

	for _, tt := range tests {
		// Clean up all files first.
		for _, tc := range tests {
			os.Remove(tc.file)
		}

		if err := os.WriteFile(tt.file, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		got := detectLanguage()
		if got != tt.want {
			t.Errorf("detectLanguage() with %s = %q, want %q", tt.file, got, tt.want)
		}
	}

	// Clean all files — should return empty.
	for _, tc := range tests {
		os.Remove(tc.file)
	}
	if got := detectLanguage(); got != "" {
		t.Errorf("detectLanguage() with no files = %q, want empty", got)
	}
}

func TestWriteEnvFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	t.Run("creates new file", func(t *testing.T) {
		os.Remove(".env")
		if err := writeEnvFile("FAULTLINE_DSN=http://key@localhost:8080/1"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(".env")
		if !strings.Contains(string(data), "FAULTLINE_DSN=http://key@localhost:8080/1") {
			t.Errorf("got %q, want DSN line", string(data))
		}
	})

	t.Run("rejects duplicate", func(t *testing.T) {
		err := writeEnvFile("FAULTLINE_DSN=http://key2@localhost:8080/2")
		if err == nil {
			t.Fatal("expected error for duplicate DSN")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("appends to existing file without DSN", func(t *testing.T) {
		os.Remove(".env")
		os.WriteFile(".env", []byte("OTHER_VAR=hello\n"), 0o600)
		if err := writeEnvFile("FAULTLINE_DSN=http://key@localhost:8080/3"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(".env")
		content := string(data)
		if !strings.Contains(content, "OTHER_VAR=hello") {
			t.Error("lost existing content")
		}
		if !strings.Contains(content, "FAULTLINE_DSN=http://key@localhost:8080/3") {
			t.Error("DSN not appended")
		}
	})
}

func TestCmdRegister_Integration(t *testing.T) {
	// Mock the faultline register API.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/register" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req struct {
			Name     string `json:"name"`
			Rig      string `json:"rig"`
			Language string `json:"language"`
			URL      string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "name required"})
			return
		}

		resp := map[string]interface{}{
			"project_id": 42,
			"name":       req.Name,
			"rig":        req.Rig,
			"public_key": "abc123def456",
			"dsn":        "http://abc123def456@localhost:8080/42",
			"endpoints": map[string]string{
				"envelope":  "http://localhost:8080/api/42/envelope/",
				"store":     "http://localhost:8080/api/42/store/",
				"heartbeat": "http://localhost:8080/api/42/heartbeat",
			},
			"env_var": "FAULTLINE_DSN=http://abc123def456@localhost:8080/42",
			"setup":   "# Example setup\nimport sentry_sdk",
			"notes":   []string{"Note 1", "Note 2"},
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	dir := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	// Create a go.mod to test language detection.
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)

	// Set env vars for the command.
	t.Setenv("FAULTLINE_SERVER", srv.Listener.Addr().String())
	t.Setenv("FAULTLINE_API_TOKEN", "test-token")

	// We can't call cmdRegister() directly because it uses os.Exit,
	// but we can test the building blocks that it uses.

	// Test that language detection works in this context.
	lang := detectLanguage()
	if lang != "go" {
		t.Errorf("detectLanguage() = %q, want \"go\"", lang)
	}
}
