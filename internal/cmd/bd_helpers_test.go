package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestBdCmd_Build(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *bdCmd
		wantArgs []string
		wantDir  string
		wantEnv  map[string]string
	}{
		{
			name: "basic command with defaults",
			setup: func() *bdCmd {
				return BdCmd("show", "test-id", "--json")
			},
			wantArgs: []string{"bd", "show", "test-id", "--json"},
			wantDir:  "",
			wantEnv:  map[string]string{},
		},
		{
			name: "with directory",
			setup: func() *bdCmd {
				return BdCmd("list").Dir("/some/path")
			},
			wantArgs: []string{"bd", "list"},
			wantDir:  "/some/path",
			wantEnv:  map[string]string{},
		},
		{
			name: "with auto commit",
			setup: func() *bdCmd {
				return BdCmd("update", "id").WithAutoCommit()
			},
			wantArgs: []string{"bd", "update", "id"},
			wantEnv: map[string]string{
				"BD_DOLT_AUTO_COMMIT": "on",
			},
		},
		{
			name: "with GT_ROOT",
			setup: func() *bdCmd {
				return BdCmd("cook", "formula").WithGTRoot("/town/root")
			},
			wantArgs: []string{"bd", "cook", "formula"},
			wantEnv: map[string]string{
				"GT_ROOT": "/town/root",
			},
		},
		{
			name: "with StripBdBranch",
			setup: func() *bdCmd {
				return BdCmd("show", "id").StripBdBranch()
			},
			wantArgs: []string{"bd", "show", "id"},
			wantEnv: map[string]string{
				"BD_BRANCH": "", // Should be stripped
			},
		},
		{
			name: "chained configuration",
			setup: func() *bdCmd {
				return BdCmd("mol", "wisp", "formula").
					Dir("/work/dir").
					WithAutoCommit().
					WithGTRoot("/town/root")
			},
			wantArgs: []string{"bd", "mol", "wisp", "formula"},
			wantDir:  "/work/dir",
			wantEnv: map[string]string{
				"BD_DOLT_AUTO_COMMIT": "on",
				"GT_ROOT":             "/town/root",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bdc := tt.setup()
			cmd := bdc.Build()

			// Verify command arguments
			if len(cmd.Args) != len(tt.wantArgs) {
				t.Errorf("Args length = %d, want %d", len(cmd.Args), len(tt.wantArgs))
			}
			for i, arg := range tt.wantArgs {
				if i >= len(cmd.Args) || cmd.Args[i] != arg {
					t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], arg)
				}
			}

			// Verify working directory
			if cmd.Dir != tt.wantDir {
				t.Errorf("Dir = %q, want %q", cmd.Dir, tt.wantDir)
			}

			// Verify environment variables
			envMap := parseEnv(cmd.Env)
			for key, wantVal := range tt.wantEnv {
				if key == "BD_BRANCH" {
					// Special case: BD_BRANCH should be absent when stripped
					if _, exists := envMap[key]; exists && tt.wantEnv[key] == "" {
						t.Errorf("BD_BRANCH should be stripped from env but found: %s", envMap[key])
					}
					continue
				}
				if gotVal, ok := envMap[key]; !ok {
					t.Errorf("Env %q not found, want %q", key, wantVal)
				} else if gotVal != wantVal {
					t.Errorf("Env %q = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestBdCmd_StripBdBranch(t *testing.T) {
	// Create an environment with BD_BRANCH set
	baseEnv := append(os.Environ(), "BD_BRANCH=test-branch", "OTHER_VAR=value")

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}

	// Verify BD_BRANCH is present initially
	envBefore := parseEnv(bdc.env)
	if _, ok := envBefore["BD_BRANCH"]; !ok {
		t.Fatal("BD_BRANCH should be in base environment for test setup")
	}

	// Apply StripBdBranch
	bdc.StripBdBranch()
	cmd := bdc.Build()
	envAfter := parseEnv(cmd.Env)

	// Verify BD_BRANCH is removed
	if _, ok := envAfter["BD_BRANCH"]; ok {
		t.Error("BD_BRANCH should be stripped from environment")
	}

	// Verify OTHER_VAR is preserved
	if envAfter["OTHER_VAR"] != "value" {
		t.Error("OTHER_VAR should be preserved")
	}
}

func TestBdCmd_Stderr(t *testing.T) {
	var stderrBuf bytes.Buffer

	bdc := BdCmd("show", "nonexistent-id").
		StripBdBranch().
		Stderr(&stderrBuf)

	cmd := bdc.Build()

	// Verify stderr writer is set
	if cmd.Stderr != &stderrBuf {
		t.Error("Stderr should be set to custom writer")
	}
}

func TestBdCmd_DefaultStderr(t *testing.T) {
	bdc := BdCmd("list")
	cmd := bdc.Build()

	// Verify default stderr is os.Stderr
	if cmd.Stderr != os.Stderr {
		t.Error("Default Stderr should be os.Stderr")
	}
}

func TestBdCmd_Output(t *testing.T) {
	// Use "bd version" or similar that should work
	// Note: This requires bd to be installed. If not available, skip.
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping integration test: " + err.Error())
	}

	bdc := BdCmd("--version")
	out, err := bdc.Output()

	// Should not error and should produce output
	if err != nil {
		t.Errorf("Output() error = %v", err)
	}
	if len(out) == 0 {
		t.Error("Output() produced no output")
	}
}

func TestBdCmd_Run(t *testing.T) {
	// Use "bd --version" or similar that should work
	// Note: This requires bd to be installed. If not available, skip.
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping integration test: " + err.Error())
	}

	bdc := BdCmd("--version")
	err := bdc.Run()

	// Should not error
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}
}

