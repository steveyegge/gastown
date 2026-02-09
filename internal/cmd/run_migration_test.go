package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/formula"
)

func TestExtractCommands(t *testing.T) {
	tests := []struct {
		name        string
		description string
		townRoot    string
		wantCount   int
		wantFirst   string
	}{
		{
			name: "single bash block",
			description: `Do something.

` + "```bash" + `
gt version
` + "```" + `

Done.`,
			townRoot:  "/tmp/gt",
			wantCount: 1,
			wantFirst: "gt version",
		},
		{
			name: "multiple bash blocks",
			description: `Step one:

` + "```bash" + `
gt version
` + "```" + `

Step two:

` + "```bash" + `
gt doctor
` + "```",
			townRoot:  "/tmp/gt",
			wantCount: 2,
			wantFirst: "gt version",
		},
		{
			name: "template variable replacement",
			description: `Check rigs:

` + "```bash" + `
ls -d {{town_root}}/*/
` + "```",
			townRoot:  "/home/user/gt",
			wantCount: 1,
			wantFirst: "ls -d /home/user/gt/*/",
		},
		{
			name: "comment-only block excluded",
			description: `Explanation:

` + "```bash" + `
# This is just a comment
# Another comment
` + "```",
			townRoot:  "/tmp/gt",
			wantCount: 0,
		},
		{
			name: "multiline block preserved",
			description: `Loop:

` + "```bash" + `
for dir in {{town_root}}/*/; do
  echo "$dir"
done
` + "```",
			townRoot:  "/tmp/gt",
			wantCount: 1,
		},
		{
			name:        "no code blocks",
			description: "Just some prose instructions without any code.",
			townRoot:    "/tmp/gt",
			wantCount:   0,
		},
		{
			name: "sh block also works",
			description: `Run:

` + "```sh" + `
echo hello
` + "```",
			townRoot:  "/tmp/gt",
			wantCount: 1,
			wantFirst: "echo hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := extractCommands(tt.description, tt.townRoot)
			if len(commands) != tt.wantCount {
				t.Errorf("got %d commands, want %d", len(commands), tt.wantCount)
				for i, c := range commands {
					t.Logf("  command[%d]: %q", i, c)
				}
			}
			if tt.wantFirst != "" && len(commands) > 0 {
				got := strings.TrimSpace(commands[0])
				if got != tt.wantFirst {
					t.Errorf("first command = %q, want %q", got, tt.wantFirst)
				}
			}
		})
	}
}

func TestIsCommentOnly(t *testing.T) {
	tests := []struct {
		block string
		want  bool
	}{
		{"# just a comment", true},
		{"# line 1\n# line 2", true},
		{"# comment\necho hello", false},
		{"echo hello", false},
		{"", true},
		{"  \n  \n  ", true},
	}

	for _, tt := range tests {
		got := isCommentOnly(tt.block)
		if got != tt.want {
			t.Errorf("isCommentOnly(%q) = %v, want %v", tt.block, got, tt.want)
		}
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hi", 2, "hi"},
		{"abc", 3, "abc"},
		// Bug fix: maxLen < 4 should not panic
		{"hello", 3, "hel"},
		{"hello", 2, "he"},
		{"hello", 1, "h"},
	}

	for _, tt := range tests {
		got := truncateOutput(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateOutput(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestMigrationCheckpointRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"detect": {
				ID:     "detect",
				Title:  "Detect current state",
				Status: "completed",
			},
			"backup": {
				ID:     "backup",
				Title:  "Backup all data",
				Status: "pending",
			},
		},
	}

	// Save
	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, migrationCheckpointFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("checkpoint file not created")
	}

	// Load
	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.FormulaVersion != 1 {
		t.Errorf("formula version = %d, want 1", loaded.FormulaVersion)
	}
	if loaded.TownRoot != tmpDir {
		t.Errorf("town root = %q, want %q", loaded.TownRoot, tmpDir)
	}
	if len(loaded.Steps) != 2 {
		t.Errorf("steps count = %d, want 2", len(loaded.Steps))
	}
	if loaded.Steps["detect"].Status != "completed" {
		t.Errorf("detect status = %q, want completed", loaded.Steps["detect"].Status)
	}
	if loaded.Steps["backup"].Status != "pending" {
		t.Errorf("backup status = %q, want pending", loaded.Steps["backup"].Status)
	}
}

