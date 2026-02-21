package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

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