func TestBdCmd_Chaining(t *testing.T) {
	// Test that all builder methods return the receiver for chaining
	bdc := BdCmd("test")

	// Each method should return the same pointer for fluent chaining
	if bdc.WithAutoCommit() != bdc {
		t.Error("WithAutoCommit() should return receiver for chaining")
	}
	if bdc.StripBdBranch() != bdc {
		t.Error("StripBdBranch() should return receiver for chaining")
	}
	if bdc.WithGTRoot("/test") != bdc {
		t.Error("WithGTRoot() should return receiver for chaining")
	}
	if bdc.Dir("/test") != bdc {
		t.Error("Dir() should return receiver for chaining")
	}
	if bdc.Stderr(os.Stdout) != bdc {
		t.Error("Stderr() should return receiver for chaining")
	}
}

// parseEnv converts an environment slice to a map for easier testing
func parseEnv(env []string) map[string]string {
	m := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		} else if len(parts) == 1 {
			m[parts[0]] = ""
		}
	}
	return m
}

// Ensure beads.StripBdBranch is used correctly (compile-time check)
var _ = beads.StripBdBranch

// ===================================================================
// Corner case tests for bdCmd environment handling
// ===================================================================

func TestBdCmd_WithAutoCommit_OverridesParentOff(t *testing.T) {
	// Test that WithAutoCommit() overrides a parent env BD_DOLT_AUTO_COMMIT=off
	// In exec, last value wins, so appending BD_DOLT_AUTO_COMMIT=on should win
	baseEnv := []string{"PATH=/usr/bin", "BD_DOLT_AUTO_COMMIT=off", "HOME=/home/user"}

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}
	bdc.WithAutoCommit()
	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// The last value should win in exec
	if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
		t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'on' (should override parent's 'off')", envMap["BD_DOLT_AUTO_COMMIT"])
	}
}

func TestBdCmd_StripBdBranch_LeavesAutoCommit(t *testing.T) {
	// Test that StripBdBranch() only strips BD_BRANCH, leaves BD_DOLT_AUTO_COMMIT intact
	baseEnv := []string{"BD_BRANCH=test-branch", "BD_DOLT_AUTO_COMMIT=on", "OTHER=value"}

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}
	bdc.StripBdBranch()
	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// BD_BRANCH should be gone
	if _, exists := envMap["BD_BRANCH"]; exists {
		t.Error("BD_BRANCH should be stripped from environment")
	}

	// BD_DOLT_AUTO_COMMIT should still be there
	if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
		t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'on' (should not be stripped)", envMap["BD_DOLT_AUTO_COMMIT"])
	}

	// OTHER should still be there
	if envMap["OTHER"] != "value" {
		t.Error("OTHER should be preserved")
	}
}

func TestBdCmd_Chain_AutoCommitThenStrip(t *testing.T) {
	// Test chaining: WithAutoCommit().StripBdBranch()
	baseEnv := []string{"BD_BRANCH=test-branch", "PATH=/usr/bin"}

	bdc := BdCmd("show", "id").
		WithAutoCommit().
		StripBdBranch()

	// Override env for this test
	bdc.env = baseEnv
	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// BD_BRANCH should be stripped
	if _, exists := envMap["BD_BRANCH"]; exists {
		t.Error("BD_BRANCH should be stripped")
	}

	// BD_DOLT_AUTO_COMMIT should be present
	if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
		t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'on'", envMap["BD_DOLT_AUTO_COMMIT"])
	}
}