// =============================================================================
// Checkpoint edge case tests
// =============================================================================

func TestLoadMigrationCheckpoint_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Write corrupt checkpoint
	cpPath := filepath.Join(tmpDir, migrationCheckpointFile)
	if err := os.WriteFile(cpPath, []byte(`{corrupt json!!!`), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadMigrationCheckpoint(tmpDir)
	if err == nil {
		t.Fatal("expected error for invalid JSON checkpoint, got nil")
	}
	if !strings.Contains(err.Error(), "parsing checkpoint") {
		t.Errorf("expected 'parsing checkpoint' in error, got: %v", err)
	}
}

func TestLoadMigrationCheckpoint_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Write empty checkpoint
	cpPath := filepath.Join(tmpDir, migrationCheckpointFile)
	if err := os.WriteFile(cpPath, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadMigrationCheckpoint(tmpDir)
	if err == nil {
		t.Fatal("expected error for empty checkpoint, got nil")
	}
}

func TestLoadMigrationCheckpoint_Nonexistent(t *testing.T) {
	tmpDir := t.TempDir()

	cp, err := loadMigrationCheckpoint(tmpDir)
	if cp != nil {
		t.Errorf("expected nil checkpoint, got %+v", cp)
	}
	if err == nil {
		t.Fatal("expected error for nonexistent checkpoint, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestCheckpoint_RunningStatus(t *testing.T) {
	// Simulates a crash mid-step: step has status "running"
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		StartedAt:      time.Now().Add(-10 * time.Minute),
		UpdatedAt:      time.Now().Add(-5 * time.Minute),
		Steps: map[string]StepRun{
			"detect": {
				ID:     "detect",
				Status: "completed",
			},
			"backup": {
				ID:        "backup",
				Status:    "running",
				StartedAt: time.Now().Add(-5 * time.Minute),
			},
			"migrate-rigs": {
				ID:     "migrate-rigs",
				Status: "pending",
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Verify the "running" step is preserved (so orchestrator can detect crash)
	if loaded.Steps["backup"].Status != "running" {
		t.Errorf("backup status = %q, want running", loaded.Steps["backup"].Status)
	}
}

func TestCheckpoint_DifferentFormulaVersion(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 99,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"old-step": {
				ID:     "old-step",
				Status: "completed",
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.FormulaVersion != 99 {
		t.Errorf("formula version = %d, want 99", loaded.FormulaVersion)
	}
}

func TestCheckpoint_DifferentTownRoot(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       "/some/other/town",
		Steps:          map[string]StepRun{},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// The checkpoint stores a different town root than where it's loaded from
	// This is a detectable mismatch the orchestrator should catch
	if loaded.TownRoot != "/some/other/town" {
		t.Errorf("town root = %q, want /some/other/town", loaded.TownRoot)
	}
}

func TestCheckpoint_AllStepsCompleted(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"detect":  {ID: "detect", Status: "completed"},
			"backup":  {ID: "backup", Status: "completed"},
			"migrate": {ID: "migrate", Status: "completed"},
			"verify":  {ID: "verify", Status: "completed"},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	completedCount := 0
	for _, s := range loaded.Steps {
		if s.Status == "completed" {
			completedCount++
		}
	}
	if completedCount != 4 {
		t.Errorf("completed count = %d, want 4", completedCount)
	}
}

func TestCheckpoint_FailedStep(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"detect": {ID: "detect", Status: "completed"},
			"backup": {
				ID:     "backup",
				Status: "failed",
				Error:  "disk full: /dev/sda1 is 100% full",
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Steps["backup"].Status != "failed" {
		t.Errorf("backup status = %q, want failed", loaded.Steps["backup"].Status)
	}
	if loaded.Steps["backup"].Error != "disk full: /dev/sda1 is 100% full" {
		t.Errorf("backup error = %q, want disk full message", loaded.Steps["backup"].Error)
	}
}

func TestCheckpoint_LargeStepOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate a step with large output
	largeOutput := strings.Repeat("Migration log line\n", 1000)

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"detect": {
				ID:     "detect",
				Status: "completed",
				Output: largeOutput,
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Steps["detect"].Output != largeOutput {
		t.Errorf("large output not preserved (got %d bytes, want %d)",
			len(loaded.Steps["detect"].Output), len(largeOutput))
	}
}

// =============================================================================
// extractCommands edge case tests
// =============================================================================

func TestExtractCommands_UnclosedBlock(t *testing.T) {
	// Unclosed code block should not produce commands
	desc := "Start:\n\n```bash\ngt version\n"
	commands := extractCommands(desc, "/tmp/gt")
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from unclosed block, got %d: %v", len(commands), commands)
	}
}

func TestExtractCommands_NestedCodeBlocks(t *testing.T) {
	// Code block containing backtick-3 inside should not double-close
	desc := "Run:\n\n```bash\necho '```'\necho done\n```\n"
	commands := extractCommands(desc, "/tmp/gt")
	// The inner ``` will be treated as closing the block
	// This is a known limitation — documenting current behavior
	if len(commands) == 0 {
		t.Log("nested code blocks: inner ``` closed the block (expected)")
	}
}

func TestExtractCommands_EmptyCodeBlock(t *testing.T) {
	desc := "Run:\n\n```bash\n```\n"
	commands := extractCommands(desc, "/tmp/gt")
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from empty code block, got %d", len(commands))
	}
}

func TestExtractCommands_WhitespaceOnlyBlock(t *testing.T) {
	desc := "Run:\n\n```bash\n   \n  \n```\n"
	commands := extractCommands(desc, "/tmp/gt")
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from whitespace-only block, got %d", len(commands))
	}
}

func TestExtractCommands_ShebangPreserved(t *testing.T) {
	// Lines starting with #! should be preserved (shebangs)
	desc := "Run:\n\n```bash\n#!/bin/bash\necho hello\n```\n"
	commands := extractCommands(desc, "/tmp/gt")
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if !strings.Contains(commands[0], "#!/bin/bash") {
		t.Errorf("shebang not preserved in command: %q", commands[0])
	}
}

func TestExtractCommands_MultipleTemplateVars(t *testing.T) {
	desc := "Run:\n\n```bash\ncp {{town_root}}/a {{town_root}}/b\n```\n"
	commands := extractCommands(desc, "/my/town")
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0] != "cp /my/town/a /my/town/b" {
		t.Errorf("template vars not replaced: %q", commands[0])
	}
}

func TestExtractCommands_NonBashBlockIgnored(t *testing.T) {
	// Python, yaml, etc. code blocks should be ignored
	desc := "Config:\n\n```yaml\nkey: value\n```\n\n```python\nprint('hello')\n```\n"
	commands := extractCommands(desc, "/tmp/gt")
	if len(commands) != 0 {
		t.Errorf("expected 0 commands from non-bash blocks, got %d: %v", len(commands), commands)
	}
}

func TestExtractCommands_PathWithSpaces(t *testing.T) {
	desc := "Run:\n\n```bash\nls {{town_root}}/my dir/\n```\n"
	commands := extractCommands(desc, "/path with spaces")
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0] != "ls /path with spaces/my dir/" {
		t.Errorf("path with spaces not handled: %q", commands[0])
	}
}

// =============================================================================
// truncateOutput edge case tests
// =============================================================================

func TestTruncateOutput_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"empty string", "", 10, ""},
		{"exact length", "hello", 5, "hello"},
		{"one over", "hello!", 5, "he..."},
		{"maxLen equals 4", "hello", 4, "h..."},
		{"very long string", strings.Repeat("x", 1000), 10, "xxxxxxx..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateOutput(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateOutput(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Checkpoint JSON serialization edge cases
// =============================================================================

func TestCheckpoint_PreservesTimestamps(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		StartedAt:      now,
		Steps: map[string]StepRun{
			"step1": {
				ID:          "step1",
				Status:      "completed",
				StartedAt:   now.Add(-time.Minute),
				CompletedAt: now,
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// StartedAt should be preserved
	if !loaded.StartedAt.Equal(now) {
		t.Errorf("started_at = %v, want %v", loaded.StartedAt, now)
	}
	// UpdatedAt should be updated by save
	if loaded.UpdatedAt.IsZero() {
		t.Error("updated_at should not be zero after save")
	}
	// Step timestamps should be preserved
	step := loaded.Steps["step1"]
	if !step.StartedAt.Equal(now.Add(-time.Minute)) {
		t.Errorf("step started_at = %v, want %v", step.StartedAt, now.Add(-time.Minute))
	}
	if !step.CompletedAt.Equal(now) {
		t.Errorf("step completed_at = %v, want %v", step.CompletedAt, now)
	}
}

func TestCheckpoint_JSONStructure(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 2,
		TownRoot:       "/home/user/gt",
		Steps: map[string]StepRun{
			"detect": {ID: "detect", Status: "completed"},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Read raw JSON and verify structure
	data, err := os.ReadFile(filepath.Join(tmpDir, migrationCheckpointFile))
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("checkpoint is not valid JSON: %v", err)
	}

	// Verify required top-level fields
	for _, field := range []string{"formula_version", "town_root", "started_at", "updated_at", "steps"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field %q in checkpoint JSON", field)
		}
	}
}

func TestExecuteMigrationStep_Timeout(t *testing.T) {
	townRoot := t.TempDir()

	// Save and restore global timeout
	origTimeout := runMigrationTimeout
	runMigrationTimeout = 500 * time.Millisecond
	defer func() { runMigrationTimeout = origTimeout }()

	cp := &MigrationCheckpoint{
		TownRoot:  townRoot,
		StartedAt: time.Now(),
		Steps:     make(map[string]StepRun),
	}

	// Use a platform-appropriate long-running command
	slowCmd := "sleep 30"
	if runtime.GOOS == "windows" {
		slowCmd = "ping -n 31 127.0.0.1"
	}

	step := &formula.Step{
		ID:    "slow-step",
		Title: "Step that should time out",
		Description: "Run a slow command.\n\n```bash\n" + slowCmd + "\n```\n",
	}

	start := time.Now()
	err := executeMigrationStep(nil, cp, step, townRoot)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error message, got: %v", err)
	}

	// Should have returned quickly (within 2s), not waited for sleep 30
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v (expected ~500ms)", elapsed)
	}

	// Checkpoint should record the failure
	sr, ok := cp.Steps["slow-step"]
	if !ok {
		t.Fatal("step not recorded in checkpoint")
	}
	if sr.Status != "failed" {
		t.Errorf("step status = %q, want failed", sr.Status)
	}
	if !strings.Contains(sr.Error, "timed out") {
		t.Errorf("checkpoint error should mention timeout, got: %s", sr.Error)
	}
}

