package git

import "testing"

func TestIsToolPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// .beads/ paths
		{".beads/db.json", true},
		{".beads/issues/gt-123.json", true},
		// .copilot/ paths
		{".copilot/session-state/foo", true},
		{".copilot/config.json", true},
		// .github/hooks/ paths
		{".github/hooks/pre-commit", true},
		{".github/hooks/post-checkout", true},
		// Non-tool paths
		{"main.go", false},
		{"internal/cmd/done.go", false},
		{".github/workflows/ci.yml", false},
		{".gitignore", false},
		{"", false},
		// isBeadsPath false positive: .beads/ nested inside another path
		{"pkg/.beads/data.json", false},
		{"vendor/.beads/db.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isToolPath(tt.path); got != tt.want {
				t.Errorf("isToolPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCleanExcludingToolFiles(t *testing.T) {
	tests := []struct {
		name   string
		status UncommittedWorkStatus
		want   bool
	}{
		{
			name: "completely clean",
			status: UncommittedWorkStatus{},
			want: true,
		},
		{
			name: "only copilot untracked files",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UntrackedFiles:        []string{".copilot/session-state/foo", ".copilot/config.json"},
			},
			want: true,
		},
		{
			name: "only github hooks untracked",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UntrackedFiles:        []string{".github/hooks/pre-commit"},
			},
			want: true,
		},
		{
			name: "only beads modified",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				ModifiedFiles:         []string{".beads/db.json"},
			},
			want: true,
		},
		{
			name: "mix of tool files only",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UntrackedFiles:        []string{".copilot/state", ".github/hooks/pre-commit"},
				ModifiedFiles:         []string{".beads/issues/gt-1.json"},
			},
			want: true,
		},
		{
			name: "real modified file blocks",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				ModifiedFiles:         []string{"main.go"},
			},
			want: false,
		},
		{
			name: "real untracked file blocks",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UntrackedFiles:        []string{"scratch.txt"},
			},
			want: false,
		},
		{
			name: "tool files plus real file blocks",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UntrackedFiles:        []string{".copilot/state", "TODO.md"},
			},
			want: false,
		},
		{
			name: "stashes block even with only tool files",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				StashCount:            1,
				UntrackedFiles:        []string{".copilot/state"},
			},
			want: false,
		},
		{
			name: "unpushed commits block even with only tool files",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
				UnpushedCommits:       1,
				UntrackedFiles:        []string{".copilot/state"},
			},
			want: false,
		},
		{
			name: "uncommitted changes with no classified files blocks",
			status: UncommittedWorkStatus{
				HasUncommittedChanges: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.CleanExcludingToolFiles(); got != tt.want {
				t.Errorf("CleanExcludingToolFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}