func TestBdCmd_Chain_StripThenAutoCommit(t *testing.T) {
	// Test chaining: StripBdBranch().WithAutoCommit() (order shouldn't matter for result)
	baseEnv := []string{"BD_BRANCH=test-branch", "PATH=/usr/bin"}

	bdc := BdCmd("show", "id").
		StripBdBranch().
		WithAutoCommit()

	// Override env for this test
	bdc.env = baseEnv
	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// BD_BRANCH should be stripped
	if _, exists := envMap["BD_BRANCH"]; exists {
		t.Error("BD_BRANCH should be stripped")
	}

	// BD_DOLT_AUTO_COMMIT should be present
	if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
		t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'on'", envMap["BD_DOLT_AUTO_COMMIT"])
	}
}

func TestBdCmd_StripBdBranch_NoBdBranch(t *testing.T) {
	// Test StripBdBranch when BD_BRANCH is not in env - should be no-op
	baseEnv := []string{"PATH=/usr/bin", "OTHER=value"}

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}
	bdc.StripBdBranch()
	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// OTHER should still be there
	if envMap["OTHER"] != "value" {
		t.Error("OTHER should be preserved when BD_BRANCH not present")
	}

	// PATH should still be there
	if envMap["PATH"] != "/usr/bin" {
		t.Error("PATH should be preserved when BD_BRANCH not present")
	}
}

func TestBdCmd_MultipleAutoCommit_LastWins(t *testing.T) {
	// Test that multiple BD_DOLT_AUTO_COMMIT values - last one wins in exec
	baseEnv := []string{"BD_DOLT_AUTO_COMMIT=off"}

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}
	bdc.WithAutoCommit()
	cmd := bdc.Build()

	// Count occurrences
	offCount := 0
	onCount := 0
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "BD_DOLT_AUTO_COMMIT=") {
			if e == "BD_DOLT_AUTO_COMMIT=off" {
				offCount++
			} else if e == "BD_DOLT_AUTO_COMMIT=on" {
				onCount++
			}
		}
	}

	// parseEnv returns last value
	envMap := parseEnv(cmd.Env)
	if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
		t.Errorf("Expected last value 'on' to win, got %q", envMap["BD_DOLT_AUTO_COMMIT"])
	}

	// Verify we have both (the override pattern)
	if offCount != 1 || onCount != 1 {
		t.Errorf("Expected one 'off' and one 'on', got off=%d, on=%d", offCount, onCount)
	}
}

func TestBdCmd_EmptyGTRoot_Skipped(t *testing.T) {
	// Test that empty GT_ROOT is not added to env.
	// Use a clean env to avoid inheriting GT_ROOT from the test runner.
	bdc := BdCmd("show", "id").
		WithGTRoot("")
	bdc.env = filterEnv(bdc.env, "GT_ROOT")

	cmd := bdc.Build()

	// Check that GT_ROOT is not in env
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GT_ROOT=") {
			t.Errorf("GT_ROOT should not be added when empty, found: %s", e)
		}
	}
}

func TestBdCmd_ReadOperations_StripBdBranch(t *testing.T) {
	// Verify that read operations (show, list, etc.) strip BD_BRANCH
	testCases := []struct {
		name     string
		bdc      *bdCmd
		expected bool // true if BD_BRANCH should be stripped
	}{
		{
			name:     "show with StripBdBranch",
			bdc:      BdCmd("show", "id").StripBdBranch(),
			expected: true,
		},
		{
			name:     "list without StripBdBranch",
			bdc:      BdCmd("list"),
			expected: false,
		},
		{
			name:     "update without StripBdBranch (write op)",
			bdc:      BdCmd("update", "id", "--status=open"),
			expected: false,
		},
	}

	baseEnv := []string{"BD_BRANCH=test-branch"}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Inject test env
			testBdc := tc.bdc
			testBdc.env = baseEnv

			cmd := testBdc.Build()
			envMap := parseEnv(cmd.Env)

			_, hasBranch := envMap["BD_BRANCH"]
			if tc.expected && hasBranch {
				t.Error("BD_BRANCH should be stripped for this operation")
			}
			if !tc.expected && !hasBranch {
				t.Error("BD_BRANCH should be preserved for this operation")
			}
		})
	}
}

func TestBdCmd_WriteOperations_KeepBdBranch(t *testing.T) {
	// Verify that write operations do NOT strip BD_BRANCH (they need branch isolation)
	bdc := BdCmd("update", "id", "--status=hooked")

	baseEnv := []string{"BD_BRANCH=test-branch", "PATH=/usr/bin"}
	bdc.env = baseEnv

	cmd := bdc.Build()
	envMap := parseEnv(cmd.Env)

	// BD_BRANCH should be preserved for write operations
	if envMap["BD_BRANCH"] != "test-branch" {
		t.Errorf("BD_BRANCH = %q, want 'test-branch' (should be preserved for writes)", envMap["BD_BRANCH"])
	}
}