func TestExecuteMigrationStep_NoTimeout(t *testing.T) {
	townRoot := t.TempDir()

	// Save and restore global timeout
	origTimeout := runMigrationTimeout
	runMigrationTimeout = 5 * time.Second
	defer func() { runMigrationTimeout = origTimeout }()

	cp := &MigrationCheckpoint{
		TownRoot:  townRoot,
		StartedAt: time.Now(),
		Steps:     make(map[string]StepRun),
	}

	step := &formula.Step{
		ID:    "fast-step",
		Title: "Step that completes quickly",
		Description: "Run a fast command.\n\n```bash\necho hello\n```\n",
	}

	err := executeMigrationStep(nil, cp, step, townRoot)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sr, ok := cp.Steps["fast-step"]
	if !ok {
		t.Fatal("step not recorded in checkpoint")
	}
	if sr.Status != "completed" {
		t.Errorf("step status = %q, want completed", sr.Status)
	}
}

// =============================================================================
// Per-command progress tracking tests (gt-1e907)
// =============================================================================

func TestCheckpoint_CommandsCompletedRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	cp := &MigrationCheckpoint{
		FormulaVersion: 1,
		TownRoot:       tmpDir,
		Steps: map[string]StepRun{
			"migrate": {
				ID:                "migrate",
				Status:            "failed",
				CommandsCompleted: 3,
				Error:             "command 4 failed",
			},
		},
	}

	if err := saveMigrationCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadMigrationCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Steps["migrate"].CommandsCompleted != 3 {
		t.Errorf("commands_completed = %d, want 3", loaded.Steps["migrate"].CommandsCompleted)
	}
}

