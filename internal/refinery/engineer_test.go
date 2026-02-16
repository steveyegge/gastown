package refinery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestDefaultMergeQueueConfig(t *testing.T) {
	cfg := DefaultMergeQueueConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true by default")
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("expected PollInterval to be 30s, got %v", cfg.PollInterval)
	}
	if cfg.MaxConcurrent != 1 {
		t.Errorf("expected MaxConcurrent to be 1, got %d", cfg.MaxConcurrent)
	}
	if cfg.OnConflict != "assign_back" {
		t.Errorf("expected OnConflict to be 'assign_back', got %q", cfg.OnConflict)
	}
	if cfg.StaleClaimTimeout != DefaultStaleClaimTimeout {
		t.Errorf("expected StaleClaimTimeout to be %v, got %v", DefaultStaleClaimTimeout, cfg.StaleClaimTimeout)
	}
}

func TestEngineer_LoadConfig_NoFile(t *testing.T) {
	// Create a temp directory without config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	// Should not error with missing config file
	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error with missing config: %v", err)
	}

	// Should use defaults
	if e.config.PollInterval != 30*time.Second {
		t.Errorf("expected default PollInterval, got %v", e.config.PollInterval)
	}
}

func TestEngineer_LoadConfig_WithMergeQueue(t *testing.T) {
	// Create a temp directory with config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
		"merge_queue": map[string]interface{}{
			"enabled":             true,
			"poll_interval":       "10s",
			"max_concurrent":      2,
			"run_tests":           false,
			"test_command":        "make test",
			"stale_claim_timeout": "1h",
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error loading config: %v", err)
	}

	// Check that config values were loaded
	if e.config.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", e.config.PollInterval)
	}
	if e.config.MaxConcurrent != 2 {
		t.Errorf("expected MaxConcurrent 2, got %d", e.config.MaxConcurrent)
	}
	if e.config.RunTests != false {
		t.Errorf("expected RunTests false, got %v", e.config.RunTests)
	}
	if e.config.TestCommand != "make test" {
		t.Errorf("expected TestCommand 'make test', got %q", e.config.TestCommand)
	}
	if e.config.StaleClaimTimeout != 1*time.Hour {
		t.Errorf("expected StaleClaimTimeout 1h, got %v", e.config.StaleClaimTimeout)
	}

	// Check that defaults are preserved for unspecified fields
	if e.config.OnConflict != "assign_back" {
		t.Errorf("expected OnConflict default 'assign_back', got %q", e.config.OnConflict)
	}
}

func TestEngineer_LoadConfig_NoMergeQueueSection(t *testing.T) {
	// Create a temp directory with config.json without merge_queue
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file without merge_queue
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	if err := e.LoadConfig(); err != nil {
		t.Errorf("unexpected error loading config: %v", err)
	}

	// Should use all defaults
	if e.config.PollInterval != 30*time.Second {
		t.Errorf("expected default PollInterval, got %v", e.config.PollInterval)
	}
}

func TestEngineer_LoadConfig_InvalidPollInterval(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := map[string]interface{}{
		"merge_queue": map[string]interface{}{
			"poll_interval": "not-a-duration",
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}

	e := NewEngineer(r)

	err = e.LoadConfig()
	if err == nil {
		t.Error("expected error for invalid poll_interval")
	}
}

func TestEngineer_LoadConfig_InvalidStaleClaimTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		timeout string
	}{
		{"not a duration", "not-a-duration"},
		{"zero", "0s"},
		{"negative", "-5m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]interface{}{
				"merge_queue": map[string]interface{}{
					"stale_claim_timeout": tt.timeout,
				},
			}

			data, _ := json.MarshalIndent(config, "", "  ")
			if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
				t.Fatal(err)
			}

			r := &rig.Rig{
				Name: "test-rig",
				Path: tmpDir,
			}

			e := NewEngineer(r)

			err := e.LoadConfig()
			if err == nil {
				t.Errorf("expected error for stale_claim_timeout %q", tt.timeout)
			}
		})
	}
}

func TestNewEngineer(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}

	e := NewEngineer(r)

	if e.rig != r {
		t.Error("expected rig to be set")
	}
	if e.beads == nil {
		t.Error("expected beads client to be initialized")
	}
	if e.git == nil {
		t.Error("expected git client to be initialized")
	}
	if e.config == nil {
		t.Error("expected config to be initialized with defaults")
	}
}

func TestEngineer_DeleteMergedBranchesConfig(t *testing.T) {
	// Test that DeleteMergedBranches is true by default
	cfg := DefaultMergeQueueConfig()
	if !cfg.DeleteMergedBranches {
		t.Error("expected DeleteMergedBranches to be true by default")
	}
}

func TestIsClaimStale(t *testing.T) {
	timeout := DefaultStaleClaimTimeout

	tests := []struct {
		name      string
		updatedAt string
		want      bool
		wantErr   bool
	}{
		{
			name:      "stale claim (> threshold)",
			updatedAt: time.Now().Add(-timeout - 5*time.Minute).Format(time.RFC3339),
			want:      true,
		},
		{
			name:      "recent claim (< threshold)",
			updatedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
			want:      false,
		},
		{
			name:      "exactly at threshold",
			updatedAt: time.Now().Add(-timeout).Format(time.RFC3339),
			want:      true,
		},
		{
			name:      "just under threshold",
			updatedAt: time.Now().Add(-timeout + time.Second).Format(time.RFC3339),
			want:      false,
		},
		{
			name:      "empty timestamp",
			updatedAt: "",
			want:      false,
		},
		{
			name:      "invalid timestamp format",
			updatedAt: "not-a-timestamp",
			want:      false,
			wantErr:   true,
		},
		{
			name:      "wrong date format",
			updatedAt: "2026-01-14 12:00:00",
			want:      false,
			wantErr:   true,
		},
		{
			name:      "custom short timeout",
			updatedAt: time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to := timeout
			if tt.name == "custom short timeout" {
				to = 1 * time.Minute // Test configurable timeout
			}
			got, err := isClaimStale(tt.updatedAt, to)
			if (err != nil) != tt.wantErr {
				t.Errorf("isClaimStale(%q) error = %v, wantErr %v", tt.updatedAt, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("isClaimStale(%q) = %v, want %v", tt.updatedAt, got, tt.want)
			}
		})
	}
}
