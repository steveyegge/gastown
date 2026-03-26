package githubci

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type scriptedRunner struct {
	t     *testing.T
	steps []scriptStep
	calls []string
}

type scriptStep struct {
	match string
	out   string
	err   error
}

func (r *scriptedRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if len(r.steps) == 0 {
		r.t.Fatalf("unexpected command: %s", call)
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if !strings.Contains(call, step.match) {
		r.t.Fatalf("command %q does not match expected substring %q", call, step.match)
	}
	return []byte(step.out), step.err
}

func TestEnsureBranchCIDispatchesFallback(t *testing.T) {
	runner := &scriptedRunner{
		t: t,
		steps: []scriptStep{
			{
				match: "gh run list --repo owner/repo --workflow CI --branch feature",
				out:   "[]",
			},
			{
				match: "gh run list --repo owner/repo --workflow CI --branch feature",
				out:   "[]",
			},
			{
				match: "gh workflow run CI --repo owner/repo --ref feature",
				out:   "",
			},
			{
				match: "gh run list --repo owner/repo --workflow CI --branch feature",
				out:   `[{"databaseId":42,"headSha":"abc123","headBranch":"feature","event":"workflow_dispatch","status":"queued","conclusion":"","url":"https://example.test/runs/42","createdAt":"2026-03-25T20:00:00Z"}]`,
			},
			{
				match: "gh run view 42 --repo owner/repo",
				out:   `{"databaseId":42,"headSha":"abc123","headBranch":"feature","event":"workflow_dispatch","status":"completed","conclusion":"success","url":"https://example.test/runs/42","createdAt":"2026-03-25T20:00:00Z"}`,
			},
		},
	}

	client := NewWithRunner(runner)
	run, err := client.EnsureBranchCI(context.Background(), EnsureOptions{
		Repo:         "owner/repo",
		Workflow:     "CI",
		Branch:       "feature",
		SHA:          "abc123",
		PushWait:     time.Millisecond,
		PollInterval: time.Millisecond,
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run == nil || run.DatabaseID != 42 {
		t.Fatalf("expected run 42, got %+v", run)
	}
	if got := strings.Join(runner.calls, "\n"); !strings.Contains(got, "gh workflow run CI --repo owner/repo --ref feature") {
		t.Fatalf("expected workflow_dispatch fallback, calls:\n%s", got)
	}
}

func TestRepoFromRemoteURL(t *testing.T) {
	tests := map[string]string{
		"git@github.com:owner/repo.git":       "owner/repo",
		"https://github.com/owner/repo.git":   "owner/repo",
		"ssh://git@github.com/owner/repo.git": "owner/repo",
	}
	for input, want := range tests {
		t.Run(fmt.Sprintf("parse-%s", want), func(t *testing.T) {
			got, err := RepoFromRemoteURL(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != want {
				t.Fatalf("RepoFromRemoteURL(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