func TestExecuteMigrationStep_TracksPerCommandProgress(t *testing.T) {
	townRoot := t.TempDir()

	origTimeout := runMigrationTimeout
	runMigrationTimeout = 5 * time.Second
	defer func() { runMigrationTimeout = origTimeout }()

	cp := &MigrationCheckpoint{
		TownRoot:  townRoot,
		StartedAt: time.Now(),
		Steps:     make(map[string]StepRun),
	}

	// Step with 3 commands: first two succeed, third fails
	step := &formula.Step{
		ID:    "multi-cmd",
		Title: "Multi-command step",
		Description: "Run multiple commands.\n\n```bash\necho cmd1\n```\n\n```bash\necho cmd2\n```\n\n```bash\nfalse\n```\n",
	}

	err := executeMigrationStep(nil, cp, step, townRoot)
	if err == nil {
		t.Fatal("expected error from failing 3rd command, got nil")
	}

	sr := cp.Steps["multi-cmd"]
	if sr.Status != "failed" {
		t.Errorf("status = %q, want failed", sr.Status)
	}
	if sr.CommandsCompleted != 2 {
		t.Errorf("commands_completed = %d, want 2 (first two succeeded)", sr.CommandsCompleted)
	}
}

func TestExecuteMigrationStep_SkipsCompletedCommandsOnRetry(t *testing.T) {
	townRoot := t.TempDir()

	origTimeout := runMigrationTimeout
	runMigrationTimeout = 5 * time.Second
	defer func() { runMigrationTimeout = origTimeout }()

	// Simulate a prior failed run where 2 commands completed
	cp := &MigrationCheckpoint{
		TownRoot:  townRoot,
		StartedAt: time.Now(),
		Steps: map[string]StepRun{
			"retry-step": {
				ID:                "retry-step",
				Status:            "failed",
				CommandsCompleted: 2,
				Error:             "previous failure",
			},
		},
	}

	// Step has 3 commands. The first two write marker files, the third succeeds.
	// On retry, the first two should be skipped (markers NOT created), third runs.
	marker1 := filepath.Join(townRoot, "marker1.txt")
	marker2 := filepath.Join(townRoot, "marker2.txt")
	marker3 := filepath.Join(townRoot, "marker3.txt")

	// Use platform-appropriate file-creation commands
	touchCmd := func(path string) string {
		if runtime.GOOS == "windows" {
			// Use copy nul which handles Windows paths better than type nul >
			return fmt.Sprintf(`copy nul "%s"`, path)
		}
		return fmt.Sprintf("touch %s", path)
	}

	step := &formula.Step{
		ID:    "retry-step",
		Title: "Retry step",
		Description: fmt.Sprintf("Commands.\n\n```bash\n%s\n```\n\n```bash\n%s\n```\n\n```bash\n%s\n```\n",
			touchCmd(marker1), touchCmd(marker2), touchCmd(marker3)),
	}

	err := executeMigrationStep(nil, cp, step, townRoot)
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}

	// First two commands should have been skipped — no marker files
	if _, err := os.Stat(marker1); err == nil {
		t.Error("marker1 should NOT exist (command was skipped on retry)")
	}
	if _, err := os.Stat(marker2); err == nil {
		t.Error("marker2 should NOT exist (command was skipped on retry)")
	}
	// Third command should have executed
	if _, err := os.Stat(marker3); err != nil {
		t.Error("marker3 should exist (command ran on retry)")
	}

	sr := cp.Steps["retry-step"]
	if sr.Status != "completed" {
		t.Errorf("status = %q, want completed", sr.Status)
	}
	if sr.CommandsCompleted != 3 {
		t.Errorf("commands_completed = %d, want 3", sr.CommandsCompleted)
	}
}

