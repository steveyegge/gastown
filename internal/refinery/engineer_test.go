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
	if cfg.TargetBranch != "main" {
		t.Errorf("expected TargetBranch to be 'main', got %q", cfg.TargetBranch)
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
			"enabled":        true,
			"target_branch":  "develop",
			"poll_interval":  "10s",
			"max_concurrent": 2,
			"run_tests":      false,
			"test_command":   "make test",
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
	if e.config.TargetBranch != "develop" {
		t.Errorf("expected TargetBranch 'develop', got %q", e.config.TargetBranch)
	}
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

func TestDefaultMergeQueueConfig_BatchDefaults(t *testing.T) {
	cfg := DefaultMergeQueueConfig()

	// Batch merge should be disabled by default (opt-in feature)
	if cfg.BatchMerge {
		t.Error("expected BatchMerge to be false by default")
	}
	if cfg.BatchSize != 5 {
		t.Errorf("expected BatchSize to be 5, got %d", cfg.BatchSize)
	}
	if cfg.BatchWindow != 5*time.Minute {
		t.Errorf("expected BatchWindow to be 5m, got %v", cfg.BatchWindow)
	}
	if cfg.BatchStrategy != "all-or-nothing" {
		t.Errorf("expected BatchStrategy to be 'all-or-nothing', got %q", cfg.BatchStrategy)
	}
}

func TestEngineer_LoadConfig_WithBatchOptions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
		"merge_queue": map[string]interface{}{
			"batch_merge":    true,
			"batch_size":     10,
			"batch_window":   "10m",
			"batch_strategy": "bisect-on-fail",
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

	if !e.config.BatchMerge {
		t.Error("expected BatchMerge to be true")
	}
	if e.config.BatchSize != 10 {
		t.Errorf("expected BatchSize 10, got %d", e.config.BatchSize)
	}
	if e.config.BatchWindow != 10*time.Minute {
		t.Errorf("expected BatchWindow 10m, got %v", e.config.BatchWindow)
	}
	if e.config.BatchStrategy != "bisect-on-fail" {
		t.Errorf("expected BatchStrategy 'bisect-on-fail', got %q", e.config.BatchStrategy)
	}
}

func TestEngineer_LoadConfig_InvalidBatchWindow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := map[string]interface{}{
		"merge_queue": map[string]interface{}{
			"batch_window": "invalid",
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
		t.Error("expected error for invalid batch_window")
	}
}

func TestBatchResult_Empty(t *testing.T) {
	// An empty batch result should indicate success with no merged/ejected MRs
	result := BatchResult{Success: true}
	if !result.Success {
		t.Error("expected empty batch result to be successful")
	}
	if len(result.Merged) != 0 {
		t.Error("expected no merged MRs in empty batch")
	}
	if len(result.Ejected) != 0 {
		t.Error("expected no ejected MRs in empty batch")
	}
}

func TestBatchResult_Fields(t *testing.T) {
	// Test that BatchResult can hold all expected fields
	merged := []*MRInfo{{ID: "mr-001"}, {ID: "mr-002"}}
	ejected := []*MRInfo{{ID: "mr-003"}}

	result := BatchResult{
		Success:       true,
		MergeCommit:   "abc123",
		Merged:        merged,
		Ejected:       ejected,
		StagingBranch: "batch-staging-12345",
	}

	if !result.Success {
		t.Error("expected success")
	}
	if result.MergeCommit != "abc123" {
		t.Errorf("expected MergeCommit 'abc123', got %q", result.MergeCommit)
	}
	if len(result.Merged) != 2 {
		t.Errorf("expected 2 merged MRs, got %d", len(result.Merged))
	}
	if len(result.Ejected) != 1 {
		t.Errorf("expected 1 ejected MR, got %d", len(result.Ejected))
	}
	if result.StagingBranch != "batch-staging-12345" {
		t.Errorf("expected staging branch name, got %q", result.StagingBranch)
	}
}

func TestBatchResult_FailedBatch(t *testing.T) {
	// Test batch failure scenario
	failed := []*MRInfo{{ID: "mr-001"}, {ID: "mr-002"}}

	result := BatchResult{
		Success:     false,
		TestsFailed: true,
		Error:       "tests failed: exit code 1",
		Failed:      failed,
	}

	if result.Success {
		t.Error("expected failure")
	}
	if !result.TestsFailed {
		t.Error("expected TestsFailed to be true")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
	if len(result.Failed) != 2 {
		t.Errorf("expected 2 failed MRs, got %d", len(result.Failed))
	}
}
