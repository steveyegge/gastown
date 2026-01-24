package refinery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultMergeQueueConfig_Strategy(t *testing.T) {
	cfg := DefaultMergeQueueConfig()
	if cfg.Strategy != StrategyDirectMerge {
		t.Errorf("expected Strategy %q, got %q", StrategyDirectMerge, cfg.Strategy)
	}
}

func TestEngineer_LoadConfig_WithStrategy(t *testing.T) {
	// Create a temp directory with config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file with strategy
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
		"merge_queue": map[string]interface{}{
			"enabled":       true,
			"strategy":      "pr_to_main",
			"target_branch": "main",
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

	if e.config.Strategy != StrategyPRToMain {
		t.Errorf("expected Strategy %q, got %q", StrategyPRToMain, e.config.Strategy)
	}
}

func TestEngineer_LoadConfig_WithPROptions(t *testing.T) {
	// Create a temp directory with config.json
	tmpDir, err := os.MkdirTemp("", "engineer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file with PR options
	config := map[string]interface{}{
		"type":    "rig",
		"version": 1,
		"name":    "test-rig",
		"merge_queue": map[string]interface{}{
			"enabled":       true,
			"strategy":      "pr_to_main",
			"target_branch": "main",
			"pr_options": map[string]interface{}{
				"template":   ".github/PR_TEMPLATE.md",
				"auto_merge": true,
				"labels":     []string{"automated", "from-gastown"},
				"reviewers":  []string{"reviewer1"},
				"draft":      false,
			},
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

	if e.config.PROptions == nil {
		t.Fatal("expected PROptions to be set")
	}

	opts := e.config.PROptions
	if opts.Template != ".github/PR_TEMPLATE.md" {
		t.Errorf("expected Template '.github/PR_TEMPLATE.md', got %q", opts.Template)
	}
	if !opts.AutoMerge {
		t.Error("expected AutoMerge true")
	}
	if len(opts.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(opts.Labels))
	}
	if len(opts.Reviewers) != 1 || opts.Reviewers[0] != "reviewer1" {
		t.Errorf("expected reviewers [reviewer1], got %v", opts.Reviewers)
	}
}

func TestValidMergeStrategies(t *testing.T) {
	strategies := ValidMergeStrategies()
	if len(strategies) != 4 {
		t.Errorf("expected 4 strategies, got %d", len(strategies))
	}

	expected := map[string]bool{
		"direct_merge":     true,
		"pr_to_main":       true,
		"pr_to_branch":     true,
		"direct_to_branch": true,
	}

	for _, s := range strategies {
		if !expected[s] {
			t.Errorf("unexpected strategy %q", s)
		}
	}
}

func TestIsValidMergeStrategy(t *testing.T) {
	tests := []struct {
		strategy string
		want     bool
	}{
		{StrategyDirectMerge, true},
		{StrategyPRToMain, true},
		{StrategyPRToBranch, true},
		{StrategyDirectToBranch, true},
		{"invalid", false},
		{"", false},
		{"DIRECT_MERGE", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			got := IsValidMergeStrategy(tt.strategy)
			if got != tt.want {
				t.Errorf("IsValidMergeStrategy(%q) = %v, want %v", tt.strategy, got, tt.want)
			}
		})
	}
}

func TestIsPRStrategy(t *testing.T) {
	tests := []struct {
		strategy string
		want     bool
	}{
		{StrategyDirectMerge, false},
		{StrategyPRToMain, true},
		{StrategyPRToBranch, true},
		{StrategyDirectToBranch, false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			got := IsPRStrategy(tt.strategy)
			if got != tt.want {
				t.Errorf("IsPRStrategy(%q) = %v, want %v", tt.strategy, got, tt.want)
			}
		})
	}
}

func TestEngineer_LoadConfig_InvalidStrategy(t *testing.T) {
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
			"enabled":  true,
			"strategy": "invalid_strategy",
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
		t.Error("expected error for invalid strategy")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid merge strategy") {
		t.Errorf("expected 'invalid merge strategy' error, got: %v", err)
	}
}