func TestExecuteMigrationStep_TimeoutKillsChildProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows: process group kill uses Unix-specific SIGKILL")
	}

	townRoot := t.TempDir()

	origTimeout := runMigrationTimeout
	runMigrationTimeout = 1 * time.Second
	defer func() { runMigrationTimeout = origTimeout }()

	markerFile := filepath.Join(townRoot, "child-was-here")

	cp := &MigrationCheckpoint{
		TownRoot:  townRoot,
		StartedAt: time.Now(),
		Steps:     make(map[string]StepRun),
	}

	// Spawn a background child that sleeps 3s then creates a marker file.
	// The parent bash sleeps 30s and will be timed out after 1s.
	// If process group kill works, the child dies too and no marker appears.
	step := &formula.Step{
		ID:    "orphan-test",
		Title: "Step that spawns child processes",
		Description: fmt.Sprintf(
			"Run:\n\n```bash\n(sleep 3 && touch %s) &\nsleep 30\n```\n", markerFile),
	}

	err := executeMigrationStep(nil, cp, step, townRoot)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Wait longer than the child's sleep to verify it was killed
	time.Sleep(4 * time.Second)

	if _, err := os.Stat(markerFile); err == nil {
		t.Error("child process was NOT killed — orphaned process detected (marker file exists)")
	}
}
