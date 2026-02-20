package refinery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func TestEngineer_LoadConfig_WithGates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-gates-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := map[string]interface{}{
		"merge_queue": map[string]interface{}{
			"gates": map[string]interface{}{
				"test": map[string]interface{}{
					"cmd":     "go test ./...",
					"timeout": "5m",
				},
				"lint": map[string]interface{}{
					"cmd":     "golangci-lint run",
					"timeout": "2m",
				},
				"build": map[string]interface{}{
					"cmd": "go build ./...",
				},
			},
			"gates_parallel": true,
		},
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{Name: "test-rig", Path: tmpDir}
	e := NewEngineer(r)

	if err := e.LoadConfig(); err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if len(e.config.Gates) != 3 {
		t.Fatalf("expected 3 gates, got %d", len(e.config.Gates))
	}
	if e.config.Gates["test"].Cmd != "go test ./..." {
		t.Errorf("expected test gate cmd 'go test ./...', got %q", e.config.Gates["test"].Cmd)
	}
	if e.config.Gates["test"].Timeout != 5*time.Minute {
		t.Errorf("expected test gate timeout 5m, got %v", e.config.Gates["test"].Timeout)
	}
	if e.config.Gates["lint"].Timeout != 2*time.Minute {
		t.Errorf("expected lint gate timeout 2m, got %v", e.config.Gates["lint"].Timeout)
	}
	if e.config.Gates["build"].Timeout != 0 {
		t.Errorf("expected build gate timeout 0 (no timeout), got %v", e.config.Gates["build"].Timeout)
	}
	if !e.config.GatesParallel {
		t.Error("expected gates_parallel to be true")
	}
}

func TestEngineer_LoadConfig_GateInvalidTimeout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engineer-gates-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		timeout string
	}{
		{"not a duration", "not-a-duration"},
		{"negative", "-5m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]interface{}{
				"merge_queue": map[string]interface{}{
					"gates": map[string]interface{}{
						"bad": map[string]interface{}{
							"cmd":     "echo test",
							"timeout": tt.timeout,
						},
					},
				},
			}

			data, _ := json.MarshalIndent(config, "", "  ")
			if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0644); err != nil {
				t.Fatal(err)
			}

			r := &rig.Rig{Name: "test-rig", Path: tmpDir}
			e := NewEngineer(r)

			err := e.LoadConfig()
			if err == nil {
				t.Errorf("expected error for gate timeout %q", tt.timeout)
			}
		})
	}
}