func TestBdCmd_AllCombinations(t *testing.T) {
	// Test all possible option combinations
	baseEnv := []string{"BD_BRANCH=test-branch", "BD_DOLT_AUTO_COMMIT=off", "PATH=/usr/bin"}

	tests := []struct {
		name             string
		stripBdBranch    bool
		autoCommit       bool
		gtRoot           string
		wantBdBranch     bool // true = should exist
		wantAutoCommitOn bool
		wantGTRoot       bool
	}{
		{"none", false, false, "", true, false, false},
		{"strip only", true, false, "", false, false, false},
		{"autocommit only", false, true, "", true, true, false},
		{"gtroot only", false, false, "/town", true, false, true},
		{"strip+autocommit", true, true, "", false, true, false},
		{"strip+gtroot", true, false, "/town", false, false, true},
		{"autocommit+gtroot", false, true, "/town", true, true, true},
		{"all three", true, true, "/town", false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bdc := &bdCmd{
				args:   []string{"show", "id"},
				env:    append([]string{}, baseEnv...), // Copy to avoid mutation
				stderr: os.Stderr,
			}

			if tt.stripBdBranch {
				bdc.stripBdBranch = true
			}
			if tt.autoCommit {
				bdc.autoCommit = true
			}
			bdc.gtRoot = tt.gtRoot

			cmd := bdc.Build()
			envMap := parseEnv(cmd.Env)

			// Check BD_BRANCH
			_, hasBdBranch := envMap["BD_BRANCH"]
			if tt.wantBdBranch && !hasBdBranch {
				t.Error("BD_BRANCH should be present")
			}
			if !tt.wantBdBranch && hasBdBranch {
				t.Error("BD_BRANCH should be stripped")
			}

			// Check BD_DOLT_AUTO_COMMIT
			if tt.wantAutoCommitOn {
				if envMap["BD_DOLT_AUTO_COMMIT"] != "on" {
					t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'on'", envMap["BD_DOLT_AUTO_COMMIT"])
				}
			} else {
				// When not explicitly set via WithAutoCommit, should keep original
				if envMap["BD_DOLT_AUTO_COMMIT"] != "off" {
					t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want 'off' (original value)", envMap["BD_DOLT_AUTO_COMMIT"])
				}
			}

			// Check GT_ROOT
			_, hasGTRoot := envMap["GT_ROOT"]
			if tt.wantGTRoot && !hasGTRoot {
				t.Error("GT_ROOT should be present")
			}
			if !tt.wantGTRoot && hasGTRoot {
				t.Error("GT_ROOT should not be present")
			}
		})
	}
}

func TestBdCmd_ConcurrentBuild(t *testing.T) {
	// Test that concurrent Build() calls are safe
	// Each Build() gets a snapshot via os.Environ(), so they should be independent
	bdc := BdCmd("show", "id")

	done := make(chan bool, 2)

	go func() {
		cmd1 := bdc.Build()
		_ = cmd1.Env
		done <- true
	}()

	go func() {
		cmd2 := bdc.Build()
		_ = cmd2.Env
		done <- true
	}()

	// Wait for both goroutines
	for i := 0; i < 2; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent builds")
		}
	}
}

func TestBdCmd_EnvImmutability(t *testing.T) {
	// Test that buildEnv doesn't mutate the original b.env
	baseEnv := []string{"BD_BRANCH=test-branch", "PATH=/usr/bin"}
	originalLen := len(baseEnv)

	bdc := &bdCmd{
		args:   []string{"show", "id"},
		env:    baseEnv,
		stderr: os.Stderr,
	}
	bdc.WithAutoCommit().WithGTRoot("/town").StripBdBranch()

	// Call buildEnv multiple times
	_ = bdc.buildEnv()
	_ = bdc.buildEnv()

	// Original env should be unchanged
	if len(baseEnv) != originalLen {
		t.Errorf("Original env was mutated: length %d, expected %d", len(baseEnv), originalLen)
	}

	// BD_BRANCH should still be in baseEnv
	found := false
	for _, e := range baseEnv {
		if strings.HasPrefix(e, "BD_BRANCH=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("BD_BRANCH was removed from original env (should be immutable)")
	}
}

// filterEnv returns env with all entries matching the given key prefix removed.
func filterEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}
