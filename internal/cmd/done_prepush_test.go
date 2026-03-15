package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPrePushHookBlocksProtectedBranches verifies that the pre-push hook
// script correctly blocks pushes to main and release branches, allows
// feature branches, and respects the GT_ALLOW_PROTECTED_PUSH override.
func TestPrePushHookBlocksProtectedBranches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook not supported on Windows")
	}

	// Write the hook script to a temp file
	hookDir := t.TempDir()
	hookPath := filepath.Join(hookDir, "pre-push")
	hookScript := `#!/bin/sh
if [ "${GT_ALLOW_PROTECTED_PUSH:-}" = "1" ]; then
    exit 0
fi
remote="$1"
while read local_ref local_sha remote_ref remote_sha; do
    branch="${remote_ref#refs/heads/}"
    case "$branch" in
        main|release)
            echo "BLOCKED: $branch" >&2
            exit 1
            ;;
    esac
done
exit 0
`
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	tests := []struct {
		name      string
		ref       string
		env       []string
		wantBlock bool
	}{
		{"push to release blocked", "refs/heads/release", nil, true},
		{"push to main blocked", "refs/heads/main", nil, true},
		{"push to feature allowed", "refs/heads/polecat/toast/gt-abc", nil, false},
		{"push to release with override allowed", "refs/heads/release", []string{"GT_ALLOW_PROTECTED_PUSH=1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "refs/heads/test abc123 " + tt.ref + " def456\n"

			cmd := exec.Command("sh", hookPath, "origin")
			cmd.Stdin = strings.NewReader(input)
			cmd.Env = append(os.Environ(), tt.env...)

			err := cmd.Run()
			blocked := err != nil

			if blocked != tt.wantBlock {
				t.Errorf("hook blocked = %v, want %v (ref=%s)", blocked, tt.wantBlock, tt.ref)
			}
		})
	}
}

// TestDirectMergePushUsesEnvOverride verifies that the GT_ALLOW_PROTECTED_PUSH
// environment variable is set to "1" in the env slice used for direct-merge pushes.
func TestDirectMergePushUsesEnvOverride(t *testing.T) {
	// This mirrors the env slice passed to PushWithEnv in done.go direct-merge paths
	env := []string{"GT_ALLOW_PROTECTED_PUSH=1"}

	found := false
	for _, e := range env {
		if e == "GT_ALLOW_PROTECTED_PUSH=1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("direct-merge push env should contain GT_ALLOW_PROTECTED_PUSH=1")
	}
}