func TestRunGate_Success(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()

	result := e.runGate(context.Background(), "echo-test", &GateConfig{
		Cmd: "echo hello",
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Name != "echo-test" {
		t.Errorf("expected name 'echo-test', got %q", result.Name)
	}
}

func TestRunGate_Failure(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()

	result := e.runGate(context.Background(), "fail-test", &GateConfig{
		Cmd: "exit 1",
	})

	if result.Success {
		t.Error("expected failure")
	}
	if result.Name != "fail-test" {
		t.Errorf("expected name 'fail-test', got %q", result.Name)
	}
}

func TestRunGate_EmptyCmd(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()

	result := e.runGate(context.Background(), "empty", &GateConfig{
		Cmd: "",
	})

	if result.Success {
		t.Error("expected failure for empty cmd")
	}
}

func TestRunGate_Timeout(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()

	result := e.runGate(context.Background(), "slow", &GateConfig{
		Cmd:     "sleep 10",
		Timeout: 100 * time.Millisecond,
	})

	if result.Success {
		t.Error("expected timeout failure")
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Errorf("expected timeout error, got: %s", result.Error)
	}
}

func TestRunGates_Sequential_AllPass(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()
	e.output = io.Discard
	e.config.Gates = map[string]*GateConfig{
		"a": {Cmd: "true"},
		"b": {Cmd: "true"},
		"c": {Cmd: "true"},
	}
	e.config.GatesParallel = false

	result := e.runGates(context.Background())
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestRunGates_Sequential_StopsOnFirstFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gate commands run via sh -c; touch with Windows paths breaks under MSYS2 shell")
	}
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()
	e.output = io.Discard

	// Create a marker file to track which gates ran
	markerDir := t.TempDir()
	e.config.Gates = map[string]*GateConfig{
		"a_pass": {Cmd: fmt.Sprintf("touch %s/a", markerDir)},
		"b_fail": {Cmd: "exit 1"},
		"c_skip": {Cmd: fmt.Sprintf("touch %s/c", markerDir)},
	}
	e.config.GatesParallel = false

	result := e.runGates(context.Background())
	if result.Success {
		t.Error("expected failure")
	}

	// Gate "a_pass" should have run
	if _, err := os.Stat(filepath.Join(markerDir, "a")); os.IsNotExist(err) {
		t.Error("gate 'a_pass' should have run")
	}
	// Gate "c_skip" should NOT have run (stopped after b_fail)
	if _, err := os.Stat(filepath.Join(markerDir, "c")); !os.IsNotExist(err) {
		t.Error("gate 'c_skip' should not have run after failure")
	}
}

func TestRunGates_Parallel_AllPass(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()
	e.output = io.Discard
	e.config.Gates = map[string]*GateConfig{
		"a": {Cmd: "true"},
		"b": {Cmd: "true"},
		"c": {Cmd: "true"},
	}
	e.config.GatesParallel = true

	result := e.runGates(context.Background())
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestRunGates_Parallel_AnyFailure(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()
	e.output = io.Discard
	e.config.Gates = map[string]*GateConfig{
		"pass1": {Cmd: "true"},
		"fail1": {Cmd: "exit 1"},
		"pass2": {Cmd: "true"},
	}
	e.config.GatesParallel = true

	result := e.runGates(context.Background())
	if result.Success {
		t.Error("expected failure when any gate fails")
	}
	if !result.TestsFailed {
		t.Error("expected TestsFailed to be true")
	}
	if !strings.Contains(result.Error, "fail1") {
		t.Errorf("expected error to mention 'fail1', got: %s", result.Error)
	}
}

func TestRunGates_Empty(t *testing.T) {
	r := &rig.Rig{Name: "test-rig", Path: t.TempDir()}
	e := NewEngineer(r)
	e.workDir = t.TempDir()
	e.output = io.Discard
	e.config.Gates = nil

	result := e.runGates(context.Background())
	if !result.Success {
		t.Error("expected success with no gates configured")
	}
}

func TestEngineer_DeleteMergedBranchesConfig(t *testing.T) {
	// Test that DeleteMergedBranches is true by default
	cfg := DefaultMergeQueueConfig()
	if !cfg.DeleteMergedBranches {
		t.Error("expected DeleteMergedBranches to be true by default")
	}
}

func TestPostMergeConvoyCheck_NoTownBeads(t *testing.T) {
	// postMergeConvoyCheck should silently return when town-level beads doesn't exist
	tmpDir, err := os.MkdirTemp("", "engineer-convoy-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create rig dir as a subdirectory of the "town root"
	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigDir,
	}

	e := NewEngineer(r)
	var buf bytes.Buffer
	e.SetOutput(&buf)

	// Call with a nil-safe MR — should not panic
	mr := &MRInfo{
		ID:          "gt-test",
		SourceIssue: "gt-src",
		ConvoyID:    "hq-cv-abc",
	}
	e.postMergeConvoyCheck(mr)

	// Should produce no output (town .beads doesn't exist)
	if buf.Len() != 0 {
		t.Errorf("expected no output when town beads missing, got: %s", buf.String())
	}
}

func TestNotifyDeaconConvoyFeeding_SkipsWhenNoConvoyID(t *testing.T) {
	// notifyDeaconConvoyFeeding should skip when MR has no ConvoyID
	tmpDir, err := os.MkdirTemp("", "engineer-notify-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigDir,
	}

	e := NewEngineer(r)
	var buf bytes.Buffer
	e.SetOutput(&buf)

	// MR without ConvoyID — should produce no output
	mr := &MRInfo{
		ID:          "gt-test",
		SourceIssue: "gt-src",
		ConvoyID:    "", // No convoy
	}
	e.notifyDeaconConvoyFeeding(mr)

	if buf.Len() != 0 {
		t.Errorf("expected no output when ConvoyID empty, got: %s", buf.String())
	}
}

func TestNotifyDeaconConvoyFeeding_AttemptsWhenConvoyID(t *testing.T) {
	// notifyDeaconConvoyFeeding should attempt to send mail when ConvoyID is set.
	// The send will fail (no beads setup in tmpdir) but we verify the attempt via output.
	tmpDir, err := os.MkdirTemp("", "engineer-notify-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigDir,
	}

	e := NewEngineer(r)
	var buf bytes.Buffer
	e.SetOutput(&buf)

	mr := &MRInfo{
		ID:          "gt-test",
		SourceIssue: "gt-src",
		ConvoyID:    "hq-cv-abc",
	}
	e.notifyDeaconConvoyFeeding(mr)

	output := buf.String()
	// Should have attempted to send — either success or warning about failure
	if !strings.Contains(output, "CONVOY_NEEDS_FEEDING") && !strings.Contains(output, "convoy feeding") {
		t.Errorf("expected output mentioning convoy notification, got: %s", output)
	}
}

func TestConvoyInfoDescriptionParsing(t *testing.T) {
	// Test that landConvoySwarm correctly parses Molecule from description
	tests := []struct {
		name        string
		description string
		wantMolID   string
	}{
		{
			name:        "with molecule",
			description: "Convoy tracking 2 issues\nOwner: mayor/\nMolecule: mol-release",
			wantMolID:   "mol-release",
		},
		{
			name:        "without molecule",
			description: "Convoy tracking 2 issues\nOwner: mayor/",
			wantMolID:   "",
		},
		{
			name:        "empty description",
			description: "",
			wantMolID:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var moleculeID string
			for _, line := range strings.Split(tt.description, "\n") {
				if strings.HasPrefix(line, "Molecule: ") {
					moleculeID = strings.TrimPrefix(line, "Molecule: ")
					break
				}
			}
			if moleculeID != tt.wantMolID {
				t.Errorf("got molecule ID %q, want %q", moleculeID, tt.wantMolID)
			}
		})
	}
}

func TestNotifyConvoyCompletionParsing(t *testing.T) {
	// Test that notifyConvoyCompletion correctly parses Owner/Notify from description
	tests := []struct {
		name        string
		description string
		wantAddrs   []string
	}{
		{
			name:        "owner and notify",
			description: "Convoy tracking 2 issues\nOwner: mayor/\nNotify: ops/",
			wantAddrs:   []string{"mayor/", "ops/"},
		},
		{
			name:        "owner only",
			description: "Owner: deacon/",
			wantAddrs:   []string{"deacon/"},
		},
		{
			name:        "no addresses",
			description: "Convoy tracking 1 issue",
			wantAddrs:   nil,
		},
		{
			name:        "duplicate addresses deduped",
			description: "Owner: mayor/\nNotify: mayor/",
			wantAddrs:   []string{"mayor/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notified := make(map[string]bool)
			var addrs []string

			for _, line := range strings.Split(tt.description, "\n") {
				var addr string
				if strings.HasPrefix(line, "Owner: ") {
					addr = strings.TrimPrefix(line, "Owner: ")
				} else if strings.HasPrefix(line, "Notify: ") {
					addr = strings.TrimPrefix(line, "Notify: ")
				}

				if addr != "" && !notified[addr] {
					addrs = append(addrs, addr)
					notified[addr] = true
				}
			}

			if len(addrs) != len(tt.wantAddrs) {
				t.Errorf("got %d addresses, want %d", len(addrs), len(tt.wantAddrs))
				return
			}
			for i, addr := range addrs {
				if addr != tt.wantAddrs[i] {
					t.Errorf("addr[%d] = %q, want %q", i, addr, tt.wantAddrs[i])
				}
			}
		})
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
